package gitdata

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/schovi/git-treehouse/internal/config"
)

type fakeRunner map[string]fakeResult

type fakeResult struct {
	output string
	err    error
}

func (runner fakeRunner) Run(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	key := dir + "|" + name + " " + strings.Join(args, " ")
	result, ok := runner[key]
	if !ok {
		return nil, errors.New("unexpected command: " + key)
	}
	return []byte(result.output), result.err
}

func (runner fakeRunner) RunWithEnv(ctx context.Context, dir string, _ []string, name string, args ...string) ([]byte, error) {
	return runner.Run(ctx, dir, name, args...)
}

type recordingFakeRunner struct {
	mutex    sync.Mutex
	commands []string
}

func (runner *recordingFakeRunner) Run(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	key := dir + "|" + name + " " + strings.Join(args, " ")
	runner.mutex.Lock()
	runner.commands = append(runner.commands, key)
	runner.mutex.Unlock()
	if name == "git" && len(args) > 0 {
		switch args[0] {
		case "rev-parse":
			switch strings.Join(args[1:], " ") {
			case "--show-toplevel":
				return []byte("/repo/main\n"), nil
			case "--git-common-dir":
				return []byte(".git\n"), nil
			case "--path-format=absolute --git-common-dir":
				return []byte("/repo/main/.git\n"), nil
			}
		case "worktree":
			return []byte("worktree /repo/main\nHEAD aaaaaaaa\nbranch refs/heads/main\n\nworktree /repo/feature\nHEAD bbbbbbbb\nbranch refs/heads/feature\n"), nil
		case "symbolic-ref":
			return nil, errors.New("no origin")
		case "show-ref":
			return nil, nil
		case "remote":
			return []byte(""), nil
		case "for-each-ref":
			return []byte("main\x00aaaaaaaa\x00aaaaaaa\x001780000000\x00main commit\x00origin/main\x00\x000 0\n" +
				"feature\x00bbbbbbbb\x00bbbbbbb\x001780000100\x00feature commit\x00origin/feature\x00ahead 2, behind 1\x002 5\n"), nil
		case "status":
			return []byte("## feature\n"), nil
		}
	}
	return nil, errors.New("unexpected command: " + key)
}

func (runner *recordingFakeRunner) RunWithEnv(ctx context.Context, dir string, _ []string, name string, args ...string) ([]byte, error) {
	return runner.Run(ctx, dir, name, args...)
}

type branchRowFakeRunner struct{}

func (runner branchRowFakeRunner) Run(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	if name != "git" {
		return nil, errors.New("unexpected command: " + name)
	}
	command := strings.Join(args, " ")
	switch {
	case dir == "/repo/main" && command == "rev-parse --show-toplevel":
		return []byte("/repo/main\n"), nil
	case dir == "/repo/main" && command == "rev-parse --git-common-dir":
		return []byte(".git\n"), nil
	case dir == "/repo/main" && command == "rev-parse --path-format=absolute --git-common-dir":
		return []byte("/repo/main/.git\n"), nil
	case dir == "/repo/main" && command == "worktree list --porcelain":
		return []byte("worktree /repo/main\nHEAD aaaaaaaa\nbranch refs/heads/main\n"), nil
	case dir == "/repo/main" && command == "symbolic-ref --short refs/remotes/origin/HEAD":
		return nil, errors.New("no origin")
	case dir == "/repo/main" && command == "show-ref --verify --quiet refs/heads/main":
		return nil, nil
	case dir == "/repo/main" && command == "remote":
		return []byte("origin\n"), nil
	case dir == "/repo/main" && strings.HasPrefix(command, "for-each-ref --format="):
		return []byte("main\x00aaaaaaaa\x00aaaaaaa\x001780000000\x00main commit\x00origin/main\x00\x000 0\n" +
			"feature/branch\x00bbbbbbbb\x00bbbbbbb\x001780000100\x00branch commit\x00origin/feature/branch\x00ahead 1\x003 4\n"), nil
	case dir == "/repo/main" && command == "status --porcelain=v1 -b --untracked-files=normal":
		return []byte("## main...origin/main\n"), nil
	}
	return nil, errors.New("unexpected command: " + dir + "|" + name + " " + command)
}

func (runner branchRowFakeRunner) RunWithEnv(ctx context.Context, dir string, _ []string, name string, args ...string) ([]byte, error) {
	return runner.Run(ctx, dir, name, args...)
}

