package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	for _, unwanted := range []string{"? help", "q quit", "/ filter", "g/G top/bottom", "Esc close/clear", "r refresh", "n new", "m main", "a active", "Tab special", "Tab notable"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("detailPanel() should not contain app control %q:\n%s", unwanted, output)
		}
	}
}

func TestTitleLineIncludesHelpAndQuit(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	model := Model{
		width:         120,
		lastRefreshAt: now.Add(-12 * time.Second),
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/main"},
			Rows: []gitdata.Worktree{{Branch: "main"}},
		},
	}

	output := model.titleContentAtWidthAtTime(1, model.width, now)

	for _, want := range []string{"gwt", "main", "1 worktrees", "n", "new", "r", "refresh", "12 seconds ago", "?", "help", "q", "quit"} {
		if !strings.Contains(output, want) {
			t.Fatalf("titleLine() missing %q:\n%s", want, output)
		}
	}
}

func TestRefreshAgeText(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		lastRefreshAt time.Time
		want          string
	}{
		{name: "zero timestamp", lastRefreshAt: time.Time{}, want: ""},
		{name: "seconds", lastRefreshAt: now.Add(-12 * time.Second), want: "12 seconds ago"},
		{name: "future timestamp", lastRefreshAt: now.Add(3 * time.Second), want: "0 seconds ago"},
		{name: "singular minute", lastRefreshAt: now.Add(-time.Minute), want: "1 minute ago"},
		{name: "plural minutes", lastRefreshAt: now.Add(-3 * time.Minute), want: "3 minutes ago"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := refreshAgeText(test.lastRefreshAt, now); got != test.want {
				t.Fatalf("refreshAgeText() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAppControlsDropRefreshAgeBeforeCoreControls(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	model := Model{lastRefreshAt: now.Add(-12 * time.Second)}

	wide := model.appControlsAtWidthAtTime(80, now)
	for _, want := range []string{"n", "new", "r", "refresh", "12 seconds ago", "?", "help", "q", "quit"} {
		if !strings.Contains(wide, want) {
			t.Fatalf("wide controls missing %q:\n%s", want, wide)
		}
	}

	narrow := model.appControlsAtWidthAtTime(40, now)
	for _, want := range []string{"r", "refresh", "?", "help", "q", "quit"} {
		if !strings.Contains(narrow, want) {
			t.Fatalf("narrow controls missing %q:\n%s", want, narrow)
		}
	}
	if strings.Contains(narrow, "seconds ago") {
		t.Fatalf("narrow controls should drop refresh age first:\n%s", narrow)
	}
}

func TestAppTopLineIncludesRefreshAgeWhenWide(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	model := Model{
		lastRefreshAt: now.Add(-12 * time.Second),
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/git-worktree-tui"},
			Rows: []gitdata.Worktree{{Branch: "main"}},
		},
	}

	output := model.appTopLineAtTime(1, 120, now)

	for _, want := range []string{"╭─", "gwt", "r", "refresh", "12 seconds ago", "─╮"} {
		if !strings.Contains(output, want) {
			t.Fatalf("appTopLineAtTime() missing %q:\n%s", want, output)
		}
	}
	if width := lipgloss.Width(output); width != 120 {
		t.Fatalf("appTopLineAtTime() width = %d, want 120:\n%s", width, output)
	}
}

func TestStatusBarSplitsAppControlsAndDirtyLegend(t *testing.T) {
	model := Model{width: 120}

	output := model.statusBar()

	for _, want := range []string{"g/G", "top/bottom", "m", "main", "a", "active", "Tab", "notable", "/", "filter", "Esc", "close/clear", "+", "staged", "~", "modified", "untracked"} {
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
	output := joinPartsWithin([]string{"g/G top/bottom", "m main", "a active", "Tab notable"}, 35)

	if strings.Contains(output, "…") {
		t.Fatalf("joinPartsWithin() should avoid partial keybinds: %q", output)
	}
	if strings.Contains(output, "Tab") {
		t.Fatalf("joinPartsWithin() should drop keybinds that do not fit: %q", output)
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
			model: Model{refreshID: 7, loading: "fetching…"},
		},
		{
			name:  "filtering",
			model: Model{refreshID: 7, filtering: true},
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
			name:  "delete dialog",
			model: Model{refreshID: 7, deleteDialog: &deleteDialog{}},
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
	if cmd == nil {
		t.Fatal("auto refresh should schedule the next tick and reload command")
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

func TestManualReloadSuccessFlashes(t *testing.T) {
	completedAt := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	model := Model{
		refreshID:       4,
		refreshInFlight: true,
		loading:         "fetching…",
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

	if updated.flash != "reloaded" {
		t.Fatalf("manual reload flash = %q, want reloaded", updated.flash)
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

func TestStaleReloadMessageIsIgnored(t *testing.T) {
	lastRefreshAt := time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC)
	model := Model{
		refreshID:       4,
		refreshInFlight: true,
		loading:         "fetching…",
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
	if updated.loading != "fetching…" {
		t.Fatalf("stale reload changed loading = %q", updated.loading)
	}
	if cmd != nil {
		t.Fatalf("stale reload returned unexpected command: %v", cmd)
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

func updateModel(t *testing.T, model Model, message tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := model.Update(message)
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", updated)
	}
	return next, cmd
}
