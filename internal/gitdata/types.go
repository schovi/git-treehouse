package gitdata

import (
	"fmt"
	"sort"
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
	Repo     Repository
	Rows     []Worktree
	Branches []Branch
}

type RowKind int

const (
	RowKindWorktree RowKind = iota
	RowKindBranch
)

type Row struct {
	Kind     RowKind
	Worktree Worktree
	Branch   Branch
}

type Branch struct {
	Name               string
	Head               string
	Upstream           string
	UpstreamGone       bool
	HeadSync           SyncState
	MainSync           SyncState
	CommitShort        string
	CommitSubject      string
	CommitTime         time.Time
	BranchMergedToMain bool
	PR                 *PullRequest
}

func RowsFromWorktrees(worktrees []Worktree) []Row {
	rows := make([]Row, 0, len(worktrees))
	for _, worktree := range worktrees {
		rows = append(rows, Row{Kind: RowKindWorktree, Worktree: worktree})
	}
	return rows
}

func (state State) TableRows(showBranches bool) []Row {
	rows := RowsFromWorktrees(state.Rows)
	if showBranches {
		for _, branch := range state.Branches {
			rows = append(rows, Row{Kind: RowKindBranch, Branch: branch})
		}
	}
	sortRows(rows)
	return rows
}

func (row Row) IsWorktree() bool {
	return row.Kind == RowKindWorktree
}

func (row Row) IsBranch() bool {
	return row.Kind == RowKindBranch
}

func (row Row) DisplayBranch() string {
	if row.IsBranch() {
		return row.Branch.DisplayBranch()
	}
	return row.Worktree.DisplayBranch()
}

func (row Row) ListBranch() string {
	if row.IsBranch() {
		return row.Branch.DisplayBranch()
	}
	return row.Worktree.ListBranch()
}

func (row Row) BranchName() string {
	if row.IsBranch() {
		return row.Branch.Name
	}
	return row.Worktree.Branch
}

func (row Row) Head() string {
	if row.IsBranch() {
		return row.Branch.Head
	}
	return row.Worktree.Head
}

func (row Row) CommitShort() string {
	if row.IsBranch() {
		return row.Branch.CommitShort
	}
	return row.Worktree.CommitShort
}

func (row Row) CommitSubject() string {
	if row.IsBranch() {
		return row.Branch.CommitSubject
	}
	return row.Worktree.CommitSubject
}

func (row Row) CommitTime() time.Time {
	if row.IsBranch() {
		return row.Branch.CommitTime
	}
	return row.Worktree.CommitTime
}

func (row Row) Upstream() string {
	if row.IsBranch() {
		return row.Branch.Upstream
	}
	return row.Worktree.Upstream
}

func (row Row) UpstreamGone() bool {
	if row.IsBranch() {
		return row.Branch.UpstreamGone
	}
	return row.Worktree.UpstreamGone
}

func (row Row) HeadSync() SyncState {
	if row.IsBranch() {
		return row.Branch.HeadSync
	}
	return row.Worktree.HeadSync
}

func (row Row) MainSync() SyncState {
	if row.IsBranch() {
		return row.Branch.MainSync
	}
	return row.Worktree.MainSync
}

func (row Row) PullRequest() *PullRequest {
	if row.IsBranch() {
		return row.Branch.PR
	}
	return row.Worktree.PR
}

func (row Row) TypeIcon() string {
	if row.IsBranch() {
		return "⑂"
	}
	if row.Worktree.IsMain {
		return "⌂"
	}
	return "▣"
}

func (row Row) StateIcon() string {
	if row.IsBranch() {
		return ""
	}
	switch {
	case row.Worktree.Prunable:
		return "×"
	case row.Worktree.Locked:
		return "!"
	default:
		return ""
	}
}

func (row Row) StatusText() string {
	if row.IsBranch() {
		return "-"
	}
	return row.Worktree.StatusText()
}

func (row Row) LocalMetadataLoaded() bool {
	return row.IsBranch() || row.Worktree.LocalMetadataLoaded
}

func (row Row) TableSize() (int64, bool) {
	if row.IsBranch() {
		return 0, false
	}
	return row.Worktree.TableSize()
}

func (branch Branch) DisplayBranch() string {
	if branch.Name == "" {
		return "(unknown)"
	}
	return branch.Name
}

func sortRows(rows []Row) {
	sort.SliceStable(rows, func(leftIndex, rightIndex int) bool {
		left := rows[leftIndex]
		right := rows[rightIndex]
		if left.IsWorktree() && right.IsWorktree() && left.Worktree.IsMain != right.Worktree.IsMain {
			return left.Worktree.IsMain
		}
		if left.IsWorktree() && left.Worktree.IsMain {
			return true
		}
		if right.IsWorktree() && right.Worktree.IsMain {
			return false
		}
		return left.CommitTime().After(right.CommitTime())
	})
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

func (sync SyncState) RemoteCompact(upstreamGone bool) string {
	if upstreamGone {
		return "gone"
	}
	if sync.NoUpstream {
		return "-"
	}
	if !sync.Available {
		return ""
	}
	if sync.Ahead == 0 && sync.Behind == 0 {
		return "✓"
	}
	return sync.Compact()
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
	Path                string
	GitDir              string
	Head                string
	Branch              string
	Detached            bool
	Bare                bool
	Locked              bool
	LockReason          string
	Prunable            bool
	PruneReason         string
	IsActive            bool
	IsMain              bool
	LocalMetadataLoaded bool
	Status              StatusCounts
	Upstream            string
	UpstreamGone        bool
	HeadSync            SyncState
	MainSync            SyncState
	CommitShort         string
	CommitSubject       string
	CommitTime          time.Time
	BranchMergedToMain  bool
	PR                  *PullRequest
	GitSizeBytes        int64
	GitSizeLoaded       bool
	FullSizeBytes       int64
	FullSizeLoaded      bool
	SizeBytes           int64
	SizeLoaded          bool
}

func (worktree Worktree) TableSize() (int64, bool) {
	if worktree.GitSizeLoaded {
		return worktree.GitSizeBytes, true
	}
	if worktree.SizeLoaded {
		return worktree.SizeBytes, true
	}
	return 0, false
}

func (worktree Worktree) FullSize() (int64, bool) {
	if worktree.FullSizeLoaded {
		return worktree.FullSizeBytes, true
	}
	if worktree.SizeLoaded {
		return worktree.SizeBytes, true
	}
	return 0, false
}

func (worktree Worktree) DisplayBranch() string {
	if worktree.Detached {
		if worktree.Head == "" {
			return "detached"
		}
		return shortHash(worktree.Head) + " detached"
	}
	if worktree.Branch == "" {
		return "(unknown)"
	}
	branch := worktree.Branch
	if worktree.Locked {
		branch += " locked"
	}
	if worktree.Prunable {
		branch += " prunable"
	}
	return branch
}

func (worktree Worktree) Marker() string {
	if worktree.Prunable {
		return "×"
	}
	if worktree.Locked {
		return "!"
	}
	if worktree.IsMain {
		return "⌂"
	}
	return ""
}

func (worktree Worktree) ListBranch() string {
	if worktree.Detached {
		if worktree.Head == "" {
			return "detached"
		}
		return shortHash(worktree.Head) + " detached"
	}
	if worktree.Branch == "" {
		return "(unknown)"
	}
	return worktree.Branch
}

func (worktree Worktree) StatusText() string {
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
