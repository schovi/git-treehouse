package listview

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/schovi/git-treehouse/internal/gitdata"
)

const LoadingPlaceholder = "⋯"

type Options struct {
	Width             int
	Color             bool
	Hyperlinks        bool
	ShowHeader        bool
	ShowPR            bool
	Pending           string
	PRPending         bool
	HighlightSelected bool
	SelectedIndex     int
}

type column struct {
	key     string
	title   string
	width   int
	elastic bool
	align   string
}

func Render(state gitdata.State, options Options, now time.Time) string {
	return RenderRows(state.Rows, options, now)
}

func RenderRows(rows []gitdata.Worktree, options Options, now time.Time) string {
	return RenderMixedRows(gitdata.RowsFromWorktrees(rows), options, now)
}

func RenderMixedRows(rows []gitdata.Row, options Options, now time.Time) string {
	width := options.Width
	if width <= 0 {
		width = 100
	}
	gap := columnGap(options)
	columns := chooseColumns(width, options.ShowPR, gap)
	lines := make([]string, 0, len(rows)+1)
	if options.ShowHeader {
		header := renderHeader(columns, options)
		lines = append(lines, header)
	}
	for index, row := range rows {
		lines = append(lines, renderRow(row, columns, options, now, index))
	}
	return strings.Join(lines, "\n")
}

func chooseColumns(width int, showPR bool, gap int) []column {
	columns := []column{
		{key: "branch", title: "  name", width: 20, elastic: true},
		{key: "status", title: "status", width: 8},
	}
	if width < 40 {
		statusWidth := min(10, max(6, width/3))
		columns[0].width = max(6, width-gap-statusWidth)
		columns[1].width = statusWidth
		return columns
	}
	optional := []column{
		{key: "remote", title: "remote", width: 7},
		{key: "main", title: "main±", width: 8},
		{key: "commit", title: "commit", width: 28, elastic: true},
	}
	if width >= 90 {
		optional = append(optional, column{key: "age", title: "age", width: 5})
	}
	if showPR && ShowsPullRequestColumn(width) {
		optional = append(optional, column{key: "pr", title: "PR", width: 13})
	}
	if ShowsGitSizeColumn(width) {
		optional = append(optional, column{key: "size", title: "size", width: 8, align: "right"})
	}
	columns = append(columns, optional...)
	totalFixed := gap * (len(columns) - 1)
	elasticCount := 0
	for _, column := range columns {
		totalFixed += column.width
		if column.elastic {
			elasticCount++
		}
	}
	extra := width - totalFixed
	if extra > 0 && elasticCount > 0 {
		for index := range columns {
			if columns[index].elastic {
				addition := extra / elasticCount
				columns[index].width += addition
				extra -= addition
				elasticCount--
			}
		}
	}
	if extra < 0 {
		for index := range columns {
			if columns[index].key == "commit" {
				reduction := min(columns[index].width-12, -extra)
				columns[index].width -= reduction
				extra += reduction
			}
		}
	}
	return columns
}

func ShowsPullRequestColumn(width int) bool {
	return width >= 128
}

func ShowsGitSizeColumn(width int) bool {
	return width >= 144
}

func renderHeader(columns []column, options Options) string {
	cells := make([]string, 0, len(columns))
	for _, column := range columns {
		cell := pad(column.title, column.width, column.align)
		if options.Color {
			cell = headerStyle.Render(cell)
		}
		cells = append(cells, cell)
	}
	return strings.Join(cells, strings.Repeat(" ", columnGap(options)))
}

func renderRow(row gitdata.Row, columns []column, options Options, now time.Time, rowIndex int) string {
	cells := make([]string, 0, len(columns))
	selected := options.HighlightSelected && rowIndex == options.SelectedIndex
	for _, column := range columns {
		value := cellValue(row, column.key, now, options)
		cell := padCell(row, column, value)
		if options.Color {
			cell = colorCell(row, column.key, value, cell, selected)
		}
		cells = append(cells, cell)
	}
	joiner := strings.Repeat(" ", columnGap(options))
	if options.Color && selected {
		joiner = selectedSegment(joiner)
	}
	line := strings.Join(cells, joiner)
	if options.Color && selected {
		return padSelectedRow(line, options.Width)
	}
	if options.Color {
		return inactiveRowStyle.Render(line)
	}
	return line
}

