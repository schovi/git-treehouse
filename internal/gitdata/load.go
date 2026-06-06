package gitdata

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/schovi/git-treehouse/internal/config"
)

type BaseOption struct {
	Label string
	Rev   string
}

func Load(ctx context.Context, cwd string, config config.Config, runner Runner) (State, error) {
	repo, err := ResolveRepository(ctx, cwd, config, runner)
	if err != nil {
		return State{}, err
	}
	output, err := runner.Run(ctx, repo.Root, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return State{}, err
	}
	rows := ParseWorktreeList(string(output))
	realRows := make([]Worktree, 0, len(rows))
	for index, row := range rows {
		if row.Bare {
			continue
		}
		if repo.MainWorktree == "" && index == 0 {
			repo.MainWorktree = row.Path
		}
		row.IsActive = samePath(row.Path, repo.ActiveWorktree)
		row.IsMain = samePath(row.Path, repo.MainWorktree)
		enrichWorktree(ctx, repo, &row, runner)
		realRows = append(realRows, row)
	}
	sortWorktrees(realRows)
	return State{Repo: repo, Rows: realRows}, nil
}

func ResolveRepository(ctx context.Context, cwd string, config config.Config, runner Runner) (Repository, error) {
	rootOutput, err := runner.Run(ctx, cwd, "git", "rev-parse", "--show-toplevel")
	activeRoot := strings.TrimSpace(string(rootOutput))
	bareInvocation := false
	if err != nil {
		bareOutput, bareErr := runner.Run(ctx, cwd, "git", "rev-parse", "--is-bare-repository")
		if bareErr != nil || strings.TrimSpace(string(bareOutput)) != "true" {
			return Repository{}, fmt.Errorf("not inside a git repository")
		}
		bareInvocation = true
		activeRoot = cwd
	}
	commonOutput, err := runner.Run(ctx, activeRoot, "git", "rev-parse", "--git-common-dir")
	if err != nil {
		return Repository{}, err
	}
	commonGitDir := strings.TrimSpace(string(commonOutput))
	if !filepath.IsAbs(commonGitDir) {
		commonGitDir = filepath.Join(activeRoot, commonGitDir)
	}
	repoRoot := activeRoot
	if output, err := runner.Run(ctx, activeRoot, "git", "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
		commonGitDir = strings.TrimSpace(string(output))
	}
	if output, err := runner.Run(ctx, activeRoot, "git", "worktree", "list", "--porcelain"); err == nil {
		for index, row := range ParseWorktreeList(string(output)) {
			if row.Bare {
				continue
			}
			if index == 0 || repoRoot == activeRoot && !samePath(row.Path, activeRoot) {
				repoRoot = row.Path
				break
			}
		}
	}
	activeWorktree := activeRoot
	if bareInvocation {
		activeWorktree = ""
	}
	repo := Repository{
		Root:           repoRoot,
		CommonGitDir:   commonGitDir,
		Cwd:            cwd,
		ActiveWorktree: activeWorktree,
		MainWorktree:   repoRoot,
		Parent:         filepath.Dir(repoRoot),
	}
	repo.MainBranch = detectMainBranch(ctx, repoRoot, config.MainBranch, runner)
	repo.RemoteConfigured = hasRemote(ctx, repoRoot, runner)
	return repo, nil
}

func enrichWorktree(ctx context.Context, repo Repository, row *Worktree, runner Runner) {
	if row.Prunable {
		return
	}
	statusOutput, err := runner.Run(ctx, row.Path, "git", "status", "--porcelain=v1", "-b", "--untracked-files=normal")
	if err == nil {
		status := ParseStatusPorcelain(string(statusOutput))
		row.Status = status.Counts
		row.UpstreamGone = status.UpstreamGone
		row.Upstream = status.Upstream
	}
	if row.Upstream == "" {
		if output, err := runner.Run(ctx, row.Path, "git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
			row.Upstream = strings.TrimSpace(string(output))
		}
	}
	if row.Upstream == "" {
		row.HeadSync = SyncState{NoUpstream: true}
	} else if !row.UpstreamGone {
		if output, err := runner.Run(ctx, row.Path, "git", "rev-list", "--left-right", "--count", "HEAD...@{u}"); err == nil {
			ahead, behind, ok := ParseAheadBehind(string(output))
			row.HeadSync = SyncState{Available: ok, Ahead: ahead, Behind: behind}
		}
	}
	if (row.Detached || row.Branch != repo.MainBranch) && repo.MainBranch != "" && refExists(ctx, repo.Root, "refs/heads/"+repo.MainBranch, runner) {
		if output, err := runner.Run(ctx, row.Path, "git", "rev-list", "--left-right", "--count", "HEAD...refs/heads/"+repo.MainBranch); err == nil {
			ahead, behind, ok := ParseAheadBehind(string(output))
			row.MainSync = SyncState{Available: ok, Ahead: ahead, Behind: behind}
		}
	}
	if output, err := runner.Run(ctx, row.Path, "git", "log", "-1", "--format=%h%x00%ct%x00%s"); err == nil {
		parts := strings.SplitN(strings.TrimRight(string(output), "\n"), "\x00", 3)
		if len(parts) == 3 {
			row.CommitShort = parts[0]
			if unixSeconds, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				row.CommitTime = time.Unix(unixSeconds, 0)
			}
			row.CommitSubject = parts[2]
		}
	}
	if row.Branch != "" && !row.Detached && repo.MainBranch != "" && refExists(ctx, repo.Root, "refs/heads/"+row.Branch, runner) && refExists(ctx, repo.Root, "refs/heads/"+repo.MainBranch, runner) {
		if _, err := runner.Run(ctx, repo.Root, "git", "merge-base", "--is-ancestor", "refs/heads/"+row.Branch, "refs/heads/"+repo.MainBranch); err == nil {
			row.BranchMergedToMain = true
		}
	}
}

