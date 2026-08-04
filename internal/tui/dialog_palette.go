package tui

import (
	"context"
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/github"
	"strings"
)

type paletteDialog struct {
	input    textinput.Model
	selected int
}

type filterDialog struct {
	selected int
}

type filterOption struct {
	filter  worktreeFilter
	count   int
	enabled bool
}

type paletteCommandID string

const (
	paletteGoSelected          paletteCommandID = "go-selected"
	paletteCreate              paletteCommandID = "create"
	paletteDelete              paletteCommandID = "delete"
	paletteOpenEditor          paletteCommandID = "open-editor"
	paletteOpenPullRequest     paletteCommandID = "open-pull-request"
	paletteCheckoutPullRequest paletteCommandID = "checkout-pull-request"
	paletteCleanUpMerged       paletteCommandID = "clean-up-merged"
	paletteCopyPath            paletteCommandID = "copy-path"
	paletteCopyPullRequestURL  paletteCommandID = "copy-pull-request-url"
	paletteRefresh             paletteCommandID = "refresh"
	paletteSearch              paletteCommandID = "search"
	paletteJumpRoot            paletteCommandID = "jump-root"
	paletteJumpActive          paletteCommandID = "jump-active"
	paletteJumpTop             paletteCommandID = "jump-top"
	paletteJumpBottom          paletteCommandID = "jump-bottom"
	paletteCycleFilter         paletteCommandID = "cycle-filter"
	paletteFilterAll           paletteCommandID = "filter-all"
	paletteFilterModified      paletteCommandID = "filter-modified"
	paletteFilterMerged        paletteCommandID = "filter-merged"
	paletteFilterPrunable      paletteCommandID = "filter-prunable"
	paletteFilterLocked        paletteCommandID = "filter-locked"
	paletteFilterDetached      paletteCommandID = "filter-detached"
	paletteOpenConfig          paletteCommandID = "open-config"
	paletteToggleHelp          paletteCommandID = "toggle-help"
	paletteQuit                paletteCommandID = "quit"
)

type paletteCommand struct {
	id       paletteCommandID
	title    string
	shortcut string
	keywords string
}

var paletteCommands = []paletteCommand{
	{id: paletteGoSelected, title: "Go to selected row", shortcut: "Enter", keywords: "cd switch create worktree branch"},
	{id: paletteCreate, title: "Create worktree", shortcut: "n", keywords: "new branch"},
	{id: paletteDelete, title: "Delete selected row", shortcut: "d", keywords: "remove prune branch"},
	{id: paletteOpenEditor, title: "Open in editor", shortcut: "o", keywords: "code cursor"},
	{id: paletteOpenPullRequest, title: "Open PR or branch page", shortcut: "p", keywords: "github browser"},
	{id: paletteCheckoutPullRequest, title: "Checkout PR", keywords: "github pr worktree branch"},
	{id: paletteCopyPath, title: "Copy path or branch name", shortcut: "y", keywords: "clipboard branch path"},
	{id: paletteCopyPullRequestURL, title: "Copy PR URL", keywords: "clipboard pull request link github url"},
	{id: paletteCleanUpMerged, title: "Clean up merged", keywords: "done safe remove delete worktree branch cleanup"},
	{id: paletteRefresh, title: "Fetch and reload", shortcut: "r", keywords: "refresh prune"},
	{id: paletteSearch, title: "Search branches", shortcut: "s", keywords: "find filter"},
	{id: paletteJumpRoot, title: "Jump to root repository", shortcut: "h", keywords: "main"},
	{id: paletteJumpActive, title: "Jump to active worktree", shortcut: "a", keywords: "current"},
	{id: paletteJumpTop, title: "Jump to top", shortcut: "g", keywords: "first"},
	{id: paletteJumpBottom, title: "Jump to bottom", shortcut: "G", keywords: "last"},
	{id: paletteCycleFilter, title: "Open filter picker", shortcut: "Tab", keywords: "all modified branches merged prunable locked detached"},
	{id: paletteFilterAll, title: "Filter: all", keywords: "show everything"},
	{id: paletteFilterModified, title: "Filter: modified", keywords: "dirty changes"},
	{id: paletteFilterMerged, title: "Filter: merged", keywords: "done clean safe remove cleanup"},
	{id: paletteFilterPrunable, title: "Filter: prunable", keywords: "missing stale prune"},
	{id: paletteFilterLocked, title: "Filter: locked", keywords: "lock"},
	{id: paletteFilterDetached, title: "Filter: detached", keywords: "head sha"},
	{id: paletteOpenConfig, title: "Open config", shortcut: "ctrl+o", keywords: "settings toml"},
	{id: paletteToggleHelp, title: "Toggle help", shortcut: "?", keywords: "keys shortcuts"},
	{id: paletteQuit, title: "Quit", shortcut: "q", keywords: "exit"},
}

