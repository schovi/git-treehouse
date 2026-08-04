package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/github"
	"strings"
	"time"
)

type selectionAnchor struct {
	path   string
	branch string
	head   string
}

// detailHeightCache stores only the tallest rendered detail region. Its input
// fingerprint deliberately excludes selection, whose blocks render every frame.
type detailHeightCache struct {
	input         string
	maxBlockLines int
}

type worktreeFilter int

const (
	filterAll worktreeFilter = iota
	filterModified
	filterBranches
	filterMerged
	filterPrunable
	filterLocked
	filterDetached
)

var orderedFilters = []worktreeFilter{
	filterAll,
	filterModified,
	filterBranches,
	filterMerged,
	filterPrunable,
	filterLocked,
	filterDetached,
}

func (filter worktreeFilter) label() string {
	switch filter {
	case filterModified:
		return "modified"
	case filterBranches:
		return "branches"
	case filterMerged:
		return "merged"
	case filterPrunable:
		return "prunable"
	case filterLocked:
		return "locked"
	case filterDetached:
		return "detached"
	default:
		return "all"
	}
}

func (filter worktreeFilter) matches(row gitdata.Row) bool {
	switch filter {
	case filterModified:
		return row.IsWorktree() && !row.Worktree.Status.Clean()
	case filterBranches:
		return row.IsBranch()
	case filterMerged:
		return mergedFilterMatches(row)
	case filterPrunable:
		return row.IsWorktree() && row.Worktree.Prunable
	case filterLocked:
		return row.IsWorktree() && row.Worktree.Locked
	case filterDetached:
		return row.IsWorktree() && row.Worktree.Detached
	default:
		return true
	}
}

func mergedFilterMatches(row gitdata.Row) bool {
	if row.IsBranch() {
		return cleanupMergedDone(row)
	}
	if !row.IsWorktree() || row.Worktree.IsMain || row.Worktree.Detached {
		return false
	}
	return row.Worktree.Status.Clean() && cleanupMergedDone(row)
}

func prMergedOrClosed(row gitdata.Row) bool {
	pr := row.PullRequest()
	if pr == nil {
		return false
	}
	return strings.EqualFold(pr.State, "⎇") || strings.EqualFold(pr.State, "✕")
}

func (model Model) updateList(message tea.KeyMsg) (Model, tea.Cmd) {
	switch message.String() {
	case "q":
		model = model.cancelEnrichment()
		return model, tea.Quit
	case "ctrl+p":
		return model.openPalette()
	case "ctrl+o":
		return model, openConfigCmd(model.config.Editor, model.config)
	case "esc":
		if model.help {
			model.help = false
			return model, nil
		}
		if model.filter != filterAll {
			anchor := model.selectionAnchor()
			model.filter = filterAll
			if !model.restoreSelection(anchor) && len(model.visibleIndexes()) > 0 {
				model.selected = 0
			}
			return model, nil
		}
		if model.search.Value() != "" {
			model.search.SetValue("")
			model.selected = 0
			return model, nil
		}
		return model, nil
	case "up", "k":
		model.selected = clamp(model.selected-1, 0, max(0, len(model.visibleIndexes())-1))
	case "down", "j":
		model.selected = clamp(model.selected+1, 0, max(0, len(model.visibleIndexes())-1))
	case "g":
		model.selected = 0
	case "G":
		model.selected = max(0, len(model.visibleIndexes())-1)
	case "h":
		model.selectMatching(func(row gitdata.Row) bool { return row.IsWorktree() && row.Worktree.IsMain })
	case "a":
		model.selectMatching(func(row gitdata.Row) bool { return row.IsWorktree() && row.Worktree.IsActive })
	case "tab":
		return model.openFilterDialog()
	case "enter":
		row, ok := model.selectedTableRow()
		if !ok {
			return model, nil
		}
		if row.IsBranch() {
			return model.openBranchWorktree(row.Branch)
		}
		if row.Worktree.Prunable {
			return model.setFlash("cannot enter a prunable worktree")
		}
		if row.Worktree.IsActive {
			return model, tea.Quit
		}
		model.selectedPath = row.Worktree.Path
		return model, tea.Quit
	case "n":
		return model.openCreate()
	case "c":
		row, ok := model.selectedTableRow()
		if !ok || !row.IsBranch() {
			return model.setFlash("checkout root is only available for branch rows")
		}
		return model.openCheckoutRoot(row.Branch)
	case "delete", "backspace", "d":
		return model.openDelete()
	case "o":
		row, ok := model.selectedWorktree()
		if !ok || row.Prunable {
			return model.setFlash("cannot open this worktree")
		}
		return model, openEditorCmd(model.config.Editor, row.Path)
	case "p":
		if !model.githubEnabled() {
			return model.setFlash("GitHub is disabled")
		}
		row, ok := model.selectedTableRow()
		if !ok {
			return model, nil
		}
		return model, func() tea.Msg {
			err := github.OpenRowPullRequestOrBranch(context.Background(), model.state.Repo.Root, row, model.runner)
			return actionMsg{text: "opened", err: err}
		}
	case "y":
		text, flash, ok := model.selectedCopyText()
		if !ok {
			return model, nil
		}
		return model, copyTextCmd(text, flash)
	case "r":
		return model.startRefresh(true, false)
	case "u":
		if !model.hasPendingRestore() || model.deleteInFlight || model.cleanupMergedInFlight {
			return model, nil
		}
		return model.startRestore()
	case "s":
		model.searching = true
		model.search.Focus()
	case "b":
		anchor := model.selectionAnchor()
		model.showBranches = !model.showBranches
		model.config.ShowBranches = model.showBranches
		if !model.restoreSelection(anchor) && len(model.visibleIndexes()) > 0 {
			model.selected = 0
		}
		return model, persistShowBranchesCmd(model.showBranches)
	case "?":
		model.help = !model.help
	}
	reviewCommand := model.selectedReviewCommand(model.enrichmentID)
	graphCommand := model.selectedBranchGraphCommand(model.enrichmentID)
	return model, tea.Batch(reviewCommand, graphCommand)
}

