package tui

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/listview"
	"strings"
	"time"
)

type viewSnapshot struct {
	rows        []gitdata.Row
	visibleRows []gitdata.Row
	selectedRow gitdata.Row
	hasSelected bool
	// blocks are the finished, bordered boxes that stack below the Worktrees panel
	// (the Details box, possibly paired with Git context, then the secondary frames).
	blocks []string
	// reservedBlockLines is the height the detail region is sized for: the tallest
	// blocks any row would render. Shorter rows pad up to it so moving the selection
	// never resizes the list above.
	reservedBlockLines int
	start              int
	scrollbar          listScrollbar
}

type listScrollbar struct {
	total   int
	visible int
	start   int
}

func (model Model) View() string {
	now := time.Now()
	width := viewWidth(model)
	outerWidth := max(4, width)
	contentWidth := max(1, outerWidth-4)
	panelWidth := max(4, contentWidth)
	panelContentWidth := max(1, panelWidth-2)
	rowCount := model.totalRowCount()
	var blocks []string
	reservedBlockLines := 0
	lines := []string{"Loading worktrees…"}
	worktreeScrollbar := listScrollbar{}
	if model.localMetadataReady() {
		snapshot := model.viewSnapshot(now, panelContentWidth)
		rowCount = len(snapshot.rows)
		blocks = snapshot.blocks
		reservedBlockLines = snapshot.reservedBlockLines
		worktreeScrollbar = snapshot.scrollbar
		tableWidth := tableContentWidth(panelContentWidth, snapshot.scrollbar)
		table := listview.RenderMixedRows(snapshot.visibleRows, listview.Options{
			Width:             tableWidth,
			Color:             true,
			Hyperlinks:        true,
			ShowHeader:        true,
			ShowPR:            model.showPR,
			Pending:           listview.LoadingPlaceholder,
			PRPending:         model.pullRequestsPending(),
			HighlightSelected: true,
			SelectedIndex:     model.selected - snapshot.start,
		}, now)
		lines = strings.Split(table, "\n")
		if len(snapshot.rows) == 0 {
			lines = strings.Split(model.noRowsMessage(), "\n")
			for index := range lines {
				lines[index] = truncatePlain(lines[index], panelContentWidth)
			}
		}
		lines = renderLinesWithListScrollbar(lines, panelContentWidth, snapshot.scrollbar)
	}
	leftFooter, rightFooter := model.listFooterHintsForScrollbar(worktreeScrollbar, panelContentWidth)
	parts := []string{
		model.appTopLine(rowCount, outerWidth),
		model.wrapOuter(sectionBoxWithSplitFooterTopRight("Worktrees", lines, leftFooter, rightFooter, model.worktreesFeedback(), panelWidth), outerWidth),
	}
	for _, block := range blocks {
		if block != "" {
			parts = append(parts, model.wrapOuter(block, outerWidth))
		}
	}
	// The list is sized for the tallest row's detail region, so pad a shorter one up
	// to that height; otherwise the bottom line would jump as the selection moves.
	for range max(0, reservedBlockLines-blockLinesTotal(blocks)) {
		parts = append(parts, model.wrapOuter("", outerWidth))
	}
	if model.flash != "" {
		parts = append(parts, model.wrapOuter(model.flashLineAtWidth(panelWidth), outerWidth))
	}
	parts = append(parts, model.appBottomLine(outerWidth))
	output := strings.Join(parts, "\n")
	overlayHeight := lineCount(output)
	if model.help {
		output = centeredOverlay(output, model.renderHelpAtWidth(helpDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.paletteDialog != nil {
		output = centeredOverlay(output, model.renderPaletteAtWidth(paletteDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.filterDialog != nil {
		output = centeredOverlay(output, model.renderFilterAtWidth(filterDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.pullRequestDialog != nil {
		output = centeredOverlay(output, model.renderPullRequestCheckoutAtWidth(pullRequestDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.deleteDialog != nil {
		output = centeredOverlay(output, model.renderDeleteAtWidth(deleteDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.cleanupMergedDialog != nil {
		output = centeredOverlay(output, model.renderCleanupMergedAtWidth(deleteDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.branchWorktreeDialog != nil {
		output = centeredOverlay(output, model.renderBranchWorktreeAtWidth(checkoutDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.checkoutDialog != nil {
		output = centeredOverlay(output, model.renderCheckoutAtWidth(checkoutDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.createDialog != nil {
		output = centeredOverlay(output, model.renderCreateAtWidth(createDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	return model.frame(output)
}

func (model Model) localMetadataReady() bool {
	for _, row := range model.state.Rows {
		if !row.LocalMetadataLoaded {
			return false
		}
	}
	return true
}

func (model Model) totalRowCount() int {
	return len(model.tableRows())
}

func (model Model) viewSnapshot(now time.Time, panelContentWidth int) viewSnapshot {
	indexes := model.visibleIndexes()
	tableRows := model.tableRows()
	rows := make([]gitdata.Row, 0, len(indexes))
	for _, index := range indexes {
		rows = append(rows, tableRows[index])
	}
	snapshot := viewSnapshot{rows: rows}
	if len(indexes) > 0 && model.selected >= 0 && model.selected < len(indexes) {
		snapshot.selectedRow = tableRows[indexes[model.selected]]
		snapshot.hasSelected = true
		snapshot.blocks = model.detailBlocks(snapshot.selectedRow, now, panelContentWidth+2)
	}
	snapshot.reservedBlockLines = model.reservedDetailBlockLines(rows, now, panelContentWidth+2)
	availableHeight := model.availableTableHeightForBlockLines(snapshot.reservedBlockLines)
	if model.selected >= availableHeight {
		snapshot.start = model.selected - availableHeight + 1
	}
	if snapshot.start > len(rows) {
		snapshot.start = len(rows)
	}
	end := min(len(rows), snapshot.start+availableHeight)
	if snapshot.start < end {
		snapshot.visibleRows = rows[snapshot.start:end]
	}
	snapshot.scrollbar = listScrollbar{
		total:   len(rows),
		visible: len(snapshot.visibleRows),
		start:   snapshot.start,
	}
	return snapshot
}

// detailSideBySideMinWidth is the narrowest panel at which the Details box and the
// Git context frame render side by side. Below it they stack. The threshold keeps
// each half at or above the frames' 50-column minimum plus the gap between them.
const detailSideBySideMinWidth = 104

// detailSideBySideGap is the blank gutter between the side-by-side boxes.
const detailSideBySideGap = 1

// detailBlocks renders the Details box together with the context frames as finished,
// bordered blocks ready to stack below the Worktrees panel. On a wide panel the Git
// context frame sits beside the Details box; otherwise it stacks directly beneath it.
// The remaining frames (PR review and Changes) always stack full width below.
func (model Model) detailBlocks(row gitdata.Row, now time.Time, panelWidth int) []string {
	var blocks []string
	if left, right, ok := model.detailWithGitContext(row, now, panelWidth); ok {
		gap := strings.Repeat(" ", detailSideBySideGap)
		blocks = append(blocks, lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right))
	} else {
		body := model.rowDetailPanelAtWidth(row, now, panelWidth-2)
		blocks = append(blocks, sectionBoxWithFooter(rowDetailTitle(row), strings.Split(body, "\n"), detailFooterHints(row, panelWidth), panelWidth))
		if box := changesFrame(row, panelWidth); box != "" {
			blocks = append(blocks, box)
		}
		if box := prReviewFrame(model.reviewForRow(row), model.reviewPendingNumberForRow(row), panelWidth); box != "" {
			blocks = append(blocks, box)
		}
		if box := gitContextFrame(row, model.state.Repo.MainBranch, panelWidth, 0); box != "" {
			blocks = append(blocks, box)
		}
	}
	return blocks
}

// detailWithGitContext renders the Details box and the Git context frame at half
// width each so they can be joined horizontally. It reports ok=false when the panel
// is too narrow or the row has no Git context to pair with, leaving the caller to
// stack them instead.
func (model Model) detailWithGitContext(row gitdata.Row, now time.Time, panelWidth int) (left, right string, ok bool) {
	if panelWidth < detailSideBySideMinWidth {
		return "", "", false
	}
	leftWidth := (panelWidth - detailSideBySideGap) / 2
	rightWidth := panelWidth - detailSideBySideGap - leftWidth
	// The left column stacks the Details, Changes, and PR review boxes; the Git
	// context frame on the right then grows to match that combined height, keeping
	// the outer bottom borders aligned.
	body := model.rowDetailPanelAtWidth(row, now, leftWidth-2)
	left = sectionBoxWithFooter(rowDetailTitle(row), strings.Split(body, "\n"), detailFooterHints(row, leftWidth), leftWidth)
	if changes := changesFrame(row, leftWidth); changes != "" {
		left = lipgloss.JoinVertical(lipgloss.Left, left, changes)
	}
	if pr := prReviewFrame(model.reviewForRow(row), model.reviewPendingNumberForRow(row), leftWidth); pr != "" {
		left = lipgloss.JoinVertical(lipgloss.Left, left, pr)
	}
	right = gitContextFrame(row, model.state.Repo.MainBranch, rightWidth, lineCount(left))
	if right == "" {
		return "", "", false
	}
	return left, right, true
}

func tableContentWidth(width int, scrollbar listScrollbar) int {
	if scrollbar.shouldRender(width) {
		return max(1, width-scrollbarGutterWidth)
	}
	return width
}

func renderLinesWithListScrollbar(lines []string, width int, scrollbar listScrollbar) []string {
	if !scrollbar.shouldRender(width) {
		return lines
	}
	contentWidth := tableContentWidth(width, scrollbar)
	output := make([]string, len(lines))
	for index, line := range lines {
		output[index] = padStyled(truncateStyled(line, contentWidth), contentWidth) + " " + scrollbar.glyphAt(index, len(lines))
	}
	return output
}

func (scrollbar listScrollbar) shouldRender(width int) bool {
	return width > scrollbarGutterWidth && scrollbar.total > scrollbar.visible && scrollbar.visible > 0
}

func (scrollbar listScrollbar) positionText() string {
	if scrollbar.total <= 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", scrollbar.start, scrollbar.total)
}

func (scrollbar listScrollbar) glyphAt(index, height int) string {
	if index == 0 {
		return scrollbar.arrow("↑", scrollbar.start > 0)
	}
	if index == height-1 {
		return scrollbar.arrow("↓", scrollbar.start+scrollbar.visible < scrollbar.total)
	}
	if scrollbar.trackHasThumbAt(index-1, max(0, height-2)) {
		return scrollbarThumbStyle.Render("█")
	}
	return scrollbarTrackStyle.Render("│")
}

func (scrollbar listScrollbar) arrow(value string, enabled bool) string {
	if !enabled {
		return scrollbarTrackStyle.Render(value)
	}
	return scrollbarArrowStyle.Render(value)
}

func (scrollbar listScrollbar) trackHasThumbAt(index, trackHeight int) bool {
	if trackHeight <= 0 {
		return false
	}
	thumbHeight := max(1, trackHeight*scrollbar.visible/scrollbar.total)
	thumbHeight = min(trackHeight, thumbHeight)
	availablePositions := trackHeight - thumbHeight
	maxStart := max(1, scrollbar.total-scrollbar.visible)
	thumbStart := 0
	if availablePositions > 0 {
		thumbStart = (scrollbar.start*availablePositions + maxStart/2) / maxStart
	}
	return index >= thumbStart && index < thumbStart+thumbHeight
}

func (model Model) selectedInspector(row gitdata.Worktree, now time.Time) string {
	return model.selectedRowInspector(gitdata.Row{Kind: gitdata.RowKindWorktree, Worktree: row}, now)
}

func (model Model) selectedRowInspector(row gitdata.Row, now time.Time) string {
	return model.selectedRowInspectorAtWidth(row, now, viewWidth(model))
}

func (model Model) selectedRowInspectorAtWidth(row gitdata.Row, now time.Time, width int) string {
	if row.IsBranch() {
		return model.selectedBranchInspectorAtWidth(row.Branch, width)
	}
	worktree := row.Worktree
	lines := []string{
		model.inspectorRenderedFieldAtWidth("Branch", branchText(worktree), func(value string) string {
			return branchStyle(worktree).Render(value)
		}, width),
		model.inspectorRenderedFieldAtWidth("HEAD", headText(worktree), renderHeadValue, width),
		model.inspectorFieldAtWidth("Path", model.relativePath(worktree.Path), inspectorValueStyle, width),
		model.inspectorFieldAtWidth("Status", statusText(worktree), statusStyle(worktree), width),
		model.inspectorRenderedFieldAtWidth("Dirty", dirtyDetailText(worktree.Status), renderDirtyDetailValue, width),
		model.inspectorFieldAtWidth("Size", sizeText(worktree), inspectorValueStyle, width),
	}
	lines = append(lines,
		model.inspectorFieldAtWidth("Delete", deleteSafetyText(worktree), deleteSafetyStyle(worktree), width),
	)
	if hint := model.safeToRemoveDetailHint(row); hint != "" {
		lines = append(lines, inspectorCleanStyle.Render(truncatePlain(hint, width)))
	}
	return strings.Join(lines, "\n")
}

func (model Model) safeToRemoveDetailHint(row gitdata.Row) string {
	if !row.IsWorktree() || !mergedFilterMatches(row) || model.cleanupMergedWorktreeSkipReason(row.Worktree) != "" {
		return ""
	}
	switch {
	case row.Worktree.UpstreamGone:
		return "finished: clean, merged; remote branch deleted — safe to remove (d)"
	case prMergedOrClosed(row):
		return "finished: clean, PR merged/closed — safe to remove (d)"
	default:
		return "finished: clean, merged to main — safe to remove (d)"
	}
}

func (model Model) selectedBranchInspectorAtWidth(branch gitdata.Branch, width int) string {
	lines := []string{
		model.inspectorFieldAtWidth("Branch", branch.DisplayBranch(), branchOnlyDetailStyle, width),
		model.inspectorRenderedFieldAtWidth("HEAD", branchHeadText(branch), renderHeadValue, width),
		model.inspectorFieldAtWidth("Path", "not checked out", inspectorValueStyle, width),
		model.inspectorFieldAtWidth("Status", "no worktree", inspectorValueStyle, width),
		model.inspectorFieldAtWidth("Dirty", "-", inspectorValueStyle, width),
		model.inspectorFieldAtWidth("Size", "-", inspectorValueStyle, width),
		model.inspectorFieldAtWidth("Action", "create worktree; checkout root from palette", inspectorCleanStyle, width),
	}
	return strings.Join(lines, "\n")
}

func (model Model) inspectorFieldAtWidth(label, value string, style lipgloss.Style, width int) string {
	return model.inspectorRenderedFieldAtWidth(label, value, func(value string) string {
		return style.Render(value)
	}, width)
}

func (model Model) inspectorRenderedFieldAtWidth(label, value string, render func(string) string, width int) string {
	labelWidth := 8
	separatorWidth := 2
	if width <= 0 {
		width = 80
	}
	if width <= labelWidth+separatorWidth {
		return truncatePlain(label+": "+value, width)
	}
	valueWidth := width - labelWidth - separatorWidth
	labelText := padRight(label, labelWidth)
	return inspectorLabelStyle.Render(labelText) + "  " + render(truncatePlain(value, valueWidth))
}

func (model Model) detailPanel(row gitdata.Worktree, now time.Time) string {
	return model.rowDetailPanel(gitdata.Row{Kind: gitdata.RowKindWorktree, Worktree: row}, now)
}

func (model Model) rowDetailPanel(row gitdata.Row, now time.Time) string {
	return model.rowDetailPanelAtWidth(row, now, viewWidth(model))
}

func (model Model) rowDetailPanelAtWidth(row gitdata.Row, now time.Time, width int) string {
	return model.selectedRowInspectorAtWidth(row, now, width)
}

func detailTitle(row gitdata.Worktree) string {
	return rowDetailTitle(gitdata.Row{Kind: gitdata.RowKindWorktree, Worktree: row})
}

func rowDetailTitle(row gitdata.Row) string {
	context := selectionContextTitle(row)
	if context == "" {
		return "Details"
	}
	return "Details · " + context
}

func renderSectionTitle(title string, width int) string {
	name, detail, found := strings.Cut(title, " · ")
	if !found {
		return panelTitleStyle.Render(truncatePlain(title, width))
	}
	separator := titleSeparatorStyle.Render(" · ")
	nameWidth := runewidth.StringWidth(name)
	detailWidth := max(0, width-nameWidth-lipgloss.Width(separator))
	if detailWidth <= 0 {
		return panelTitleStyle.Render(truncatePlain(name, width))
	}
	return panelTitleStyle.Render(name) + separator + titleRepoStyle.Render(truncatePlain(detail, detailWidth))
}

func detailFooterHints(row gitdata.Row, width int) string {
	actionParts := []string{"↵ go", "o editor", "d delete", "y abs path", "p PR"}
	if row.IsBranch() {
		actionParts = []string{"↵ create+go", "d delete", "y name", "p PR"}
	}
	availableWidth := max(0, width-5)
	return joinPartsWithin(actionParts, availableWidth)
}

func renderDirtyDetailValue(value string) string {
	if value == "none" {
		return inspectorCleanStyle.Render(value)
	}
	parts := strings.Split(value, "  ")
	for index, part := range parts {
		key, rest, found := strings.Cut(part, " ")
		if !found {
			parts[index] = inspectorWarnStyle.Render(part)
			continue
		}
		switch key {
		case "+":
			parts[index] = inspectorCleanStyle.Render(key) + hintStyle.Render(" "+rest)
		case "~":
			parts[index] = inspectorWarnStyle.Render(key) + hintStyle.Render(" "+rest)
		case "?":
			parts[index] = inspectorCommitStyle.Render(key) + hintStyle.Render(" "+rest)
		default:
			parts[index] = inspectorWarnStyle.Render(part)
		}
	}
	return strings.Join(parts, hintStyle.Render("  "))
}

func renderHeadValue(value string) string {
	head, rest, found := strings.Cut(value, " ")
	if !found {
		return inspectorCommitStyle.Render(value)
	}
	return inspectorCommitStyle.Render(head) + inspectorSubjectStyle.Render(" "+rest)
}

func statusStyle(row gitdata.Worktree) lipgloss.Style {
	if row.Status.Clean() {
		return inspectorCleanStyle
	}
	return inspectorWarnStyle
}

func branchStyle(row gitdata.Worktree) lipgloss.Style {
	if row.Detached {
		return inspectorCommitStyle
	}
	return inspectorValueStyle
}

func branchText(row gitdata.Worktree) string {
	return row.DisplayBranch()
}

func headText(row gitdata.Worktree) string {
	if row.Head == "" {
		if row.Detached {
			return "detached"
		}
		if row.Branch != "" {
			return "on " + row.Branch
		}
		return "-"
	}
	if row.Detached {
		return shortRef(row.Head) + " detached"
	}
	if row.Branch != "" {
		return shortRef(row.Head) + " on " + row.Branch
	}
	return shortRef(row.Head)
}

func branchHeadText(branch gitdata.Branch) string {
	if branch.Head == "" {
		if branch.Name != "" {
			return "on " + branch.Name
		}
		return "-"
	}
	if branch.Name != "" {
		return shortRef(branch.Head) + " on " + branch.Name
	}
	return shortRef(branch.Head)
}

func statusText(row gitdata.Worktree) string {
	if row.Status.Clean() {
		return "clean"
	}
	return "dirty"
}

func selectionContextTitle(row gitdata.Row) string {
	if row.IsBranch() {
		return "Local branch"
	}
	worktree := row.Worktree
	switch {
	case worktree.IsActive && worktree.IsMain:
		return "Current root repository"
	case worktree.IsActive:
		return "Current worktree"
	case worktree.IsMain:
		return "Root repository"
	default:
		return ""
	}
}

func dirtyDetailText(counts gitdata.StatusCounts) string {
	parts := make([]string, 0, 3)
	if counts.Staged > 0 {
		parts = append(parts, fmt.Sprintf("+ staged %d", counts.Staged))
	}
	if counts.Modified > 0 {
		parts = append(parts, fmt.Sprintf("~ modified %d", counts.Modified))
	}
	if counts.Untracked > 0 {
		parts = append(parts, fmt.Sprintf("? untracked %d", counts.Untracked))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "  ")
}

func (model Model) prText(row gitdata.Worktree) string {
	return model.rowPRText(gitdata.Row{Kind: gitdata.RowKindWorktree, Worktree: row})
}

func (model Model) rowPRText(row gitdata.Row) string {
	if row.PullRequest() == nil {
		if model.pullRequestsPending() {
			return listview.LoadingPlaceholder
		}
		return "none"
	}
	text := row.PullRequest().Text()
	if text == "" {
		return "none"
	}
	return text
}

func (model Model) deletePRText(row gitdata.Worktree) string {
	if row.PR != nil {
		if text := row.PR.Text(); text != "" {
			return text
		}
	}
	if model.pullRequestsPending() {
		return listview.LoadingPlaceholder
	}
	if model.showPR {
		return "none"
	}
	return "unknown"
}

func deleteSafetyText(row gitdata.Worktree) string {
	switch {
	case row.IsActive && row.IsMain:
		return "blocked, active root repository"
	case row.IsActive:
		return "blocked, active worktree"
	case row.IsMain:
		return "blocked, root repository"
	case row.Prunable:
		return "allowed, prunes missing worktree metadata"
	case row.Locked:
		return "blocked, locked worktree"
	case !row.Status.Clean():
		return "allowed with force, dirty worktree"
	case deleteBranchDefault(row):
		return "allowed, branch deletion checked"
	default:
		return "allowed, branch deletion optional"
	}
}

func deleteSafetyStyle(row gitdata.Worktree) lipgloss.Style {
	if row.IsActive || row.IsMain || row.Prunable || row.Locked || !row.Status.Clean() {
		return inspectorWarnStyle
	}
	return inspectorCleanStyle
}

func sizeText(row gitdata.Worktree) string {
	parts := []string{}
	if row.GitSizeLoaded {
		parts = append(parts, "git "+formatByteSize(row.GitSizeBytes))
	} else {
		parts = append(parts, "git "+listview.LoadingPlaceholder)
	}
	if row.FullSizeLoaded {
		parts = append(parts, "full "+formatByteSize(row.FullSizeBytes))
	} else {
		parts = append(parts, "full "+listview.LoadingPlaceholder)
	}
	return strings.Join(parts, ", ")
}

func formatByteSize(bytes int64) string {
	units := []string{"B", "K", "M", "G", "T"}
	value := float64(bytes)
	unitIndex := 0
	for value >= 1024 && unitIndex < len(units)-1 {
		value /= 1024
		unitIndex++
	}
	if unitIndex == 0 {
		return fmt.Sprintf("%dB", bytes)
	}
	if value < 10 {
		return fmt.Sprintf("%.1f%s", value, units[unitIndex])
	}
	return fmt.Sprintf("%.0f%s", value, units[unitIndex])
}

func shortRef(ref string) string {
	if len(ref) <= 7 {
		return ref
	}
	return ref[:7]
}
