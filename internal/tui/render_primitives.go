package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"strings"
)

var (
	appBorderStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("65"))
	panelBorderStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	panelTitleStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true)
	titleNameStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true)
	titleRepoStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	titleSeparatorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	titleMetaStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	flashStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("58"))
	paletteSelectedStyle  = lipgloss.NewStyle().Background(lipgloss.Color("62"))
	deleteDangerStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	deleteCommandStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	inspectorLabelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("67"))
	inspectorValueStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	inspectorCleanStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	inspectorWarnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	inspectorCommitStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	inspectorSubjectStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	mergedGlyphStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("99")) // purple, like GitHub's merged badge
	branchOnlyDetailStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	keyStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	hintStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	scrollbarArrowStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	scrollbarThumbStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	scrollbarTrackStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpCategoryStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	statusMessageStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	refreshActivityStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	refreshSuccessStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

func (model Model) wrapOuter(content string, width int) string {
	if width < 4 {
		return content
	}
	innerWidth := width - 4
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		lines[index] = appBorderStyle.Render("│ ") + padStyled(line, innerWidth) + appBorderStyle.Render(" │")
	}
	return strings.Join(lines, "\n")
}

func sectionBoxWithFooter(title string, bodyLines []string, footer string, width int) string {
	return sectionBoxWithFooterTopRight(title, bodyLines, footer, "", width)
}

func sectionBoxWithFooterTopRight(title string, bodyLines []string, footer, topRight string, width int) string {
	return sectionBoxWithSplitFooterTopRight(title, bodyLines, footer, "", topRight, width)
}

func sectionBoxWithSplitFooterTopRight(title string, bodyLines []string, leftFooter, rightFooter, topRight string, width int) string {
	if width < 4 {
		return strings.Join(bodyLines, "\n")
	}
	innerWidth := width - 2
	lines := make([]string, 0, len(bodyLines)+2)
	lines = append(lines, sectionTopLineWithRight(title, topRight, width))
	for _, line := range bodyLines {
		lines = append(lines, panelBorderStyle.Render("│")+padStyled(line, innerWidth)+panelBorderStyle.Render("│"))
	}
	lines = append(lines, sectionBottomLineSplit(leftFooter, rightFooter, width))
	return strings.Join(lines, "\n")
}

func sectionTopLine(title string, width int) string {
	return sectionTopLineWithRight(title, "", width)
}

func sectionTopLineWithRight(title, right string, width int) string {
	innerWidth := width - 2
	label := ""
	if title != "" {
		labelWidth := max(0, innerWidth-3)
		label = " " + renderSectionTitle(title, labelWidth) + " "
	}
	labelWidth := lipgloss.Width(label)
	rightLabel := ""
	if right != "" {
		candidate := " " + right + " "
		if lipgloss.Width(candidate) <= width-labelWidth-5 {
			rightLabel = candidate
		}
	}
	if rightLabel != "" {
		ruleWidth := width - 4 - labelWidth - lipgloss.Width(rightLabel)
		if ruleWidth < 1 {
			ruleWidth = 1
		}
		return panelBorderStyle.Render("╭─") + panelTitleStyle.Render(label) + panelBorderStyle.Render(strings.Repeat("─", ruleWidth)) + rightLabel + panelBorderStyle.Render("─╮")
	}
	ruleWidth := innerWidth - 1 - labelWidth
	if ruleWidth < 0 {
		ruleWidth = 0
	}
	return panelBorderStyle.Render("╭─") + panelTitleStyle.Render(label) + panelBorderStyle.Render(strings.Repeat("─", ruleWidth)+"╮")
}

func sectionBottomLine(footer string, width int) string {
	return sectionBottomLineSplit(footer, "", width)
}

func sectionBottomLineSplit(leftFooter, rightFooter string, width int) string {
	return bottomBorderLine(width, panelBorderStyle, borderControls{parts: hintParts(leftFooter)}, borderControls{parts: hintParts(rightFooter)})
}

type borderControls struct {
	parts     []string
	text      string
	hasStatus bool
}

func bottomBorderLine(width int, style lipgloss.Style, left, right borderControls) string {
	if width <= 0 {
		return ""
	}
	if width < 4 {
		return style.Render(strings.Repeat("─", width))
	}
	contentLimit := max(0, width-6)
	leftText := renderBorderControls(left, contentLimit)
	rightText := renderBorderControls(right, contentLimit)
	if leftText == "" && rightText == "" {
		return style.Render("╰" + strings.Repeat("─", max(0, width-2)) + "╯")
	}
	if leftText != "" && rightText != "" {
		ruleWidth := width - lipgloss.Width(leftText) - lipgloss.Width(rightText) - 8
		if ruleWidth >= 1 {
			return style.Render("╰─ ") + leftText + style.Render(" "+strings.Repeat("─", ruleWidth)+" ") + rightText + style.Render(" ─╯")
		}
		rightText = ""
	}
	if leftText != "" {
		ruleWidth := max(1, width-lipgloss.Width(leftText)-5)
		return style.Render("╰─ ") + leftText + style.Render(" "+strings.Repeat("─", ruleWidth)+"╯")
	}
	ruleWidth := max(1, width-lipgloss.Width(rightText)-5)
	return style.Render("╰"+strings.Repeat("─", ruleWidth)+" ") + rightText + style.Render(" ─╯")
}

func renderBorderControls(controls borderControls, width int) string {
	if width <= 0 {
		return ""
	}
	if len(controls.parts) > 0 {
		text := joinPartsWithin(controls.parts, width)
		if text == "" {
			return ""
		}
		return colorKeyHints(text, controls.hasStatus)
	}
	if lipgloss.Width(controls.text) == 0 {
		return ""
	}
	return truncateStyled(controls.text, width)
}

func hintParts(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, " · ")
}

func padRight(value string, width int) string {
	visible := runewidth.StringWidth(value)
	if visible >= width {
		return value
	}
	return value + strings.Repeat(" ", width-visible)
}

func padStyled(value string, width int) string {
	visible := lipgloss.Width(value)
	if visible >= width {
		return value
	}
	return value + strings.Repeat(" ", width-visible)
}

func truncatePlain(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(value) <= width {
		return value
	}
	if width == 1 {
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
