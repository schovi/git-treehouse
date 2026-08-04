package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"path/filepath"
	"strings"
	"time"
)

func (model Model) relativePath(path string) string {
	base := model.state.Repo.ActiveWorktree
	if base == "" {
		base = model.state.Repo.Root
	}
	relative, err := filepath.Rel(base, path)
	if err != nil || relative == "" {
		return path
	}
	return relative
}

func (model Model) appTopLine(visibleCount, width int) string {
	return model.appTopLineAtTime(visibleCount, width, time.Now())
}

func (model Model) appTopLineAtTime(visibleCount, width int, now time.Time) string {
	if width <= 0 {
		return ""
	}
	if width < 4 {
		return appBorderStyle.Render(strings.Repeat("─", width))
	}
	innerWidth := width - 4
	right := model.appControlsAtWidthAtTime(innerWidth, now)
	if right != "" {
		right = " " + right + " "
	}
	leftMaxWidth := innerWidth - lipgloss.Width(right) - 3
	if leftMaxWidth < 3 {
		right = ""
		leftMaxWidth = innerWidth - 2
	}
	left := model.titleLeftContentAtWidth(visibleCount, leftMaxWidth)
	if left != "" {
		left = " " + left + " "
	}
	gapWidth := innerWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if gapWidth < 0 {
		gapWidth = 0
	}
	return appBorderStyle.Render("╭─") + left + appBorderStyle.Render(strings.Repeat("─", gapWidth)) + right + appBorderStyle.Render("─╮")
}

func (model Model) appBottomLine(width int) string {
	return bottomBorderLine(width, appBorderStyle, borderControls{
		parts:     model.statusLeftParts(),
		hasStatus: model.loading != "",
	}, borderControls{})
}

func (model Model) statusBar() string {
	return model.statusBarAtWidth(viewWidth(model))
}

func (model Model) statusBarAtWidth(width int) string {
	leftParts := model.statusLeftParts()
	leftText := joinPartsWithin(leftParts, width)
	return colorKeyHints(leftText, model.loading != "" && strings.Contains(leftText, model.loading))
}

func (model Model) statusLeftParts() []string {
	if model.loading != "" {
		return []string{model.loading}
	}
	return nil
}

func (model Model) listFooterLeftHints() string {
	if model.searching {
		return "search " + model.search.Value() + "▌"
	}
	if model.search.Value() != "" {
		return "search: " + model.search.Value()
	}
	return "n new worktree"
}

func (model Model) listFooterRightHints() string {
	branchHint := "b branches"
	if model.showBranches {
		branchHint = "b hide branches"
	}
	if model.searching {
		return "Esc clear · Tab filter: " + model.filter.label() + " · " + branchHint
	}
	if model.filter != filterAll {
		return "h root · a active · Tab filter: " + model.filter.label() + " · Esc clear filter · s search · " + branchHint
	}
	return "h root · a active · Tab filter: " + model.filter.label() + " · s search · " + branchHint
}

func (model Model) listFooterHintsForScrollbar(scrollbar listScrollbar, width int) (string, string) {
	leftFooter := model.listFooterLeftHints()
	if scrollbar.shouldRender(width) {
		rightParts := []string{"Tab filter: " + model.filter.label(), "s search", scrollbar.positionText()}
		if model.searching {
			rightParts = []string{"Esc clear", "Tab filter: " + model.filter.label(), scrollbar.positionText()}
		} else if model.filter != filterAll {
			rightParts = []string{"Tab filter: " + model.filter.label(), "Esc clear filter", "s search", scrollbar.positionText()}
		}
		return leftFooter, joinPartsWithin(rightParts, max(0, width-runewidth.StringWidth(leftFooter)-7))
	}
	return leftFooter, model.listFooterRightHints()
}

func (model Model) noRowsMessage() string {
	filter := model.filter != filterAll
	search := model.search.Value() != ""
	switch {
	case filter && search:
		return "No rows match filter: " + model.filter.label() + " and search: " + model.search.Value() + "\nEsc to clear filter · s then Esc to clear search"
	case filter:
		return "No rows match filter: " + model.filter.label() + " (Esc to clear)"
	case search:
		return "No rows match search: " + model.search.Value() + " (s then Esc to clear)"
	default:
		return "No rows"
	}
}

func (model Model) worktreesFeedback() string {
	if model.refreshInFlight && model.refreshProgressVisible {
		frame := refreshSpinnerFrames[model.refreshSpinnerFrame%len(refreshSpinnerFrames)]
		return refreshActivityStyle.Render(frame + " refreshing")
	}
	return model.feedbackFor(feedbackFrameWorktrees)
}

