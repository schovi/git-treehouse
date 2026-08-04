package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/github"
)

// Auxiliary frames are bordered context blocks stacked below the Details panel.
// Each frame renders itself fully (including borders) for a given outer width, or
// returns "" when it has nothing to show. belowDetailFrames collects the frames
// that apply to the selected row, in priority order. See docs/features.

// changesFrameMinWidth is the narrowest panel width at which the Changes frame is
// worth showing; below it the Details panel alone carries the dirty summary.
const changesFrameMinWidth = 50

// changesFrameMaxFiles caps how many file rows the Changes frame lists before
// collapsing the remainder into a "+N more" line, keeping the frame height bounded.
const changesFrameMaxFiles = 8

// belowDetailFrames returns the secondary frame boxes that stack full width below
// the Detail region. The Details, Changes, and PR review boxes (left column) and the
// Git context frame (right column) are composed separately, so they are not included
// here; only the Disk frame remains, and it is currently disabled.
func belowDetailFrames(row gitdata.Row, panelWidth int) []string {
	var frames []string
	// The Disk frame is built and tested but kept out of the rendered stack for
	// now: it is a promising idea, just not earning its space by default yet.
	if diskFrameEnabled {
		if box := diskFrame(row, panelWidth); box != "" {
			frames = append(frames, box)
		}
	}
	return frames
}

// diskFrameEnabled gates whether the Disk usage frame appears in the rendered
// frame stack. It stays off by default; flip it to bring the frame back.
const diskFrameEnabled = false

// changesFrame renders the per-file git status preview for a worktree row. It always
// renders for a worktree row (showing "no changes" when the tree is clean) so it can
// pair beneath the Details box; it returns "" only for branch rows or a panel too
// narrow to help.
func changesFrame(row gitdata.Row, panelWidth int) string {
	if row.Kind != gitdata.RowKindWorktree || panelWidth < changesFrameMinWidth {
		return ""
	}
	innerWidth := panelWidth - 2
	files := row.Worktree.ChangedFiles
	if len(files) == 0 {
		clean := inspectorCleanStyle.Render(truncatePlain("no changes", innerWidth))
		return sectionBoxWithFooter("Changes", []string{clean}, "", panelWidth)
	}
	lines := []string{changesSummaryLine(row.Worktree, len(files), innerWidth)}
	lines = append(lines, changesFileLines(files, innerWidth)...)
	return sectionBoxWithFooter("Changes", lines, "", panelWidth)
}

// changesSummaryLine is the roll-up header: file count, total churn, and the
// staged/modified/untracked split that mirrors the Details panel summary.
func changesSummaryLine(worktree gitdata.Worktree, fileCount, innerWidth int) string {
	totalAdded, totalDeleted := 0, 0
	for _, file := range worktree.ChangedFiles {
		if file.HasStats() {
			totalAdded += file.Added
			totalDeleted += file.Deleted
		}
	}
	summary := fmt.Sprintf("%d files · +%d/-%d · %d staged %d modified %d untracked",
		fileCount, totalAdded, totalDeleted,
		worktree.Status.Staged, worktree.Status.Modified, worktree.Status.Untracked)
	return inspectorLabelStyle.Render(truncatePlain(summary, innerWidth))
}

// changesFileLines renders the sorted file rows plus an optional "+N more" line.
func changesFileLines(files []gitdata.ChangedFile, innerWidth int) []string {
	sorted := sortChangedFiles(files)
	limit := len(sorted)
	overflow := 0
	if limit > changesFrameMaxFiles {
		limit = changesFrameMaxFiles - 1
		overflow = len(sorted) - limit
	}
	lines := make([]string, 0, limit+1)
	for _, file := range sorted[:limit] {
		lines = append(lines, changesFileLine(file, innerWidth))
	}
	if overflow > 0 {
		lines = append(lines, hintStyle.Render(fmt.Sprintf("  +%d more files", overflow)))
	}
	return lines
}

// sortChangedFiles orders files by group (staged, then unstaged tracked, then
// untracked) and within each group by descending churn, so the heaviest changes
// surface first. The input slice is not mutated.
func sortChangedFiles(files []gitdata.ChangedFile) []gitdata.ChangedFile {
	sorted := append([]gitdata.ChangedFile(nil), files...)
	sort.SliceStable(sorted, func(left, right int) bool {
		leftRank, rightRank := changedFileGroup(sorted[left]), changedFileGroup(sorted[right])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return changedFileChurn(sorted[left]) > changedFileChurn(sorted[right])
	})
	return sorted
}

func changedFileGroup(file gitdata.ChangedFile) int {
	switch {
	case file.Untracked():
		return 2
	case file.Staged():
		return 0
	default:
		return 1
	}
}

func changedFileChurn(file gitdata.ChangedFile) int {
	if !file.HasStats() {
		return 0
	}
	return file.Added + file.Deleted
}

