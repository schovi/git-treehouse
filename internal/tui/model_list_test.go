package tui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	appconfig "github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"strings"
	"testing"
	"time"
)

func TestSearchInputRendersInWorktreesFooter(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true},
	})
	model.width = 120
	model.height = 14
	model.searching = true

	emptyOutput := model.View()
	if !strings.Contains(emptyOutput, "search ▌") {
		t.Fatalf("View() should show a cursor for empty search:\n%s", emptyOutput)
	}

	model.search.SetValue("docs")

	output := model.View()

	for _, want := range []string{"search", "docs▌", "Esc", "clear", "Tab", "filter:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("View() missing search footer element %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"h root", "a active"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("View() should not show normal-mode jump hints while searching %q:\n%s", unwanted, output)
		}
	}
	if strings.Contains(output, "Enter apply") {
		t.Fatalf("View() should not show apply for live search:\n%s", output)
	}
	status := model.statusBar()
	for _, unwanted := range []string{"h root", "a active", "search docs", "Enter apply", "Esc clear"} {
		if strings.Contains(status, unwanted) {
			t.Fatalf("statusBar() should not include search footer element %q:\n%s", unwanted, status)
		}
	}
}

func TestFilterFooterRendersEscClearFilter(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true},
		{Path: "/repo/dirty", Branch: "dirty", Status: gitdata.StatusCounts{Modified: 1}},
	})
	model.width = 120
	model.height = 14
	model.setFilter(filterModified)

	output := model.View()

	for _, want := range []string{"Tab", "filter:", "modified", "Esc", "clear filter"} {
		if !strings.Contains(output, want) {
			t.Fatalf("View() missing filter footer element %q:\n%s", want, output)
		}
	}
}

func TestCommittedSearchRendersInWorktreesFooter(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true},
		{Path: "/repo/docs", Branch: "docs"},
	})
	model.width = 120
	model.height = 14
	model.search.SetValue("docs")
	model.searching = true

	model, _ = model.updateSearch(tea.KeyMsg{Type: tea.KeyEnter})
	output := ansi.Strip(model.View())

	if !strings.Contains(output, "search: docs") {
		t.Fatalf("View() should name a committed search in the footer:\n%s", output)
	}
}

func TestEmptyListExplainsActiveFilterAndSearch(t *testing.T) {
	for _, test := range []struct {
		name  string
		query string
		want  []string
	}{
		{
			name: "filter",
			want: []string{"No rows match filter: locked", "Esc to clear"},
		},
		{
			name:  "filter and search",
			query: "fix",
			want: []string{
				"No rows match filter: locked and search: fix",
				"Esc to clear filter",
				"s then Esc to clear search",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := testModelWithRows([]gitdata.Worktree{
				{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true},
			})
			model.width = 80
			model.height = 14
			model.setFilter(filterLocked)
			model.search.SetValue(test.query)

			output := ansi.Strip(model.View())

			for _, want := range test.want {
				if !strings.Contains(output, want) {
					t.Fatalf("View() missing empty-list explanation %q:\n%s", want, output)
				}
			}
			for _, line := range strings.Split(output, "\n") {
				if width := lipgloss.Width(line); width != model.width {
					t.Fatalf("View() line width = %d, want %d:\n%q\n%s", width, model.width, line, output)
				}
			}
		})
	}
}

func TestStatusBarDropsWholeHintsWhenNarrow(t *testing.T) {
	output := joinPartsWithin([]string{"h root", "a active", "Tab filter: all", "s search"}, 28)

	if strings.Contains(output, "…") {
		t.Fatalf("joinPartsWithin() should avoid partial keybinds: %q", output)
	}
	if strings.Contains(output, "Tab") {
		t.Fatalf("joinPartsWithin() should drop keybinds that do not fit: %q", output)
	}
}

