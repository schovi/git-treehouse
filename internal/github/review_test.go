package github

import (
	"context"
	"strings"
	"testing"
)

func TestParsePullRequestReview(t *testing.T) {
	output := []byte(`{
		"number": 7,
		"url": "https://github.com/o/r/pull/7",
		"state": "OPEN",
		"isDraft": false,
		"mergeable": "MERGEABLE",
		"mergeStateStatus": "BLOCKED",
		"reviewDecision": "CHANGES_REQUESTED",
		"statusCheckRollup": [
			{"name": "build", "status": "COMPLETED", "conclusion": "SUCCESS"},
			{"name": "lint", "status": "COMPLETED", "conclusion": "FAILURE", "detailsUrl": "https://github.com/o/r/runs/9"},
			{"name": "e2e", "status": "IN_PROGRESS"},
			{"name": "flaky", "status": "COMPLETED", "conclusion": "SKIPPED"},
			{"context": "legacy/status", "state": "SUCCESS", "targetUrl": "https://ci.example/legacy"}
		],
		"reviews": [
			{"author": {"login": "alice"}, "state": "COMMENTED", "body": "looks ok"},
			{"author": {"login": "alice"}, "state": "CHANGES_REQUESTED", "body": "first ask\nignored second line"},
			{"author": {"login": "bob"}, "state": "CHANGES_REQUESTED", "body": "panic risk"}
		]
	}`)

	review, ok := ParsePullRequestReview(output)
	if !ok || !review.Loaded {
		t.Fatalf("ParsePullRequestReview ok=%v loaded=%v", ok, review.Loaded)
	}
	if review.Number != 7 || review.Mergeable != "MERGEABLE" || review.ReviewDecision != "CHANGES_REQUESTED" {
		t.Fatalf("scalar fields parsed incorrectly: %+v", review)
	}
	if review.URL != "https://github.com/o/r/pull/7" || review.MergeStateStatus != "BLOCKED" {
		t.Fatalf("url/mergeStateStatus parsed incorrectly: %+v", review)
	}

	checkURLByName := map[string]string{}
	for _, check := range review.Checks {
		checkURLByName[check.Name] = check.URL
	}
	if checkURLByName["lint"] != "https://github.com/o/r/runs/9" {
		t.Fatalf("lint check detailsUrl not captured: %+v", review.Checks)
	}
	if checkURLByName["legacy/status"] != "https://ci.example/legacy" {
		t.Fatalf("legacy status targetUrl not captured: %+v", review.Checks)
	}
	if review.ChangeRequests[0].URL != "https://github.com/o/r/pull/7" {
		t.Fatalf("change request should link to the PR: %+v", review.ChangeRequests[0])
	}

	pass, fail, running, skipped := review.CheckCounts()
	if pass != 2 || fail != 1 || running != 1 || skipped != 1 {
		t.Fatalf("CheckCounts = %d/%d/%d/%d, want 2/1/1/1", pass, fail, running, skipped)
	}

	if len(review.ChangeRequests) != 2 {
		t.Fatalf("ChangeRequests = %+v, want 2 (deduped by author)", review.ChangeRequests)
	}
	if review.ChangeRequests[0].Author != "alice" || review.ChangeRequests[0].Body != "first ask" {
		t.Fatalf("alice change request parsed incorrectly: %+v", review.ChangeRequests[0])
	}
	if review.ChangeRequests[1].Author != "bob" {
		t.Fatalf("bob change request missing: %+v", review.ChangeRequests)
	}
}

func TestParseChecksClassifiesStates(t *testing.T) {
	output := []byte(`{"statusCheckRollup": [
		{"name": "ok", "conclusion": "SUCCESS"},
		{"name": "boom", "conclusion": "TIMED_OUT"},
		{"name": "wait", "status": "QUEUED"},
		{"context": "ctx-fail", "state": "FAILURE"}
	]}`)
	review, ok := ParsePullRequestReview(output)
	if !ok {
		t.Fatal("parse failed")
	}
	want := map[string]string{"ok": CheckPass, "boom": CheckFail, "wait": CheckRunning, "ctx-fail": CheckFail}
	for _, check := range review.Checks {
		if want[check.Name] != check.State {
			t.Fatalf("check %q state = %q, want %q", check.Name, check.State, want[check.Name])
		}
	}
}

func TestParsePullRequestReviewInvalid(t *testing.T) {
	if _, ok := ParsePullRequestReview([]byte("not json")); ok {
		t.Fatal("expected parse failure on invalid JSON")
	}
}

func TestLoadPullRequestReviewUsesPRViewOnly(t *testing.T) {
	runner := &fakeRunner{output: []byte(`{"number":7,"url":"https://github.com/o/r/pull/7"}`)}

	if _, ok := LoadPullRequestReview(context.Background(), "/repo", runner, 7); !ok {
		t.Fatal("LoadPullRequestReview() failed")
	}
	if len(runner.commands) != 1 {
		t.Fatalf("command count = %d, want 1: %v", len(runner.commands), runner.commands)
	}
	if command := runner.commands[0]; !strings.HasPrefix(command, "gh pr view 7 ") || strings.Contains(command, "api graphql") || strings.Contains(command, "reviewThreads") {
		t.Fatalf("LoadPullRequestReview() command = %q, want gh pr view without GraphQL review threads", command)
	}
}