// changesFileLine renders one file row: staged marker, status glyph, left-truncated
// path, and right-aligned +/- stats, filling exactly innerWidth columns.
func changesFileLine(file gitdata.ChangedFile, innerWidth int) string {
	marker := "  "
	if file.Staged() {
		marker = inspectorCommitStyle.Render("* ")
	}
	glyph := changedFileGlyphStyle(file).Render(string(file.Glyph()))
	prefixWidth := 4 // marker(2) + glyph(1) + space(1)

	stats, statsWidth := changesStatsCell(file)
	pathBudget := innerWidth - prefixWidth - statsWidth - 1
	if pathBudget < 1 {
		pathBudget = 1
	}
	path := truncatePathTail(changesDisplayPath(file), pathBudget)
	gap := innerWidth - prefixWidth - lipgloss.Width(path) - statsWidth
	if gap < 1 {
		gap = 1
	}
	return marker + glyph + " " + inspectorValueStyle.Render(path) + strings.Repeat(" ", gap) + stats
}

// changesStatsCell returns the styled +/- (or "new") cell and its plain width.
func changesStatsCell(file gitdata.ChangedFile) (string, int) {
	if file.Untracked() {
		return hintStyle.Render("new"), 3
	}
	if !file.HasStats() {
		return "", 0
	}
	added := fmt.Sprintf("+%d", file.Added)
	deleted := fmt.Sprintf("-%d", file.Deleted)
	plainWidth := len(added) + 1 + len(deleted)
	return inspectorCleanStyle.Render(added) + " " + deleteDangerStyle.Render(deleted), plainWidth
}

func changesDisplayPath(file gitdata.ChangedFile) string {
	if file.OrigPath != "" {
		return file.OrigPath + " -> " + file.Path
	}
	return file.Path
}

func changedFileGlyphStyle(file gitdata.ChangedFile) lipgloss.Style {
	switch file.Glyph() {
	case '?':
		return hintStyle
	case 'A':
		return inspectorCleanStyle
	case 'M':
		return inspectorWarnStyle
	case 'D':
		return deleteDangerStyle
	case 'R':
		return inspectorCommitStyle
	default:
		return inspectorValueStyle
	}
}

// truncatePathTail shortens a path from the left, keeping the filename visible,
// prefixing an ellipsis when it does not fit.
func truncatePathTail(path string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(path) <= width {
		return path
	}
	runes := []rune(path)
	if width == 1 {
		return "…"
	}
	tail := runes[len(runes)-(width-1):]
	return "…" + string(tail)
}

// gitContextFrameMinWidth is the narrowest panel width at which the Git context
// frame is worth showing.
const gitContextFrameMinWidth = 50

// graphDisplayCap is how many commits per side the Git context frame lists before
// collapsing the rest into a "+N more" line.
const graphDisplayCap = 2

// minGraphCommits is the number of commit nodes the Git context graph tries to
// show. When the divergence is small, shared ancestors below the fork point pad
// the graph up to this many rows so it never collapses to one or two lines.
const minGraphCommits = 5

// graphSource is the subset of a row needed to render the Git context graph,
// extracted so the same renderer serves both worktree rows and branch-only rows.
type graphSource struct {
	graph        gitdata.ContextGraph
	mainSync     gitdata.SyncState
	headSync     gitdata.SyncState
	upstream     string
	upstreamGone bool
	branch       string
	commitShort  string
}

// graphSourceForRow unifies a worktree row and a branch-only row into one input for
// the graph renderer; a branch row's tip ref stands in for the worktree's HEAD.
func graphSourceForRow(row gitdata.Row) graphSource {
	if row.IsBranch() {
		branch := row.Branch
		return graphSource{
			graph:        branch.Graph,
			mainSync:     branch.MainSync,
			headSync:     branch.HeadSync,
			upstream:     branch.Upstream,
			upstreamGone: branch.UpstreamGone,
			branch:       branch.Name,
			commitShort:  branch.CommitShort,
		}
	}
	worktree := row.Worktree
	return graphSource{
		graph:        worktree.Graph,
		mainSync:     worktree.MainSync,
		headSync:     worktree.HeadSync,
		upstream:     worktree.Upstream,
		upstreamGone: worktree.UpstreamGone,
		branch:       worktree.Branch,
		commitShort:  worktree.CommitShort,
	}
}

