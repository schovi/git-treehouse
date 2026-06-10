package github

import (
	"context"
	"encoding/json"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/schovi/git-treehouse/internal/gitdata"
)

type rawPullRequest struct {
	Number              int               `json:"number"`
	Title               string            `json:"title"`
	State               string            `json:"state"`
	IsDraft             bool              `json:"isDraft"`
	HeadRefName         string            `json:"headRefName"`
	HeadRepositoryOwner rawOwner          `json:"headRepositoryOwner"`
	URL                 string            `json:"url"`
	ReviewDecision      string            `json:"reviewDecision"`
	StatusCheckRollup   []json.RawMessage `json:"statusCheckRollup"`
	UpdatedAt           time.Time         `json:"updatedAt"`
}

type rawOwner struct {
	Login string `json:"login"`
}

type rawRepository struct {
	Owner rawOwner `json:"owner"`
}

const pullRequestSummaryFields = "number,title,state,isDraft,headRefName,headRepositoryOwner,url,reviewDecision,updatedAt"

type PullRequestSummary struct {
	Number              int
	Title               string
	State               string
	IsDraft             bool
	ReviewDecision      string
	CI                  string
	URL                 string
	HeadRefName         string
	HeadRepositoryOwner string
	BaseRepositoryOwner string
	UpdatedAt           time.Time
}

func (pullRequest PullRequestSummary) StateGlyph() string {
	return stateGlyph(pullRequest.State, pullRequest.IsDraft, pullRequest.ReviewDecision)
}

func (pullRequest PullRequestSummary) BranchName() string {
	if pullRequest.HeadRefName == "" {
		return ""
	}
	if pullRequest.HeadRepositoryOwner != "" &&
		pullRequest.BaseRepositoryOwner != "" &&
		!strings.EqualFold(pullRequest.HeadRepositoryOwner, pullRequest.BaseRepositoryOwner) {
		return pullRequest.HeadRepositoryOwner + "/" + pullRequest.HeadRefName
	}
	return pullRequest.HeadRefName
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

func LoadPullRequestSummaries(ctx context.Context, repoRoot string, runner gitdata.Runner) ([]PullRequestSummary, error) {
	baseOwner, err := loadRepositoryOwner(ctx, repoRoot, runner)
	if err != nil {
		return nil, err
	}
	output, err := runner.Run(ctx, repoRoot, "gh", "pr", "list", "--limit", "200", "--state", "all", "--json", pullRequestSummaryFields)
	if err != nil {
		return nil, err
	}
	var rawPullRequests []rawPullRequest
	if err := json.Unmarshal(output, &rawPullRequests); err != nil {
		return nil, err
	}
	pullRequests := make([]PullRequestSummary, 0, len(rawPullRequests))
	for _, raw := range rawPullRequests {
		pullRequests = append(pullRequests, summaryFromRaw(raw, baseOwner))
	}
	sortPullRequestSummaries(pullRequests)
	return pullRequests, nil
}

func LoadPullRequestSummary(ctx context.Context, repoRoot, query string, runner gitdata.Runner) (PullRequestSummary, error) {
	baseOwner, err := loadRepositoryOwner(ctx, repoRoot, runner)
	if err != nil {
		return PullRequestSummary{}, err
	}
	output, err := runner.Run(ctx, repoRoot, "gh", "pr", "view", strings.TrimSpace(query), "--json", pullRequestSummaryFields)
	if err != nil {
		return PullRequestSummary{}, err
	}
	var raw rawPullRequest
	if err := json.Unmarshal(output, &raw); err != nil {
		return PullRequestSummary{}, err
	}
	if raw.Number == 0 {
		return PullRequestSummary{}, errPullRequestNotFound{}
	}
	return summaryFromRaw(raw, baseOwner), nil
}

func OpenPullRequest(ctx context.Context, repoRoot, query string, runner gitdata.Runner) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return errPullRequestNotFound{}
	}
	_, err := runner.Run(ctx, repoRoot, "gh", "pr", "view", query, "--web")
	return err
}

type errPullRequestNotFound struct{}

func (errPullRequestNotFound) Error() string {
	return "pull request not found"
}

func loadRepositoryOwner(ctx context.Context, repoRoot string, runner gitdata.Runner) (string, error) {
	output, err := runner.Run(ctx, repoRoot, "gh", "repo", "view", "--json", "owner")
	if err != nil {
		return "", err
	}
	var repository rawRepository
	if err := json.Unmarshal(output, &repository); err != nil {
		return "", err
	}
	return repository.Owner.Login, nil
}

func summaryFromRaw(raw rawPullRequest, baseOwner string) PullRequestSummary {
	return PullRequestSummary{
		Number:              raw.Number,
		Title:               raw.Title,
		State:               raw.State,
		IsDraft:             raw.IsDraft,
		ReviewDecision:      raw.ReviewDecision,
		CI:                  ciGlyph(raw.StatusCheckRollup),
		URL:                 raw.URL,
		HeadRefName:         raw.HeadRefName,
		HeadRepositoryOwner: raw.HeadRepositoryOwner.Login,
		BaseRepositoryOwner: baseOwner,
		UpdatedAt:           raw.UpdatedAt,
	}
}

func sortPullRequestSummaries(pullRequests []PullRequestSummary) {
	sort.SliceStable(pullRequests, func(leftIndex, rightIndex int) bool {
		return pullRequests[leftIndex].UpdatedAt.After(pullRequests[rightIndex].UpdatedAt)
	})
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
