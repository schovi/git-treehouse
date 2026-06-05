package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/schovi/git-worktree-tui/internal/config"
	"github.com/schovi/git-worktree-tui/internal/gitdata"
)

func TestSelectedInspectorUsesLabeledRelativeFields(t *testing.T) {
	model := Model{
		width: 80,
		state: gitdata.State{
			Repo: gitdata.Repository{
				Root:           "/repo/main",
				ActiveWorktree: "/repo/main",
			},
		},
	}
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	row := gitdata.Worktree{
		Path:          "/repo/main",
		Branch:        "main",
		Status:        gitdata.StatusCounts{Modified: 2, Untracked: 1},
		Upstream:      "origin/main",
		HeadSync:      gitdata.SyncState{Available: true},
		CommitShort:   "c00b701",
		CommitSubject: "Add strategy-review analysis artifacts",
		CommitTime:    now.Add(-4 * time.Hour),
	}

	output := model.selectedInspector(row, now)

	for _, want := range []string{
		"Branch",
		"main",
		"Path",
		".",
		"Status",
		"dirty",
		"Dirty",
		"~ modified 2  ? untracked 1",
		"Sync",
		"origin/main, synced",
		"Commit",
		"c00b701 Add strategy-review analysis artifacts, 4h",
		"PR",
		"none",
		"Delete",
		"allowed with force, dirty worktree",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("selectedInspector() missing %q:\n%s", want, output)
		}
	}
}

func TestSelectedInspectorKeepsDirtyFieldForCleanRows(t *testing.T) {
	model := Model{
		width: 80,
		state: gitdata.State{
			Repo: gitdata.Repository{
				Root:           "/repo/main",
				ActiveWorktree: "/repo/main",
			},
		},
	}
	row := gitdata.Worktree{
		Path:          "/repo/main",
		Branch:        "main",
		CommitShort:   "abc1234",
		CommitSubject: "clean row",
	}

	output := model.selectedInspector(row, time.Now())

	if !strings.Contains(output, "Dirty") || !strings.Contains(output, "none") {
		t.Fatalf("selectedInspector() should keep clean dirty field:\n%s", output)
	}
}

func TestDetailPanelSplitsInspectorAndKeybindings(t *testing.T) {
	model := Model{
		width: 100,
		state: gitdata.State{
			Repo: gitdata.Repository{
				Root:           "/repo/main",
				ActiveWorktree: "/repo/main",
			},
		},
	}
	row := gitdata.Worktree{
		Path:   "/repo/main",
		Branch: "main",
	}

	output := model.detailPanel(row, time.Now())

	for _, want := range []string{"Branch", "main", "│", "Current", "↵", "go", "o", "editor", "d", "delete", "y", "abs path", "p", "PR", "Dirty", "none"} {
		if !strings.Contains(output, want) {
			t.Fatalf("detailPanel() missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"? help", "q quit", "s search", "g/G top/bottom", "Esc close/clear", "r refresh", "n new", "m main", "a active", "Tab special", "Tab notable", "Tab filter"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("detailPanel() should not contain app control %q:\n%s", unwanted, output)
		}
	}
}

func TestTitleLineIncludesHelpAndQuit(t *testing.T) {
	model := Model{
		width: 80,
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/main"},
			Rows: []gitdata.Worktree{{Branch: "main"}},
		},
	}

	output := model.titleLine(1)

	for _, want := range []string{"gwt", "main", "1 worktrees", "n", "new", "r", "refresh", "?", "help", "q", "quit"} {
		if !strings.Contains(output, want) {
			t.Fatalf("titleLine() missing %q:\n%s", want, output)
		}
	}
}

func TestStatusBarSplitsAppControlsAndDirtyLegend(t *testing.T) {
	model := Model{width: 120}

	output := model.statusBar()

	for _, want := range []string{"g/G", "top/bottom", "m", "main", "a", "active", "Tab", "filter:", "all", "s", "search", "Esc", "close/clear", "+", "staged", "~", "modified", "untracked"} {
		if !strings.Contains(output, want) {
			t.Fatalf("statusBar() missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"help", "quit"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("statusBar() should not include header controls %q:\n%s", unwanted, output)
		}
	}
	if strings.Contains(output, "delete") || strings.Contains(output, "editor") {
		t.Fatalf("statusBar() should not contain persistent keybindings:\n%s", output)
	}
}

func TestStatusBarDropsWholeHintsWhenNarrow(t *testing.T) {
	output := joinPartsWithin([]string{"g/G top/bottom", "m main", "a active", "Tab filter: all"}, 35)

	if strings.Contains(output, "…") {
		t.Fatalf("joinPartsWithin() should avoid partial keybinds: %q", output)
	}
	if strings.Contains(output, "Tab") {
		t.Fatalf("joinPartsWithin() should drop keybinds that do not fit: %q", output)
	}
}

func TestTabCyclesFiltersInOrder(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
		{Path: "/repo/modified", Branch: "modified", Status: gitdata.StatusCounts{Modified: 1}},
		{Path: "/repo/prunable", Branch: "prunable", Prunable: true},
		{Path: "/repo/locked", Branch: "locked", Locked: true},
		{Path: "/repo/detached", Head: "abc123456", Detached: true},
	})

	for _, want := range []worktreeFilter{filterModified, filterPrunable, filterLocked, filterDetached, filterAll} {
		model = pressTab(model)
		if model.filter != want {
			t.Fatalf("filter after Tab = %q, want %q", model.filter.label(), want.label())
		}
	}
}