func TestBTogglesBranchRows(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})
	model.state.Branches = []gitdata.Branch{{Name: "feature/branch"}}

	if got := strings.Join(visibleBranches(model), ","); got != "main" {
		t.Fatalf("visible branches before b = %q, want main", got)
	}

	model, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

	if cmd == nil {
		t.Fatal("b returned nil command, want settings persistence")
	}
	if !model.showBranches {
		t.Fatal("b should show branch rows")
	}
	if !model.config.ShowBranches {
		t.Fatal("b should update config ShowBranches")
	}
	if got := strings.Join(visibleBranches(model), ","); got != "main,feature/branch" {
		t.Fatalf("visible branches after b = %q, want main,feature/branch", got)
	}
}

func TestNewInitializesBranchToggleFromConfig(t *testing.T) {
	model := New(gitdata.State{
		Repo: gitdata.Repository{Root: "/repo/main"},
		Rows: []gitdata.Worktree{{Path: "/repo/main", Branch: "main"}},
		Branches: []gitdata.Branch{
			{Name: "feature/branch"},
		},
	}, appconfig.Config{ShowBranches: true, GitHub: true}, nil, false, false)

	if !model.showBranches {
		t.Fatal("New() should initialize showBranches from config")
	}
	if got := strings.Join(visibleBranches(model), ","); got != "main,feature/branch" {
		t.Fatalf("visible branches = %q, want main,feature/branch", got)
	}
}

func TestBTogglePersistsBranchVisibilitySetting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := appconfig.SaveDefault(appconfig.Config{
		Editor:       "vim",
		PathTemplate: "{repo_parent}/custom/{branch}",
		MainBranch:   "trunk",
	}); err != nil {
		t.Fatalf("save initial config: %v", err)
	}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})

	model, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if cmd == nil {
		t.Fatal("b returned nil command, want settings persistence")
	}
	message := cmd().(settingsSavedMsg)
	if message.err != nil {
		t.Fatalf("settings save error = %v", message.err)
	}
	config, err := appconfig.LoadDefault()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !config.ShowBranches {
		t.Fatal("ShowBranches persisted false, want true")
	}
	if config.Editor != "vim" || config.PathTemplate != "{repo_parent}/custom/{branch}" || config.MainBranch != "trunk" {
		t.Fatalf("persisting ShowBranches should preserve other config: %+v", config)
	}

	model, cmd = model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if cmd == nil {
		t.Fatal("second b returned nil command, want settings persistence")
	}
	if model.showBranches {
		t.Fatal("second b should hide branch rows")
	}
	message = cmd().(settingsSavedMsg)
	if message.err != nil {
		t.Fatalf("settings save error = %v", message.err)
	}
	config, err = appconfig.LoadDefault()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.ShowBranches {
		t.Fatal("ShowBranches persisted true after second toggle, want false")
	}
}

func TestFilterPickerSkipsEmptyFilters(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
		{Path: "/repo/locked", Branch: "locked", Locked: true},
	})
	model, _ = model.openFilterDialog()

	model, _ = model.updateFilterDialog(tea.KeyMsg{Type: tea.KeyTab})
	options := model.filterOptions()

	if selected := options[model.filterDialog.selected].filter; selected != filterLocked {
		t.Fatalf("selected filter after Tab = %q, want locked", selected.label())
	}
	model, _ = model.updateFilterDialog(tea.KeyMsg{Type: tea.KeyEnter})
	if model.filter != filterLocked {
		t.Fatalf("filter after Enter = %q, want locked", model.filter.label())
	}
	if got := strings.Join(visibleBranches(model), ","); got != "locked locked" {
		t.Fatalf("visible branches = %q, want locked locked", got)
	}

	model, _ = model.openFilterDialog()
	model, _ = model.updateFilterDialog(tea.KeyMsg{Type: tea.KeyTab})
	options = model.filterOptions()

	if selected := options[model.filterDialog.selected].filter; selected != filterAll {
		t.Fatalf("selected filter after wrapped Tab = %q, want all", selected.label())
	}
}