func joinPartsWithin(parts []string, width int) string {
	if len(parts) == 0 || width <= 0 {
		return ""
	}
	for count := len(parts); count > 0; count-- {
		text := strings.Join(parts[:count], " · ")
		if runewidth.StringWidth(text) <= width {
			return text
		}
	}
	return truncatePlain(parts[0], width)
}

func colorKeyHints(text string, hasStatus bool) string {
	if text == "" {
		return ""
	}
	parts := strings.Split(text, " · ")
	for index, part := range parts {
		parts[index] = colorKeyHintPart(part, hasStatus && index == len(parts)-1)
	}
	return strings.Join(parts, hintStyle.Render(" · "))
}

func colorKeyHintPart(part string, isStatus bool) string {
	key, rest, found := strings.Cut(part, " ")
	if found && key != "" {
		return keyStyle.Render(key) + hintStyle.Render(" "+rest)
	}
	if isStatus {
		return statusMessageStyle.Render(part)
	}
	return hintStyle.Render(part)
}

func (model Model) titleContentAtWidthAtTime(visibleCount, width int, now time.Time) string {
	if width <= 0 {
		return ""
	}
	right := model.appControlsAtWidthAtTime(width, now)
	leftWidth := width - lipgloss.Width(right) - 2
	if leftWidth < 3 {
		right = ""
		leftWidth = width
	}
	left := model.titleLeftContentAtWidth(visibleCount, leftWidth)
	if right == "" {
		return padStyled(left, width)
	}
	spacerWidth := width - lipgloss.Width(left) - lipgloss.Width(right)
	if spacerWidth < 1 {
		spacerWidth = 1
	}
	return left + strings.Repeat(" ", spacerWidth) + right
}

func (model Model) titleLeftContentAtWidth(visibleCount, width int) string {
	if width <= 0 {
		return ""
	}
	repoName := filepath.Base(model.state.Repo.Root)
	if repoName == "." || repoName == string(filepath.Separator) {
		repoName = model.state.Repo.Root
	}
	count := model.rowCountText(len(model.tableRows()))
	if model.search.Value() != "" || model.filter != filterAll {
		count = fmt.Sprintf("%d/%s", visibleCount, model.rowCountText(len(model.tableRows())))
	}
	rootBranch := model.rootBranchTitle()
	if width <= runewidth.StringWidth(appTitle) {
		return titleNameStyle.Render(truncatePlain(appTitle, width))
	}
	title := titleNameStyle.Render(appTitle)
	separator := titleSeparatorStyle.Render(" · ")
	titleAndSeparatorWidth := runewidth.StringWidth(appTitle) + lipgloss.Width(separator)
	staticWidth := titleAndSeparatorWidth + runewidth.StringWidth("  ") + runewidth.StringWidth(count)
	if rootBranch != "" {
		staticWidth += runewidth.StringWidth("  root: ")
	}
	repoWidth := width - staticWidth
	if repoWidth < 4 {
		compactWidth := width - titleAndSeparatorWidth - runewidth.StringWidth(count)
		if compactWidth >= 0 {
			meta := titleMetaStyle.Render(count)
			return title + separator + meta
		}
		repoWidth = width - titleAndSeparatorWidth
		if repoWidth <= 0 {
			return titleNameStyle.Render(truncatePlain(appTitle, width))
		}
		return title + separator + titleRepoStyle.Render(truncatePlain(repoName, repoWidth))
	}
	repoName = truncatePlain(repoName, repoWidth)
	repo := titleRepoStyle.Render(repoName)
	meta := titleMetaStyle.Render(count)
	if rootBranch == "" {
		return title + separator + repo + "  " + meta
	}
	rootWidth := width - lipgloss.Width(title+separator+repo+"  "+meta+"  "+titleMetaStyle.Render("root: "))
	if rootWidth < 3 {
		return title + separator + repo + "  " + meta
	}
	return title + separator + repo + "  " + meta + "  " + titleMetaStyle.Render("root: ") + titleRepoStyle.Render(truncatePlain(rootBranch, rootWidth))
}

func (model Model) rowCountText(total int) string {
	if model.showBranches || model.filter == filterBranches {
		return fmt.Sprintf("%d rows", total)
	}
	return fmt.Sprintf("%d worktrees", len(model.state.Rows))
}

