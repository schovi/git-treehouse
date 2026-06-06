package github

import (
	"context"
	"strings"
	"testing"
)

type fakeRunner struct {
	commands []string
	output   []byte
	err      error
}

func (runner *fakeRunner) Run(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
	runner.commands = append(runner.commands, name+" "+strings.Join(args, " "))
	return runner.output, runner.err
}

func TestLoadPullRequestsFromAuthenticatedCLISkipsAuthStatus(t *testing.T) {
	runner := &fakeRunner{
		output: []byte(`[{"number":12,"state":"OPEN","isDraft":false,"headRefName":"feature/login","url":"https://github.com/acme/repo/pull/12","statusCheckRollup":[]}]`),
	}

	pullRequests, enabled := LoadPullRequestsFromAuthenticatedCLI(context.Background(), "/repo", runner)

	if !enabled {
		t.Fatal("LoadPullRequestsFromAuthenticatedCLI() enabled = false, want true")
	}
	if len(runner.commands) != 1 {
		t.Fatalf("command count = %d, want 1: %v", len(runner.commands), runner.commands)
	}
	if strings.Contains(runner.commands[0], "auth status") {
		t.Fatalf("LoadPullRequestsFromAuthenticatedCLI() ran auth status: %v", runner.commands)
	}
	if !strings.HasPrefix(runner.commands[0], "gh pr list ") {
		t.Fatalf("command = %q, want gh pr list", runner.commands[0])
	}
	pullRequest, ok := pullRequests["feature/login"]
	if !ok {
		t.Fatalf("missing pull request for feature/login: %#v", pullRequests)
	}
	if pullRequest.Number != 12 || pullRequest.State != "○" || pullRequest.URL == "" {
		t.Fatalf("pull request = %#v, want parsed PR 12", pullRequest)
	}
}
