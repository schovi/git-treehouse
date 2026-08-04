package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/schovi/git-treehouse/internal/listview"
	"strings"
)

type helpEntryKind int

const (
	helpEntryKey helpEntryKind = iota
	helpEntryRoot
	helpEntryWorktree
	helpEntryBranch
	helpEntryActive
	helpEntryLocked
	helpEntryPrunable
	helpEntryClean
	helpEntryStaged
	helpEntryModified
	helpEntryUntracked
	helpEntryRemote
	helpEntryPullRequest
	helpEntryApproved
	helpEntryMerged
	helpEntryClosed
	helpEntryRunning
	helpEntryError
)

type helpEntry struct {
	lead        string
	description string
	kind        helpEntryKind
}

type helpSection struct {
	title   string
	entries []helpEntry
}

func (model Model) renderHelpAtWidth(width int) string {
	contentWidth := max(1, width-4)
	lines := helpLinesAtWidth(contentWidth)
	return dialogBox("Help", lines, colorKeyHints("Esc close", false), width)
}

func helpLinesAtWidth(contentWidth int) []string {
	columns := helpColumnsForWidth(contentWidth)
	if len(columns) == 1 {
		return renderHelpSections(columns[0], contentWidth)
	}
	gap := "  "
	gapWidth := runewidth.StringWidth(gap) * (len(columns) - 1)
	columnWidth := max(1, (contentWidth-gapWidth)/len(columns))
	renderedColumns := make([][]string, 0, len(columns))
	height := 0
	for _, column := range columns {
		rendered := renderHelpSections(column, columnWidth)
		renderedColumns = append(renderedColumns, rendered)
		height = max(height, len(rendered))
	}
	lines := make([]string, 0, height)
	for row := 0; row < height; row++ {
		parts := make([]string, 0, len(renderedColumns))
		for _, column := range renderedColumns {
			line := ""
			if row < len(column) {
				line = column[row]
			}
			parts = append(parts, padStyled(line, columnWidth))
		}
		lines = append(lines, strings.Join(parts, gap))
	}
	return lines
}

func helpColumnsForWidth(contentWidth int) [][]helpSection {
	keySections := helpKeySections()
	legendSections := helpLegendSections()
	if contentWidth >= 62 {
		return [][]helpSection{
			{keySections[0], legendSections[0]},
			{keySections[1], legendSections[1]},
			{keySections[2], legendSections[2]},
		}
	}
	if contentWidth >= 42 {
		return [][]helpSection{
			{keySections[0], keySections[1], keySections[2]},
			{legendSections[0], legendSections[1], legendSections[2]},
		}
	}
	sections := make([]helpSection, 0, len(keySections)+len(legendSections))
	sections = append(sections, keySections...)
	sections = append(sections, legendSections...)
	return [][]helpSection{sections}
}

func helpKeySections() []helpSection {
	return []helpSection{
		{
			title: "Global",
			entries: []helpEntry{
				{lead: "r", description: "refresh", kind: helpEntryKey},
				{lead: "ctrl+p", description: "commands", kind: helpEntryKey},
				{lead: "?", description: "help", kind: helpEntryKey},
				{lead: "Esc", description: "close/cancel", kind: helpEntryKey},
				{lead: "q", description: "quit", kind: helpEntryKey},
			},
		},
		{
			title: "Worktree List",
			entries: []helpEntry{
				{lead: "↑/↓ k/j", description: "move", kind: helpEntryKey},
				{lead: "g/G", description: "top/bottom", kind: helpEntryKey},
				{lead: "h", description: "root", kind: helpEntryKey},
				{lead: "a", description: "active", kind: helpEntryKey},
				{lead: "n", description: "new worktree", kind: helpEntryKey},
				{lead: "Tab", description: "filter", kind: helpEntryKey},
				{lead: "s", description: "search", kind: helpEntryKey},
				{lead: "b", description: "branches", kind: helpEntryKey},
				{lead: "u", description: "restore deleted branch", kind: helpEntryKey},
			},
		},
		{
			title: "Worktree Detail",
			entries: []helpEntry{
				{lead: "Enter", description: "go/create", kind: helpEntryKey},
				{lead: "c", description: "checkout root", kind: helpEntryKey},
				{lead: "o", description: "editor", kind: helpEntryKey},
				{lead: "d", description: "delete", kind: helpEntryKey},
				{lead: "y", description: "copy", kind: helpEntryKey},
				{lead: "p", description: "PR/branch", kind: helpEntryKey},
			},
		},
	}
}