func (model Model) startRefresh(fetch, automatic bool) (Model, tea.Cmd) {
	if model.refreshInFlight || model.cleanupMergedInFlight {
		return model, nil
	}
	model = model.cancelEnrichment()
	model.refreshID++
	model.refreshInFlight = true
	model.refreshAnchor = model.selectionAnchor()
	model.refreshProgressVisible = !automatic
	model.refreshSpinnerFrame = 0
	model = model.clearFeedback()
	commands := []tea.Cmd{reloadCmd(model.reloadCwd(), model.config, model.runner, model.state.Repo, fetch, automatic, model.refreshID)}
	if model.refreshProgressVisible {
		commands = append(commands, refreshSpinnerTickCmd(model.refreshID))
	}
	return model, tea.Batch(commands...)
}

func (model Model) updateAutoRefresh() (Model, tea.Cmd) {
	nextTick := autoRefreshTickCmd()
	if !model.canAutoRefresh() {
		return model, nextTick
	}
	model, refreshCmd := model.startRefresh(false, true)
	return model, tea.Batch(nextTick, refreshCmd)
}

func (model Model) canAutoRefresh() bool {
	return !model.refreshInFlight &&
		model.canApplyAutoRefresh()
}

func (model Model) canApplyAutoRefresh() bool {
	return model.loading == "" &&
		!model.deleteInFlight &&
		!model.cleanupMergedInFlight &&
		!model.searching &&
		!model.help &&
		model.createDialog == nil &&
		model.checkoutDialog == nil &&
		model.branchWorktreeDialog == nil &&
		model.deleteDialog == nil &&
		model.cleanupMergedDialog == nil &&
		model.paletteDialog == nil &&
		model.filterDialog == nil &&
		model.pullRequestDialog == nil &&
		!model.hasPendingRestore()
}

func (model Model) reloadCwd() string {
	if model.state.Repo.ActiveWorktree != "" {
		return model.state.Repo.ActiveWorktree
	}
	return model.state.Repo.Root
}

func (model Model) updateSearch(message tea.KeyMsg) (Model, tea.Cmd) {
	switch message.String() {
	case "esc":
		model.searching = false
		model.search.SetValue("")
		model.selected = 0
		return model, nil
	case "enter":
		model.searching = false
		return model, nil
	case "tab":
		return model.openFilterDialog()
	}
	var cmd tea.Cmd
	model.search, cmd = model.search.Update(message)
	model.selected = clamp(model.selected, 0, max(0, len(model.visibleIndexes())-1))
	return model, cmd
}

func (model Model) selectionAnchor() selectionAnchor {
	row, ok := model.selectedTableRow()
	if !ok {
		return selectionAnchor{}
	}
	if row.IsBranch() {
		return selectionAnchor{branch: row.Branch.Name, head: row.Branch.Head}
	}
	return selectionAnchor{path: row.Worktree.Path, branch: row.Worktree.Branch, head: row.Worktree.Head}
}

