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