// gitContextFrame places the selected row's tip in its local history: the commits it
// is behind on main (above), the implicit fork point, and its own commits with HEAD
// marked (below), plus a remote-sync header line. It renders for both worktree rows
// and branch-only rows (a branch's tip ref stands in for HEAD). Returns "" when there
// is no graph to show. When minBoxLines > 0 the frame grows to that total box height
// (the Details box it pairs with), showing more shared ancestors and, only if it runs
// out of real history, blank lines, so their bottom borders align.
func gitContextFrame(row gitdata.Row, mainBranch string, panelWidth, minBoxLines int) string {
	if panelWidth < gitContextFrameMinWidth {
		return ""
	}
	source := graphSourceForRow(row)
	graph := source.graph
	if !graph.Loaded {
		return ""
	}
	innerWidth := panelWidth - 2
	ahead, behind := graphTotals(source)

	lines := graphHeaderLines(source, mainBranch, ahead, behind, innerWidth)

	nodesShown := 0
	if ahead != 0 || behind != 0 {
		// A blank line detaches the labeled header from the commit rows below it.
		lines = append(lines, "")
		core, nodes := graphCoreLines(source, ahead, behind, innerWidth)
		lines = append(lines, core...)
		nodesShown = nodes
	}

	lines = fillGraphLines(lines, graph.BaseCommits, nodesShown, ahead, behind, minBoxLines, innerWidth)
	return sectionBoxWithFooter("Git context", lines, "", panelWidth)
}

// graphHeaderLabelCap bounds the label column so a long main-branch name cannot
// crowd out the values.
const graphHeaderLabelCap = 12

// graphHeaderLines renders the two-row header above the commit graph: how HEAD sits
// relative to the parent (main) branch, then relative to its remote. Each row is an
// aligned label (the real ref name) plus a colored value, so the pair reads as a
// small key/value header distinct from the flush-left `●` commit rows below.
func graphHeaderLines(source graphSource, mainBranch string, ahead, behind, innerWidth int) []string {
	parentLabel := mainBranch
	if parentLabel == "" {
		parentLabel = "main"
	}
	remoteLabel, remoteValue := graphRemoteParts(source)

	labelWidth := lipgloss.Width(parentLabel)
	if width := lipgloss.Width(remoteLabel); width > labelWidth {
		labelWidth = width
	}
	if labelWidth > graphHeaderLabelCap {
		labelWidth = graphHeaderLabelCap
	}
	valueBudget := max(1, innerWidth-labelWidth-2)
	return []string{
		graphHeaderRow(parentLabel, graphParentValue(source, mainBranch, ahead, behind, valueBudget), labelWidth),
		graphHeaderRow(remoteLabel, remoteValue, labelWidth),
	}
}

func graphHeaderRow(label, value string, labelWidth int) string {
	return inspectorLabelStyle.Render(padRight(truncatePlain(label, labelWidth), labelWidth)) + "  " + value
}

// graphParentValue is the value cell for the parent row: colored ahead/behind arrows
// (vs main) and the current branch name as `→ <current>`, or an in-sync note.
func graphParentValue(source graphSource, mainBranch string, ahead, behind, budget int) string {
	if ahead == 0 && behind == 0 {
		text := "in sync"
		if source.commitShort != "" {
			text += " · " + source.commitShort
		}
		return inspectorCleanStyle.Render(truncatePlain(text, budget))
	}
	plain, styled := graphSyncArrows(ahead, behind)
	current := source.branch
	if current == "" {
		current = "detached"
	}
	suffix := ""
	if current != mainBranch {
		suffix = " → " + current
	}
	rest := budget - lipgloss.Width(plain)
	if rest < 1 {
		return inspectorLabelStyle.Render(truncatePlain(plain+suffix, budget))
	}
	return styled + hintStyle.Render(truncatePlain(suffix, rest))
}

// graphRemoteParts returns the label and colored value for the remote row, mirroring
// the table's remote column: a green `✓ synced`, colored `↑/↓` arrows, `gone`, or a
// `no upstream` note. This is the sole place the remote sync state is shown for a
// worktree row (the Details panel no longer repeats it).
func graphRemoteParts(source graphSource) (label, value string) {
	if source.upstream == "" {
		return "remote", hintStyle.Render("no upstream")
	}
	name := remoteShortName(source.upstream)
	if source.upstreamGone {
		return name, deleteDangerStyle.Render("gone")
	}
	sync := source.headSync
	if !sync.Available || (sync.Ahead == 0 && sync.Behind == 0) {
		return name, inspectorCleanStyle.Render("✓ synced")
	}
	_, styled := graphSyncArrows(sync.Ahead, sync.Behind)
	return name, styled
}

// fillGraphLines appends shared ancestors below the fork point so the graph reaches
// either its node-count floor (minGraphCommits, the standalone case) or a target box
// height (minBoxLines, when paired with Details). Once the real ancestors run out it
// pads with blank lines, but only when growing to a target box height.
func fillGraphLines(lines []string, ancestors []gitdata.GraphCommit, nodesShown, ahead, behind, minBoxLines, innerWidth int) []string {
	nodeFloor := 0
	if ahead != 0 || behind != 0 {
		// nodesShown already counts the fork node, so the floor compares against it.
		nodeFloor = minGraphCommits - nodesShown
	}
	lineFloor := 0
	if minBoxLines > 0 {
		lineFloor = (minBoxLines - 2) - len(lines)
	}
	wanted := nodeFloor
	if lineFloor > wanted {
		wanted = lineFloor
	}
	for index, commit := range ancestors {
		if index >= wanted {
			break
		}
		lines = append(lines, graphNodeLine("", commit, hintStyle, "", lipgloss.Style{}, innerWidth))
	}
	if minBoxLines > 0 {
		for len(lines) < minBoxLines-2 {
			lines = append(lines, "")
		}
	}
	return lines
}