func TestResolveRepositorySupportsBareInvocation(t *testing.T) {
	runner := fakeRunner{
		"/repo.git|git rev-parse --show-toplevel":                         {err: errors.New("no work tree")},
		"/repo.git|git rev-parse --is-bare-repository":                    {output: "true\n"},
		"/repo.git|git rev-parse --git-common-dir":                        {output: ".\n"},
		"/repo.git|git rev-parse --path-format=absolute --git-common-dir": {output: "/repo.git\n"},
		"/repo.git|git worktree list --porcelain":                         {output: "worktree /repo.git\nbare\n\nworktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n"},
		"/repo/main|git symbolic-ref --short refs/remotes/origin/HEAD":    {err: errors.New("no origin")},
		"/repo/main|git show-ref --verify --quiet refs/heads/main":        {},
		"/repo/main|git remote":                                           {output: ""},
	}

	repo, err := ResolveRepository(context.Background(), "/repo.git", config.Config{}, runner)
	if err != nil {
		t.Fatalf("ResolveRepository() error = %v", err)
	}
	if repo.Root != "/repo/main" {
		t.Fatalf("ResolveRepository().Root = %q, want /repo/main", repo.Root)
	}
	if repo.ActiveWorktree != "" {
		t.Fatalf("ResolveRepository().ActiveWorktree = %q, want empty for bare invocation", repo.ActiveWorktree)
	}
	if repo.MainWorktree != "/repo/main" {
		t.Fatalf("ResolveRepository().MainWorktree = %q, want /repo/main", repo.MainWorktree)
	}
}

func TestResolveRepositorySupportsWorktreePath(t *testing.T) {
	worktreeList := "worktree /repo/main\nHEAD aaaaaaaa\nbranch refs/heads/main\n\nworktree /repo/feature\nHEAD bbbbbbbb\nbranch refs/heads/feature\n"
	runner := fakeRunner{
		"/repo/feature|git rev-parse --show-toplevel":                         {output: "/repo/feature\n"},
		"/repo/feature|git rev-parse --git-common-dir":                        {output: "/repo/main/.git\n"},
		"/repo/feature|git rev-parse --path-format=absolute --git-common-dir": {output: "/repo/main/.git\n"},
		"/repo/feature|git worktree list --porcelain":                         {output: worktreeList},
		"/repo/main|git symbolic-ref --short refs/remotes/origin/HEAD":        {err: errors.New("no origin")},
		"/repo/main|git show-ref --verify --quiet refs/heads/main":            {},
		"/repo/main|git remote":                                               {output: ""},
	}

	repo, err := ResolveRepository(context.Background(), "/repo/feature", config.Config{}, runner)
	if err != nil {
		t.Fatalf("ResolveRepository() error = %v", err)
	}
	if repo.Root != "/repo/main" {
		t.Fatalf("ResolveRepository().Root = %q, want /repo/main", repo.Root)
	}
	if repo.ActiveWorktree != "/repo/feature" {
		t.Fatalf("ResolveRepository().ActiveWorktree = %q, want /repo/feature", repo.ActiveWorktree)
	}
	if repo.Cwd != "/repo/feature" {
		t.Fatalf("ResolveRepository().Cwd = %q, want /repo/feature", repo.Cwd)
	}
}

func TestLoadComputesMainSyncForRootWorktreeOnFeatureBranch(t *testing.T) {
	worktreeList := "worktree /repo/main\nHEAD abc123456789\nbranch refs/heads/codex/list-rendering-polish\n"
	runner := fakeRunner{
		"/repo/main|git rev-parse --show-toplevel":                                        {output: "/repo/main\n"},
		"/repo/main|git rev-parse --git-common-dir":                                       {output: ".git\n"},
		"/repo/main|git rev-parse --path-format=absolute --git-common-dir":                {output: "/repo/main/.git\n"},
		"/repo/main|git worktree list --porcelain":                                        {output: worktreeList},
		"/repo/main|git symbolic-ref --short refs/remotes/origin/HEAD":                    {err: errors.New("no origin")},
		"/repo/main|git show-ref --verify --quiet refs/heads/main":                        {},
		"/repo/main|git remote":                                                           {output: ""},
		"/repo/main|git status --porcelain=v1 -b --untracked-files=normal":                {output: "## codex/list-rendering-polish\n"},
		"/repo/main|git rev-parse --abbrev-ref --symbolic-full-name @{u}":                 {err: errors.New("no upstream")},
		"/repo/main|git rev-list --left-right --count HEAD...refs/heads/main":             {output: "1 14\n"},
		"/repo/main|git log -1 --format=%h%x00%ct%x00%s":                                  {output: "abc1234\x001780000000\x00feature commit\n"},
		"/repo/main|git show-ref --verify --quiet refs/heads/codex/list-rendering-polish": {err: errors.New("missing local branch ref")},
	}

	state, err := Load(context.Background(), "/repo/main", config.Config{}, runner)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(state.Rows) != 1 {
		t.Fatalf("Load() rows = %d, want 1", len(state.Rows))
	}
	row := state.Rows[0]
	if !row.IsMain {
		t.Fatalf("root row IsMain = false, want true")
	}
	if !row.MainSync.Available || row.MainSync.Ahead != 1 || row.MainSync.Behind != 14 {
		t.Fatalf("root row MainSync = %+v, want available ↑1 ↓14", row.MainSync)
	}
}

