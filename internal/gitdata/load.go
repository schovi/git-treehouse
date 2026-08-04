package gitdata

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/schovi/git-treehouse/internal/config"
)

type BaseOption struct {
	Label string
	Rev   string
}

func Load(ctx context.Context, cwd string, config config.Config, runner Runner) (State, error) {
	state, err := LoadSkeleton(ctx, cwd, config, runner)
	if err != nil {
		return State{}, err
	}
	return EnrichLocalMetadata(ctx, state, runner)
}

func LoadSkeleton(ctx context.Context, cwd string, config config.Config, runner Runner) (State, error) {
	repo, rows, err := resolveRepositoryWithWorktrees(ctx, cwd, config, runner)
	if err != nil {
		return State{}, err
	}
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
		realRows = append(realRows, row)
	}
	sortWorktrees(realRows)
	return State{Repo: repo, Rows: realRows}, nil
}

func ResolveRepository(ctx context.Context, cwd string, config config.Config, runner Runner) (Repository, error) {
	repo, _, err := resolveRepositoryWithWorktrees(ctx, cwd, config, runner)
	return repo, err
}

func resolveRepositoryWithWorktrees(ctx context.Context, cwd string, config config.Config, runner Runner) (Repository, []Worktree, error) {
	rootOutput, err := runner.Run(ctx, cwd, "git", "rev-parse", "--show-toplevel")
	activeRoot := strings.TrimSpace(string(rootOutput))
	bareInvocation := false
	if err != nil {
		bareOutput, bareErr := runner.Run(ctx, cwd, "git", "rev-parse", "--is-bare-repository")
		if bareErr != nil || strings.TrimSpace(string(bareOutput)) != "true" {
			return Repository{}, nil, fmt.Errorf("not inside a git repository")
		}
		bareInvocation = true
		activeRoot = cwd
	}
	commonOutput, err := runner.Run(ctx, activeRoot, "git", "rev-parse", "--git-common-dir")
	if err != nil {
		return Repository{}, nil, err
	}
	commonGitDir := strings.TrimSpace(string(commonOutput))
	if !filepath.IsAbs(commonGitDir) {
		commonGitDir = filepath.Join(activeRoot, commonGitDir)
	}
	repoRoot := activeRoot
	if output, err := runner.Run(ctx, activeRoot, "git", "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
		commonGitDir = strings.TrimSpace(string(output))
	}
	output, err := runner.Run(ctx, activeRoot, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return Repository{}, nil, err
	}
	rows := ParseWorktreeList(string(output))
	for index, row := range rows {
		if row.Bare {
			continue
		}
		if index == 0 || repoRoot == activeRoot && !samePath(row.Path, activeRoot) {
			repoRoot = row.Path
			break
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
	return repo, rows, nil
}

func EnrichLocalMetadata(ctx context.Context, state State, runner Runner) (State, error) {
	state.Rows = append([]Worktree(nil), state.Rows...)
	refMetadataByBranch, refMetadataErr := loadRefMetadata(ctx, state.Repo, runner)
	if refMetadataErr != nil {
		for index := range state.Rows {
			enrichWorktree(ctx, state.Repo, &state.Rows[index], runner)
			state.Rows[index].LocalMetadataLoaded = true
		}
		sortWorktrees(state.Rows)
		return state, nil
	}
	enrichStatusCounts(ctx, state.Rows, runner)
	mainExists := state.Repo.MainBranch != "" && refMetadataByBranch[state.Repo.MainBranch].Branch != ""
	for index := range state.Rows {
		row := &state.Rows[index]
		if row.Prunable {
			row.LocalMetadataLoaded = true
			continue
		}
		if row.Branch != "" && !row.Detached {
			if metadata, ok := refMetadataByBranch[row.Branch]; ok {
				applyRefMetadata(row, metadata, state.Repo.MainBranch)
			} else {
				enrichWorktree(ctx, state.Repo, row, runner)
			}
		} else {
			enrichDetachedWorktree(ctx, state.Repo, row, mainExists, runner)
		}
		row.LocalMetadataLoaded = true
	}
	if mainExists {
		enrichContextGraphs(ctx, state.Repo, state.Rows, runner)
	}
	state.Branches = branchRowsFromMetadata(refMetadataByBranch, state.Rows, state.Repo.MainBranch)
	sortWorktrees(state.Rows)
	return state, nil
}

// graphCommitFetch is how many commits per side the context graph fetches. The
// frame displays fewer; the surplus lets it detect overflow without relying on
// ahead/behind totals.
const graphCommitFetch = 5

// graphBaseFetch is how many shared commits below the fork point the graph fetches.
// It runs deeper than graphCommitFetch because those ancestors pad the graph to
// match the Details box height when the two render side by side.
const graphBaseFetch = 12

// enrichContextGraphs attaches a local ContextGraph to each non-main, non-prunable
// worktree row by collecting its commits ahead of and behind the main branch. Runs
// concurrently like enrichStatusCounts. Failures leave Graph.Loaded false.
func enrichContextGraphs(ctx context.Context, repo Repository, rows []Worktree, runner Runner) {
	mainRef := "refs/heads/" + repo.MainBranch
	concurrency := min(4, runtime.NumCPU())
	if concurrency < 1 {
		concurrency = 1
	}
	limit := make(chan struct{}, concurrency)
	var waitGroup sync.WaitGroup
	for index := range rows {
		row := &rows[index]
		if row.Prunable || row.Bare || row.IsMain || row.Path == "" {
			continue
		}
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			select {
			case limit <- struct{}{}:
				defer func() { <-limit }()
			case <-ctx.Done():
				return
			}
			row.Graph = loadContextGraph(ctx, row.Path, "HEAD", "@{u}", mainRef, runner)
		}()
	}
	waitGroup.Wait()
}

// LoadBranchContextGraph builds the context graph for a single local branch that has
// no worktree, so the Git context frame can render for branch-only rows too. Unlike
// worktree rows there is no checkout to log from, so it runs from the repo root
// against the branch ref (and its upstream) directly. It is loaded lazily for the
// selected row, not eagerly for every branch, since a repo can have many branches and
// only the selected one is ever shown. Returns an unloaded graph when there is no main
// branch to compare against.
func LoadBranchContextGraph(ctx context.Context, repo Repository, branchName string, runner Runner) ContextGraph {
	if repo.MainBranch == "" || branchName == "" || branchName == repo.MainBranch {
		return ContextGraph{}
	}
	ref := "refs/heads/" + branchName
	mainRef := "refs/heads/" + repo.MainBranch
	return loadContextGraph(ctx, repo.Root, ref, branchName+"@{u}", mainRef, runner)
}

// loadContextGraph builds the commit graph for one ref relative to main and its
// upstream. ref is the tip to view from ("HEAD" for a worktree, "refs/heads/<name>"
// for a branch row); upstreamRef resolves that ref's upstream ("@{u}" for the
// checked-out HEAD, "<name>@{u}" for a branch). path is where the git commands run
// (the worktree path, or the repo root for a branch with no checkout).
func loadContextGraph(ctx context.Context, path, ref, upstreamRef, mainRef string, runner Runner) ContextGraph {
	graph := ContextGraph{}
	fetch := strconv.Itoa(graphCommitFetch)
	if output, err := runner.Run(ctx, path, "git", "log", "-n", fetch, "--format=%h%x1f%s", mainRef+".."+ref); err == nil {
		graph.BranchCommits = ParseGraphCommits(string(output))
		graph.Loaded = true
	}
	if output, err := runner.Run(ctx, path, "git", "log", "-n", fetch, "--format=%h%x1f%s", ref+".."+mainRef); err == nil {
		graph.MainCommits = ParseGraphCommits(string(output))
		graph.Loaded = true
	}
	// Commits on the upstream tracking branch that the ref lacks (what a pull would
	// bring in). Errors when there is no upstream; that is fine, we just skip them.
	if output, err := runner.Run(ctx, path, "git", "log", "-n", fetch, "--format=%h%x1f%s", ref+".."+upstreamRef); err == nil {
		graph.RemoteCommits = ParseGraphCommits(string(output))
	}
	// Fetch the fork point (merge-base) and a few shared ancestors below it, so the
	// frame can label the fork commit and pad short graphs with shared history.
	if output, err := runner.Run(ctx, path, "git", "merge-base", ref, mainRef); err == nil {
		base := strings.TrimSpace(string(output))
		if base != "" {
			if log, err := runner.Run(ctx, path, "git", "log", "-n", strconv.Itoa(graphBaseFetch), "--format=%h%x1f%s", base); err == nil {
				if commits := ParseGraphCommits(string(log)); len(commits) > 0 {
					graph.ForkPoint = commits[0]
					graph.BaseCommits = commits[1:]
					graph.Loaded = true
				}
			}
		}
	}
	return graph
}

func loadRefMetadata(ctx context.Context, repo Repository, runner Runner) (map[string]refMetadata, error) {
	mainRef := ""
	if repo.MainBranch != "" {
		mainRef = "refs/heads/" + repo.MainBranch
	}
	format := strings.Join([]string{
		"%(refname:short)",
		"%(objectname)",
		"%(objectname:short)",
		"%(committerdate:unix)",
		"%(contents:subject)",
		"%(upstream:short)",
		"%(upstream:track,nobracket)",
		"%(ahead-behind:" + mainRef + ")",
	}, "%00")
	output, err := runner.Run(ctx, repo.Root, "git", "for-each-ref", "--format="+format, "refs/heads")
	if err != nil {
		return nil, err
	}
	return ParseRefMetadata(string(output)), nil
}

func applyRefMetadata(row *Worktree, metadata refMetadata, mainBranch string) {
	row.Head = metadata.ObjectName
	row.CommitShort = metadata.ObjectShort
	row.CommitTime = metadata.CommitTime
	row.CommitSubject = metadata.Subject
	row.Upstream = metadata.Upstream
	row.UpstreamGone = metadata.UpstreamGone
	row.HeadSync = metadata.HeadSync
	if row.Upstream == "" {
		row.HeadSync = SyncState{NoUpstream: true}
	}
	if mainBranch != "" && row.Branch != mainBranch {
		row.MainSync = metadata.MainSync
	}
	if row.Branch == mainBranch {
		row.BranchMergedToMain = true
	} else if metadata.MainSync.Available && metadata.MainSync.Ahead == 0 {
		row.BranchMergedToMain = true
	}
}

func branchRowsFromMetadata(refMetadataByBranch map[string]refMetadata, worktrees []Worktree, mainBranch string) []Branch {
	checkedOut := make(map[string]bool, len(worktrees))
	for _, row := range worktrees {
		if row.Branch != "" && !row.Detached {
			checkedOut[row.Branch] = true
		}
	}
	branches := make([]Branch, 0, len(refMetadataByBranch))
	for name, metadata := range refMetadataByBranch {
		if checkedOut[name] {
			continue
		}
		branches = append(branches, branchFromMetadata(metadata, mainBranch))
	}
	sort.SliceStable(branches, func(leftIndex, rightIndex int) bool {
		return branches[leftIndex].CommitTime.After(branches[rightIndex].CommitTime)
	})
	return branches
}

func branchFromMetadata(metadata refMetadata, mainBranch string) Branch {
	branch := Branch{
		Name:          metadata.Branch,
		Head:          metadata.ObjectName,
		CommitShort:   metadata.ObjectShort,
		CommitTime:    metadata.CommitTime,
		CommitSubject: metadata.Subject,
		Upstream:      metadata.Upstream,
		UpstreamGone:  metadata.UpstreamGone,
		HeadSync:      metadata.HeadSync,
	}
	if branch.Upstream == "" {
		branch.HeadSync = SyncState{NoUpstream: true}
	}
	if mainBranch != "" && branch.Name != mainBranch {
		branch.MainSync = metadata.MainSync
	}
	if branch.Name == mainBranch {
		branch.BranchMergedToMain = true
	} else if metadata.MainSync.Available && metadata.MainSync.Ahead == 0 {
		branch.BranchMergedToMain = true
	}
	return branch
}

func enrichStatusCounts(ctx context.Context, rows []Worktree, runner Runner) {
	concurrency := min(4, runtime.NumCPU())
	if concurrency < 1 {
		concurrency = 1
	}
	type result struct {
		index  int
		status ParsedStatus
		ok     bool
	}
	results := make(chan result, len(rows))
	limit := make(chan struct{}, concurrency)
	var waitGroup sync.WaitGroup
	for index, row := range rows {
		if row.Prunable {
			continue
		}
		path := row.Path
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			select {
			case limit <- struct{}{}:
				defer func() { <-limit }()
			case <-ctx.Done():
				return
			}
			output, err := runner.Run(ctx, path, "git", "status", "--porcelain=v1", "-b", "--untracked-files=normal")
			if err != nil {
				results <- result{index: index}
				return
			}
			status := ParseStatusPorcelain(string(output))
			fillNumstat(ctx, path, &status, runner)
			results <- result{index: index, status: status, ok: true}
		}()
	}
	go func() {
		waitGroup.Wait()
		close(results)
	}()
	for result := range results {
		if !result.ok {
			continue
		}
		rows[result.index].Status = result.status.Counts
		rows[result.index].ChangedFiles = result.status.Files
		if rows[result.index].Upstream == "" {
			rows[result.index].Upstream = result.status.Upstream
			rows[result.index].UpstreamGone = result.status.UpstreamGone
		}
	}
}

