package gitdata

import (
	"context"
	"errors"
	"strings"
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