func cellValue(row gitdata.Row, key string, now time.Time, options Options) string {
	switch key {
	case "branch":
		value := row.TypeIcon() + " " + row.ListBranch()
		if state := row.StateIcon(); state != "" {
			value += " " + state
		}
		return value
	case "status":
		if !row.LocalMetadataLoaded() && options.Pending != "" {
			return options.Pending
		}
		return row.StatusText()
	case "remote":
		if !row.LocalMetadataLoaded() && options.Pending != "" {
			return options.Pending
		}
		return row.HeadSync().RemoteCompact(row.UpstreamGone())
	case "main":
		if !row.LocalMetadataLoaded() && options.Pending != "" {
			return options.Pending
		}
		return row.MainSync().Compact()
	case "commit":
		if !row.LocalMetadataLoaded() && options.Pending != "" {
			return options.Pending
		}
		if row.CommitShort() == "" {
			return ""
		}
		return row.CommitShort() + " " + row.CommitSubject()
	case "age":
		if !row.LocalMetadataLoaded() && options.Pending != "" {
			return options.Pending
		}
		return gitdata.RelativeAge(now, row.CommitTime())
	case "pr":
		pr := row.PullRequest()
		if pr == nil {
			if options.PRPending && options.Pending != "" {
				return options.Pending
			}
			return ""
		}
		text := pr.Text()
		if options.Hyperlinks && pr.URL != "" {
			return osc8(pr.URL, text)
		}
		return text
	case "size":
		size, loaded := row.TableSize()
		if !loaded {
			if options.Pending != "" {
				return options.Pending
			}
			return LoadingPlaceholder
		}
		return formatSize(size)
	default:
		return ""
	}
}

var (
	headerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	inactiveRowStyle = lipgloss.NewStyle()
	selectedRowStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("62"))
	branchStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	branchOnlyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	inactiveMarkerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	mainMarkerStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true)
	worktreeMarkerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("108"))
	branchMarkerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("103"))
	cleanStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	dirtyStagedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	dirtyModifiedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	dirtyUnknownStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	detachedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	warningStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	lockedStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("141"))
	commitHashStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	commitSubjectStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	mutedStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	prStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	sizeStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("108"))
)

func colorCell(row gitdata.Row, key, raw, value string, selected bool) string {
	switch key {
	case "branch":
		return colorBranchCell(row, raw, value, selected)
	case "status":
		return colorStatusCell(row, raw, value, selected)
	case "remote":
		return colorRemoteCell(row, raw, value, selected)
	case "main":
		return colorSyncCell(row, row.MainSync(), raw, value, selected)
	case "commit":
		return colorCommitCell(row, raw, value, selected)
	case "age":
		return mutedCell(row, raw, value, selected)
	case "pr":
		return colorPullRequestCell(row, raw, value, selected)
	case "size":
		return colorSizeCell(row, raw, value, selected)
	}
	return value
}

func colorBranchCell(row gitdata.Row, raw, padded string, selected bool) string {
	content, padding := splitPadding(raw, padded)
	if content == "" {
		return selectedSegmentWhen(padding, selected)
	}
	marker, rest := splitFirstRune(content)
	if marker == " " {
		return selectedSegmentWhen(marker, selected) + colorBranchText(row, rest, selected) + selectedSegmentWhen(padding, selected)
	}
	return styleForRow(row, typeIconStyle(row), selected).Render(marker) + colorBranchText(row, rest, selected) + selectedSegmentWhen(padding, selected)
}

func typeIconStyle(row gitdata.Row) lipgloss.Style {
	if row.IsBranch() {
		return BranchTypeIconStyle()
	}
	if row.Worktree.IsMain {
		return RootTypeIconStyle()
	}
	return WorktreeTypeIconStyle()
}

func RootTypeIconStyle() lipgloss.Style {
	return mainMarkerStyle
}

func WorktreeTypeIconStyle() lipgloss.Style {
	return worktreeMarkerStyle
}

func BranchTypeIconStyle() lipgloss.Style {
	return branchMarkerStyle
}

func stateIconStyle(row gitdata.Row) lipgloss.Style {
	if row.IsBranch() {
		return inactiveMarkerStyle
	}
	switch {
	case row.Worktree.Prunable:
		return warningStyle
	case row.Worktree.Locked:
		return lockedStyle
	default:
		return inactiveMarkerStyle
	}
}

func colorBranchText(row gitdata.Row, value string, selected bool) string {
	if value == "" {
		return value
	}
	prefix := ""
	if strings.HasPrefix(value, " ") {
		prefix = " "
		value = strings.TrimPrefix(value, " ")
	}
	lifecycleSuffix := ""
	if state := row.StateIcon(); state != "" && strings.HasSuffix(value, " "+state) {
		value = strings.TrimSuffix(value, " "+state)
		lifecycleSuffix = " " + state
	}
	renderLifecycleSuffix := func() string {
		if lifecycleSuffix == "" {
			return ""
		}
		return styleForRow(row, stateIconStyle(row), selected).Render(lifecycleSuffix)
	}
	if row.IsBranch() {
		return selectedSegmentWhen(prefix, selected) +
			styleForRow(row, branchOnlyStyle, selected).Render(value) +
			renderLifecycleSuffix()
	}
	if row.Worktree.Detached {
		head, state, found := strings.Cut(value, " ")
		if found {
			return selectedSegmentWhen(prefix, selected) +
				styleForRow(row, commitHashStyle, selected).Render(head) +
				styleForRow(row, detachedStyle, selected).Render(" "+state) +
				renderLifecycleSuffix()
		}
		return selectedSegmentWhen(prefix, selected) +
			styleForRow(row, detachedStyle, selected).Render(value) +
			renderLifecycleSuffix()
	}
	return selectedSegmentWhen(prefix, selected) +
		styleForRow(row, branchStyleFor(row), selected).Render(value) +
		renderLifecycleSuffix()
}