func (model *Model) restoreSelection(anchor selectionAnchor) bool {
	indexes := model.visibleIndexes()
	rows := model.tableRows()
	if len(indexes) == 0 {
		model.selected = 0
		return false
	}
	if anchor.path != "" {
		for visibleIndex, rowIndex := range indexes {
			row := rows[rowIndex]
			if row.IsWorktree() && row.Worktree.Path == anchor.path {
				model.selected = visibleIndex
				return true
			}
		}
	}
	if anchor.branch != "" || anchor.head != "" {
		for visibleIndex, rowIndex := range indexes {
			row := rows[rowIndex]
			if anchor.branch != "" && row.BranchName() == anchor.branch {
				model.selected = visibleIndex
				return true
			}
			if anchor.head != "" && row.Head() == anchor.head {
				model.selected = visibleIndex
				return true
			}
		}
	}
	model.selected = clamp(model.selected, 0, len(indexes)-1)
	return false
}

func (model *Model) selectMatching(match func(gitdata.Row) bool) {
	rows := model.tableRows()
	for visibleIndex, rowIndex := range model.visibleIndexes() {
		if match(rows[rowIndex]) {
			model.selected = visibleIndex
			return
		}
	}
}

func (model *Model) setFilter(filter worktreeFilter) {
	anchor := model.selectionAnchor()
	model.filter = filter
	if !model.restoreSelection(anchor) && len(model.visibleIndexes()) > 0 {
		model.selected = 0
	}
}

func (model *Model) applyCachedPullRequests() {
	if !model.githubEnabled() {
		model.showPR = false
		return
	}
	if len(model.prCache) == 0 {
		return
	}
	if model.prCacheRepoRoot != model.state.Repo.Root {
		model.prCache = nil
		model.prCacheRepoRoot = ""
		model.showPR = model.pullRequestsEnabled()
		return
	}
	model.showPR = true
	model.state.Rows = github.AttachPullRequests(model.state.Rows, model.prCache, model.state.Repo.MainBranch)
	model.state.Branches = github.AttachBranchPullRequests(model.state.Branches, model.prCache, model.state.Repo.MainBranch)
}

func (model Model) visibleTableWindow(now time.Time) (int, int) {
	indexes := model.visibleIndexes()
	availableHeight := model.availableTableHeight(now)
	start := 0
	if model.selected >= availableHeight {
		start = model.selected - availableHeight + 1
	}
	if start > len(indexes) {
		start = len(indexes)
	}
	return start, min(len(indexes), start+availableHeight)
}

func (model Model) visibleTableIndexes(now time.Time) []int {
	indexes := model.visibleIndexes()
	start, end := model.visibleTableWindow(now)
	if start >= end {
		return nil
	}
	return indexes[start:end]
}

func (model Model) availableTableHeight(now time.Time) int {
	width := viewWidth(model)
	outerWidth := max(4, width)
	contentWidth := max(1, outerWidth-4)
	panelWidth := max(4, contentWidth)
	panelContentWidth := max(1, panelWidth-2)
	tableRows := model.tableRows()
	rows := make([]gitdata.Row, 0, len(tableRows))
	for _, index := range model.visibleIndexes() {
		rows = append(rows, tableRows[index])
	}
	return model.availableTableHeightForBlockLines(model.reservedDetailBlockLines(rows, now, panelContentWidth+2))
}

// reviewForRow returns the loaded PR review for the row's open pull request, or
// nil when none has been fetched.
func (model Model) reviewForRow(row gitdata.Row) *github.PullRequestReview {
	if !model.showPR {
		return nil
	}
	pullRequest := row.PullRequest()
	if pullRequest == nil || pullRequest.Number == 0 || model.prReview == nil {
		return nil
	}
	if review, ok := model.prReview[pullRequest.Number]; ok {
		return &review
	}
	return nil
}

// reviewPendingNumberForRow returns the PR number whose review detail is still
// being fetched for the row, or 0 when nothing is pending. It powers the PR review
// frame's loading state. A finished attempt (success or failure) leaves a prReview
// map entry, so it is no longer pending; this also keeps a failed or gh-less
// lookup from showing "loading" forever.
func (model Model) reviewPendingNumberForRow(row gitdata.Row) int {
	if !model.showPR {
		return 0
	}
	pullRequest := row.PullRequest()
	if pullRequest == nil || pullRequest.Number == 0 {
		return 0
	}
	if model.prReview != nil {
		if _, attempted := model.prReview[pullRequest.Number]; attempted {
			return 0
		}
	}
	return pullRequest.Number
}