func TestFilterPickerRendersCounts(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
		{Path: "/repo/dirty", Branch: "dirty", Status: gitdata.StatusCounts{Modified: 1}},
	})
	model, _ = model.openFilterDialog()

	output := ansi.Strip(model.renderFilterAtWidth(60))

	for _, want := range []string{"Filters", "all", "2 rows", "modified", "1 row", "branches", "0 rows", "Enter", "apply", "Tab", "next", "Esc", "cancel"} {
		if !strings.Contains(output, want) {
			t.Fatalf("filter picker missing %q:\n%s", want, output)
		}
	}
}

func TestModifiedFilterIncludesAnyDirtyState(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/clean", Branch: "clean"},
		{Path: "/repo/staged", Branch: "staged", Status: gitdata.StatusCounts{Staged: 1}},
		{Path: "/repo/modified", Branch: "modified", Status: gitdata.StatusCounts{Modified: 1}},
		{Path: "/repo/untracked", Branch: "untracked", Status: gitdata.StatusCounts{Untracked: 1}},
	})

	model.setFilter(filterModified)

	if model.filter != filterModified {
		t.Fatalf("filter = %q, want modified", model.filter.label())
	}
	if got := strings.Join(visibleBranches(model), ","); got != "staged,modified,untracked" {
		t.Fatalf("visible branches = %q, want staged,modified,untracked", got)
	}
}

func TestMergedFilterIncludesSafeCleanupRows(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, BranchMergedToMain: true},
		{Path: "/repo/merged-clean", Branch: "merged-clean", BranchMergedToMain: true},
		{Path: "/repo/merged-dirty", Branch: "merged-dirty", Status: gitdata.StatusCounts{Modified: 1}, BranchMergedToMain: true},
		{Path: "/repo/unmerged-clean", Branch: "unmerged-clean"},
		{Path: "/repo/detached", Head: "abc123456", Detached: true, BranchMergedToMain: true},
		{Path: "/repo/pr-merged", Branch: "pr-merged", PR: &gitdata.PullRequest{Number: 1, State: "⎇"}},
		{Path: "/repo/pr-closed", Branch: "pr-closed", PR: &gitdata.PullRequest{Number: 2, State: "✕"}},
		{Path: "/repo/pr-open", Branch: "pr-open", PR: &gitdata.PullRequest{Number: 3, State: "○"}},
	})
	model.state.Branches = []gitdata.Branch{
		{Name: "branch-merged", BranchMergedToMain: true},
	}

	model.setFilter(filterMerged)

	if got := strings.Join(visibleBranches(model), ","); got != "merged-clean,pr-merged,pr-closed,branch-merged" {
		t.Fatalf("visible branches = %q, want merged-clean,pr-merged,pr-closed,branch-merged", got)
	}
}

func TestFilterCombinesWithBranchSearch(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/alpha-clean", Branch: "alpha-clean"},
		{Path: "/repo/alpha-dirty", Branch: "alpha-dirty", Status: gitdata.StatusCounts{Staged: 1}},
		{Path: "/repo/beta-dirty", Branch: "beta-dirty", Status: gitdata.StatusCounts{Modified: 1}},
	})
	model.search.SetValue("alpha")

	model.setFilter(filterModified)

	if model.filter != filterModified {
		t.Fatalf("filter = %q, want modified", model.filter.label())
	}
	if got := strings.Join(visibleBranches(model), ","); got != "alpha-dirty" {
		t.Fatalf("visible branches = %q, want alpha-dirty", got)
	}
}

func TestSOpensBranchSearch(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
	})

	model, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	if cmd != nil {
		t.Fatalf("s returned a command, want nil")
	}
	if !model.searching {
		t.Fatalf("s should open search mode")
	}
}

func TestHSelectsRootWorktree(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/feature", Branch: "feature"},
		{Path: "/repo/main", Branch: "root-branch", IsMain: true},
	})

	model, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	if cmd != nil {
		t.Fatalf("h returned a command, want nil")
	}
	row, ok := model.selectedRow()
	if !ok || row.Path != "/repo/main" {
		t.Fatalf("selected row after h = %#v, want root worktree", row)
	}
}

