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
	results  map[string]fakeResult
}

type fakeResult struct {
	output []byte
	err    error
}

func (runner *fakeRunner) Run(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
	command := name + " " + strings.Join(args, " ")
	runner.commands = append(runner.commands, command)
	if result, ok := runner.results[command]; ok {
		return result.output, result.err
	}
	return runner.output, runner.err
}

func (runner *fakeRunner) RunWithEnv(ctx context.Context, dir string, _ []string, name string, args ...string) ([]byte, error) {
	return runner.Run(ctx, dir, name, args...)
}

func TestLoadPullRequestsFromAuthenticatedCLISkipsAuthStatus(t *testing.T) {
	runner := &fakeRunner{
		output: []byte(`[{"number":12,"state":"OPEN","isDraft":false,"headRefName":"feature/login","url":"https://github.com/acme/repo/pull/12"}]`),
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
	if !strings.Contains(runner.commands[0], "reviewDecision") {
		t.Fatalf("command = %q, want reviewDecision JSON field", runner.commands[0])
	}
	if strings.Contains(runner.commands[0], "statusCheckRollup") {
		t.Fatalf("command = %q, must not request statusCheckRollup (times out on large repos)", runner.commands[0])
	}
	pullRequest, ok := pullRequests["feature/login"]
	if !ok {
		t.Fatalf("missing pull request for feature/login: %#v", pullRequests)
	}
	if pullRequest.Number != 12 || pullRequest.State != "○" || pullRequest.URL == "" {
		t.Fatalf("pull request = %#v, want parsed PR 12", pullRequest)
	}
}

func TestLoadPullRequestsFromAuthenticatedCLIShowsApprovedPullRequest(t *testing.T) {
	runner := &fakeRunner{
		output: []byte(`[{"number":24,"state":"OPEN","isDraft":false,"reviewDecision":"APPROVED","headRefName":"feature/approved","url":"https://github.com/acme/repo/pull/24","statusCheckRollup":[]}]`),
	}

	pullRequests, enabled := LoadPullRequestsFromAuthenticatedCLI(context.Background(), "/repo", runner)

	if !enabled {
		t.Fatal("LoadPullRequestsFromAuthenticatedCLI() enabled = false, want true")
	}
	pullRequest, ok := pullRequests["feature/approved"]
	if !ok {
		t.Fatalf("missing pull request for feature/approved: %#v", pullRequests)
	}
	if pullRequest.State != "◆" {
		t.Fatalf("pull request state = %q, want approved glyph", pullRequest.State)
	}
}

func TestLoadPullRequestCIFetchesSinglePullRequestRollup(t *testing.T) {
	runner := &fakeRunner{
		output: []byte(`{"statusCheckRollup":[{"status":"COMPLETED","conclusion":"SUCCESS"}]}`),
	}

	glyph, ok := LoadPullRequestCI(context.Background(), "/repo", runner, 42)

	if !ok {
		t.Fatal("LoadPullRequestCI() ok = false, want true")
	}
	if glyph != "✓" {
		t.Fatalf("LoadPullRequestCI() glyph = %q, want success glyph", glyph)
	}
	if len(runner.commands) != 1 || !strings.HasPrefix(runner.commands[0], "gh pr view 42 ") {
		t.Fatalf("command = %v, want gh pr view 42", runner.commands)
	}
	if !strings.Contains(runner.commands[0], "statusCheckRollup") {
		t.Fatalf("command = %q, want statusCheckRollup JSON field", runner.commands[0])
	}
}

func TestLoadPullRequestForBranchPrefersOpenAndIncludesCI(t *testing.T) {
	runner := &fakeRunner{
		output: []byte(`[
			{"number":7,"state":"CLOSED","isDraft":false,"headRefName":"feature/x","url":"https://github.com/acme/repo/pull/7","statusCheckRollup":[]},
			{"number":9,"state":"OPEN","isDraft":false,"headRefName":"feature/x","url":"https://github.com/acme/repo/pull/9","statusCheckRollup":[{"status":"COMPLETED","conclusion":"SUCCESS"}]}
		]`),
	}

	pullRequest, ok := LoadPullRequestForBranch(context.Background(), "/repo", runner, "feature/x")

	if !ok {
		t.Fatal("LoadPullRequestForBranch() ok = false, want true")
	}
	if pullRequest.Number != 9 {
		t.Fatalf("Number = %d, want 9 (the open PR)", pullRequest.Number)
	}
	if pullRequest.State != "○" || pullRequest.CI != "✓" {
		t.Fatalf("pull request = %#v, want open state and passing CI", pullRequest)
	}
	if len(runner.commands) != 1 || !strings.Contains(runner.commands[0], "--head feature/x") {
		t.Fatalf("command = %v, want gh pr list --head feature/x", runner.commands)
	}
	if !strings.Contains(runner.commands[0], "statusCheckRollup") {
		t.Fatalf("command = %q, want statusCheckRollup (cheap for a single branch)", runner.commands[0])
	}
}

func TestLoadPullRequestForBranchReturnsFalseWhenNoPR(t *testing.T) {
	runner := &fakeRunner{output: []byte(`[]`)}

	if _, ok := LoadPullRequestForBranch(context.Background(), "/repo", runner, "feature/none"); ok {
		t.Fatal("LoadPullRequestForBranch() ok = true, want false for a branch with no PR")
	}
}

func TestLoadPullRequestSummariesSortsRecentFirstAndParsesBranchNames(t *testing.T) {
	runner := &fakeRunner{results: map[string]fakeResult{
		"gh repo view --json owner": {
			output: []byte(`{"owner":{"login":"schovi"}}`),
		},
		"gh pr list --limit 200 --state all --json number,title,state,isDraft,headRefName,headRepositoryOwner,url,reviewDecision,updatedAt": {
			output: []byte(`[
				{"number":12,"title":"Older same repo","state":"OPEN","isDraft":false,"headRefName":"feature/login","headRepositoryOwner":{"login":"schovi"},"url":"https://github.com/acme/repo/pull/12","updatedAt":"2026-06-01T10:00:00Z"},
				{"number":24,"title":"Newer fork","state":"MERGED","isDraft":false,"headRefName":"fix/docs","headRepositoryOwner":{"login":"alice"},"url":"https://github.com/acme/repo/pull/24","updatedAt":"2026-06-02T10:00:00Z"}
			]`),
		},
	}}

	summaries, err := LoadPullRequestSummaries(context.Background(), "/repo", runner)

	if err != nil {
		t.Fatalf("LoadPullRequestSummaries() error = %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("summary count = %d, want 2: %+v", len(summaries), summaries)
	}
	if summaries[0].Number != 24 || summaries[0].BranchName() != "alice/fix/docs" || summaries[0].StateGlyph() != "⬡" {
		t.Fatalf("first summary = %+v, want recent fork PR", summaries[0])
	}
	if summaries[1].Number != 12 || summaries[1].BranchName() != "feature/login" || summaries[1].StateGlyph() != "○" {
		t.Fatalf("second summary = %+v, want older same-repo PR", summaries[1])
	}
}

func TestLoadPullRequestSummaryLooksUpSinglePullRequest(t *testing.T) {
	runner := &fakeRunner{results: map[string]fakeResult{
		"gh repo view --json owner": {
			output: []byte(`{"owner":{"login":"schovi"}}`),
		},
		"gh pr view https://github.com/acme/repo/pull/42 --json number,title,state,isDraft,headRefName,headRepositoryOwner,url,reviewDecision,updatedAt": {
			output: []byte(`{"number":42,"title":"Direct lookup","state":"OPEN","isDraft":true,"headRefName":"draft-pr","headRepositoryOwner":{"login":"schovi"},"url":"https://github.com/acme/repo/pull/42","updatedAt":"2026-06-02T10:00:00Z"}`),
		},
	}}

	summary, err := LoadPullRequestSummary(context.Background(), "/repo", "https://github.com/acme/repo/pull/42", runner)

	if err != nil {
		t.Fatalf("LoadPullRequestSummary() error = %v", err)
	}
	if summary.Number != 42 || summary.BranchName() != "draft-pr" || summary.StateGlyph() != "◌" {
		t.Fatalf("summary = %+v, want draft PR 42", summary)
	}
}

func TestOpenPullRequestOpensQueryInBrowser(t *testing.T) {
	runner := &fakeRunner{results: map[string]fakeResult{
		"gh pr view 42 --web": {},
	}}

	err := OpenPullRequest(context.Background(), "/repo", "42", runner)

	if err != nil {
		t.Fatalf("OpenPullRequest() error = %v", err)
	}
	if len(runner.commands) != 1 || runner.commands[0] != "gh pr view 42 --web" {
		t.Fatalf("commands = %+v, want gh pr view --web", runner.commands)
	}
}

func TestLoadPullRequestSummariesReturnsGhErrors(t *testing.T) {
	runner := &fakeRunner{results: map[string]fakeResult{
		"gh repo view --json owner": {
			output: []byte(`{"owner":{"login":"schovi"}}`),
		},
		"gh pr list --limit 200 --state all --json number,title,state,isDraft,headRefName,headRepositoryOwner,url,reviewDecision,updatedAt": {
			err: assertError("gh auth failed"),
		},
	}}

	_, err := LoadPullRequestSummaries(context.Background(), "/repo", runner)

	if err == nil || !strings.Contains(err.Error(), "gh auth failed") {
		t.Fatalf("LoadPullRequestSummaries() error = %v, want gh auth failure", err)
	}
}

type assertError string

func (err assertError) Error() string {
	return string(err)
}