func branchStyleFor(row gitdata.Row) lipgloss.Style {
	if row.IsWorktree() && row.Worktree.Detached {
		return detachedStyle
	}
	return branchStyle
}

func colorStatusCell(row gitdata.Row, raw, padded string, selected bool) string {
	padding := strings.TrimPrefix(padded, raw)
	if padding == padded {
		padding = ""
	}
	switch {
	case row.IsBranch() || raw == "-":
		return styleForRow(row, mutedStyle, selected).Render(raw) + selectedSegmentWhen(padding, selected)
	case row.Worktree.Status.Clean():
		return styleForRow(row, cleanStyle, selected).Render(raw) + selectedSegmentWhen(padding, selected)
	default:
		return colorDirtyTokens(row, raw, selected) + selectedSegmentWhen(padding, selected)
	}
}

func colorRemoteCell(row gitdata.Row, raw, padded string, selected bool) string {
	content, padding := splitPadding(raw, padded)
	if content == "" {
		return selectedSegmentWhen(padding, selected)
	}
	if row.UpstreamGone() || content == "gone" {
		return styleForRow(row, warningStyle, selected).Render(content) + selectedSegmentWhen(padding, selected)
	}
	return colorSyncCell(row, row.HeadSync(), raw, padded, selected)
}

func colorDirtyTokens(row gitdata.Row, value string, selected bool) string {
	parts := strings.Split(value, " ")
	for index, part := range parts {
		switch {
		case strings.HasPrefix(part, "+"):
			parts[index] = styleForRow(row, dirtyStagedStyle, selected).Render(part)
		case strings.HasPrefix(part, "~"):
			parts[index] = styleForRow(row, dirtyModifiedStyle, selected).Render(part)
		case strings.HasPrefix(part, "?"):
			parts[index] = styleForRow(row, dirtyUnknownStyle, selected).Render(part)
		}
	}
	return strings.Join(parts, selectedSegmentWhen(" ", selected))
}

func colorSyncCell(row gitdata.Row, sync gitdata.SyncState, raw, padded string, selected bool) string {
	content, padding := splitPadding(raw, padded)
	if content == "" {
		return selectedSegmentWhen(padding, selected)
	}
	if content == "-" || sync.NoUpstream || !sync.Available {
		return styleForRow(row, mutedStyle, selected).Render(content) + selectedSegmentWhen(padding, selected)
	}
	parts := strings.Split(content, " ")
	for index, part := range parts {
		switch {
		case strings.HasPrefix(part, "↑"):
			parts[index] = styleForRow(row, dirtyModifiedStyle, selected).Render(part)
		case strings.HasPrefix(part, "↓"):
			parts[index] = styleForRow(row, warningStyle, selected).Render(part)
		default:
			parts[index] = styleForRow(row, cleanStyle, selected).Render(part)
		}
	}
	return strings.Join(parts, selectedSegmentWhen(" ", selected)) + selectedSegmentWhen(padding, selected)
}

func colorCommitCell(row gitdata.Row, raw, padded string, selected bool) string {
	content, padding := splitPadding(raw, padded)
	if content == "" {
		return selectedSegmentWhen(padding, selected)
	}
	commitShort := row.CommitShort()
	if commitShort != "" && strings.HasPrefix(content, commitShort) {
		rest := strings.TrimPrefix(content, commitShort)
		return styleForRow(row, commitHashStyle, selected).Render(commitShort) +
			styleForRow(row, commitSubjectStyle, selected).Render(rest) +
			selectedSegmentWhen(padding, selected)
	}
	return styleForRow(row, commitSubjectStyle, selected).Render(content) + selectedSegmentWhen(padding, selected)
}