func TestASelectsActiveWorktree(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/root", Branch: "root"},
		{Path: "/repo/main", Branch: "active", IsActive: true},
	})

	model, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	if cmd != nil {
		t.Fatalf("a returned a command, want nil")
	}
	row, ok := model.selectedRow()
	if !ok || row.Path != "/repo/main" {
		t.Fatalf("selected row after a = %#v, want active worktree", row)
	}
}

func TestEscClearsFilterWithoutQuitting(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
		{Path: "/repo/dirty", Branch: "dirty", Status: gitdata.StatusCounts{Modified: 1}},
	})
	model.setFilter(filterModified)

	var cmd tea.Cmd
	model, cmd = model.updateList(tea.KeyMsg{Type: tea.KeyEsc})

	if cmd != nil {
		t.Fatalf("Esc with filter returned a command, want nil")
	}
	if model.filter != filterAll {
		t.Fatalf("filter after Esc = %q, want all", model.filter.label())
	}

	_, cmd = model.updateList(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("Esc after clearing filter returned a command, want nil")
	}
}

func TestFilterPickerEscClosesBeforeFilterClear(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
		{Path: "/repo/dirty", Branch: "dirty", Status: gitdata.StatusCounts{Modified: 1}},
	})
	model.setFilter(filterModified)
	model, _ = model.openFilterDialog()

	model, cmd := model.updateFilterDialog(tea.KeyMsg{Type: tea.KeyEsc})

	if cmd != nil {
		t.Fatalf("Esc in filter picker returned a command, want nil")
	}
	if model.filterDialog != nil {
		t.Fatal("Esc should close filter picker")
	}
	if model.filter != filterModified {
		t.Fatalf("filter after closing picker = %q, want modified", model.filter.label())
	}

	model, cmd = model.updateList(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("Esc after picker close returned a command, want nil")
	}
	if model.filter != filterAll {
		t.Fatalf("filter after second Esc = %q, want all", model.filter.label())
	}
}

func TestQQuitsFromList(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
	})

	_, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	if cmd == nil {
		t.Fatalf("q returned nil command, want quit command")
	}
}