func (model Model) rootBranchTitle() string {
	for _, row := range model.state.Rows {
		if row.IsMain {
			return row.DisplayBranch()
		}
	}
	return ""
}

func (model Model) appControlsAtWidthAtTime(width int, now time.Time) string {
	refresh := refreshControlText(model.lastRefreshAt, now)
	fullWithAge := colorKeyHints(refresh+" · ? help · q quit", false)
	if lipgloss.Width(fullWithAge) <= width {
		return fullWithAge
	}
	full := colorKeyHints("r refresh · ? help · q quit", false)
	if lipgloss.Width(full) <= width {
		return full
	}
	medium := colorKeyHints("r · ? help · q quit", false)
	if lipgloss.Width(medium) <= width {
		return medium
	}
	short := colorKeyHints("? help · q quit", false)
	if lipgloss.Width(short) <= width {
		return short
	}
	tiny := colorKeyHints("? · q", false)
	if lipgloss.Width(tiny) <= width {
		return tiny
	}
	return ""
}

func refreshControlText(lastRefreshAt, now time.Time) string {
	age := refreshAgeText(lastRefreshAt, now)
	if age == "" {
		return "r refresh"
	}
	return "r refresh (" + age + ")"
}

func refreshAgeText(lastRefreshAt, now time.Time) string {
	if lastRefreshAt.IsZero() {
		return ""
	}
	elapsed := now.Sub(lastRefreshAt)
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed < time.Minute {
		return fmt.Sprintf("%d seconds ago", int(elapsed.Seconds()))
	}
	minutes := int(elapsed.Minutes())
	if minutes == 1 {
		return "1 minute ago"
	}
	return fmt.Sprintf("%d minutes ago", minutes)
}

func clockTickCmd(lastRefreshAt time.Time) tea.Cmd {
	return tea.Tick(nextClockTickDelay(lastRefreshAt, time.Now()), func(time.Time) tea.Msg {
		return clockTickMsg{}
	})
}

func refreshSpinnerTickCmd(id int) tea.Cmd {
	return tea.Tick(refreshTickInterval, func(time.Time) tea.Msg {
		return refreshSpinnerTickMsg{id: id}
	})
}

func deleteSpinnerTickCmd(id int) tea.Cmd {
	return tea.Tick(refreshTickInterval, func(time.Time) tea.Msg {
		return deleteSpinnerTickMsg{id: id}
	})
}

func cleanupMergedSpinnerTickCmd(id int) tea.Cmd {
	return tea.Tick(refreshTickInterval, func(time.Time) tea.Msg {
		return cleanupMergedSpinnerTickMsg{id: id}
	})
}

func pullRequestSpinnerTickCmd(id int) tea.Cmd {
	return tea.Tick(refreshTickInterval, func(time.Time) tea.Msg {
		return pullRequestSpinnerTickMsg{id: id}
	})
}

func nextClockTickDelay(lastRefreshAt, now time.Time) time.Duration {
	if lastRefreshAt.IsZero() {
		return time.Minute
	}
	elapsed := now.Sub(lastRefreshAt)
	if elapsed < 0 || elapsed < time.Minute {
		return clockTickInterval
	}
	minutes := int(elapsed / time.Minute)
	nextBoundary := lastRefreshAt.Add(time.Duration(minutes+1) * time.Minute)
	delay := nextBoundary.Sub(now)
	if delay <= 0 {
		return time.Minute
	}
	return delay
}

func autoRefreshTickCmd() tea.Cmd {
	return tea.Tick(autoRefreshInterval, func(time.Time) tea.Msg {
		return autoRefreshMsg{}
	})
}

func (model Model) flashLineAtWidth(width int) string {
	return flashStyle.Render(truncatePlain(model.flash, width))
}

func (model Model) setFlash(text string) (Model, tea.Cmd) {
	model.flashID++
	model.flash = text
	id := model.flashID
	return model, tea.Tick(2200*time.Millisecond, func(time.Time) tea.Msg {
		return clearFlashMsg{id: id}
	})
}

func (model Model) setSuccessFeedbackFor(frame feedbackFrame, text string, timeout time.Duration) (Model, tea.Cmd) {
	return model.setFeedbackFor(successFeedback(frame, text), timeout)
}

func (model Model) setFeedbackFor(feedback transientFeedback, timeout time.Duration) (Model, tea.Cmd) {
	model.pendingRestore = nil
	model.pendingRestoreBatch = nil
	model.feedbackID++
	model.feedback = feedback
	id := model.feedbackID
	return model, tea.Tick(timeout, func(time.Time) tea.Msg {
		return clearFeedbackMsg{id: id}
	})
}

