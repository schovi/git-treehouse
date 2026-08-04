package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"strings"
)

func helpDialogWidth(viewWidth int) int {
	return modalWidth(viewWidth, 68)
}

func deleteDialogWidth(viewWidth int) int {
	return modalWidth(viewWidth, 76)
}

func checkoutDialogWidth(viewWidth int) int {
	return modalWidth(viewWidth, 76)
}

func paletteDialogWidth(viewWidth int) int {
	return modalWidth(viewWidth, 72)
}

func filterDialogWidth(viewWidth int) int {
	return modalWidth(viewWidth, 60)
}

func pullRequestDialogWidth(viewWidth int) int {
	return modalWidth(viewWidth, 76)
}

func modalWidth(viewWidth, maximum int) int {
	if viewWidth <= 0 {
		return maximum
	}
	inset := 8
	if viewWidth < 48 {
		inset = 2
	}
	return max(4, min(maximum, viewWidth-inset))
}

func createDialogWidth(viewWidth int) int {
	if viewWidth <= 0 {
		return 72
	}
	inset := 8
	if viewWidth < 48 {
		inset = 2
	}
	return max(4, min(72, viewWidth-inset))
}

func dialogBox(title string, bodyLines []string, bottomContent string, width int) string {
	width = max(4, width)
	contentWidth := max(1, width-4)
	lines := make([]string, 0, len(bodyLines)+2)
	lines = append(lines, dialogTopLine(title, width))
	for _, line := range bodyLines {
		lines = append(lines, appBorderStyle.Render("│ ")+padStyled(line, contentWidth)+appBorderStyle.Render(" │"))
	}
	lines = append(lines, dialogBottomLine(bottomContent, width))
	return strings.Join(lines, "\n")
}

func dialogTopLine(title string, width int) string {
	innerWidth := width - 2
	label := ""
	if title != "" {
		label = " " + title + " "
		label = truncatePlain(label, max(0, innerWidth-1))
	}
	labelWidth := runewidth.StringWidth(label)
	ruleWidth := max(0, innerWidth-1-labelWidth)
	return appBorderStyle.Render("╭─") + panelTitleStyle.Render(label) + appBorderStyle.Render(strings.Repeat("─", ruleWidth)+"╮")
}

func dialogBottomLine(content string, width int) string {
	return bottomBorderLine(width, appBorderStyle, borderControls{text: content}, borderControls{})
}

func createDialogHintsAtWidth(width int) string {
	full := colorKeyHints("Enter create · Tab switch base · ctrl+o config · Esc cancel", false)
	if lipgloss.Width(full) <= width {
		return full
	}
	medium := colorKeyHints("Enter create · Tab base · ctrl+o config · Esc cancel", false)
	if lipgloss.Width(medium) <= width {
		return medium
	}
	short := colorKeyHints("Enter · Tab · ctrl+o · Esc", false)
	if lipgloss.Width(short) <= width {
		return short
	}
	return ""
}

func filterDialogHintsAtWidth(width int) string {
	full := colorKeyHints("Enter apply · ↑/↓ move · Tab next · Esc cancel", false)
	if lipgloss.Width(full) <= width {
		return full
	}
	medium := colorKeyHints("Enter apply · Tab next · Esc", false)
	if lipgloss.Width(medium) <= width {
		return medium
	}
	short := colorKeyHints("Enter · Tab · Esc", false)
	if lipgloss.Width(short) <= width {
		return short
	}
	return ""
}

func pullRequestCheckoutHintsAtWidth(width int, loading bool) string {
	content := "Enter checkout · o open · ↑/↓ move · Esc cancel"
	if loading {
		content = "↑/↓ move · Esc cancel"
	}
	full := colorKeyHints(content, false)
	if lipgloss.Width(full) <= width {
		return full
	}
	short := "Enter · o · ↑/↓ · Esc"
	if loading {
		short = "↑/↓ · Esc"
	}
	short = colorKeyHints(short, false)
	if lipgloss.Width(short) <= width {
		return short
	}
	return ""
}

func centeredOverlay(base, popup string, width, height int) string {
	lines := strings.Split(base, "\n")
	if height <= 0 {
		height = len(lines)
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	popupLines := strings.Split(popup, "\n")
	popupWidth := 0
	for _, line := range popupLines {
		popupWidth = max(popupWidth, lipgloss.Width(line))
	}
	top := max(0, (height-len(popupLines))/2)
	left := max(0, (width-popupWidth)/2)
	haloTop := top
	haloBottom := min(height, top+len(popupLines))
	haloLeft := max(0, left-1)
	haloRight := min(width, left+popupWidth+1)
	for index := haloTop; index < haloBottom; index++ {
		baseLine := padStyled(lines[index], width)
		leftText := ansi.Cut(baseLine, 0, haloLeft)
		rightText := ansi.Cut(baseLine, haloRight, width)
		lines[index] = padStyled(leftText, haloLeft) + strings.Repeat(" ", max(0, haloRight-haloLeft)) + padStyled(rightText, max(0, width-haloRight))
	}
	for index, line := range popupLines {
		target := top + index
		if target >= len(lines) {
			break
		}
		baseLine := padStyled(lines[target], width)
		leftText := ansi.Cut(baseLine, 0, left)
		rightStart := min(width, left+popupWidth)
		rightText := ansi.Cut(baseLine, rightStart, width)
		lines[target] = padStyled(leftText, left) + padStyled(line, popupWidth) + padStyled(rightText, max(0, width-rightStart))
	}
	return strings.Join(lines, "\n")
}
