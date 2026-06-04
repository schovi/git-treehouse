package listview

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/schovi/git-worktree-tui/internal/gitdata"
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
		cells := make([]string, 0, len(columns))
		for _, column := range columns {
			cells = append(cells, pad(column.title, column.width, column.align))
		}
		header := strings.Join(cells, strings.Repeat(" ", gap))
		if options.Color {
			header = lipgloss.NewStyle().Faint(true).Render(header)
		}
		lines = append(lines, header)
	}
	for index, row := range rows {
		lines = append(lines, renderRow(row, columns, options, now, index))
	}
	return strings.Join(lines, "\n")
}

func chooseColumns(width int, showPR bool, gap int) []column {
	columns := []column{
		{key: "marker", title: "", width: 2},
		{key: "branch", title: "branch", width: 18, elastic: true},
		{key: "status", title: "status", width: 12},
	}
	if width < 40 {
		columns[1].width = max(8, width-2-gap-10)
		columns[2].width = 10
		return columns
	}
	optional := []column{
		{key: "head", title: "head±", width: 8},
		{key: "main", title: "main±", width: 8},
		{key: "commit", title: "commit", width: 28, elastic: true},
	}
	if width >= 90 {
		optional = append(optional, column{key: "age", title: "age", width: 5})
	}
	if showPR && width >= 105 {
		optional = append(optional, column{key: "pr", title: "PR", width: 13})
	}
	if width >= 118 {
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

func renderRow(row gitdata.Worktree, columns []column, options Options, now time.Time, rowIndex int) string {
	cells := make([]string, 0, len(columns))
	selected := options.HighlightSelected && rowIndex == options.SelectedIndex
	for _, column := range columns {
		value := cellValue(row, column.key, now, options)
		cell := pad(value, column.width, column.align)
		if options.Color && !selected {
			cell = colorCell(row, column.key, cell)
		}
		cells = append(cells, cell)
	}
	line := strings.Join(cells, strings.Repeat(" ", columnGap(options)))
	if options.Color && selected {
		return selectedCellStyle().Render(line)
	}
	return line
}

func cellValue(row gitdata.Worktree, key string, now time.Time, options Options) string {
	switch key {
	case "marker":
		return row.Marker()
	case "branch":
		return row.DisplayBranch()
	case "status":
		return row.StatusText()
	case "head":
		return row.HeadSync.Compact()
	case "main":
		if row.IsMain {
			return ""
		}
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

func colorCell(row gitdata.Worktree, key, value string) string {
	if key == "marker" && row.IsActive {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render(value)
	}
	if key == "status" {
		if row.Prunable || row.UpstreamGone {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(value)
		}
		if !row.Status.Clean() {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render(value)
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render(value)
	}
	return value
}

func selectedCellStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Background(lipgloss.Color("220")).
		Foreground(lipgloss.Color("0"))
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
