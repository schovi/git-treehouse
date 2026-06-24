package github

import "testing"

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

func TestParseReviewThreads(t *testing.T) {
	output := []byte(`{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[
		{"isResolved":false,"isOutdated":false,"comments":{"nodes":[
			{"author":{"login":"Copilot"},"body":"consider a nil check\nmore detail","url":"https://github.com/o/r/pull/1#discussion_r1","path":"internal/tui/model.go","line":42}
		]}},
		{"isResolved":true,"isOutdated":true,"comments":{"nodes":[
			{"author":{"login":"alice"},"body":"done","url":"https://github.com/o/r/pull/1#discussion_r2","path":"main.go","line":7}
		]}},
		{"isResolved":false,"isOutdated":false,"comments":{"nodes":[]}}
	]}}}}}`)

	threads := ParseReviewThreads(output)
	if len(threads) != 2 {
		t.Fatalf("ParseReviewThreads = %d threads, want 2 (empty-comment thread skipped)", len(threads))
	}
	first := threads[0]
	if first.Author != "Copilot" || first.Resolved || first.Body != "consider a nil check" {
		t.Fatalf("first thread parsed incorrectly: %+v", first)
	}
	if first.URL != "https://github.com/o/r/pull/1#discussion_r1" || first.Path != "internal/tui/model.go" || first.Line != 42 {
		t.Fatalf("first thread location/url incorrect: %+v", first)
	}
	if !threads[1].Resolved || !threads[1].Outdated {
		t.Fatalf("second thread should be resolved+outdated: %+v", threads[1])
	}

	review := PullRequestReview{Threads: threads}
	if unresolved, resolved := review.ThreadCounts(); unresolved != 1 || resolved != 1 {
		t.Fatalf("ThreadCounts = %d/%d, want 1/1", unresolved, resolved)
	}
}

func TestParseOwnerRepo(t *testing.T) {
	owner, name, ok := parseOwnerRepo("https://github.com/productboard/pb-backend/pull/24128")
	if !ok || owner != "productboard" || name != "pb-backend" {
		t.Fatalf("parseOwnerRepo = %q/%q ok=%v, want productboard/pb-backend", owner, name, ok)
	}
	if _, _, ok := parseOwnerRepo(""); ok {
		t.Fatal("parseOwnerRepo(\"\") should fail")
	}
}
