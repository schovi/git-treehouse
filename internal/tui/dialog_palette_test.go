package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	appconfig "github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"strings"
	"testing"
)

func TestTabOpensFilterPickerWhileSearching(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
		{Path: "/repo/dirty", Branch: "dirty", Status: gitdata.StatusCounts{Modified: 1}},
	})
	model.searching = true

	model, cmd := model.updateSearch(tea.KeyMsg{Type: tea.KeyTab})

	if cmd != nil {
		t.Fatalf("Tab while searching returned a command, want nil")
	}
	if model.filterDialog == nil {
		t.Fatal("Tab while searching should open filter picker")
	}
	if model.filter != filterAll {
		t.Fatalf("filter after opening picker = %q, want all", model.filter.label())
	}
}

func TestTabOpensFilterPickerFromList(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
		{Path: "/repo/dirty", Branch: "dirty", Status: gitdata.StatusCounts{Modified: 1}},
	})

	model, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyTab})

	if cmd != nil {
		t.Fatalf("Tab returned a command, want nil")
	}
	if model.filterDialog == nil {
		t.Fatal("Tab should open filter picker")
	}
	if model.filter != filterAll {
		t.Fatalf("filter after opening picker = %q, want all", model.filter.label())
	}
}

func TestFilterPickerTabMovesThroughFiltersAndWraps(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
		{Path: "/repo/modified", Branch: "modified", Status: gitdata.StatusCounts{Modified: 1}},
		{Path: "/repo/merged", Branch: "merged", BranchMergedToMain: true},
		{Path: "/repo/prunable", Branch: "prunable", Prunable: true},
		{Path: "/repo/locked", Branch: "locked", Locked: true},
		{Path: "/repo/detached", Head: "abc123456", Detached: true},
	})
	model.state.Branches = []gitdata.Branch{{Name: "feature/branch"}}
	model, _ = model.openFilterDialog()

	for _, want := range []worktreeFilter{filterModified, filterBranches, filterMerged, filterPrunable, filterLocked, filterDetached, filterAll} {
		model, _ = model.updateFilterDialog(tea.KeyMsg{Type: tea.KeyTab})
		options := model.filterOptions()
		selected := options[model.filterDialog.selected].filter
		if selected != want {
			t.Fatalf("selected filter after Tab = %q, want %q", selected.label(), want.label())
		}
	}
}

func TestFilterPickerEnterAppliesSelectedFilter(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
		{Path: "/repo/modified", Branch: "modified", Status: gitdata.StatusCounts{Modified: 1}},
	})
	model, _ = model.openFilterDialog()
	model, _ = model.updateFilterDialog(tea.KeyMsg{Type: tea.KeyTab})

	model, cmd := model.updateFilterDialog(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		t.Fatalf("filter picker enter returned a command, want nil")
	}
	if model.filterDialog != nil {
		t.Fatal("filter picker should close after applying")
	}
	if model.filter != filterModified {
		t.Fatalf("filter after picker enter = %q, want modified", model.filter.label())
	}
	if got := strings.Join(visibleBranches(model), ","); got != "modified" {
		t.Fatalf("visible branches = %q, want modified", got)
	}
}

func TestCtrlOOpensConfigFromList(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main"}})
	model.config = appconfig.Config{
		Editor:       "true",
		PathTemplate: "{repo_parent}/custom/{branch}",
	}

	_, cmd := updateModel(t, model, tea.KeyMsg{Type: tea.KeyCtrlO})
	if cmd == nil {
		t.Fatal("ctrl+o should return config editor command")
	}
	message := cmd()
	opened, ok := message.(configOpenedMsg)
	if !ok {
		t.Fatalf("config command message = %T, want configOpenedMsg", message)
	}
	if opened.err != nil {
		t.Fatalf("config command error = %v", opened.err)
	}
}

func TestFDoesNotRefresh(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
	})

	updated, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})

	if cmd != nil {
		t.Fatalf("f returned a command, want nil")
	}
	if updated.refreshInFlight || updated.loading != "" {
		t.Fatalf("f should not refresh, got loading=%q refreshInFlight=%v", updated.loading, updated.refreshInFlight)
	}
}

func TestCtrlPOpensCommandPalette(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
	})

	model, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyCtrlP})

	if model.paletteDialog == nil {
		t.Fatal("ctrl+p did not open command palette")
	}
	if cmd == nil {
		t.Fatal("ctrl+p should focus palette input")
	}
	if !model.paletteDialog.input.Focused() {
		t.Fatal("palette input should be focused")
	}
}

func TestCommandPaletteFiltersAndExecutesCommand(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/clean", Branch: "clean"},
		{Path: "/repo/dirty", Branch: "dirty", Status: gitdata.StatusCounts{Modified: 1}},
	})
	model, _ = model.openPalette()
	model.paletteDialog.input.SetValue("dirty")

	commands := model.matchingPaletteCommands()
	if len(commands) != 1 || commands[0].id != paletteFilterModified {
		t.Fatalf("matching palette commands = %+v, want only modified filter", commands)
	}

	model, cmd := model.updatePalette(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		t.Fatalf("palette modified filter returned command, want nil")
	}
	if model.paletteDialog != nil {
		t.Fatal("palette should close after command execution")
	}
	if model.filter != filterModified {
		t.Fatalf("filter = %q, want modified", model.filter.label())
	}
}

func TestCommandPaletteFiltersMerged(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/merged", Branch: "merged", BranchMergedToMain: true},
	})
	model.paletteDialog = &paletteDialog{}

	model, cmd := model.executePaletteCommand(paletteFilterMerged)

	if cmd != nil {
		t.Fatalf("palette merged filter returned command, want nil")
	}
	if model.filter != filterMerged {
		t.Fatalf("filter = %q, want merged", model.filter.label())
	}
}

func TestCommandPaletteOpensCleanUpMerged(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/merged", Branch: "merged", BranchMergedToMain: true},
	})
	model, _ = model.openPalette()
	model.paletteDialog.input.SetValue("cleanup")

	commands := model.matchingPaletteCommands()
	if len(commands) == 0 || commands[0].id != paletteCleanUpMerged {
		t.Fatalf("matching palette commands = %+v, want clean up merged first", commands)
	}

	model, cmd := model.updatePalette(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		t.Fatalf("palette cleanup returned command, want nil")
	}
	if model.paletteDialog != nil {
		t.Fatal("palette should close after command execution")
	}
	if model.cleanupMergedDialog == nil {
		t.Fatal("cleanup command should open confirmation dialog")
	}
}

func TestCommandPaletteIncludesCopyPullRequestURL(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
	})
	model, _ = model.openPalette()

	for _, query := range []string{"url", "pull request"} {
		model.paletteDialog.input.SetValue(query)
		commands := model.matchingPaletteCommands()
		if len(commands) == 0 || commands[0].id != paletteCopyPullRequestURL || commands[0].title != "Copy PR URL" {
			t.Fatalf("matching palette commands for %q = %+v, want Copy PR URL first", query, commands)
		}
		if commands[0].shortcut != "" {
			t.Fatalf("Copy PR URL shortcut = %q, want palette-only command", commands[0].shortcut)
		}
	}
}

func TestExecutePaletteCopyPullRequestURLFlashesWithoutPullRequest(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/no-pr", Branch: "no-pr"},
	})

	model, cmd := model.executePaletteCommand(paletteCopyPullRequestURL)

	if model.flash != "no pull request URL for this row" {
		t.Fatalf("flash = %q, want no pull request URL message", model.flash)
	}
	if cmd == nil {
		t.Fatal("executePaletteCommand should return flash clear command")
	}
}