func TestCtrlCQuitsFromEveryInputState(t *testing.T) {
	list := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main"}})
	search := list
	search.searching = true

	for _, test := range []struct {
		name  string
		model Model
	}{
		{name: "list", model: list},
		{name: "search", model: search},
		{name: "command palette", model: Model{paletteDialog: &paletteDialog{}}},
		{name: "filter picker", model: Model{filterDialog: &filterDialog{}}},
		{name: "create", model: Model{createDialog: &createDialog{}}},
		{name: "checkout", model: Model{checkoutDialog: &checkoutDialog{}}},
		{name: "branch worktree", model: Model{branchWorktreeDialog: &branchWorktreeDialog{}}},
		{name: "delete", model: Model{deleteDialog: &deleteDialog{}}},
		{name: "cleanup merged", model: Model{cleanupMergedDialog: &cleanupMergedDialog{}}},
		{name: "pull request checkout", model: Model{pullRequestDialog: &pullRequestCheckoutDialog{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, cmd := updateModel(t, test.model, tea.KeyMsg{Type: tea.KeyCtrlC})
			if cmd == nil {
				t.Fatal("ctrl+c returned nil command, want quit command")
			}
			message := cmd()
			if _, ok := message.(tea.QuitMsg); !ok {
				t.Fatalf("ctrl+c command = %T, want tea.QuitMsg", message)
			}
		})
	}
}

func TestAppTopLineFitsWidth(t *testing.T) {
	model := Model{
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/git-treehouse"},
			Rows: []gitdata.Worktree{{Branch: "main"}},
		},
	}

	output := model.appTopLine(1, 80)

	for _, want := range []string{"╭─", "Git treehouse", " · ", "─╮"} {
		if !strings.Contains(output, want) {
			t.Fatalf("appTopLine() missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"╭─┐", "┌─╮"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("appTopLine() should not contain cap separator %q:\n%s", unwanted, output)
		}
	}
	if width := lipgloss.Width(output); width != 80 {
		t.Fatalf("appTopLine() width = %d, want 80:\n%s", width, output)
	}
}

func TestAutoRefreshSkipsBlockedStates(t *testing.T) {
	tests := []struct {
		name  string
		model Model
	}{
		{
			name: "refresh in flight",
			model: Model{
				refreshID:       7,
				refreshInFlight: true,
			},
		},
		{
			name:  "loading",
			model: Model{refreshID: 7, loading: "creating…"},
		},
		{
			name:  "delete in flight",
			model: Model{refreshID: 7, deleteInFlight: true},
		},
		{
			name:  "cleanup merged in flight",
			model: Model{refreshID: 7, cleanupMergedInFlight: true},
		},
		{
			name:  "searching",
			model: Model{refreshID: 7, searching: true},
		},
		{
			name:  "help",
			model: Model{refreshID: 7, help: true},
		},
		{
			name:  "create dialog",
			model: Model{refreshID: 7, createDialog: &createDialog{}},
		},
		{
			name:  "checkout dialog",
			model: Model{refreshID: 7, checkoutDialog: &checkoutDialog{}},
		},
		{
			name:  "branch worktree dialog",
			model: Model{refreshID: 7, branchWorktreeDialog: &branchWorktreeDialog{}},
		},
		{
			name:  "delete dialog",
			model: Model{refreshID: 7, deleteDialog: &deleteDialog{}},
		},
		{
			name:  "cleanup merged dialog",
			model: Model{refreshID: 7, cleanupMergedDialog: &cleanupMergedDialog{}},
		},
		{
			name:  "command palette",
			model: Model{refreshID: 7, paletteDialog: &paletteDialog{}},
		},
		{
			name:  "filter picker",
			model: Model{refreshID: 7, filterDialog: &filterDialog{}},
		},
		{
			name:  "pull request checkout",
			model: Model{refreshID: 7, pullRequestDialog: &pullRequestCheckoutDialog{}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			updated, cmd := updateModel(t, test.model, autoRefreshMsg{})

			if updated.refreshID != test.model.refreshID {
				t.Fatalf("auto refresh changed refreshID = %d, want %d", updated.refreshID, test.model.refreshID)
			}
			if updated.refreshInFlight != test.model.refreshInFlight {
				t.Fatalf("auto refresh changed refreshInFlight = %v, want %v", updated.refreshInFlight, test.model.refreshInFlight)
			}
			if cmd == nil {
				t.Fatal("auto refresh skip should still schedule the next tick")
			}
		})
	}
}

func TestAutoRefreshStartsWhenIdle(t *testing.T) {
	model := Model{
		refreshID: 7,
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/main", ActiveWorktree: "/repo/main"},
		},
	}

	updated, cmd := updateModel(t, model, autoRefreshMsg{})

	if updated.refreshID != 8 {
		t.Fatalf("refreshID = %d, want 8", updated.refreshID)
	}
	if !updated.refreshInFlight {
		t.Fatal("auto refresh should mark refreshInFlight")
	}
	if updated.refreshProgressVisible {
		t.Fatal("auto refresh should not show manual progress feedback")
	}
	if cmd == nil {
		t.Fatal("auto refresh should schedule the next tick and reload command")
	}
}

func TestManualRefreshIgnoresRepeatedKeyWhileInFlight(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
	})
	model.refreshID = 7
	model.refreshInFlight = true

	updated, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if updated.refreshID != 7 {
		t.Fatalf("repeated refresh changed refreshID = %d, want 7", updated.refreshID)
	}
	if !updated.refreshInFlight {
		t.Fatal("repeated refresh should keep existing refresh in flight")
	}
	if cmd != nil {
		t.Fatalf("repeated refresh returned command: %v", cmd)
	}
}