// graphTotals returns the ahead/behind counts vs main, preferring the row's
// MainSync and falling back to the fetched commit list lengths.
func graphTotals(source graphSource) (ahead, behind int) {
	if source.mainSync.Available {
		return source.mainSync.Ahead, source.mainSync.Behind
	}
	return len(source.graph.BranchCommits), len(source.graph.MainCommits)
}

// graphSyncArrows formats the ahead/behind counts as colored arrows matching the
// table's main± column (`↑ahead` amber, `↓behind` red), returning the plain text
// for width math and the styled text for display.
func graphSyncArrows(ahead, behind int) (plain, styled string) {
	aheadText, behindText := fmt.Sprintf("↑%d", ahead), fmt.Sprintf("↓%d", behind)
	switch {
	case ahead > 0 && behind > 0:
		return aheadText + " " + behindText, inspectorWarnStyle.Render(aheadText) + " " + deleteDangerStyle.Render(behindText)
	case ahead > 0:
		return aheadText, inspectorWarnStyle.Render(aheadText)
	default:
		return behindText, deleteDangerStyle.Render(behindText)
	}
}

// graphCoreLines draws the ASCII history. When main has commits of its own (behind >
// 0) the worktree has genuinely diverged, so it draws two rails: the parent (main)
// trunk down the left and the worktree's own commits on a rail to its right, folding
// back (`├─┘`) into the shared fork point at the bottom. When behind == 0 there is no
// divergence (the branch is a clean fast-forward ahead of main), so the side rail and
// fold would dangle with nothing to anchor them; it collapses to a single straight
// column of commits ending on the fork point. The upstream is woven into the same
// branch line: commits to pull (HEAD..@{u}) render as `◆` nodes above HEAD, the
// newest tagged with the remote name; when HEAD is ahead of the remote, the commit
// the remote points to gets a `← <remote>` ref label. Main commits are dim, our
// commits bright (newest tagged HEAD), remote commits the label color. It returns the
// rendered lines and the number of commit nodes drawn, used to size ancestor padding.
func graphCoreLines(source graphSource, ahead, behind int, innerWidth int) ([]string, int) {
	graph := source.graph
	var lines []string
	diverged := behind > 0
	rail := ""
	if diverged {
		rail = "│ "
	}

	remoteName := remoteShortName(source.upstream)
	hasUpstream := source.upstream != "" && !source.upstreamGone && source.headSync.Available
	behindRemote, aheadRemote := 0, 0
	if hasUpstream {
		behindRemote, aheadRemote = source.headSync.Behind, source.headSync.Ahead
	}

	mainShown := capGraphCommits(graph.MainCommits)
	for _, commit := range mainShown {
		lines = append(lines, graphNodeLine("", commit, hintStyle, "", lipgloss.Style{}, innerWidth))
	}
	if overflow := behind - len(mainShown); overflow > 0 {
		lines = append(lines, hintStyle.Render(truncatePlain(fmt.Sprintf("┆ +%d more on main", overflow), innerWidth)))
	}

	// Commits the remote has that we do not (a pull would bring these in) sit above
	// HEAD on the branch rail; the newest carries the remote ref label.
	remoteShown := []gitdata.GraphCommit{}
	if behindRemote > 0 {
		remoteShown = capGraphCommits(graph.RemoteCommits)
	}
	for index, commit := range remoteShown {
		tag := ""
		if index == 0 {
			tag = "← " + remoteName
		}
		lines = append(lines, graphNodeLineGlyph(rail, "◆", commit, inspectorLabelStyle, tag, inspectorLabelStyle, innerWidth))
	}
	if overflow := behindRemote - len(remoteShown); overflow > 0 {
		marker := "◆"
		if diverged {
			marker = "│ ◆"
		}
		lines = append(lines, inspectorLabelStyle.Render(truncatePlain(fmt.Sprintf("%s +%d more to pull", marker, overflow), innerWidth)))
	}

	// When HEAD is ahead of the remote (unpushed commits), the remote points at the
	// commit aheadRemote steps below HEAD; label it so everything above reads as
	// unpushed. Only labelled when that commit is visible on the rail.
	originLabelIndex := -1
	if behindRemote == 0 && aheadRemote > 0 {
		originLabelIndex = aheadRemote
	}
	branchShown := capGraphCommits(graph.BranchCommits)
	for index, commit := range branchShown {
		tag, tagStyle := "", inspectorWarnStyle
		switch index {
		case 0:
			tag = "← HEAD"
		case originLabelIndex:
			tag, tagStyle = "← "+remoteName, inspectorLabelStyle
		}
		lines = append(lines, graphNodeLine(rail, commit, inspectorCommitStyle, tag, tagStyle, innerWidth))
	}
	if overflow := ahead - len(branchShown); overflow > 0 {
		marker := "┆"
		if diverged {
			marker = "│ ┆"
		}
		lines = append(lines, hintStyle.Render(truncatePlain(fmt.Sprintf("%s +%d more ahead", marker, overflow), innerWidth)))
	}

	railHasNodes := len(remoteShown) > 0 || len(branchShown) > 0
	if ahead > 0 {
		if diverged && railHasNodes {
			// The branch rail folds left into the trunk just above the shared base.
			lines = append(lines, hintStyle.Render("├─┘"))
		}
		lines = append(lines, graphForkNodeLine(graph.ForkPoint, false, "", innerWidth))
	} else {
		if diverged && railHasNodes {
			lines = append(lines, hintStyle.Render("├─┘"))
		}
		// No commits of our own: the tip is the fork point, an ancestor of main.
		lines = append(lines, graphForkNodeLine(graph.ForkPoint, true, source.commitShort, innerWidth))
	}

	return lines, len(mainShown) + len(remoteShown) + len(branchShown) + 1
}