func (model Model) clearFeedback() Model {
	model.feedbackID++
	model.feedback = transientFeedback{}
	model.pendingRestore = nil
	model.pendingRestoreBatch = nil
	return model
}

func (model Model) setRestoreOffer(restore pendingBranchRestore) (Model, tea.Cmd) {
	model, cmd := model.setFeedbackFor(restoreOfferFeedback(restore), restoreOfferTimeout)
	model.pendingRestore = &restore
	return model, cmd
}

func (model Model) setCleanupRestoreOffer(result cleanupMergedResult) (Model, tea.Cmd) {
	model, cmd := model.setFeedbackFor(cleanupRestoreOfferFeedback(result), restoreOfferTimeout)
	model.pendingRestoreBatch = append([]pendingBranchRestore(nil), result.restores...)
	return model, cmd
}

func (model Model) hasPendingRestore() bool {
	return model.pendingRestore != nil || len(model.pendingRestoreBatch) > 0
}

func restoreOfferFeedback(restore pendingBranchRestore) transientFeedback {
	return successFeedbackWithSegments(feedbackFrameWorktrees,
		feedbackSegment{text: restoreOfferPrefix(restore)},
		feedbackSegment{text: "u", bold: true},
		feedbackSegment{text: " to restore"},
	)
}

func restoreOfferPrefix(restore pendingBranchRestore) string {
	return "deleted " + restore.branch + " (" + restore.short + ") · "
}

func cleanupRestoreOfferFeedback(result cleanupMergedResult) transientFeedback {
	return successFeedbackWithSegments(feedbackFrameWorktrees,
		feedbackSegment{text: cleanupMergedSummary(result) + " · "},
		feedbackSegment{text: "u", bold: true},
		feedbackSegment{text: " to restore branches"},
	)
}

func cleanupMergedSummary(result cleanupMergedResult) string {
	parts := []string{
		fmt.Sprintf("removed %d %s", result.removedWorktrees, pluralize(result.removedWorktrees, "worktree", "worktrees")),
		fmt.Sprintf("deleted %d %s", result.deletedBranches, pluralize(result.deletedBranches, "branch", "branches")),
	}
	if len(result.failures) > 0 {
		parts = append(parts, fmt.Sprintf("failed %d", len(result.failures)))
	}
	return "cleaned up merged: " + strings.Join(parts, ", ")
}

func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func successFeedback(frame feedbackFrame, text string) transientFeedback {
	return successFeedbackWithSegments(frame, feedbackSegment{text: text})
}

func successFeedbackWithSegments(frame feedbackFrame, segments ...feedbackSegment) transientFeedback {
	allSegments := make([]feedbackSegment, 0, len(segments)+1)
	allSegments = append(allSegments, feedbackSegment{text: successGlyph + " "})
	allSegments = append(allSegments, segments...)
	return transientFeedback{frame: frame, kind: feedbackKindSuccess, segments: allSegments}
}

func (model Model) feedbackFor(frame feedbackFrame) string {
	if model.feedback.frame != frame || len(model.feedback.segments) == 0 {
		return ""
	}
	return model.feedback.render()
}

func (feedback transientFeedback) render() string {
	style := feedback.style()
	var builder strings.Builder
	for _, segment := range feedback.segments {
		segmentStyle := style
		if segment.bold {
			segmentStyle = segmentStyle.Bold(true)
		}
		builder.WriteString(segmentStyle.Render(segment.text))
	}
	return builder.String()
}

func (feedback transientFeedback) plainText() string {
	var builder strings.Builder
	for _, segment := range feedback.segments {
		builder.WriteString(segment.text)
	}
	return builder.String()
}

func (feedback transientFeedback) style() lipgloss.Style {
	switch feedback.kind {
	default:
		return refreshSuccessStyle
	}
}

func (model Model) frame(content string) string {
	width := viewWidth(model)
	lines := strings.Split(content, "\n")
	height := model.height
	if height <= 0 {
		return strings.Join(lines, "\n")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

func lineCount(value string) int {
	if value == "" {
		return 0
	}
	return len(strings.Split(value, "\n"))
}

func viewWidth(model Model) int {
	if model.width > 0 {
		return model.width
	}
	return 80
}

func (model Model) tableContentWidth() int {
	width := viewWidth(model)
	outerWidth := max(4, width)
	contentWidth := max(1, outerWidth-4)
	panelWidth := max(4, contentWidth)
	return max(1, panelWidth-2)
}