func TestLoadUsesOneWorktreeListAndBatchedRefMetadata(t *testing.T) {
	runner := &recordingFakeRunner{}

	state, err := Load(context.Background(), "/repo/main", config.Config{}, runner)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	worktreeListCalls := 0
	for _, command := range runner.commands {
		if strings.Contains(command, "git worktree list --porcelain") {
			worktreeListCalls++
		}
		for _, unwanted := range []string{"git log -1", "git merge-base --is-ancestor", "git rev-list --left-right --count HEAD...@{u}"} {
			if strings.Contains(command, unwanted) {
				t.Fatalf("Load() should use batched refs, but ran %q in commands %v", unwanted, runner.commands)
			}
		}
	}
	if worktreeListCalls != 1 {
		t.Fatalf("worktree list calls = %d, want 1: %v", worktreeListCalls, runner.commands)
	}
	if len(state.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(state.Rows))
	}
	feature := state.Rows[1]
	if feature.Branch != "feature" || feature.CommitShort != "bbbbbbb" || feature.CommitSubject != "feature commit" {
		t.Fatalf("feature metadata = %+v, want batched commit metadata", feature)
	}
	if !feature.HeadSync.Available || feature.HeadSync.Ahead != 2 || feature.HeadSync.Behind != 1 {
		t.Fatalf("feature HeadSync = %+v, want ↑2 ↓1", feature.HeadSync)
	}
	if !feature.MainSync.Available || feature.MainSync.Ahead != 2 || feature.MainSync.Behind != 5 {
		t.Fatalf("feature MainSync = %+v, want ↑2 ↓5", feature.MainSync)
	}
}

func TestLoadAddsBranchRowsForLocalBranchesWithoutWorktrees(t *testing.T) {
	state, err := Load(context.Background(), "/repo/main", config.Config{}, branchRowFakeRunner{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(state.Rows) != 1 {
		t.Fatalf("worktree rows = %d, want 1", len(state.Rows))
	}
	if len(state.Branches) != 1 {
		t.Fatalf("branch rows = %d, want 1: %+v", len(state.Branches), state.Branches)
	}
	branch := state.Branches[0]
	if branch.Name != "feature/branch" || branch.CommitShort != "bbbbbbb" || branch.CommitSubject != "branch commit" {
		t.Fatalf("branch metadata = %+v, want feature branch metadata", branch)
	}
	if !branch.HeadSync.Available || branch.HeadSync.Ahead != 1 {
		t.Fatalf("branch HeadSync = %+v, want ↑1", branch.HeadSync)
	}
	if !branch.MainSync.Available || branch.MainSync.Ahead != 3 || branch.MainSync.Behind != 4 {
		t.Fatalf("branch MainSync = %+v, want ↑3 ↓4", branch.MainSync)
	}
}

func TestCreateBranchAtUsesGitBranchWithCommit(t *testing.T) {
	runner := fakeRunner{
		"/repo/main|git branch feature abcdef1234567890": {},
	}

	err := CreateBranchAt(context.Background(), "/repo/main", "feature", "abcdef1234567890", runner)

	if err != nil {
		t.Fatalf("CreateBranchAt() error = %v", err)
	}
}

func TestCreateBranchAtSkipsEmptyBranchOrCommit(t *testing.T) {
	runner := fakeRunner{}

	if err := CreateBranchAt(context.Background(), "/repo/main", "", "abcdef1234567890", runner); err != nil {
		t.Fatalf("CreateBranchAt() empty branch error = %v", err)
	}
	if err := CreateBranchAt(context.Background(), "/repo/main", "feature", "", runner); err != nil {
		t.Fatalf("CreateBranchAt() empty commit error = %v", err)
	}
}

func TestFullDiskUsageCanBeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := FullDiskUsage(ctx, t.TempDir())

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("FullDiskUsage() error = %v, want context.Canceled", err)
	}
}

func TestGitAwareDiskUsageUsesGitFileList(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("tracked"), 0600); err != nil {
		t.Fatalf("write tracked: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules"), 0700); err != nil {
		t.Fatalf("mkdir ignored: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "cache.bin"), []byte("ignored"), 0600); err != nil {
		t.Fatalf("write ignored: %v", err)
	}
	runner := fakeRunner{
		dir + "|git ls-files -z --cached --others --exclude-standard": {output: "tracked.txt\x00"},
	}

	size, err := GitAwareDiskUsage(context.Background(), dir, runner)

	if err != nil {
		t.Fatalf("GitAwareDiskUsage() error = %v", err)
	}
	if size != int64(len("tracked")) {
		t.Fatalf("GitAwareDiskUsage() = %d, want tracked file size only", size)
	}
}