// remoteShortName returns the remote name from an upstream ref like
// `origin/feature/x` → `origin`, or "remote" when there is no slash.
func remoteShortName(upstream string) string {
	if slash := strings.Index(upstream, "/"); slash >= 0 {
		return upstream[:slash]
	}
	if upstream == "" {
		return "remote"
	}
	return upstream
}

func capGraphCommits(commits []gitdata.GraphCommit) []gitdata.GraphCommit {
	if len(commits) > graphDisplayCap {
		return commits[:graphDisplayCap]
	}
	return commits
}

// graphNodeLine renders one commit node with the default `●` glyph. See
// graphNodeLineGlyph.
func graphNodeLine(rail string, commit gitdata.GraphCommit, nodeStyle lipgloss.Style, tag string, tagStyle lipgloss.Style, innerWidth int) string {
	return graphNodeLineGlyph(rail, "●", commit, nodeStyle, tag, tagStyle, innerWidth)
}

// graphNodeLineGlyph renders one commit node: an optional rail prefix (the trunk
// passing to the left of a branch node), the node glyph, its short SHA, the subject
// truncated to fit, and an optional tag (e.g. HEAD or fork point) in the given style.
func graphNodeLineGlyph(rail, glyph string, commit gitdata.GraphCommit, nodeStyle lipgloss.Style, tag string, tagStyle lipgloss.Style, innerWidth int) string {
	railPart, railWidth := "", 0
	if rail != "" {
		railPart = hintStyle.Render(rail)
		railWidth = lipgloss.Width(rail)
	}
	tagPart, tagWidth := "", 0
	if tag != "" {
		tagPart = " " + tagStyle.Render(tag)
		tagWidth = 1 + lipgloss.Width(tag)
	}
	node := nodeStyle.Render(glyph) + " "
	sha := inspectorCommitStyle.Render(commit.Short) + " "
	used := railWidth + 2 + lipgloss.Width(commit.Short) + 1 + tagWidth
	subjectBudget := innerWidth - used
	if subjectBudget < 1 {
		subjectBudget = 1
	}
	subject := inspectorValueStyle.Render(truncatePlain(commit.Subject, subjectBudget))
	return railPart + node + sha + subject + tagPart
}

// graphForkNodeLine renders the shared base both sides diverge from as a real
// commit (SHA + subject) tagged `← fork point`. When the worktree has no commits
// of its own (ahead == 0), HEAD coincides with the fork point and is tagged HEAD
// instead. Falls back to a label-only node when the merge-base was not fetched.
func graphForkNodeLine(fork gitdata.GraphCommit, isHead bool, headShort string, innerWidth int) string {
	commit := fork
	tag, tagStyle := "← fork point", inspectorLabelStyle
	if isHead {
		tag, tagStyle = "← HEAD", inspectorWarnStyle
		if commit.Short == "" {
			commit.Short = headShort
		}
	}
	if commit.Short == "" {
		label := inspectorLabelStyle.Render(truncatePlain("fork point", innerWidth-2))
		return hintStyle.Render("●") + " " + label
	}
	return graphNodeLine("", commit, hintStyle, tag, tagStyle, innerWidth)
}

// diskFrameMinWidth is the narrowest panel width at which the Disk frame renders.
const diskFrameMinWidth = 50

// diskFrameThreshold is the total worktree size below which the Disk frame stays
// hidden; small worktrees do not need a cleanup pitch and the Details panel
// already shows their size.
const diskFrameThreshold int64 = 50 * 1024 * 1024

const (
	diskLabelColumn   = 13
	diskSizeColumn    = 5
	diskPercentColumn = 4
)