func TestTabSkipsEmptyFilters(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
		{Path: "/repo/locked", Branch: "locked", Locked: true},
	})

	model = pressTab(model)

	if model.filter != filterLocked {
		t.Fatalf("filter after Tab = %q, want locked", model.filter.label())
	}
	if got := strings.Join(visibleBranches(model), ","); got != "locked" {
		t.Fatalf("visible branches = %q, want locked", got)
	}

	model = pressTab(model)

	if model.filter != filterAll {
		t.Fatalf("filter after second Tab = %q, want all", model.filter.label())
	}
}

func TestModifiedFilterIncludesAnyDirtyState(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/clean", Branch: "clean"},
		{Path: "/repo/staged", Branch: "staged", Status: gitdata.StatusCounts{Staged: 1}},
		{Path: "/repo/modified", Branch: "modified", Status: gitdata.StatusCounts{Modified: 1}},
		{Path: "/repo/untracked", Branch: "untracked", Status: gitdata.StatusCounts{Untracked: 1}},
	})

	model = pressTab(model)

	if model.filter != filterModified {
		t.Fatalf("filter after Tab = %q, want modified", model.filter.label())
	}
	if got := strings.Join(visibleBranches(model), ","); got != "staged,modified,untracked" {
		t.Fatalf("visible branches = %q, want staged,modified,untracked", got)
	}
}

func TestFilterCombinesWithBranchSearch(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/alpha-clean", Branch: "alpha-clean"},
		{Path: "/repo/alpha-dirty", Branch: "alpha-dirty", Status: gitdata.StatusCounts{Staged: 1}},
		{Path: "/repo/beta-dirty", Branch: "beta-dirty", Status: gitdata.StatusCounts{Modified: 1}},
	})
	model.search.SetValue("alpha")

	model = pressTab(model)

	if model.filter != filterModified {
		t.Fatalf("filter after Tab = %q, want modified", model.filter.label())
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

func TestEscClearsFilterBeforeQuitting(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
		{Path: "/repo/dirty", Branch: "dirty", Status: gitdata.StatusCounts{Modified: 1}},
	})
	model = pressTab(model)

	var cmd tea.Cmd
	model, cmd = model.updateList(tea.KeyMsg{Type: tea.KeyEsc})

	if cmd != nil {
		t.Fatalf("Esc with filter returned a command, want nil")
	}
	if model.filter != filterAll {
		t.Fatalf("filter after Esc = %q, want all", model.filter.label())
	}

	_, cmd = model.updateList(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("Esc after clearing filter returned nil command, want quit command")
	}
}

func TestViewRendersBoxedAppSections(t *testing.T) {
	model := Model{
		width:  100,
		height: 18,
		state: gitdata.State{
			Repo: gitdata.Repository{
				Root:           "/repo/main",
				ActiveWorktree: "/repo/main",
			},
			Rows: []gitdata.Worktree{{
				Path:          "/repo/main",
				Branch:        "main",
				IsMain:        true,
				IsActive:      true,
				CommitShort:   "abc1234",
				CommitSubject: "boxed app",
			}},
		},
	}

	output := model.View()

	for _, want := range []string{"gwt", "Worktrees", "Details", "╭─", "╰", "Current", "g/G", "staged", " · "} {
		if !strings.Contains(output, want) {
			t.Fatalf("View() missing boxed app element %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"╭─┐", "└┘", "└─╯"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("View() should not contain cap separator %q:\n%s", unwanted, output)
		}
	}
}

func TestAppBottomLineEmbedsStatusWithDotSeparators(t *testing.T) {
	model := Model{width: 100}

	output := model.appBottomLine(100)

	for _, want := range []string{"╰─ ", "g/G", "top/bottom", " · ", "m", "main", "+", "staged", "~", "modified", "? untracked", " ─╯"} {
		if !strings.Contains(output, want) {
			t.Fatalf("appBottomLine() missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "g/G top/bottom ─ m main") {
		t.Fatalf("appBottomLine() should use dot separators, not rule separators:\n%s", output)
	}
	for _, unwanted := range []string{"└┘", "╰─┘", "└─╯"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("appBottomLine() should not contain cap separator %q:\n%s", unwanted, output)
		}
	}
	if width := lipgloss.Width(output); width != 100 {
		t.Fatalf("appBottomLine() width = %d, want 100:\n%s", width, output)
	}
}

func TestAppTopLineFitsWidth(t *testing.T) {
	model := Model{
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/git-worktree-tui"},
			Rows: []gitdata.Worktree{{Branch: "main"}},
		},
	}

	output := model.appTopLine(1, 80)

	for _, want := range []string{"╭─", "gwt", "─╮"} {
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

func TestFramePadsToViewportHeight(t *testing.T) {
	model := Model{width: 12, height: 4}

	output := model.frame("one\ntwo")
	lines := strings.Split(output, "\n")

	if len(lines) != 4 {
		t.Fatalf("frame() line count = %d, want 4:\n%q", len(lines), output)
	}
	if lines[2] != strings.Repeat(" ", 12) || lines[3] != strings.Repeat(" ", 12) {
		t.Fatalf("frame() did not pad blank lines to viewport width:\n%q", output)
	}
}

func testModelWithRows(rows []gitdata.Worktree) Model {
	return New(gitdata.State{
		Repo: gitdata.Repository{
			Root:           "/repo/main",
			ActiveWorktree: "/repo/main",
		},
		Rows: rows,
	}, config.Config{}, nil)
}

func pressTab(model Model) Model {
	model, _ = model.updateList(tea.KeyMsg{Type: tea.KeyTab})
	return model
}

func visibleBranches(model Model) []string {
	indexes := model.visibleIndexes()
	branches := make([]string, 0, len(indexes))
	for _, index := range indexes {
		branches = append(branches, model.state.Rows[index].DisplayBranch())
	}
	return branches
}