func (model Model) openPalette() (Model, tea.Cmd) {
	input := textinput.New()
	input.Prompt = "> "
	input.CharLimit = 200
	input.Width = 36
	input.Cursor.Style = flashStyle
	focusCmd := input.Focus()
	model.help = false
	model.paletteDialog = &paletteDialog{input: input}
	return model, focusCmd
}

func (model Model) updatePalette(message tea.KeyMsg) (Model, tea.Cmd) {
	dialog := model.paletteDialog
	switch message.String() {
	case "esc", "ctrl+p":
		model.paletteDialog = nil
		return model, nil
	case "up", "k":
		dialog.selected = clamp(dialog.selected-1, 0, max(0, len(model.matchingPaletteCommands())-1))
		return model, nil
	case "down", "j":
		dialog.selected = clamp(dialog.selected+1, 0, max(0, len(model.matchingPaletteCommands())-1))
		return model, nil
	case "enter":
		commands := model.matchingPaletteCommands()
		if len(commands) == 0 {
			return model, nil
		}
		command := commands[clamp(dialog.selected, 0, len(commands)-1)]
		model.paletteDialog = nil
		return model.executePaletteCommand(command.id)
	}
	previousValue := dialog.input.Value()
	var cmd tea.Cmd
	dialog.input, cmd = dialog.input.Update(message)
	if dialog.input.Value() != previousValue {
		dialog.selected = 0
	}
	dialog.selected = clamp(dialog.selected, 0, max(0, len(model.matchingPaletteCommands())-1))
	return model, cmd
}

func (model Model) executePaletteCommand(id paletteCommandID) (Model, tea.Cmd) {
	switch id {
	case paletteGoSelected:
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
	case paletteCreate:
		return model.openCreate()
	case paletteDelete:
		return model.openDelete()
	case paletteOpenEditor:
		row, ok := model.selectedWorktree()
		if !ok || row.Prunable {
			return model.setFlash("cannot open this worktree")
		}
		return model, openEditorCmd(model.config.Editor, row.Path)
	case paletteOpenPullRequest:
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
	case paletteCheckoutPullRequest:
		return model.openPullRequestCheckout()
	case paletteCleanUpMerged:
		return model.openCleanupMerged()
	case paletteCopyPath:
		text, flash, ok := model.selectedCopyText()
		if !ok {
			return model, nil
		}
		return model, copyTextCmd(text, flash)
	case paletteCopyPullRequestURL:
		text, flash, ok := model.selectedPullRequestCopy()
		if !ok {
			return model.setFlash("no pull request URL for this row")
		}
		return model, copyTextCmd(text, flash)
	case paletteRefresh:
		return model.startRefresh(true, false)
	case paletteSearch:
		model.searching = true
		return model, model.search.Focus()
	case paletteJumpRoot:
		model.selectMatching(func(row gitdata.Row) bool { return row.IsWorktree() && row.Worktree.IsMain })
	case paletteJumpActive:
		model.selectMatching(func(row gitdata.Row) bool { return row.IsWorktree() && row.Worktree.IsActive })
	case paletteJumpTop:
		model.selected = 0
	case paletteJumpBottom:
		model.selected = max(0, len(model.visibleIndexes())-1)
	case paletteCycleFilter:
		return model.openFilterDialog()
	case paletteFilterAll:
		model.setFilter(filterAll)
	case paletteFilterModified:
		model.setFilter(filterModified)
	case paletteFilterMerged:
		model.setFilter(filterMerged)
	case paletteFilterPrunable:
		model.setFilter(filterPrunable)
	case paletteFilterLocked:
		model.setFilter(filterLocked)
	case paletteFilterDetached:
		model.setFilter(filterDetached)
	case paletteOpenConfig:
		return model, openConfigCmd(model.config.Editor, model.config)
	case paletteToggleHelp:
		model.help = !model.help
	case paletteQuit:
		return model, tea.Quit
	}
	return model, nil
}

func (model Model) matchingPaletteCommands() []paletteCommand {
	if model.paletteDialog == nil {
		return paletteCommands
	}
	query := strings.TrimSpace(model.paletteDialog.input.Value())
	if query == "" {
		return paletteCommands
	}
	matches := make([]paletteCommand, 0, len(paletteCommands))
	for _, command := range paletteCommands {
		haystack := command.title + " " + command.shortcut + " " + command.keywords
		if fuzzyMatch(haystack, query) {
			matches = append(matches, command)
		}
	}
	return matches
}

func (model Model) openFilterDialog() (Model, tea.Cmd) {
	model.help = false
	model.paletteDialog = nil
	options := model.filterOptions()
	model.filterDialog = &filterDialog{selected: selectedFilterOptionIndex(options, model.filter)}
	return model, nil
}