// diskFrame renders where a worktree's bytes go as a small bar chart. It appears
// only once disk usage has loaded and the total crosses diskFrameThreshold.
func diskFrame(row gitdata.Row, panelWidth int) string {
	if row.Kind != gitdata.RowKindWorktree || panelWidth < diskFrameMinWidth {
		return ""
	}
	breakdown := row.Worktree.DiskBreakdown
	if !breakdown.Loaded || breakdown.Total < diskFrameThreshold || len(breakdown.Buckets) == 0 {
		return ""
	}
	innerWidth := panelWidth - 2
	maxBytes := breakdown.Buckets[0].Bytes
	lines := make([]string, 0, len(breakdown.Buckets)+1)
	for _, bucket := range breakdown.Buckets {
		lines = append(lines, diskBucketLine(bucket, maxBytes, breakdown.Total, innerWidth))
	}
	if breakdown.ReclaimableBytes > 0 {
		note := fmt.Sprintf("reclaimable %s (deps + build), no committed work lost", formatByteSize(breakdown.ReclaimableBytes))
		lines = append(lines, inspectorCleanStyle.Render(truncatePlain(note, innerWidth)))
	}
	title := "Disk · " + formatByteSize(breakdown.Total)
	return sectionBoxWithFooter(title, lines, "", panelWidth)
}

func diskBucketLine(bucket gitdata.DiskBucket, maxBytes, total int64, innerWidth int) string {
	label := inspectorValueStyle.Render(padRight(truncatePlain(bucket.Label, diskLabelColumn), diskLabelColumn))
	size := inspectorLabelStyle.Render(padRight(formatByteSize(bucket.Bytes), diskSizeColumn))
	percent := 0
	if total > 0 {
		percent = int(bucket.Bytes * 100 / total)
	}
	percentText := hintStyle.Render(padLeft(fmt.Sprintf("%d%%", percent), diskPercentColumn))

	barBudget := innerWidth - diskLabelColumn - 1 - diskSizeColumn - 1 - 1 - diskPercentColumn
	if barBudget < 1 {
		barBudget = 1
	}
	barLength := 0
	if maxBytes > 0 {
		barLength = int(bucket.Bytes * int64(barBudget) / maxBytes)
	}
	if barLength < 1 {
		barLength = 1
	}
	bar := diskBucketStyle(bucket.Label).Render(strings.Repeat("▓", barLength))
	barCell := padStyled(bar, barBudget)
	return label + " " + size + " " + barCell + " " + percentText
}

func diskBucketStyle(label string) lipgloss.Style {
	switch label {
	case "dependencies":
		return inspectorWarnStyle
	case "build output":
		return inspectorCommitStyle
	case "git data":
		return hintStyle
	default:
		return inspectorCleanStyle
	}
}

func padLeft(value string, width int) string {
	if gap := width - lipgloss.Width(value); gap > 0 {
		return strings.Repeat(" ", gap) + value
	}
	return value
}