// blockLinesTotal is the rendered height of a detail region; each block already
// includes its own top/bottom borders.
func blockLinesTotal(blocks []string) int {
	total := 0
	for _, block := range blocks {
		if block != "" {
			total += lineCount(block)
		}
	}
	return total
}

// reservedDetailBlockLines is the tallest detail region any row in the list would
// render. Sizing the list against this instead of the selected row's own blocks
// keeps the visible rows fixed while navigating; shorter rows pad the gap.
func (model Model) reservedDetailBlockLines(rows []gitdata.Row, now time.Time, panelWidth int) int {
	input := model.detailHeightCacheInput(rows, panelWidth)
	if model.detailHeightCache != nil && model.detailHeightCache.input == input {
		return model.detailHeightCache.maxBlockLines
	}

	reserved := 0
	for _, row := range rows {
		reserved = max(reserved, blockLinesTotal(model.detailBlocks(row, now, panelWidth)))
	}
	if model.detailHeightCache != nil {
		model.detailHeightCache.input = input
		model.detailHeightCache.maxBlockLines = reserved
	}
	return reserved
}

func (model Model) detailHeightCacheInput(rows []gitdata.Row, panelWidth int) string {
	return fmt.Sprintf("%d:%t:%#v:%#v:%#v", panelWidth, model.showPR, model.state.Repo, rows, model.prReview)
}

func (model Model) availableTableHeightForBlockLines(blockLines int) int {
	fixedLines := 1 + 2 + 1 + blockLines + 1
	if model.flash != "" {
		fixedLines++
	}
	if model.height <= 0 {
		return 8
	}
	return max(1, model.height-fixedLines)
}

func (model Model) visibleIndexes() []int {
	return model.visibleIndexesForFilter(model.filter)
}

func (model Model) visibleIndexesForFilter(filter worktreeFilter) []int {
	pattern := model.search.Value()
	rows := model.tableRowsForFilter(filter)
	indexes := make([]int, 0, len(rows))
	for index, row := range rows {
		branchMatches := pattern == "" || fuzzyMatch(row.DisplayBranch(), pattern)
		if branchMatches && filter.matches(row) {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func (model Model) tableRows() []gitdata.Row {
	return model.tableRowsForFilter(model.filter)
}

func (model Model) tableRowsForFilter(filter worktreeFilter) []gitdata.Row {
	return model.state.TableRows(model.showBranches || filter == filterBranches || filter == filterMerged)
}

func (model Model) selectedTableRow() (gitdata.Row, bool) {
	indexes := model.visibleIndexes()
	if len(indexes) == 0 || model.selected < 0 || model.selected >= len(indexes) {
		return gitdata.Row{}, false
	}
	rows := model.tableRows()
	return rows[indexes[model.selected]], true
}

func (model Model) selectedWorktree() (gitdata.Worktree, bool) {
	row, ok := model.selectedTableRow()
	if !ok || !row.IsWorktree() {
		return gitdata.Worktree{}, false
	}
	return row.Worktree, true
}

func (model Model) rootWorktree() (gitdata.Worktree, bool) {
	for _, row := range model.state.Rows {
		if row.IsMain || row.Path == model.state.Repo.Root || row.Path == model.state.Repo.MainWorktree {
			return row, true
		}
	}
	return gitdata.Worktree{}, false
}

func (model Model) selectedCopyText() (string, string, bool) {
	row, ok := model.selectedTableRow()
	if !ok {
		return "", "", false
	}
	if row.IsBranch() {
		name := row.Branch.Name
		if name == "" {
			return "", "", false
		}
		return name, "copied branch name: " + name, true
	}
	if row.Worktree.Path == "" {
		return "", "", false
	}
	return row.Worktree.Path, "copied absolute path: " + row.Worktree.Path, true
}

func (model Model) selectedPullRequestCopy() (string, string, bool) {
	row, ok := model.selectedTableRow()
	if !ok {
		return "", "", false
	}
	pr := row.PullRequest()
	if pr == nil || pr.URL == "" {
		return "", "", false
	}
	return pr.URL, "copied PR URL: " + pr.URL, true
}

func (model Model) selectedRow() (gitdata.Worktree, bool) {
	return model.selectedWorktree()
}

func fuzzyMatch(text, pattern string) bool {
	textRunes := []rune(strings.ToLower(text))
	pattern = strings.ToLower(pattern)
	if pattern == "" {
		return true
	}
	textIndex := 0
	for _, character := range pattern {
		found := false
		for textIndex < len(textRunes) {
			if textRunes[textIndex] == character {
				found = true
				textIndex++
				break
			}
			textIndex++
		}
		if !found {
			return false
		}
	}
	return true
}

func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