func helpLegendSections() []helpSection {
	return []helpSection{
		{
			title: "Row Icons",
			entries: []helpEntry{
				{lead: "⌂", description: "root", kind: helpEntryRoot},
				{lead: "⊡", description: "worktree", kind: helpEntryWorktree},
				{lead: "⎇", description: "branch", kind: helpEntryBranch},
				{lead: "!", description: "locked", kind: helpEntryLocked},
				{lead: "×", description: "prunable", kind: helpEntryPrunable},
				{lead: "bold", description: "active row", kind: helpEntryActive},
			},
		},
		{
			title: "Git Status",
			entries: []helpEntry{
				{lead: "✓", description: "clean", kind: helpEntryClean},
				{lead: "+", description: "staged", kind: helpEntryStaged},
				{lead: "~", description: "modified", kind: helpEntryModified},
				{lead: "?", description: "untracked", kind: helpEntryUntracked},
				{lead: "remote ✓", description: "synced", kind: helpEntryRemote},
				{lead: "remote -", description: "none", kind: helpEntryRemote},
				{lead: "remote gone", description: "deleted", kind: helpEntryRemote},
			},
		},
		{
			title: "Pull Requests",
			entries: []helpEntry{
				{lead: "◌", description: "draft", kind: helpEntryPullRequest},
				{lead: "○", description: "ready/open", kind: helpEntryPullRequest},
				{lead: "◆", description: "approved", kind: helpEntryApproved},
				{lead: "⎇", description: "merged", kind: helpEntryMerged},
				{lead: "✕", description: "closed", kind: helpEntryClosed},
				{lead: "✓", description: "CI passed", kind: helpEntryClean},
				{lead: "✗", description: "CI error", kind: helpEntryError},
				{lead: "●", description: "CI running", kind: helpEntryRunning},
			},
		},
	}
}

func renderHelpSections(sections []helpSection, width int) []string {
	lines := make([]string, 0)
	for sectionIndex, section := range sections {
		if sectionIndex > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, truncateStyled(helpCategoryStyle.Render(section.title), width))
		for _, entry := range section.entries {
			lines = append(lines, truncateStyled(renderHelpEntry(entry), width))
		}
	}
	return lines
}

func renderHelpEntry(entry helpEntry) string {
	return helpEntryStyle(entry.kind).Render(entry.lead) + hintStyle.Render(" "+entry.description)
}

func helpEntryStyle(kind helpEntryKind) lipgloss.Style {
	switch kind {
	case helpEntryRoot:
		return listview.RootTypeIconStyle()
	case helpEntryWorktree:
		return listview.WorktreeTypeIconStyle()
	case helpEntryBranch:
		return listview.BranchTypeIconStyle()
	case helpEntryPullRequest:
		return inspectorCommitStyle
	case helpEntryMerged:
		return mergedGlyphStyle
	case helpEntryActive:
		return inspectorValueStyle.Bold(true)
	case helpEntryLocked, helpEntryPrunable, helpEntryModified, helpEntryClosed, helpEntryRunning, helpEntryError:
		return inspectorWarnStyle
	case helpEntryClean, helpEntryStaged, helpEntryApproved:
		return inspectorCleanStyle
	case helpEntryUntracked:
		return inspectorCommitStyle
	case helpEntryRemote:
		return keyStyle
	default:
		return keyStyle
	}
}