func detectMainBranch(ctx context.Context, repoRoot, configured string, runner Runner) string {
	if configured != "" {
		return configured
	}
	if output, err := runner.Run(ctx, repoRoot, "git", "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		value := strings.TrimSpace(string(output))
		if branch := strings.TrimPrefix(value, "origin/"); branch != "" && branch != value {
			return branch
		}
	}
	for _, branch := range []string{"main", "master"} {
		if refExists(ctx, repoRoot, "refs/heads/"+branch, runner) {
			return branch
		}
	}
	return "main"
}

func ExistingBranches(ctx context.Context, repoRoot string, runner Runner) (map[string]bool, error) {
	output, err := runner.Run(ctx, repoRoot, "git", "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	branches := map[string]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		branch := strings.TrimSpace(line)
		if branch != "" {
			branches[branch] = true
		}
	}
	return branches, nil
}

func ValidateBranchName(ctx context.Context, repoRoot, name string, runner Runner) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("branch name is required")
	}
	if _, err := runner.Run(ctx, repoRoot, "git", "check-ref-format", "--branch", name); err != nil {
		return fmt.Errorf("invalid branch name")
	}
	branches, err := ExistingBranches(ctx, repoRoot, runner)
	if err != nil {
		return err
	}
	if branches[name] {
		return fmt.Errorf("branch already exists")
	}
	return nil
}

func BaseOptions(ctx context.Context, repo Repository, row Worktree, runner Runner) []BaseOption {
	options := make([]BaseOption, 0, 3)
	if row.Detached {
		if row.Head != "" {
			options = append(options, BaseOption{Label: "(detached) " + shortHash(row.Head) + " (local)", Rev: row.Head})
		}
	} else if row.Branch != "" {
		options = append(options, BaseOption{Label: row.Branch + " (local)", Rev: row.Branch})
		if refExists(ctx, repo.Root, "refs/remotes/origin/"+row.Branch, runner) {
			options = append(options, BaseOption{Label: "origin/" + row.Branch, Rev: "origin/" + row.Branch})
		}
	}
	if repo.MainBranch != "" && refExists(ctx, repo.Root, "refs/remotes/origin/"+repo.MainBranch, runner) {
		remoteMain := "origin/" + repo.MainBranch
		alreadyIncluded := false
		for _, option := range options {
			if option.Rev == remoteMain {
				alreadyIncluded = true
				break
			}
		}
		if !alreadyIncluded {
			options = append(options, BaseOption{Label: remoteMain, Rev: remoteMain})
		}
	}
	if len(options) == 0 && repo.MainBranch != "" {
		options = append(options, BaseOption{Label: repo.MainBranch + " (local)", Rev: repo.MainBranch})
	}
	return options
}

func CreateWorktree(ctx context.Context, repoRoot, branch, path, base string, runner Runner) error {
	_, err := runner.Run(ctx, repoRoot, "git", "worktree", "add", "-b", branch, path, base)
	return err
}

func RemoveWorktree(ctx context.Context, repoRoot, path string, force bool, runner Runner) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	_, err := runner.Run(ctx, repoRoot, "git", args...)
	return err
}

func PruneWorktrees(ctx context.Context, repoRoot string, runner Runner) error {
	_, err := runner.Run(ctx, repoRoot, "git", "worktree", "prune")
	return err
}

func DeleteBranch(ctx context.Context, repoRoot, branch string, force bool, runner Runner) error {
	if branch == "" {
		return nil
	}
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := runner.Run(ctx, repoRoot, "git", "branch", flag, branch)
	return err
}

func FetchPrune(ctx context.Context, repoRoot string, runner Runner) error {
	_, err := runner.Run(ctx, repoRoot, "git", "fetch", "--prune")
	return err
}

func DiskUsage(path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func sortWorktrees(rows []Worktree) {
	sort.SliceStable(rows, func(leftIndex, rightIndex int) bool {
		left := rows[leftIndex]
		right := rows[rightIndex]
		if left.IsMain != right.IsMain {
			return left.IsMain
		}
		return left.CommitTime.After(right.CommitTime)
	})
}

func refExists(ctx context.Context, repoRoot, ref string, runner Runner) bool {
	_, err := runner.Run(ctx, repoRoot, "git", "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

func hasRemote(ctx context.Context, repoRoot string, runner Runner) bool {
	output, err := runner.Run(ctx, repoRoot, "git", "remote")
	return err == nil && strings.TrimSpace(string(output)) != ""
}

func samePath(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	leftClean, leftErr := filepath.Abs(left)
	rightClean, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	return filepath.Clean(leftClean) == filepath.Clean(rightClean)
}