func TestManualRefreshRestoresSelectedWorktreeAfterReorder(t *testing.T) {
	completedAt := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
		{Path: "/repo/docs", Branch: "docs"},
		{Path: "/repo/login", Branch: "login"},
	})
	model.selected = 1

	started, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatal("manual refresh should start reload and spinner commands")
	}
	if !started.refreshProgressVisible {
		t.Fatal("manual refresh should show progress feedback")
	}
	if started.refreshAnchor.path != "/repo/docs" {
		t.Fatalf("refresh anchor path = %q, want selected docs worktree", started.refreshAnchor.path)
	}

	updated, _ := updateModel(t, started, reloadMsg{
		id:          started.refreshID,
		completedAt: completedAt,
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/main", ActiveWorktree: "/repo/main"},
			Rows: []gitdata.Worktree{
				{Path: "/repo/login", Branch: "login", LocalMetadataLoaded: true},
				{Path: "/repo/main", Branch: "main", LocalMetadataLoaded: true},
				{Path: "/repo/docs", Branch: "docs", LocalMetadataLoaded: true},
			},
		},
	})

	row, ok := updated.selectedRow()
	if !ok || row.Path != "/repo/docs" {
		t.Fatalf("selected row = %+v, want docs worktree", row)
	}
	if updated.feedback.plainText() != "✓ refreshed" {
		t.Fatalf("success feedback = %q, want refreshed badge", updated.feedback.plainText())
	}
	if updated.flash != "" {
		t.Fatalf("manual refresh should not use generic flash, got %q", updated.flash)
	}
}

func TestAutoReloadSuccessUpdatesTimestampWithoutFlash(t *testing.T) {
	previousRefreshAt := time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC)
	completedAt := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	model := Model{
		refreshID:       4,
		refreshInFlight: true,
		lastRefreshAt:   previousRefreshAt,
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/old", ActiveWorktree: "/repo/old"},
			Rows: []gitdata.Worktree{{Path: "/repo/old", Branch: "old"}},
		},
	}
	nextState := gitdata.State{
		Repo: gitdata.Repository{Root: "/repo/new", ActiveWorktree: "/repo/new"},
		Rows: []gitdata.Worktree{{Path: "/repo/new", Branch: "new"}},
	}

	updated, cmd := updateModel(t, model, reloadMsg{
		id:          4,
		automatic:   true,
		completedAt: completedAt,
		state:       nextState,
	})

	if updated.flash != "" {
		t.Fatalf("auto reload should stay quiet, flash = %q", updated.flash)
	}
	if updated.refreshInFlight {
		t.Fatal("auto reload should clear refreshInFlight")
	}
	if !updated.lastRefreshAt.Equal(completedAt) {
		t.Fatalf("lastRefreshAt = %s, want %s", updated.lastRefreshAt, completedAt)
	}
	if updated.state.Repo.Root != "/repo/new" {
		t.Fatalf("state was not replaced: %+v", updated.state.Repo)
	}
	if cmd == nil {
		t.Fatal("auto reload success should rerun enrichment")
	}
}

func TestOldGitWarningAppearsOnceAcrossAutomaticReloads(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main"}})
	model.state.Repo.GitVersion = "git version 2.40.9"

	updated, _ := updateModel(t, model, branchMetadataWarningMsg{})
	if !strings.Contains(updated.flash, "Git < 2.41") {
		t.Fatalf("warning = %q, want Git 2.41 limitation", updated.flash)
	}
	cleared, _ := updateModel(t, updated, clearFlashMsg{id: updated.flashID})
	cleared.refreshID = 1
	cleared.refreshInFlight = true
	refreshed, _ := updateModel(t, cleared, reloadMsg{
		id:        1,
		automatic: true,
		state: gitdata.State{Repo: gitdata.Repository{
			Root: "/repo/main", GitVersion: "git version 2.40.9",
		}, Rows: []gitdata.Worktree{{Path: "/repo/main", Branch: "main", LocalMetadataLoaded: true}}},
	})
	if refreshed.flash != "" {
		t.Fatalf("automatic reload repeated warning = %q", refreshed.flash)
	}
	repeated, _ := updateModel(t, refreshed, branchMetadataWarningMsg{})
	if repeated.flash != "" {
		t.Fatalf("warning appeared more than once = %q", repeated.flash)
	}
}

