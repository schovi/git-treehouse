package github

import (
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"

	"github.com/schovi/git-treehouse/internal/gitdata"
)

type rawPullRequest struct {
	Number            int               `json:"number"`
	State             string            `json:"state"`
	IsDraft           bool              `json:"isDraft"`
	HeadRefName       string            `json:"headRefName"`
	URL               string            `json:"url"`
	ReviewDecision    string            `json:"reviewDecision"`
	StatusCheckRollup []json.RawMessage `json:"statusCheckRollup"`
}

func Available(ctx context.Context, repoRoot string, runner gitdata.Runner) bool {
	if _, err := exec.LookPath("gh"); err != nil {
		return false
	}
	_, err := runner.Run(ctx, repoRoot, "gh", "auth", "status")
	return err == nil
}

func LoadPullRequests(ctx context.Context, repoRoot string, runner gitdata.Runner) (map[string]gitdata.PullRequest, bool) {
	if !Available(ctx, repoRoot, runner) {
		return nil, false
	}
	return LoadPullRequestsFromAuthenticatedCLI(ctx, repoRoot, runner)
}

func LoadPullRequestsFromAuthenticatedCLI(ctx context.Context, repoRoot string, runner gitdata.Runner) (map[string]gitdata.PullRequest, bool) {
	output, err := runner.Run(ctx, repoRoot, "gh", "pr", "list", "--limit", "200", "--state", "all", "--json", "number,state,isDraft,headRefName,url,reviewDecision,statusCheckRollup")
	if err != nil {
		return nil, false
	}
	var rawPullRequests []rawPullRequest
	if err := json.Unmarshal(output, &rawPullRequests); err != nil {
		return nil, false
	}
	pullRequests := make(map[string]gitdata.PullRequest, len(rawPullRequests))
	for _, raw := range rawPullRequests {
		if raw.HeadRefName == "" {
			continue
		}
		pullRequests[raw.HeadRefName] = gitdata.PullRequest{
			Number: raw.Number,
			State:  stateGlyph(raw.State, raw.IsDraft, raw.ReviewDecision),
			CI:     ciGlyph(raw.StatusCheckRollup),
			URL:    raw.URL,
		}
	}
	return pullRequests, true
}

func AttachPullRequests(rows []gitdata.Worktree, pullRequests map[string]gitdata.PullRequest) []gitdata.Worktree {
	for index := range rows {
		if pullRequest, ok := pullRequests[rows[index].Branch]; ok {
			rows[index].PR = &pullRequest
		}
	}
	return rows
}

func AttachBranchPullRequests(branches []gitdata.Branch, pullRequests map[string]gitdata.PullRequest) []gitdata.Branch {
	for index := range branches {
		if pullRequest, ok := pullRequests[branches[index].Name]; ok {
			branches[index].PR = &pullRequest
		}
	}
	return branches
}

func OpenPullRequestOrBranch(ctx context.Context, repoRoot string, row gitdata.Worktree, runner gitdata.Runner) error {
	if row.PR != nil && row.PR.Number > 0 {
		_, err := runner.Run(ctx, repoRoot, "gh", "pr", "view", strconv.Itoa(row.PR.Number), "--web")
		return err
	}
	if row.Branch != "" {
		_, err := runner.Run(ctx, repoRoot, "gh", "browse", "--branch", row.Branch)
		return err
	}
	_, err := runner.Run(ctx, repoRoot, "gh", "browse")
	return err
}

func OpenRowPullRequestOrBranch(ctx context.Context, repoRoot string, row gitdata.Row, runner gitdata.Runner) error {
	if row.IsBranch() {
		if row.Branch.PR != nil && row.Branch.PR.Number > 0 {
			_, err := runner.Run(ctx, repoRoot, "gh", "pr", "view", strconv.Itoa(row.Branch.PR.Number), "--web")
			return err
		}
		if row.Branch.Name != "" {
			_, err := runner.Run(ctx, repoRoot, "gh", "browse", "--branch", row.Branch.Name)
			return err
		}
		_, err := runner.Run(ctx, repoRoot, "gh", "browse")
		return err
	}
	return OpenPullRequestOrBranch(ctx, repoRoot, row.Worktree, runner)
}

func stateGlyph(state string, draft bool, reviewDecision string) string {
	if draft {
		return "◌"
	}
	switch strings.ToUpper(state) {
	case "OPEN":
		if strings.ToUpper(reviewDecision) == "APPROVED" {
			return "◆"
		}
		return "○"
	case "MERGED":
		return "⬡"
	case "CLOSED":
		return "✕"
	default:
		return "○"
	}
}

func ciGlyph(items []json.RawMessage) string {
	if len(items) == 0 {
		return ""
	}
	running := false
	success := false
	for _, item := range items {
		var object map[string]any
		if err := json.Unmarshal(item, &object); err != nil {
			continue
		}
		status := upperString(object["status"])
		conclusion := upperString(object["conclusion"])
		if status == "IN_PROGRESS" || status == "QUEUED" || status == "PENDING" || status == "EXPECTED" {
			running = true
		}
		if conclusion == "FAILURE" || conclusion == "FAILED" || conclusion == "ERROR" || conclusion == "TIMED_OUT" || conclusion == "CANCELLED" {
			return "✗"
		}
		if conclusion == "SUCCESS" || conclusion == "NEUTRAL" || conclusion == "SKIPPED" {
			success = true
		}
	}
	if running {
		return "●"
	}
	if success {
		return "✓"
	}
	return ""
}

func upperString(value any) string {
	if text, ok := value.(string); ok {
		return strings.ToUpper(text)
	}
	return ""
}