// osc8 wraps text in an OSC 8 terminal hyperlink. The escape sequences are
// zero-width, so callers can size text before wrapping it. With no URL the text
// is returned unchanged.
func osc8(url, text string) string {
	if url == "" {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// prReviewFrameMinWidth is the narrowest panel width at which the PR review frame
// renders.
const prReviewFrameMinWidth = 50

// prReviewFrameMaxChecks caps how many individual failing/running checks the frame
// lists; passed and skipped checks stay in the header roll-up only.
const prReviewFrameMaxChecks = 5

// prReviewFrameMaxThreads caps how many change-request notes the frame lists.
const prReviewFrameMaxThreads = 3

// prReviewFrameMaxComments caps how many inline review threads (line comments) the
// frame lists; unresolved threads are listed first.
const prReviewFrameMaxComments = 6

const prReviewLabelColumn = 9

// prReviewFrame surfaces the merge blockers for the selected row's pull request,
// in the order state, checks, review. Failing/running checks are listed (each
// linking to its detail page) and change-request reviews appear under the review
// line as one-line previews linking to the PR. A closing verdict reflects the real
// merge state. Returns "" when there is no loaded review.
func prReviewFrame(review *github.PullRequestReview, pendingNumber, panelWidth int) string {
	if panelWidth < prReviewFrameMinWidth {
		return ""
	}
	if review != nil && review.Loaded {
		innerWidth := panelWidth - 2
		lines := []string{
			prReviewStateLine(*review, innerWidth),
			prReviewField("checks", prReviewChecksSummary(*review), innerWidth),
		}
		lines = append(lines, prReviewCheckLines(*review, innerWidth)...)
		lines = append(lines, prReviewField("review", prReviewDecisionText(*review), innerWidth))
		lines = append(lines, prReviewThreadLines(*review, innerWidth)...)
		if len(review.Threads) > 0 {
			lines = append(lines, prReviewField("comments", prReviewCommentsSummary(*review), innerWidth))
			lines = append(lines, prReviewCommentLines(*review, innerWidth)...)
		}
		title := fmt.Sprintf("PR review · #%d", review.Number)
		return sectionBoxWithFooter(title, lines, "", panelWidth)
	}
	// The review detail is still loading. Keep the frame visible with a
	// placeholder so the layout does not jump when it arrives. A finished
	// attempt clears pendingNumber, so a failed or gh-less lookup shows nothing.
	if pendingNumber == 0 {
		return ""
	}
	title := fmt.Sprintf("PR review · #%d", pendingNumber)
	line := hintStyle.Render(truncatePlain("loading review…", panelWidth-2))
	return sectionBoxWithFooter(title, []string{line}, "", panelWidth)
}

func prReviewField(label, value string, innerWidth int) string {
	labelText := inspectorLabelStyle.Render(padRight(label, prReviewLabelColumn))
	valueWidth := innerWidth - prReviewLabelColumn
	if valueWidth < 1 {
		valueWidth = 1
	}
	return labelText + inspectorValueStyle.Render(truncatePlain(value, valueWidth))
}

// prReviewStateLine renders the state field: open/closed/merged, an optional
// `· draft`, and the real merge-state status as a colored third token (e.g.
// `blocked by merge requirements`), so the headline merge verdict reads inline
// with the state instead of as a separate trailing line. The token is dropped
// when there is nothing honest to add (draft already shows; unknown stays quiet).
func prReviewStateLine(review github.PullRequestReview, innerWidth int) string {
	label := inspectorLabelStyle.Render(padRight("state", prReviewLabelColumn))
	budget := innerWidth - prReviewLabelColumn
	if budget < 1 {
		budget = 1
	}
	// A merged PR gets a purple git glyph, matching GitHub's merged badge.
	glyph, glyphWidth := "", 0
	if strings.EqualFold(review.State, "merged") {
		glyph = mergedGlyphStyle.Render("⎇") + " "
		glyphWidth = 2
	}
	if budget-glyphWidth >= 1 {
		budget -= glyphWidth
	} else {
		glyph = "" // no room for the glyph on a very narrow panel
	}
	base := strings.ToLower(review.State)
	if base == "" {
		base = "open"
	}
	if review.IsDraft {
		base += " · draft"
	}
	token, tokenStyle := mergeStateToken(review)
	separator := " · "
	if token == "" || lipgloss.Width(base)+lipgloss.Width(separator)+1 > budget {
		return label + glyph + inspectorValueStyle.Render(truncatePlain(base, budget))
	}
	tokenBudget := budget - lipgloss.Width(base) - lipgloss.Width(separator)
	return label + glyph + inspectorValueStyle.Render(base) + hintStyle.Render(separator) +
		tokenStyle.Render(truncatePlain(token, tokenBudget))
}

// mergeStateToken describes GitHub's merge-state status as a short colored token
// matching the web UI, or "" when there is nothing actionable to show.
func mergeStateToken(review github.PullRequestReview) (string, lipgloss.Style) {
	switch strings.ToUpper(review.MergeStateStatus) {
	case "CLEAN", "HAS_HOOKS":
		_, fail, running, _ := review.CheckCounts()
		if fail == 0 && running == 0 && len(review.ChangeRequests) == 0 &&
			strings.ToUpper(review.ReviewDecision) == "APPROVED" {
			return "ready to merge", inspectorCleanStyle
		}
		return "", lipgloss.Style{}
	case "BLOCKED":
		return "blocked by merge requirements", deleteDangerStyle
	case "DIRTY":
		return "merge conflicts", deleteDangerStyle
	case "BEHIND":
		return "behind base branch", inspectorWarnStyle
	case "UNSTABLE":
		return "checks not passing", inspectorWarnStyle
	default:
		return "", lipgloss.Style{}
	}
}

func prReviewDecisionText(review github.PullRequestReview) string {
	switch strings.ToUpper(review.ReviewDecision) {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		if count := len(review.ChangeRequests); count > 0 {
			return fmt.Sprintf("changes requested by %d", count)
		}
		return "changes requested"
	case "REVIEW_REQUIRED":
		return "review required"
	default:
		return "no review yet"
	}
}

func prReviewChecksSummary(review github.PullRequestReview) string {
	pass, fail, running, skipped := review.CheckCounts()
	if pass+fail+running+skipped == 0 {
		return "none"
	}
	parts := make([]string, 0, 4)
	if pass > 0 {
		parts = append(parts, fmt.Sprintf("%d passed", pass))
	}
	if fail > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", fail))
	}
	if running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", running))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	return strings.Join(parts, " · ")
}

// prReviewCheckLines lists the actionable checks (failing first, then running);
// passed and skipped checks stay in the summary. Capped with a "+N more" line.
func prReviewCheckLines(review github.PullRequestReview, innerWidth int) []string {
	actionable := make([]github.Check, 0, len(review.Checks))
	for _, check := range review.Checks {
		if check.State == github.CheckFail {
			actionable = append(actionable, check)
		}
	}
	for _, check := range review.Checks {
		if check.State == github.CheckRunning {
			actionable = append(actionable, check)
		}
	}
	shown := actionable
	overflow := 0
	if len(shown) > prReviewFrameMaxChecks {
		shown = shown[:prReviewFrameMaxChecks]
		overflow = len(actionable) - prReviewFrameMaxChecks
	}
	lines := make([]string, 0, len(shown)+1)
	for _, check := range shown {
		glyphStyle, glyph := prReviewCheckGlyph(check.State)
		name := truncatePlain(check.Name, innerWidth-4)
		lines = append(lines, "  "+glyphStyle.Render(glyph)+" "+osc8(check.URL, inspectorValueStyle.Render(name)))
	}
	if overflow > 0 {
		lines = append(lines, hintStyle.Render(fmt.Sprintf("  +%d more checks", overflow)))
	}
	return lines
}

func prReviewCheckGlyph(state string) (lipgloss.Style, string) {
	switch state {
	case github.CheckFail:
		return deleteDangerStyle, "✗"
	case github.CheckRunning:
		return inspectorWarnStyle, "●"
	default:
		return inspectorCleanStyle, "✓"
	}
}

func prReviewThreadLines(review github.PullRequestReview, innerWidth int) []string {
	if len(review.ChangeRequests) == 0 {
		return nil
	}
	shown := review.ChangeRequests
	overflow := 0
	if len(shown) > prReviewFrameMaxThreads {
		shown = shown[:prReviewFrameMaxThreads]
		overflow = len(review.ChangeRequests) - prReviewFrameMaxThreads
	}
	lines := make([]string, 0, len(shown)+1)
	for _, note := range shown {
		authorText := "@" + note.Author
		bodyBudget := innerWidth - lipgloss.Width(authorText) - 3
		if bodyBudget < 1 {
			bodyBudget = 1
		}
		preview := inspectorCommitStyle.Render(authorText) + " " + hintStyle.Render(truncatePlain(note.Body, bodyBudget))
		lines = append(lines, "  "+osc8(note.URL, preview))
	}
	if overflow > 0 {
		lines = append(lines, hintStyle.Render(fmt.Sprintf("  +%d more", overflow)))
	}
	return lines
}

func prReviewCommentsSummary(review github.PullRequestReview) string {
	unresolved, resolved := review.ThreadCounts()
	parts := make([]string, 0, 2)
	if unresolved > 0 {
		parts = append(parts, fmt.Sprintf("%d unresolved", unresolved))
	}
	if resolved > 0 {
		parts = append(parts, fmt.Sprintf("%d resolved", resolved))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " · ")
}

// prReviewCommentLines lists inline review threads, unresolved first, each with a
// resolution glyph (`○` open / `✓` resolved), file location, and the first line of
// the opening comment, linking to the comment on the web. Capped with "+N more".
func prReviewCommentLines(review github.PullRequestReview, innerWidth int) []string {
	ordered := make([]github.ReviewThread, 0, len(review.Threads))
	for _, thread := range review.Threads {
		if !thread.Resolved {
			ordered = append(ordered, thread)
		}
	}
	for _, thread := range review.Threads {
		if thread.Resolved {
			ordered = append(ordered, thread)
		}
	}
	shown := ordered
	overflow := 0
	if len(shown) > prReviewFrameMaxComments {
		shown = shown[:prReviewFrameMaxComments]
		overflow = len(ordered) - prReviewFrameMaxComments
	}
	lines := make([]string, 0, len(shown)+1)
	for _, thread := range shown {
		lines = append(lines, prReviewCommentLine(thread, innerWidth))
	}
	if overflow > 0 {
		lines = append(lines, hintStyle.Render(fmt.Sprintf("  +%d more", overflow)))
	}
	return lines
}

func prReviewCommentLine(thread github.ReviewThread, innerWidth int) string {
	glyphStyle, glyph := inspectorWarnStyle, "○"
	if thread.Resolved {
		glyphStyle, glyph = inspectorCleanStyle, "✓"
	}
	location := commentLocation(thread)
	locationWidth := lipgloss.Width(location)
	if location != "" {
		location = inspectorCommitStyle.Render(location) + " "
		locationWidth++
	}
	bodyBudget := innerWidth - 4 - locationWidth
	if bodyBudget < 1 {
		bodyBudget = 1
	}
	body := hintStyle.Render(truncatePlain(thread.Body, bodyBudget))
	preview := location + body
	return "  " + glyphStyle.Render(glyph) + " " + osc8(thread.URL, preview)
}

// commentLocation is the compact "file:line" tag for an inline comment, using the
// file's base name to stay short. Returns "" when there is no path.
func commentLocation(thread github.ReviewThread) string {
	if thread.Path == "" {
		return ""
	}
	name := thread.Path
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	if thread.Line > 0 {
		return fmt.Sprintf("%s:%d", name, thread.Line)
	}
	return name
}