func TestGit241DoesNotShowBranchMetadataWarning(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main"}})
	model.state.Repo.GitVersion = "git version 2.41.0"

	updated, command := updateModel(t, model, branchMetadataWarningMsg{})
	if updated.flash != "" || command != nil {
		t.Fatalf("Git 2.41 warning = %q, command = %v", updated.flash, command)
	}
}

func TestManualReloadSuccessShowsRefreshBadge(t *testing.T) {
	completedAt := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	model := Model{
		refreshID:       4,
		refreshInFlight: true,
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/old", ActiveWorktree: "/repo/old"},
		},
	}

	updated, cmd := updateModel(t, model, reloadMsg{
		id:          4,
		completedAt: completedAt,
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/new", ActiveWorktree: "/repo/new"},
		},
	})

	if updated.feedback.plainText() != "✓ refreshed" {
		t.Fatalf("manual reload success feedback = %q, want refreshed", updated.feedback.plainText())
	}
	if updated.flash != "" {
		t.Fatalf("manual reload should not use generic flash, got %q", updated.flash)
	}
	if updated.loading != "" {
		t.Fatalf("manual reload should clear loading, got %q", updated.loading)
	}
	if updated.refreshInFlight {
		t.Fatal("manual reload should clear refreshInFlight")
	}
	if !updated.lastRefreshAt.Equal(completedAt) {
		t.Fatalf("lastRefreshAt = %s, want %s", updated.lastRefreshAt, completedAt)
	}
	if cmd == nil {
		t.Fatal("manual reload success should rerun enrichment and schedule flash clear")
	}
}

func TestFetchFailureClearsRefreshAndShowsError(t *testing.T) {
	runner := &recordingRunner{results: map[string]recordingResult{
		"/repo/main|git fetch --prune": {err: errors.New("authentication required")},
	}}
	model := Model{
		refreshID:              4,
		refreshInFlight:        true,
		refreshProgressVisible: true,
		state: gitdata.State{Repo: gitdata.Repository{
			Root:             "/repo/main",
			ActiveWorktree:   "/repo/main",
			RemoteConfigured: true,
		}},
	}

	message := reloadCmd("/repo/main", appconfig.Config{}, runner, model.state.Repo, gitdata.State{}, true, false, model.refreshID)().(reloadMsg)
	updated, _ := updateModel(t, model, message)

	if updated.refreshInFlight || updated.refreshProgressVisible {
		t.Fatalf("failed fetch left refresh running: %+v", updated)
	}
	if !updated.canAutoRefresh() {
		t.Fatal("failed fetch should allow the next auto-refresh")
	}
	if !strings.Contains(updated.flash, "fetch failed: authentication required") {
		t.Fatalf("flash = %q, want fetch failure", updated.flash)
	}
}

func TestStaleReloadMessageIsIgnored(t *testing.T) {
	lastRefreshAt := time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC)
	model := Model{
		refreshID:       4,
		refreshInFlight: true,
		lastRefreshAt:   lastRefreshAt,
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/current", ActiveWorktree: "/repo/current"},
		},
	}

	updated, cmd := updateModel(t, model, reloadMsg{
		id:          3,
		completedAt: time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC),
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/stale", ActiveWorktree: "/repo/stale"},
		},
	})

	if updated.state.Repo.Root != "/repo/current" {
		t.Fatalf("stale reload replaced state: %+v", updated.state.Repo)
	}
	if !updated.lastRefreshAt.Equal(lastRefreshAt) {
		t.Fatalf("stale reload changed lastRefreshAt = %s, want %s", updated.lastRefreshAt, lastRefreshAt)
	}
	if !updated.refreshInFlight {
		t.Fatal("stale reload should not clear the current in-flight refresh")
	}
	if updated.loading != "" {
		t.Fatalf("stale reload changed loading = %q", updated.loading)
	}
	if cmd != nil {
		t.Fatalf("stale reload returned unexpected command: %v", cmd)
	}
}
