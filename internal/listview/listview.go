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
		{key: "branch", title: "  branch", width: 20, elastic: true},
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
	if showPR && width >= 128 {
		optional = append(optional, column{key: "pr", title: "PR", width: 13})
	}
	if width >= 144 {
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

func renderRow(row gitdata.Worktree, columns []column, options Options, now time.Time, rowIndex int) string {
	cells := make([]string, 0, len(columns))
	selected := options.HighlightSelected && rowIndex == options.SelectedIndex
	for _, column := range columns {
		value := cellValue(row, column.key, now, options)
		cell := pad(value, column.width, column.align)
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

func cellValue(row gitdata.Worktree, key string, now time.Time, options Options) string {
	switch key {
	case "branch":
		marker := row.Marker()
		if marker == "" {
			return "  " + row.DisplayBranch()
		}
		return marker + " " + row.DisplayBranch()
	case "status":
		return row.StatusText()
	case "remote":
		return row.HeadSync.RemoteCompact(row.UpstreamGone)
	case "main":
		return row.MainSync.Compact()
	case "commit":
		if row.CommitShort == "" {
			return ""
		}
		return row.CommitShort + " " + row.CommitSubject
	case "age":
		return gitdata.RelativeAge(now, row.CommitTime)
	case "pr":
		if row.PR == nil {
			if options.PRPending && options.Pending != "" {
				return options.Pending
			}
			return ""
		}
		text := row.PR.Text()
		if options.Hyperlinks && row.PR.URL != "" {
			return osc8(row.PR.URL, text)
		}
		return text
	case "size":
		if !row.SizeLoaded {
			if options.Pending != "" {
				return options.Pending
			}
			return "…"
		}
		return formatSize(row.SizeBytes)
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
	inactiveMarkerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	mainMarkerStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true)
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

func colorCell(row gitdata.Worktree, key, raw, value string, selected bool) string {
	switch key {
	case "branch":
		return colorBranchCell(row, raw, value, selected)
	case "status":
		return colorStatusCell(row, raw, value, selected)
	case "remote":
		return colorRemoteCell(row, raw, value, selected)
	case "main":
		return colorSyncCell(row.MainSync, raw, value, selected)
	case "commit":
		return colorCommitCell(row, raw, value, selected)
	case "age":
		return mutedCell(raw, value, selected)
	case "pr":
		return colorPullRequestCell(raw, value, selected)
	case "size":
		return colorSizeCell(raw, value, selected)
	}
	return value
}

func colorBranchCell(row gitdata.Worktree, raw, padded string, selected bool) string {
	content, padding := splitPadding(raw, padded)
	if content == "" {
		return selectedSegmentWhen(padding, selected)
	}
	marker, rest := splitFirstRune(content)
	if marker == " " {
		return selectedSegmentWhen(marker, selected) + colorBranchText(row, rest, selected) + selectedSegmentWhen(padding, selected)
	}
	return styleForSelected(markerStyle(row), selected).Render(marker) + colorBranchText(row, rest, selected) + selectedSegmentWhen(padding, selected)
}

func markerStyle(row gitdata.Worktree) lipgloss.Style {
	switch {
	case row.Prunable:
		return warningStyle
	case row.Locked:
		return lockedStyle
	case row.IsMain:
		return mainMarkerStyle
	default:
		return inactiveMarkerStyle
	}
}

func colorBranchText(row gitdata.Worktree, value string, selected bool) string {
	if value == "" {
		return value
	}
	prefix := ""
	if strings.HasPrefix(value, " ") {
		prefix = " "
		value = strings.TrimPrefix(value, " ")
	}
	boldStyle := func(style lipgloss.Style) lipgloss.Style {
		if row.IsActive {
			return style.Bold(true)
		}
		return style
	}
	if row.Detached {
		head, state, found := strings.Cut(value, " ")
		if found {
			return selectedSegmentWhen(prefix, selected) +
				styleForSelected(boldStyle(commitHashStyle), selected).Render(head) +
				styleForSelected(boldStyle(detachedStyle), selected).Render(" "+state)
		}
		return selectedSegmentWhen(prefix, selected) + styleForSelected(boldStyle(detachedStyle), selected).Render(value)
	}
	if row.Prunable {
		branch, _, found := strings.Cut(value, " prunable")
		if found {
			return selectedSegmentWhen(prefix, selected) +
				styleForSelected(boldStyle(mutedStyle), selected).Render(branch) +
				styleForSelected(boldStyle(warningStyle), selected).Render(" prunable")
		}
	}
	if row.Locked {
		branch, _, found := strings.Cut(value, " locked")
		if found {
			return selectedSegmentWhen(prefix, selected) +
				styleForSelected(boldStyle(branchStyle), selected).Render(branch) +
				styleForSelected(boldStyle(lockedStyle), selected).Render(" locked")
		}
	}
	return selectedSegmentWhen(prefix, selected) + styleForSelected(boldStyle(branchStyleFor(row)), selected).Render(value)
}

func branchStyleFor(row gitdata.Worktree) lipgloss.Style {
	if row.Detached {
		return detachedStyle
	}
	return branchStyle
}

func colorStatusCell(row gitdata.Worktree, raw, padded string, selected bool) string {
	padding := strings.TrimPrefix(padded, raw)
	if padding == padded {
		padding = ""
	}
	switch {
	case row.Status.Clean():
		return styleForSelected(cleanStyle, selected).Render(raw) + selectedSegmentWhen(padding, selected)
	default:
		return colorDirtyTokens(raw, selected) + selectedSegmentWhen(padding, selected)
	}
}

func colorRemoteCell(row gitdata.Worktree, raw, padded string, selected bool) string {
	content, padding := splitPadding(raw, padded)
	if content == "" {
		return selectedSegmentWhen(padding, selected)
	}
	if row.UpstreamGone || content == "gone" {
		return styleForSelected(warningStyle, selected).Render(content) + selectedSegmentWhen(padding, selected)
	}
	return colorSyncCell(row.HeadSync, raw, padded, selected)
}

func colorDirtyTokens(value string, selected bool) string {
	parts := strings.Split(value, " ")
	for index, part := range parts {
		switch {
		case strings.HasPrefix(part, "+"):
			parts[index] = styleForSelected(dirtyStagedStyle, selected).Render(part)
		case strings.HasPrefix(part, "~"):
			parts[index] = styleForSelected(dirtyModifiedStyle, selected).Render(part)
		case strings.HasPrefix(part, "?"):
			parts[index] = styleForSelected(dirtyUnknownStyle, selected).Render(part)
		}
	}
	return strings.Join(parts, selectedSegmentWhen(" ", selected))
}

func colorSyncCell(sync gitdata.SyncState, raw, padded string, selected bool) string {
	content, padding := splitPadding(raw, padded)
	if content == "" {
		return selectedSegmentWhen(padding, selected)
	}
	if content == "-" || sync.NoUpstream || !sync.Available {
		return styleForSelected(mutedStyle, selected).Render(content) + selectedSegmentWhen(padding, selected)
	}
	parts := strings.Split(content, " ")
	for index, part := range parts {
		switch {
		case strings.HasPrefix(part, "↑"):
			parts[index] = styleForSelected(dirtyModifiedStyle, selected).Render(part)
		case strings.HasPrefix(part, "↓"):
			parts[index] = styleForSelected(warningStyle, selected).Render(part)
		default:
			parts[index] = styleForSelected(cleanStyle, selected).Render(part)
		}
	}
	return strings.Join(parts, selectedSegmentWhen(" ", selected)) + selectedSegmentWhen(padding, selected)
}

func colorCommitCell(row gitdata.Worktree, raw, padded string, selected bool) string {
	content, padding := splitPadding(raw, padded)
	if content == "" {
		return selectedSegmentWhen(padding, selected)
	}
	if row.CommitShort != "" && strings.HasPrefix(content, row.CommitShort) {
		rest := strings.TrimPrefix(content, row.CommitShort)
		return styleForSelected(commitHashStyle, selected).Render(row.CommitShort) +
			styleForSelected(commitSubjectStyle, selected).Render(rest) +
			selectedSegmentWhen(padding, selected)
	}
	return styleForSelected(commitSubjectStyle, selected).Render(content) + selectedSegmentWhen(padding, selected)
}

func colorPullRequestCell(raw, padded string, selected bool) string {
	content, padding := splitPadding(raw, padded)
	if content == "" {
		return selectedSegmentWhen(padding, selected)
	}
	if strings.Contains(content, "\x1b]8;;") {
		return styleForSelected(prStyle, selected).Render(content) + selectedSegmentWhen(padding, selected)
	}
	parts := strings.Split(content, " ")
	for index, part := range parts {
		switch {
		case strings.HasPrefix(part, "#"):
			parts[index] = styleForSelected(prStyle, selected).Render(part)
		case part == "✓":
			parts[index] = styleForSelected(cleanStyle, selected).Render(part)
		case part == "×" || part == "✗":
			parts[index] = styleForSelected(warningStyle, selected).Render(part)
		default:
			parts[index] = styleForSelected(mutedStyle, selected).Render(part)
		}
	}
	return strings.Join(parts, selectedSegmentWhen(" ", selected)) + selectedSegmentWhen(padding, selected)
}

func colorSizeCell(raw, padded string, selected bool) string {
	content, padding := splitPadding(raw, padded)
	if content == "" {
		return selectedSegmentWhen(padding, selected)
	}
	if content == "…" || content == "-" {
		return styleForSelected(mutedStyle, selected).Render(content) + selectedSegmentWhen(padding, selected)
	}
	return styleForSelected(sizeStyle, selected).Render(content) + selectedSegmentWhen(padding, selected)
}

func mutedCell(raw, padded string, selected bool) string {
	content, padding := splitPadding(raw, padded)
	if content == "" {
		return selectedSegmentWhen(padding, selected)
	}
	return styleForSelected(mutedStyle, selected).Render(content) + selectedSegmentWhen(padding, selected)
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

func styleForSelected(style lipgloss.Style, selected bool) lipgloss.Style {
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
