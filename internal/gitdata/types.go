package gitdata

import (
	"fmt"
	"strings"
	"time"
)

type Repository struct {
	Root             string
	CommonGitDir     string
	Cwd              string
	ActiveWorktree   string
	MainWorktree     string
	MainBranch       string
	Parent           string
	RemoteConfigured bool
}

type State struct {
	Repo Repository
	Rows []Worktree
}

type StatusCounts struct {
	Staged    int
	Modified  int
	Untracked int
}

func (counts StatusCounts) Clean() bool {
	return counts.Staged == 0 && counts.Modified == 0 && counts.Untracked == 0
}

func (counts StatusCounts) Text() string {
	if counts.Clean() {
		return "clean"
	}
	parts := make([]string, 0, 3)
	if counts.Staged > 0 {
		parts = append(parts, fmt.Sprintf("staged %d", counts.Staged))
	}
	if counts.Modified > 0 {
		parts = append(parts, fmt.Sprintf("modified %d", counts.Modified))
	}
	if counts.Untracked > 0 {
		parts = append(parts, fmt.Sprintf("untracked %d", counts.Untracked))
	}
	return strings.Join(parts, ", ")
}

type SyncState struct {
	Available  bool
	NoUpstream bool
	Ahead      int
	Behind     int
}

func (sync SyncState) Compact() string {
	if sync.NoUpstream {
		return "-"
	}
	if !sync.Available || sync.Ahead == 0 && sync.Behind == 0 {
		return ""
	}
	parts := make([]string, 0, 2)
	if sync.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("↑%d", sync.Ahead))
	}
	if sync.Behind > 0 {
		parts = append(parts, fmt.Sprintf("↓%d", sync.Behind))
	}
	return strings.Join(parts, " ")
}

type PullRequest struct {
	Number int
	State  string
	CI     string
	URL    string
}

func (pr PullRequest) Text() string {
	if pr.Number == 0 {
		return ""
	}
	state := pr.State
	if state == "" {
		state = "○"
	}
	ci := pr.CI
	if ci != "" {
		return fmt.Sprintf("#%d %s %s", pr.Number, state, ci)
	}
	return fmt.Sprintf("#%d %s", pr.Number, state)
}

type Worktree struct {
	Path               string
	GitDir             string
	Head               string
	Branch             string
	Detached           bool
	Bare               bool
	Locked             bool
	LockReason         string
	Prunable           bool
	PruneReason        string
	IsActive           bool
	IsMain             bool
	Status             StatusCounts
	Upstream           string
	UpstreamGone       bool
	HeadSync           SyncState
	MainSync           SyncState
	CommitShort        string
	CommitSubject      string
	CommitTime         time.Time
	BranchMergedToMain bool
	PR                 *PullRequest
	SizeBytes          int64
	SizeLoaded         bool
}

func (worktree Worktree) DisplayBranch() string {
	if worktree.Detached {
		if worktree.Head == "" {
			return "(detached)"
		}
		return "(detached) " + shortHash(worktree.Head)
	}
	if worktree.Branch == "" {
		return "(unknown)"
	}
	return worktree.Branch
}

func (worktree Worktree) Marker() string {
	if worktree.Prunable {
		return "✗"
	}
	if worktree.Locked {
		return "🔒"
	}
	if worktree.IsMain && worktree.IsActive {
		return "◉"
	}
	if worktree.IsMain {
		return "⌂"
	}
	if worktree.IsActive {
		return "●"
	}
	return "○"
}

func (worktree Worktree) StatusText() string {
	if worktree.Prunable {
		return "prunable"
	}
	if worktree.Locked {
		return "locked"
	}
	if worktree.Detached {
		return "detached"
	}
	if worktree.UpstreamGone {
		return "⚠ gone"
	}
	if worktree.Status.Clean() {
		return "✓"
	}
	parts := make([]string, 0, 3)
	if worktree.Status.Staged > 0 {
		parts = append(parts, fmt.Sprintf("+%d", worktree.Status.Staged))
	}
	if worktree.Status.Modified > 0 {
		parts = append(parts, fmt.Sprintf("~%d", worktree.Status.Modified))
	}
	if worktree.Status.Untracked > 0 {
		parts = append(parts, fmt.Sprintf("?%d", worktree.Status.Untracked))
	}
	return strings.Join(parts, " ")
}

func (worktree Worktree) Detail(now time.Time) string {
	parts := []string{worktree.Path, worktree.Status.Text()}
	if worktree.Upstream != "" {
		state := worktree.HeadSync.Compact()
		if state == "" {
			state = "synced"
		}
		if worktree.UpstreamGone {
			state = "gone"
		}
		parts = append(parts, "upstream "+worktree.Upstream+" "+state)
	} else {
		parts = append(parts, "no upstream")
	}
	if worktree.CommitShort != "" {
		parts = append(parts, worktree.CommitShort+" "+worktree.CommitSubject+" "+RelativeAge(now, worktree.CommitTime))
	}
	return strings.Join(parts, " · ")
}

func RelativeAge(now time.Time, then time.Time) string {
	if then.IsZero() {
		return ""
	}
	duration := now.Sub(then)
	if duration < 0 {
		duration = 0
	}
	if duration < time.Hour {
		minutes := int(duration.Minutes())
		if minutes < 1 {
			minutes = 1
		}
		return fmt.Sprintf("%dm", minutes)
	}
	if duration < 48*time.Hour {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	}
	if duration < 14*24*time.Hour {
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	}
	if duration < 10*7*24*time.Hour {
		return fmt.Sprintf("%dw", int(duration.Hours()/(24*7)))
	}
	return fmt.Sprintf("%dmo", int(duration.Hours()/(24*30)))
}

func shortHash(hash string) string {
	if len(hash) <= 7 {
		return hash
	}
	return hash[:7]
}