func colorPullRequestCell(row gitdata.Row, raw, padded string, selected bool) string {
	content, padding := splitPadding(raw, padded)
	if content == "" {
		return selectedSegmentWhen(padding, selected)
	}
	if strings.Contains(content, "\x1b]8;;") {
		return styleForRow(row, prStyle, selected).Render(content) + selectedSegmentWhen(padding, selected)
	}
	parts := strings.Split(content, " ")
	for index, part := range parts {
		switch {
		case strings.HasPrefix(part, "#"):
			parts[index] = styleForRow(row, prStyle, selected).Render(part)
		case part == "✓" || part == "◆":
			parts[index] = styleForRow(row, cleanStyle, selected).Render(part)
		case part == "×" || part == "✗" || part == "✕" || part == "●":
			parts[index] = styleForRow(row, warningStyle, selected).Render(part)
		default:
			parts[index] = styleForRow(row, mutedStyle, selected).Render(part)
		}
	}
	return strings.Join(parts, selectedSegmentWhen(" ", selected)) + selectedSegmentWhen(padding, selected)
}

func colorSizeCell(row gitdata.Row, raw, padded string, selected bool) string {
	content, padding := splitPadding(raw, padded)
	if content == "" {
		return selectedSegmentWhen(padding, selected)
	}
	if content == LoadingPlaceholder || content == "…" || content == "-" {
		return styleForRow(row, mutedStyle, selected).Render(content) + selectedSegmentWhen(padding, selected)
	}
	return styleForRow(row, sizeStyle, selected).Render(content) + selectedSegmentWhen(padding, selected)
}

func mutedCell(row gitdata.Row, raw, padded string, selected bool) string {
	content, padding := splitPadding(raw, padded)
	if content == "" {
		return selectedSegmentWhen(padding, selected)
	}
	return styleForRow(row, mutedStyle, selected).Render(content) + selectedSegmentWhen(padding, selected)
}

func splitPadding(raw, padded string) (string, string) {
	if strings.HasPrefix(padded, raw) {
		return raw, strings.TrimPrefix(padded, raw)
	}
	content := strings.TrimRight(padded, " ")
	return content, strings.TrimPrefix(padded, content)
}

func splitFirstRune(value string) (string, string) {
	if value == "" {
		return "", ""
	}
	_, size := utf8.DecodeRuneInString(value)
	return value[:size], value[size:]
}

func columnGap(options Options) int {
	if options.Width < 40 {
		return 1
	}
	return 2
}

func pad(value string, width int, align string) string {
	value = truncate(value, width)
	visible := runewidth.StringWidth(stripOSC8(value))
	if visible >= width {
		return value
	}
	padding := strings.Repeat(" ", width-visible)
	if align == "right" {
		return padding + value
	}
	return value + padding
}

func padCell(row gitdata.Row, column column, value string) string {
	if column.key == "branch" {
		return padBranchName(row, value, column.width)
	}
	return pad(value, column.width, column.align)
}

func padBranchName(row gitdata.Row, value string, width int) string {
	state := row.StateIcon()
	if state == "" {
		return pad(value, width, "")
	}
	suffix := " " + state
	name := strings.TrimSuffix(value, suffix)
	if name == value {
		return pad(value, width, "")
	}
	suffixWidth := runewidth.StringWidth(suffix)
	if width <= suffixWidth+1 {
		return pad(value, width, "")
	}
	value = truncate(name, width-suffixWidth) + suffix
	return pad(value, width, "")
}

func padSelectedRow(value string, width int) string {
	if width <= 0 {
		return value
	}
	visible := lipgloss.Width(value)
	if visible >= width {
		return value
	}
	return value + selectedSegment(strings.Repeat(" ", width-visible))
}

func styleForRow(row gitdata.Row, style lipgloss.Style, selected bool) lipgloss.Style {
	if row.IsWorktree() && row.Worktree.IsActive {
		style = style.Bold(true)
	}
	if selected {
		return style.Background(lipgloss.Color("62"))
	}
	return style
}

func selectedSegment(value string) string {
	if value == "" {
		return value
	}
	return selectedRowStyle.Render(value)
}

func selectedSegmentWhen(value string, selected bool) string {
	if !selected {
		return value
	}
	return selectedSegment(value)
}

func truncate(value string, width int) string {
	if runewidth.StringWidth(stripOSC8(value)) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	var builder strings.Builder
	used := 0
	for _, character := range value {
		characterWidth := runewidth.RuneWidth(character)
		if used+characterWidth > width-1 {
			break
		}
		builder.WriteRune(character)
		used += characterWidth
	}
	builder.WriteString("…")
	return builder.String()
}

func formatSize(bytes int64) string {
	units := []string{"B", "K", "M", "G", "T"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value = value / 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d%s", bytes, units[unit])
	}
	if value >= 10 {
		return fmt.Sprintf("%.0f%s", value, units[unit])
	}
	return fmt.Sprintf("%.1f%s", value, units[unit])
}

func osc8(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

func stripOSC8(value string) string {
	for {
		start := strings.Index(value, "\x1b]8;;")
		if start < 0 {
			return value
		}
		end := strings.Index(value[start:], "\x1b\\")
		if end < 0 {
			return value[:start]
		}
		value = value[:start] + value[start+end+2:]
	}
}
