package github

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/schovi/git-treehouse/internal/gitdata"
)

// Check states used by the PR review frame.
const (
	CheckPass    = "pass"
	CheckFail    = "fail"
	CheckRunning = "running"
	CheckSkipped = "skipped"
)

// Check is a single CI check or status context for a pull request. URL points at
// the check's detail page on the web, when the provider exposes one.
type Check struct {
	Name  string
	State string
	URL   string
}

// ReviewNote is a reviewer's change-request review: who asked, the first line of
// what they wrote, and a link to the pull request where the review lives.
type ReviewNote struct {
	Author string
	Body   string
	URL    string
}

// PullRequestReview is the detailed review/CI state behind a single PR, powering
// the PR review frame. It is loaded lazily for the selected row.
type PullRequestReview struct {
	Loaded           bool
	Number           int
	URL              string
	State            string
	IsDraft          bool
	Mergeable        string
	MergeStateStatus string
	ReviewDecision   string
	Checks           []Check
	ChangeRequests   []ReviewNote
}

// CheckCounts rolls the checks up into pass/fail/running/skipped totals.
func (review PullRequestReview) CheckCounts() (pass, fail, running, skipped int) {
	for _, check := range review.Checks {
		switch check.State {
		case CheckPass:
			pass++
		case CheckFail:
			fail++
		case CheckRunning:
			running++
		case CheckSkipped:
			skipped++
		}
	}
	return pass, fail, running, skipped
}

type rawReviewView struct {
	Number            int               `json:"number"`
	URL               string            `json:"url"`
	State             string            `json:"state"`
	IsDraft           bool              `json:"isDraft"`
	Mergeable         string            `json:"mergeable"`
	MergeStateStatus  string            `json:"mergeStateStatus"`
	ReviewDecision    string            `json:"reviewDecision"`
	StatusCheckRollup []json.RawMessage `json:"statusCheckRollup"`
	Reviews           []rawReview       `json:"reviews"`
}

type rawReview struct {
	Author rawOwner `json:"author"`
	State  string   `json:"state"`
	Body   string   `json:"body"`
}

// LoadPullRequestReview fetches the detailed review/CI state for one PR. ok is
// false on error (caller keeps whatever it had).
func LoadPullRequestReview(ctx context.Context, repoRoot string, runner gitdata.Runner, number int) (PullRequestReview, bool) {
	output, err := runner.Run(ctx, repoRoot, "gh", "pr", "view", strconv.Itoa(number),
		"--json", "number,url,state,isDraft,mergeable,mergeStateStatus,reviewDecision,statusCheckRollup,reviews")
	if err != nil {
		return PullRequestReview{}, false
	}
	review, ok := ParsePullRequestReview(output)
	if !ok {
		return review, ok
	}
	return review, true
}

// ParsePullRequestReview parses the JSON from `gh pr view --json ...` into a
// PullRequestReview. It is pure so it can be tested without gh.
func ParsePullRequestReview(output []byte) (PullRequestReview, bool) {
	var raw rawReviewView
	if err := json.Unmarshal(output, &raw); err != nil {
		return PullRequestReview{}, false
	}
	review := PullRequestReview{
		Loaded:           true,
		Number:           raw.Number,
		URL:              raw.URL,
		State:            raw.State,
		IsDraft:          raw.IsDraft,
		Mergeable:        raw.Mergeable,
		MergeStateStatus: raw.MergeStateStatus,
		ReviewDecision:   raw.ReviewDecision,
		Checks:           ParseChecks(raw.StatusCheckRollup),
		ChangeRequests:   parseChangeRequests(raw.Reviews, raw.URL),
	}
	return review, true
}

// ParseChecks turns statusCheckRollup items (CheckRun or StatusContext shapes)
// into named checks with a normalized state.
func ParseChecks(items []json.RawMessage) []Check {
	checks := make([]Check, 0, len(items))
	for _, item := range items {
		var object map[string]any
		if err := json.Unmarshal(item, &object); err != nil {
			continue
		}
		name := stringValue(object["name"])
		if name == "" {
			name = stringValue(object["context"])
		}
		if name == "" {
			name = "check"
		}
		url := stringValue(object["detailsUrl"])
		if url == "" {
			url = stringValue(object["targetUrl"])
		}
		checks = append(checks, Check{Name: name, State: checkState(object), URL: url})
	}
	return checks
}

func checkState(object map[string]any) string {
	status := upperString(object["status"])
	conclusion := upperString(object["conclusion"])
	state := upperString(object["state"]) // StatusContext shape
	switch {
	case isFailureConclusion(conclusion) || state == "FAILURE" || state == "ERROR":
		return CheckFail
	case conclusion == "SKIPPED" || conclusion == "NEUTRAL":
		return CheckSkipped
	case conclusion == "SUCCESS" || state == "SUCCESS":
		return CheckPass
	case isRunningStatus(status) || state == "PENDING" || state == "EXPECTED":
		return CheckRunning
	case conclusion == "":
		return CheckRunning
	default:
		return CheckRunning
	}
}

func isFailureConclusion(conclusion string) bool {
	switch conclusion {
	case "FAILURE", "FAILED", "ERROR", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE":
		return true
	default:
		return false
	}
}

func isRunningStatus(status string) bool {
	switch status {
	case "IN_PROGRESS", "QUEUED", "PENDING", "EXPECTED", "WAITING", "REQUESTED":
		return true
	default:
		return false
	}
}

// parseChangeRequests keeps the latest change-request review per author, newest
// last in the gh output, so the frame shows who currently wants changes. Each note
// links to the pull request, since gh does not expose a per-review permalink.
func parseChangeRequests(reviews []rawReview, prURL string) []ReviewNote {
	latestByAuthor := map[string]ReviewNote{}
	order := []string{}
	for _, review := range reviews {
		if strings.ToUpper(review.State) != "CHANGES_REQUESTED" {
			continue
		}
		author := review.Author.Login
		if _, seen := latestByAuthor[author]; !seen {
			order = append(order, author)
		}
		latestByAuthor[author] = ReviewNote{Author: author, Body: firstLine(review.Body), URL: prURL}
	}
	notes := make([]ReviewNote, 0, len(order))
	for _, author := range order {
		notes = append(notes, latestByAuthor[author])
	}
	return notes
}

func firstLine(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if line, _, found := strings.Cut(body, "\n"); found {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(body)
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