func (model Model) updateFilterDialog(message tea.KeyMsg) (Model, tea.Cmd) {
	switch message.String() {
	case "esc":
		model.filterDialog = nil
		return model, nil
	case "up", "k", "shift+tab":
		model.moveFilterDialogSelection(-1)
		return model, nil
	case "down", "j", "tab":
		model.moveFilterDialogSelection(1)
		return model, nil
	case "enter":
		options := model.filterOptions()
		if len(options) == 0 {
			model.filterDialog = nil
			return model, nil
		}
		selected := clamp(model.filterDialog.selected, 0, len(options)-1)
		option := options[selected]
		if !option.enabled {
			return model, nil
		}
		model.filterDialog = nil
		model.setFilter(option.filter)
		return model, nil
	}
	return model, nil
}

func (model *Model) moveFilterDialogSelection(direction int) {
	if model.filterDialog == nil || direction == 0 {
		return
	}
	options := model.filterOptions()
	if len(options) == 0 {
		model.filterDialog.selected = 0
		return
	}
	selected := clamp(model.filterDialog.selected, 0, len(options)-1)
	for offset := 1; offset <= len(options); offset++ {
		next := (selected + direction*offset) % len(options)
		if next < 0 {
			next += len(options)
		}
		if options[next].enabled {
			model.filterDialog.selected = next
			return
		}
	}
	model.filterDialog.selected = selectedFilterOptionIndex(options, model.filter)
}

func (model Model) filterOptions() []filterOption {
	options := make([]filterOption, 0, len(orderedFilters))
	for _, filter := range orderedFilters {
		count := len(model.visibleIndexesForFilter(filter))
		options = append(options, filterOption{
			filter:  filter,
			count:   count,
			enabled: filter == filterAll || count > 0,
		})
	}
	return options
}

func selectedFilterOptionIndex(options []filterOption, current worktreeFilter) int {
	for index, option := range options {
		if option.filter == current && option.enabled {
			return index
		}
	}
	for index, option := range options {
		if option.enabled {
			return index
		}
	}
	return 0
}

func (model Model) renderPaletteAtWidth(width int) string {
	dialog := model.paletteDialog
	contentWidth := max(1, width-4)
	input := dialog.input
	input.Width = max(1, contentWidth-runewidth.StringWidth(input.Prompt)-1)
	lines := []string{input.View()}
	commands := model.matchingPaletteCommands()
	if len(commands) == 0 {
		lines = append(lines, hintStyle.Render("No commands"))
	} else {
		limit := min(8, len(commands))
		selected := clamp(dialog.selected, 0, len(commands)-1)
		for index := 0; index < limit; index++ {
			command := commands[index]
			prefix := "  "
			style := inspectorValueStyle
			if index == selected {
				prefix = "› "
				style = paletteSelectedStyle
			}
			label := command.title
			if command.shortcut != "" {
				label += "  " + hintStyle.Render(command.shortcut)
			}
			line := truncateStyled(prefix+label, contentWidth)
			if index == selected {
				line = style.Render(padStyled(line, contentWidth))
			}
			lines = append(lines, line)
		}
	}
	return dialogBox("Commands", lines, paletteHintsAtWidth(width-6), width)
}

func (model Model) renderFilterAtWidth(width int) string {
	dialog := model.filterDialog
	contentWidth := max(1, width-4)
	options := model.filterOptions()
	lines := make([]string, 0, len(options))
	if len(options) == 0 {
		lines = append(lines, hintStyle.Render("No filters"))
	} else {
		selected := clamp(dialog.selected, 0, len(options)-1)
		for index, option := range options {
			prefix := "  "
			if index == selected && option.enabled {
				prefix = "› "
			}
			line := filterOptionLine(prefix, option, contentWidth)
			if !option.enabled {
				line = hintStyle.Render(line)
			}
			if index == selected && option.enabled {
				line = paletteSelectedStyle.Render(padStyled(line, contentWidth))
			}
			lines = append(lines, line)
		}
	}
	return dialogBox("Filters", lines, filterDialogHintsAtWidth(width-6), width)
}

func filterOptionLine(prefix string, option filterOption, width int) string {
	label := option.filter.label()
	count := filterCountLabel(option.count)
	labelWidth := runewidth.StringWidth(prefix + label)
	countWidth := runewidth.StringWidth(count)
	gap := max(1, width-labelWidth-countWidth)
	return truncatePlain(prefix+label+strings.Repeat(" ", gap)+count, width)
}

func filterCountLabel(count int) string {
	if count == 1 {
		return "1 row"
	}
	return fmt.Sprintf("%d rows", count)
}

func paletteHintsAtWidth(width int) string {
	full := colorKeyHints("Enter run · ↑/↓ move · Esc cancel", false)
	if lipgloss.Width(full) <= width {
		return full
	}
	short := colorKeyHints("Enter · ↑/↓ · Esc", false)
	if lipgloss.Width(short) <= width {
		return short
	}
	return ""
}