// fillNumstat runs `git diff --numstat HEAD` for the worktree and merges the
// resulting line counts into the tracked entries of status. Untracked files and
// files without a numstat entry keep their unknown (-1) counts. Errors are
// non-fatal: the Changes frame simply omits stats it could not resolve.
func fillNumstat(ctx context.Context, path string, status *ParsedStatus, runner Runner) {
	hasTracked := false
	for _, file := range status.Files {
		if !file.Untracked() {
			hasTracked = true
			break
		}
	}
	if !hasTracked {
		return
	}
	output, err := runner.Run(ctx, path, "git", "diff", "--numstat", "HEAD")
	if err != nil {
		return
	}
	stats := ParseNumstat(string(output))
	for index := range status.Files {
		if stat, ok := stats[status.Files[index].Path]; ok {
			status.Files[index].Added = stat.Added
			status.Files[index].Deleted = stat.Deleted
		}
	}
}

func enrichDetachedWorktree(ctx context.Context, repo Repository, row *Worktree, mainExists bool, runner Runner) {
	row.HeadSync = SyncState{NoUpstream: true}
	if mainExists && repo.MainBranch != "" {
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

func CheckoutBranchWorktree(ctx context.Context, repoRoot, branch, path string, runner Runner) error {
	_, err := runner.Run(ctx, repoRoot, "git", "worktree", "add", path, branch)
	return err
}

func CheckoutPullRequestWorktree(ctx context.Context, repoRoot string, number int, branch, path string, runner Runner) error {
	if number <= 0 {
		return fmt.Errorf("pull request number is required")
	}
	if branch == "" {
		return fmt.Errorf("branch name is required")
	}
	if _, err := runner.Run(ctx, repoRoot, "git", "fetch", "origin", fmt.Sprintf("pull/%d/head", number)); err != nil {
		return err
	}
	_, err := runner.Run(ctx, repoRoot, "git", "worktree", "add", "-b", branch, path, "FETCH_HEAD")
	return err
}

func StashWorktreeChanges(ctx context.Context, path, message string, runner Runner) error {
	_, err := runner.Run(ctx, path, "git", "stash", "push", "-u", "-m", message)
	return err
}

func SwitchBranch(ctx context.Context, path, branch string, runner Runner) error {
	_, err := runner.Run(ctx, path, "git", "switch", "--", branch)
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

func CreateBranchAt(ctx context.Context, repoRoot, branch, commit string, runner Runner) error {
	if branch == "" || commit == "" {
		return nil
	}
	_, err := runner.Run(ctx, repoRoot, "git", "branch", branch, commit)
	return err
}

func FetchPrune(ctx context.Context, repoRoot string, runner Runner) error {
	_, err := runner.Run(ctx, repoRoot, "git", "fetch", "--prune")
	return err
}

func GitAwareDiskUsage(ctx context.Context, path string, runner Runner) (int64, error) {
	output, err := runner.Run(ctx, path, "git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return 0, err
	}
	var total int64
	for _, name := range strings.Split(string(output), "\x00") {
		if name == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return total, err
		}
		info, err := os.Lstat(filepath.Join(path, name))
		if err != nil || info.IsDir() {
			continue
		}
		total += info.Size()
	}
	return total, nil
}

func FullDiskUsage(ctx context.Context, path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(_ string, entry os.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
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

func DiskUsage(path string) (int64, error) {
	return FullDiskUsage(context.Background(), path)
}

const (
	diskBucketDependencies = "dependencies"
	diskBucketBuild        = "build output"
	diskBucketGit          = "git data"
	diskBucketSource       = "source"
)

// diskDependencyDirs and diskBuildDirs name directory segments treated as
// regenerable (safe to delete and rebuild). Anything under them is bucketed
// accordingly regardless of depth.
var (
	diskDependencyDirs = map[string]bool{
		"node_modules": true, "vendor": true, ".venv": true, "venv": true,
		".tox": true, ".gradle": true, ".cargo": true, "Pods": true,
	}
	diskBuildDirs = map[string]bool{
		"dist": true, "build": true, "out": true, "target": true,
		".next": true, ".nuxt": true, ".turbo": true, "coverage": true,
	}
)

// BucketedDiskUsage walks a worktree once and groups file sizes into cleanup
// buckets. It is the breakdown-aware replacement for FullDiskUsage when the Disk
// frame needs detail; the returned breakdown also carries the total.
func BucketedDiskUsage(ctx context.Context, path string) (DiskBreakdown, error) {
	sizes := map[string]int64{}
	var total int64
	err := filepath.WalkDir(path, func(entryPath string, entry os.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		relative, relErr := filepath.Rel(path, entryPath)
		if relErr != nil {
			relative = entryPath
		}
		sizes[classifyDiskPath(relative)] += info.Size()
		total += info.Size()
		return nil
	})
	if err != nil {
		return DiskBreakdown{}, err
	}
	return buildDiskBreakdown(sizes, total), nil
}

func classifyDiskPath(relative string) string {
	segments := strings.Split(relative, string(filepath.Separator))
	for _, segment := range segments {
		if diskDependencyDirs[segment] {
			return diskBucketDependencies
		}
	}
	for _, segment := range segments {
		if diskBuildDirs[segment] {
			return diskBucketBuild
		}
	}
	if len(segments) > 0 && segments[0] == ".git" {
		return diskBucketGit
	}
	return diskBucketSource
}

func buildDiskBreakdown(sizes map[string]int64, total int64) DiskBreakdown {
	breakdown := DiskBreakdown{Loaded: true, Total: total}
	for label, bytes := range sizes {
		if bytes == 0 {
			continue
		}
		breakdown.Buckets = append(breakdown.Buckets, DiskBucket{Label: label, Bytes: bytes})
		if label == diskBucketDependencies || label == diskBucketBuild {
			breakdown.ReclaimableBytes += bytes
		}
	}
	sort.SliceStable(breakdown.Buckets, func(left, right int) bool {
		return breakdown.Buckets[left].Bytes > breakdown.Buckets[right].Bytes
	})
	return breakdown
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
