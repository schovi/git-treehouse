package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	appconfig "github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/listview"
)

type testRunner struct{}

func (runner testRunner) Run(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
	return nil, errors.New("unexpected command")
}

type recordingRunner struct {
	mutex    sync.Mutex
	commands []string
	results  map[string]recordingResult
}

type recordingResult struct {
	output string
	err    error
}

func (runner *recordingRunner) Run(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	key := dir + "|" + name + " " + strings.Join(args, " ")
	runner.mutex.Lock()
	runner.commands = append(runner.commands, key)
	result, ok := runner.results[key]
	runner.mutex.Unlock()
	if ok {
		return []byte(result.output), result.err
	}
	return nil, nil
}

func TestSelectedInspectorUsesLabeledRelativeFields(t *testing.T) {
	model := Model{
		width: 80,
		state: gitdata.State{
			Repo: gitdata.Repository{
				Root:           "/repo/main",
				ActiveWorktree: "/repo/main",
				MainBranch:     "main",
			},
		},
	}
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	row := gitdata.Worktree{
		Path:          "/repo/main",
		Branch:        "main",
		Head:          "c00b701abcdef",
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
		"HEAD",
		"c00b70",
		"Path",
		".",
		"Status",
		"dirty",
		"Dirty",
		"~ modified 2  ? untracked 1",
		"Remote",
		"origin/main, synced",
		"Main",
		"on local main",
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
				MainBranch:     "main",
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

func TestDetailPanelRendersInspectorOnly(t *testing.T) {
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
		Path:     "/repo/main",
		Branch:   "main",
		IsActive: true,
		IsMain:   true,
	}

	output := model.detailPanel(row, time.Now())

	for _, want := range []string{"Branch", "main", "Dirty", "none", "Remote", "no upstream", "PR", "Delete"} {
		if !strings.Contains(output, want) {
			t.Fatalf("detailPanel() missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"│", "Current", "↵", "o editor", "y abs path", "p PR", "? help", "q quit", "s search", "g/G top/bottom", "Esc close/clear", "r refresh", "n new", "h root", "m main", "a active", "Tab special", "Tab notable", "Tab filter"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("detailPanel() should not contain app control %q:\n%s", unwanted, output)
		}
	}
}

func TestDetailTitleIncludesSelectionContext(t *testing.T) {
	tests := []struct {
		name string
		row  gitdata.Worktree
		want string
	}{
		{name: "current root", row: gitdata.Worktree{IsActive: true, IsMain: true}, want: "Details · Current root repository"},
		{name: "current worktree", row: gitdata.Worktree{IsActive: true}, want: "Details · Current worktree"},
		{name: "root", row: gitdata.Worktree{IsMain: true}, want: "Details · Root repository"},
		{name: "regular", row: gitdata.Worktree{}, want: "Details"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := detailTitle(test.row); got != test.want {
				t.Fatalf("detailTitle() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSectionTopLineStylesTitleSuffix(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	output := sectionTopLine("Details · Current worktree", 80)

	for _, want := range []string{
		panelTitleStyle.Render("Details"),
		titleSeparatorStyle.Render(" · "),
		titleRepoStyle.Render("Current worktree"),
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("sectionTopLine() missing styled title part %q:\n%s", want, output)
		}
	}
	if width := lipgloss.Width(output); width != 80 {
		t.Fatalf("sectionTopLine() width = %d, want 80:\n%s", width, output)
	}
}

func TestViewRendersDetailActionsInDetailsFooter(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true},
	})
	model.width = 100
	model.height = 24

	output := model.View()
	footerLine := ""
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "↵") && strings.Contains(line, "go") {
			footerLine = line
			break
		}
	}

	if footerLine == "" {
		t.Fatalf("View() should render detail actions in the Details footer:\n%s", output)
	}
	for _, want := range []string{"╰─", "↵", "go", "o", "editor", "d", "delete", "y", "abs path", "p", "PR", "╯"} {
		if !strings.Contains(footerLine, want) {
			t.Fatalf("Details footer missing %q:\n%s", want, footerLine)
		}
	}
	for _, unwanted := range []string{"Current", "root repository", "Root repository"} {
		if strings.Contains(footerLine, unwanted) {
			t.Fatalf("Details footer should not contain selected-row context %q:\n%s", unwanted, footerLine)
		}
	}
	if width := lipgloss.Width(footerLine); width != model.width {
		t.Fatalf("Details footer width = %d, want %d:\n%s", width, model.width, footerLine)
	}

	titleLine := ""
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "Details") {
			titleLine = line
			break
		}
	}
	if titleLine == "" {
		t.Fatalf("View() should render a Details title line:\n%s", output)
	}
	for _, want := range []string{"Details", "Current", "root repository"} {
		if !strings.Contains(titleLine, want) {
			t.Fatalf("Details title missing %q:\n%s", want, titleLine)
		}
	}
}

func TestSelectedInspectorShowsPendingPRWhileLoading(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/feature", Branch: "feature"},
	})
	model.state.Repo.RemoteConfigured = true
	model.showPR = true
	model.prLoading = true

	output := model.selectedInspector(model.state.Rows[0], time.Now())

	if !strings.Contains(output, "PR") || !strings.Contains(output, listview.LoadingPlaceholder) {
		t.Fatalf("selectedInspector() should show pending PR marker:\n%s", output)
	}
	if strings.Contains(output, "PR        none") {
		t.Fatalf("selectedInspector() should not show none while PR data is loading:\n%s", output)
	}
}

func TestPullRequestLoadClearsPendingDetailState(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/feature", Branch: "feature"},
	})
	model.state.Repo.RemoteConfigured = true
	model.showPR = true
	model.prLoading = true

	updated, _ := updateModel(t, model, prLoadedMsg{
		enabled:   false,
		repoRoot:  model.state.Repo.Root,
		id:        model.enrichmentID,
		checkedAt: time.Now(),
	})

	if updated.prLoading {
		t.Fatal("PR load completion should clear pending state")
	}
	if got := updated.prText(updated.state.Rows[0]); got != "none" {
		t.Fatalf("PR detail text = %q, want none after completed load", got)
	}
}

func TestTitleLineIncludesHelpAndQuit(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	model := Model{
		width:         120,
		lastRefreshAt: now.Add(-12 * time.Second),
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/main"},
			Rows: []gitdata.Worktree{{Branch: "main", IsMain: true}},
		},
	}

	output := model.titleContentAtWidthAtTime(1, model.width, now)

	for _, want := range []string{"Git treehouse", " · ", "main", "1 worktrees", "root:", "n", "new", "r", "refresh", "12 seconds ago", "?", "help", "q", "quit"} {
		if !strings.Contains(output, want) {
			t.Fatalf("titleLine() missing %q:\n%s", want, output)
		}
	}
}

func TestTitleLineStylesNameSeparatorAndRepo(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	model := Model{
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/git-treehouse"},
			Rows: []gitdata.Worktree{{Branch: "main", IsMain: true}},
		},
	}

	output := model.titleLeftContentAtWidth(1, 100)

	for _, want := range []string{
		titleNameStyle.Render("Git treehouse"),
		titleSeparatorStyle.Render(" · "),
		titleRepoStyle.Render("git-treehouse"),
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("titleLeftContentAtWidth() missing styled title part %q:\n%s", want, output)
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

func TestRefreshSpinnerPattern(t *testing.T) {
	if refreshTickInterval != 80*time.Millisecond {
		t.Fatalf("refresh tick interval = %s, want 80ms", refreshTickInterval)
	}
	if got := strings.Join(refreshSpinnerFrames, ""); got != "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏" {
		t.Fatalf("refresh spinner frames = %q", got)
	}
	if refreshFlashTimeout != 3*time.Second {
		t.Fatalf("refresh flash timeout = %s, want 3s", refreshFlashTimeout)
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
			Repo: gitdata.Repository{Root: "/repo/git-treehouse"},
			Rows: []gitdata.Worktree{{Branch: "main", IsMain: true}},
		},
	}

	output := model.appTopLineAtTime(1, 120, now)

	for _, want := range []string{"╭─", "Git treehouse", " · ", "r", "refresh", "12 seconds ago", "─╮"} {
		if !strings.Contains(output, want) {
			t.Fatalf("appTopLineAtTime() missing %q:\n%s", want, output)
		}
	}
	if width := lipgloss.Width(output); width != 120 {
		t.Fatalf("appTopLineAtTime() width = %d, want 120:\n%s", width, output)
	}
}

func TestStatusBarIsEmptyWithoutTransientStatus(t *testing.T) {
	model := Model{width: 180}

	output := model.statusBar()

	if output != "" {
		t.Fatalf("statusBar() = %q, want empty default status", output)
	}
	for _, unwanted := range []string{"help", "quit", "g/G", "top/bottom", "h root", "m main", "a active", "Tab", "filter:", "s search", "⌂", "locked", "prunable", "remote", "+ staged", "~ modified", "? untracked"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("statusBar() should not include hidden or moved controls %q:\n%s", unwanted, output)
		}
	}
	if strings.Contains(output, "delete") || strings.Contains(output, "editor") {
		t.Fatalf("statusBar() should not contain persistent keybindings:\n%s", output)
	}
}

func TestStatusBarShowsLoadingStatus(t *testing.T) {
	model := Model{width: 180, loading: "creating…"}

	output := model.statusBar()

	if !strings.Contains(output, "creating…") {
		t.Fatalf("statusBar() missing loading status:\n%s", output)
	}
	if strings.Contains(output, "Esc") {
		t.Fatalf("statusBar() should not show Esc in default frame:\n%s", output)
	}
}

func TestViewRendersRefreshFeedbackInWorktreesTitle(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true},
	})
	model.width = 120
	model.height = 16
	model.refreshInFlight = true
	model.refreshProgressVisible = true

	output := model.View()

	if !strings.Contains(output, "Worktrees") || !strings.Contains(output, "⠋ refreshing") {
		t.Fatalf("View() should show refresh feedback in Worktrees title:\n%s", output)
	}
	if strings.Contains(output, "Loading worktrees") {
		t.Fatalf("View() should keep the current table during refresh:\n%s", output)
	}
	if strings.Contains(model.statusBar(), "refreshing") {
		t.Fatalf("statusBar() should not contain table-scoped refresh feedback:\n%s", model.statusBar())
	}
}

func TestViewRendersRefreshSuccessInWorktreesTitle(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true},
	})
	model.width = 120
	model.height = 16
	model.refreshFlash = "✓ refreshed"

	output := model.View()

	if !strings.Contains(output, "Worktrees") || !strings.Contains(output, "✓ refreshed") {
		t.Fatalf("View() should show refresh success in Worktrees title:\n%s", output)
	}
	if strings.Contains(output, "reloaded") {
		t.Fatalf("View() should not render the old reload copy:\n%s", output)
	}
}

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
	model = pressTab(model)

	output := model.View()

	for _, want := range []string{"Tab", "filter:", "modified", "Esc", "clear filter"} {
		if !strings.Contains(output, want) {
			t.Fatalf("View() missing filter footer element %q:\n%s", want, output)
		}
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

func TestTabCyclesFilterWhileSearching(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
		{Path: "/repo/dirty", Branch: "dirty", Status: gitdata.StatusCounts{Modified: 1}},
	})
	model.searching = true

	model, cmd := model.updateSearch(tea.KeyMsg{Type: tea.KeyTab})

	if cmd != nil {
		t.Fatalf("Tab while searching returned a command, want nil")
	}
	if model.filter != filterModified {
		t.Fatalf("filter after Tab while searching = %q, want modified", model.filter.label())
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
	if got := strings.Join(visibleBranches(model), ","); got != "locked locked" {
		t.Fatalf("visible branches = %q, want locked locked", got)
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
	if cmd != nil {
		t.Fatalf("Esc after clearing filter returned a command, want nil")
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
				Path:                "/repo/main",
				Branch:              "main",
				IsMain:              true,
				IsActive:            true,
				LocalMetadataLoaded: true,
				CommitShort:         "abc1234",
				CommitSubject:       "boxed app",
			}},
		},
	}

	output := model.View()

	for _, want := range []string{"Git treehouse", "Worktrees", "Details", "╭─", "╰", "Current", "h", "root", "a", "active", "Tab", "filter:", "s", "search", " · "} {
		if !strings.Contains(output, want) {
			t.Fatalf("View() missing boxed app element %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "g/G") || strings.Contains(output, "top/bottom") || strings.Contains(output, "m main") {
		t.Fatalf("View() should hide top/bottom hint outside help:\n%s", output)
	}
	for _, unwanted := range []string{"╭─┐", "└┘", "└─╯"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("View() should not contain cap separator %q:\n%s", unwanted, output)
		}
	}
}

func TestViewHidesRowsUntilLocalMetadataLoaded(t *testing.T) {
	model := New(gitdata.State{
		Repo: gitdata.Repository{
			Root:             "/repo/main",
			ActiveWorktree:   "/repo/main",
			RemoteConfigured: true,
		},
		Rows: []gitdata.Worktree{
			{Path: "/repo/main", Branch: "main"},
			{Path: "/repo/feature", Branch: "feature"},
		},
	}, appconfig.Config{}, testRunner{})
	model.width = 160
	model.height = 20

	output := model.View()

	if !strings.Contains(output, "Loading worktrees") {
		t.Fatalf("View() should show loading skeleton before local metadata:\n%s", output)
	}
	if strings.Contains(output, "feature") {
		t.Fatalf("View() should hide unsorted skeleton rows before local metadata:\n%s", output)
	}
	if strings.Contains(output, "Details") {
		t.Fatalf("View() should hide details before local metadata:\n%s", output)
	}
}

func TestNewReservesPullRequestColumnForRemoteRepository(t *testing.T) {
	model := New(gitdata.State{
		Repo: gitdata.Repository{
			Root:             "/repo/main",
			ActiveWorktree:   "/repo/main",
			RemoteConfigured: true,
		},
		Rows: []gitdata.Worktree{{
			Path:                "/repo/main",
			Branch:              "main",
			LocalMetadataLoaded: true,
			GitSizeLoaded:       true,
		}},
	}, appconfig.Config{}, testRunner{})
	model.width = 160

	output := model.View()

	if !model.showPR {
		t.Fatal("remote repositories should reserve the PR column before PR data loads")
	}
	if !strings.Contains(output, "PR") {
		t.Fatalf("View() should render reserved PR column:\n%s", output)
	}
	if !strings.Contains(output, listview.LoadingPlaceholder) {
		t.Fatalf("View() should show pending PR marker before PR data loads:\n%s", output)
	}
}

func TestHelpRendersCenteredOverlayInAppFrame(t *testing.T) {
	rows := make([]gitdata.Worktree, 24)
	rows[0] = gitdata.Worktree{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true}
	for index := 1; index < len(rows); index++ {
		rows[index] = gitdata.Worktree{Path: fmt.Sprintf("/repo/worktree-%d", index), Branch: fmt.Sprintf("worktree-%d", index)}
	}
	model := testModelWithRows(rows)
	model.width = 100
	model.height = 40
	model.help = true

	output := model.View()
	lines := strings.Split(output, "\n")
	helpLine := -1
	for index, line := range lines {
		if strings.Contains(line, "Help") {
			helpLine = index
			break
		}
	}

	if helpLine < 3 || helpLine > 12 {
		t.Fatalf("Help dialog line = %d, want centered in app frame:\n%s", helpLine, output)
	}
	if helpLine >= model.height/2 {
		t.Fatalf("Help dialog line = %d, should not be centered in terminal viewport:\n%s", helpLine, output)
	}
	plainOutput := ansi.Strip(output)
	if !strings.Contains(plainOutput, "Worktrees") || !strings.Contains(plainOutput, "ctrl+p commands") {
		t.Fatalf("Help overlay should preserve app content and show palette hint:\n%s", output)
	}
}

func TestHelpRendersGroupedKeysAndLegends(t *testing.T) {
	model := Model{}

	output := ansi.Strip(model.renderHelpAtWidth(68))

	for _, want := range []string{
		"Global",
		"Worktree List",
		"Worktree Detail",
		"Worktree Markers",
		"Git Status",
		"Pull Requests",
		"ctrl+p",
		"Esc close/cancel",
		"top/bottom",
		"PR/branch",
		"bold active branch",
		"remote gone",
		"◌ draft",
		"○ ready/open",
		"◆ approved",
		"⬡ merged",
		"✗ CI error",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderHelpAtWidth() missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"r/f", "close/clear/quit"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("renderHelpAtWidth() should not contain %q:\n%s", unwanted, output)
		}
	}
	markerOrder := []string{"⌂ root", "! locked", "× prunable", "bold active branch", "detached HEAD"}
	previousIndex := -1
	for _, marker := range markerOrder {
		index := strings.Index(output, marker)
		if index < 0 {
			t.Fatalf("renderHelpAtWidth() missing marker %q:\n%s", marker, output)
		}
		if index <= previousIndex {
			t.Fatalf("renderHelpAtWidth() marker %q is out of order:\n%s", marker, output)
		}
		previousIndex = index
	}
}

func TestHelpCategoryStyleIsBoldWhite(t *testing.T) {
	if !helpCategoryStyle.GetBold() {
		t.Fatal("helpCategoryStyle should be bold")
	}
	if got := fmt.Sprint(helpCategoryStyle.GetForeground()); got != "255" {
		t.Fatalf("helpCategoryStyle foreground = %q, want 255", got)
	}
}

func TestOpenCreateStartsWithEmptyBranchName(t *testing.T) {
	model := Model{
		runner: testRunner{},
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/main"},
			Rows: []gitdata.Worktree{{
				Path:   "/repo/main",
				Branch: "feature/source",
			}},
		},
	}

	model, cmd := model.openCreate()

	if model.createDialog == nil {
		t.Fatal("openCreate() did not open create dialog")
	}
	if cmd == nil {
		t.Fatal("openCreate() should return input focus command")
	}
	if got := model.createDialog.input.Value(); got != "" {
		t.Fatalf("create branch input = %q, want empty", got)
	}
	if model.createDialog.error != "" {
		t.Fatalf("create dialog error = %q, want empty initial error", model.createDialog.error)
	}
	if !model.createDialog.input.Focused() {
		t.Fatal("create branch input should be focused")
	}
}

func TestCreateDialogTypingUpdatesBranchInputWithoutValidation(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})

	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

	if got := model.createDialog.input.Value(); got != "fe" {
		t.Fatalf("create branch input = %q, want fe", got)
	}
	if model.createDialog.error != "" {
		t.Fatalf("typing should not validate immediately, got error %q", model.createDialog.error)
	}
}

func TestCreateDialogTextNavigationDoesNotChangeBase(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{
		{Label: "main (local)", Rev: "main"},
		{Label: "origin/main", Rev: "origin/main"},
	})
	model.createDialog.input.SetValue("ab")

	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyLeft})
	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyRight})

	if got := model.createDialog.input.Value(); got != "axb" {
		t.Fatalf("left arrow should move text cursor, input = %q, want axb", got)
	}
	if got := model.createDialog.baseIndex; got != 0 {
		t.Fatalf("left/right should not change base index, got %d", got)
	}
}

func TestCreateDialogBaseNavigationKeys(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{
		{Label: "main (local)", Rev: "main"},
		{Label: "origin/main", Rev: "origin/main"},
		{Label: "feature (local)", Rev: "feature"},
	})

	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyTab})
	if got := model.createDialog.baseIndex; got != 1 {
		t.Fatalf("tab base index = %d, want 1", got)
	}
	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyDown})
	if got := model.createDialog.baseIndex; got != 2 {
		t.Fatalf("down base index = %d, want 2", got)
	}
	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := model.createDialog.baseIndex; got != 1 {
		t.Fatalf("shift+tab base index = %d, want 1", got)
	}
	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyUp})
	if got := model.createDialog.baseIndex; got != 0 {
		t.Fatalf("up base index = %d, want 0", got)
	}
}

func TestCreateDialogValidatesOnlyOnSubmit(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})

	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if model.createDialog.error != "" {
		t.Fatalf("typing should not validate immediately, got error %q", model.createDialog.error)
	}

	model.createDialog.input.SetValue("")
	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyEnter})
	if got := model.createDialog.error; got != "branch name is required" {
		t.Fatalf("submit validation error = %q, want branch name is required", got)
	}
}

func TestCreateDialogRendersCenteredOverlay(t *testing.T) {
	input := textinput.New()
	input.Prompt = ""
	model := Model{
		width:  100,
		height: 40,
		state: gitdata.State{
			Repo: gitdata.Repository{
				Root:           "/repo/main",
				ActiveWorktree: "/repo/main",
			},
			Rows: []gitdata.Worktree{{
				Path:                "/repo/main",
				Branch:              "main",
				IsMain:              true,
				IsActive:            true,
				LocalMetadataLoaded: true,
				CommitShort:         "abc1234",
				CommitSubject:       "boxed app",
			}},
		},
		createDialog: &createDialog{
			input: input,
			bases: []gitdata.BaseOption{{Label: "main (local)", Rev: "main"}},
		},
	}

	output := model.View()
	lines := strings.Split(output, "\n")
	dialogLine := -1
	for index, line := range lines {
		if strings.Contains(line, "New worktree") {
			dialogLine = index
			break
		}
	}

	if len(lines) != model.height {
		t.Fatalf("View() line count = %d, want %d:\n%s", len(lines), model.height, output)
	}
	if dialogLine < 5 || dialogLine > 12 {
		t.Fatalf("New worktree dialog line = %d, want centered in app frame:\n%s", dialogLine, output)
	}
	if dialogLine >= model.height/2 {
		t.Fatalf("New worktree dialog line = %d, should not be centered in terminal viewport:\n%s", dialogLine, output)
	}
}

func TestCreateDialogRendersColoredBorderAndBottomHints(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})

	output := model.renderCreateAtWidth(72)

	for _, want := range []string{
		appBorderStyle.Render("╭─"),
		appBorderStyle.Render("│ "),
		appBorderStyle.Render("╰─ "),
		colorKeyHints("Enter create · Tab switch base · ctrl+o config · Esc cancel", false),
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderCreateAtWidth() missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Tab switch · Esc cancel") {
		t.Fatalf("renderCreateAtWidth() should use Tab switch base:\n%s", output)
	}
}

func TestDialogBottomLineKeepsRightBorderRuleAfterHints(t *testing.T) {
	line := dialogBottomLine(colorKeyHints("Enter delete · Esc cancel", false), 80)

	if width := lipgloss.Width(line); width != 80 {
		t.Fatalf("dialogBottomLine() width = %d, want 80:\n%s", width, line)
	}
	if !strings.Contains(line, appBorderStyle.Render(" ─")) {
		t.Fatalf("dialogBottomLine() should render a horizontal rule after hints:\n%s", line)
	}
	if strings.Contains(line, "     "+appBorderStyle.Render("─╯")) {
		t.Fatalf("dialogBottomLine() should not leave blank padding before the right corner:\n%s", line)
	}
}

func TestBottomBorderLineHandlesEmptyStyledControls(t *testing.T) {
	line := bottomBorderLine(40, appBorderStyle, borderControls{text: colorKeyHints("", false)}, borderControls{})
	plainLine := ansi.Strip(line)

	if strings.Contains(plainLine, "╰─  ─") {
		t.Fatalf("bottomBorderLine() should not reserve space for zero-width styled content:\n%s", line)
	}
	if plainLine != "╰"+strings.Repeat("─", 38)+"╯" {
		t.Fatalf("bottomBorderLine() = %q, want solid border", plainLine)
	}
}

func TestBottomBorderLinePositionsLeftAndRightControls(t *testing.T) {
	line := bottomBorderLine(72, appBorderStyle,
		borderControls{parts: []string{"Enter run", "Esc cancel"}},
		borderControls{parts: []string{"q quit"}},
	)
	plainLine := ansi.Strip(line)

	for _, want := range []string{"Enter run · Esc cancel", "q quit"} {
		if !strings.Contains(plainLine, want) {
			t.Fatalf("bottomBorderLine() missing %q:\n%s", want, line)
		}
	}
	if !strings.Contains(plainLine, "─ q quit ─╯") {
		t.Fatalf("bottomBorderLine() should right-align right controls inside the border:\n%s", line)
	}
	if width := lipgloss.Width(line); width != 72 {
		t.Fatalf("bottomBorderLine() width = %d, want 72:\n%s", width, line)
	}
}

func TestSectionBottomLineKeepsWidthWithLongFooter(t *testing.T) {
	line := sectionBottomLine("↵ go · o editor · d delete · y abs path · p PR", 32)

	if width := lipgloss.Width(line); width != 32 {
		t.Fatalf("sectionBottomLine() width = %d, want 32:\n%s", width, line)
	}
	if !strings.Contains(line, "╰─") || !strings.Contains(line, "╯") {
		t.Fatalf("sectionBottomLine() should keep bottom border geometry:\n%s", line)
	}
}

func TestCenteredOverlayPreservesBackgroundOutsidePopupHalo(t *testing.T) {
	output := centeredOverlay("01234567890123456789012345", "POP", 26, 1)

	if output != "0123456789 POP 56789012345" {
		t.Fatalf("centeredOverlay() = %q, want background preserved outside one-cell popup halo", output)
	}
}

func TestCenteredOverlayPreservesBackgroundAboveAndBelowPopup(t *testing.T) {
	output := centeredOverlay("aaaaaaaaaa\nbbbbbbbbbb\ncccccccccc", "XX", 10, 3)
	want := "aaaaaaaaaa\nbbb XX bbb\ncccccccccc"

	if output != want {
		t.Fatalf("centeredOverlay() = %q, want %q", output, want)
	}
}

func TestCreateDialogRenderShowsTypedBranchName(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.createDialog.input.SetValue("feature/login")

	output := model.renderCreateAtWidth(72)

	if !strings.Contains(output, "feature/login") {
		t.Fatalf("renderCreateAtWidth() should show typed branch name:\n%s", output)
	}
}

func TestCreateDialogRenderShowsLivePathPreview(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.state.Repo.Root = "/repo/git-treehouse"
	model.createDialog.input.SetValue("feature/login")

	output := model.renderCreateAtWidth(100)

	want := filepath.Join("/repo", ".worktrees", "git-treehouse", "feature-login")
	if !strings.Contains(output, want) {
		t.Fatalf("renderCreateAtWidth() should show path %q:\n%s", want, output)
	}
}

func TestCreateDialogConfigShortcutCreatesAndOpensConfig(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.config = appconfig.Config{
		Editor:       "true",
		PathTemplate: "{repo_parent}/custom/{branch}",
	}

	_, cmd := model.updateCreate(tea.KeyMsg{Type: tea.KeyCtrlO})
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

	path, err := appconfig.Path()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(content), `path_template = "{repo_parent}/custom/{branch}"`) {
		t.Fatalf("config should contain current path template:\n%s", content)
	}
}

func TestOpenDeleteDefaultsBranchDeletionForRegularWorktree(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{
			Path:               "/repo/feature",
			Branch:             "feature",
			BranchMergedToMain: true,
			PR:                 &gitdata.PullRequest{Number: 42, State: "○", CI: "✓"},
		},
	})
	model.showPR = true

	model, cmd := model.openDelete()

	if cmd != nil {
		t.Fatalf("openDelete() returned command, want nil")
	}
	if model.deleteDialog == nil {
		t.Fatal("openDelete() did not open delete dialog")
	}
	if model.deleteDialog.stage != deleteStageOptions {
		t.Fatalf("delete stage = %v, want options", model.deleteDialog.stage)
	}
	if !model.deleteDialog.deleteBranch {
		t.Fatal("merged branch worktree should default to deleting the branch")
	}
	output := model.renderDeleteAtWidth(80)
	for _, want := range []string{
		"Path:",
		"/repo/feature",
		"Branch:",
		"feature",
		"PR:",
		"#42 ○ ✓",
		"Worktree",
		"t",
		"toggle",
		"[x] remove worktree",
		"Command:",
		"git worktree remove",
		"Branch",
		"b",
		"[x] delete local branch",
		"git branch -d feature",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("delete dialog missing %q:\n%s", want, output)
		}
	}

	model, _ = model.updateDelete(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if model.deleteDialog.deleteBranch {
		t.Fatal("b should uncheck branch deletion")
	}
}

func TestDeleteSectionHeaderStylesShortcutInline(t *testing.T) {
	output := deleteSectionHeader("Worktree", "t", true)

	for _, want := range []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true).Render("Worktree"),
		hintStyle.Render(" · "),
		keyStyle.Render("t"),
		hintStyle.Render(" toggle"),
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("deleteSectionHeader() missing %q:\n%s", want, output)
		}
	}
}

func TestRenderDeleteToggleBlockIndentsDetailsAndCommands(t *testing.T) {
	lines := renderDeleteToggleBlock(deleteToggleBlock{
		title:   "Branch",
		key:     "b",
		enabled: true,
		checked: false,
		label:   "force delete local branch",
		details: []string{
			"Not merged into main, branch will be kept.",
			hintStyle.Render("No branch command will run."),
		},
		commands: []deleteCommand{{text: "git branch -D feature", danger: true}},
	})

	if len(lines) != 5 {
		t.Fatalf("renderDeleteToggleBlock() lines = %d, want 5: %#v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[2], "    ") || !strings.Contains(lines[2], "Not merged into main") {
		t.Fatalf("first detail line should be indented: %#v", lines[2])
	}
	if !strings.HasPrefix(lines[3], "    ") || !strings.Contains(lines[3], "No branch command") {
		t.Fatalf("second detail line should be indented: %#v", lines[3])
	}
	if !strings.HasPrefix(lines[4], "    ") || !strings.Contains(lines[4], "git branch -D feature") {
		t.Fatalf("command line should be indented: %#v", lines[4])
	}
}

func TestOpenDeleteDefaultsUnmergedBranchDeletionOff(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/feature", Branch: "feature"},
	})

	model, _ = model.openDelete()

	if model.deleteDialog.deleteBranch {
		t.Fatal("unmerged branch worktree should default to keeping the branch")
	}
	output := model.renderDeleteAtWidth(80)
	for _, want := range []string{
		"[ ] force delete local branch",
		"    Not merged into main, branch will be kept.",
		"    No branch command will run.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("unmerged delete dialog missing %q:\n%s", want, output)
		}
	}

	model, _ = model.updateDelete(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	output = model.renderDeleteAtWidth(80)
	for _, want := range []string{
		"[x] force delete local branch",
		"Not merged into main.",
		"git branch -D feature",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("checked unmerged delete dialog missing %q:\n%s", want, output)
		}
	}
}

func TestOpenDeleteShowsDirtyWarningInSingleModal(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/feature", Branch: "feature", Status: gitdata.StatusCounts{Modified: 1}, BranchMergedToMain: true},
	})

	model, _ = model.openDelete()

	if model.deleteDialog.stage != deleteStageOptions {
		t.Fatalf("delete stage = %v, want options", model.deleteDialog.stage)
	}
	if model.deleteDialog.deleteWorktree {
		t.Fatal("dirty worktree should default worktree removal off")
	}
	if model.deleteDialog.deleteBranch {
		t.Fatal("dirty worktree should keep branch deletion off until worktree removal is enabled")
	}
	output := model.renderDeleteAtWidth(80)
	for _, want := range []string{
		"Path:",
		"Branch:",
		"PR:",
		"Uncommitted changes will be discarded when removing the worktree.",
		"~ modified 1",
		"[ ] remove worktree",
		"    No worktree command will run.",
		"disabled",
		"    Enable worktree removal first",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("dirty delete dialog missing %q:\n%s", want, output)
		}
	}

	model, cmd := model.updateDelete(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

	if cmd != nil {
		t.Fatalf("disabled branch toggle returned command, want nil")
	}
	if model.deleteDialog.error != "enable worktree removal before deleting the branch" {
		t.Fatalf("disabled branch toggle error = %q", model.deleteDialog.error)
	}

	model, _ = model.updateDelete(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if !model.deleteDialog.deleteWorktree {
		t.Fatal("t should enable worktree removal")
	}
	if !model.deleteDialog.deleteBranch {
		t.Fatal("enabling a dirty merged worktree should default branch deletion on")
	}
	output = model.renderDeleteAtWidth(80)
	for _, want := range []string{
		"[x] remove worktree",
		"git worktree remove --force",
		"[x] delete local branch",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("enabled dirty delete dialog missing %q:\n%s", want, output)
		}
	}
}

func TestPrunableDeleteOmitsBranchControls(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/missing", Branch: "stale", Prunable: true, PruneReason: "directory missing"},
	})

	model, _ = model.openDelete()

	if model.deleteDialog.stage != deleteStagePrune {
		t.Fatalf("delete stage = %v, want prune", model.deleteDialog.stage)
	}
	output := model.renderDeleteAtWidth(80)
	for _, want := range []string{"[x] prune missing worktree metadata", "Reason: directory missing", "Enter", "prune"} {
		if !strings.Contains(output, want) {
			t.Fatalf("prunable delete dialog missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"delete local branch", "[ ]", "b toggle", "Force"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("prunable delete dialog should not contain %q:\n%s", unwanted, output)
		}
	}
}

func TestDetachedDeleteShowsDetachedBranchMetadataWithoutBranchControls(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/detached", Head: "abcdef123456", Detached: true},
	})

	model, _ = model.openDelete()

	output := model.renderDeleteAtWidth(80)
	for _, want := range []string{"Branch:", "abcdef1 detached", "[x] remove worktree"} {
		if !strings.Contains(output, want) {
			t.Fatalf("detached delete dialog missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"delete local branch", "force delete local branch", "b toggle"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("detached delete dialog should not contain %q:\n%s", unwanted, output)
		}
	}
}

func TestLockedDeleteShowsBlockingModal(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/locked", Branch: "locked", Locked: true, LockReason: "manual lock"},
	})

	model, _ = model.openDelete()

	if model.deleteDialog == nil || model.deleteDialog.stage != deleteStageLocked {
		t.Fatalf("delete dialog = %+v, want locked stage", model.deleteDialog)
	}
	output := model.renderDeleteAtWidth(80)
	for _, want := range []string{"Cannot delete locked worktree.", "Unlock this worktree", "Reason: manual lock"} {
		if !strings.Contains(output, want) {
			t.Fatalf("locked delete dialog missing %q:\n%s", want, output)
		}
	}

	model, cmd := model.updateDelete(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		t.Fatalf("locked Enter returned command, want nil")
	}
	if model.loading != "" {
		t.Fatalf("locked Enter should not start deletion, loading = %q", model.loading)
	}
}

func TestDeleteRowPrunableOnlyPrunes(t *testing.T) {
	runner := &recordingRunner{}
	row := gitdata.Worktree{Path: "/repo/missing", Branch: "stale", Prunable: true}
	dialog := deleteDialog{stage: deleteStagePrune, deleteBranch: true}

	err := deleteRow(context.Background(), gitdata.Repository{Root: "/repo/main"}, row, dialog, runner)

	if err != nil {
		t.Fatalf("deleteRow() error = %v", err)
	}
	want := []string{"/repo/main|git worktree prune"}
	if got := strings.Join(runner.commands, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("commands = %v, want %v", runner.commands, want)
	}
}

func TestDeleteRowUsesSafeBranchDeleteForMergedBranch(t *testing.T) {
	runner := &recordingRunner{}
	row := gitdata.Worktree{Path: "/repo/feature", Branch: "feature", BranchMergedToMain: true}
	dialog := deleteDialog{stage: deleteStageOptions, deleteWorktree: true, deleteBranch: true}

	err := deleteRow(context.Background(), gitdata.Repository{Root: "/repo/main"}, row, dialog, runner)

	if err != nil {
		t.Fatalf("deleteRow() error = %v", err)
	}
	want := []string{
		"/repo/main|git worktree remove /repo/feature",
		"/repo/main|git branch -d feature",
	}
	if got := strings.Join(runner.commands, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("commands = %v, want %v", runner.commands, want)
	}
}

func TestDeleteRowDoesNotDeleteBranchWhenWorktreeRemovalIsOff(t *testing.T) {
	runner := &recordingRunner{}
	row := gitdata.Worktree{Path: "/repo/feature", Branch: "feature", BranchMergedToMain: true}
	dialog := deleteDialog{stage: deleteStageOptions, deleteWorktree: false, deleteBranch: true}

	err := deleteRow(context.Background(), gitdata.Repository{Root: "/repo/main"}, row, dialog, runner)

	if err != nil {
		t.Fatalf("deleteRow() error = %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %v, want none when worktree removal is off", runner.commands)
	}
}

func TestDeleteRowUsesForceBranchDeleteForUnmergedBranch(t *testing.T) {
	runner := &recordingRunner{}
	row := gitdata.Worktree{Path: "/repo/feature", Branch: "feature"}
	dialog := deleteDialog{stage: deleteStageOptions, deleteWorktree: true, deleteBranch: true}

	err := deleteRow(context.Background(), gitdata.Repository{Root: "/repo/main"}, row, dialog, runner)

	if err != nil {
		t.Fatalf("deleteRow() error = %v", err)
	}
	want := []string{
		"/repo/main|git worktree remove /repo/feature",
		"/repo/main|git branch -D feature",
	}
	if got := strings.Join(runner.commands, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("commands = %v, want %v", runner.commands, want)
	}
}

func TestLoadConfigIfChangedReloadsModifiedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`path_template = "{repo_parent}/old/{branch}"`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	previousModTime := info.ModTime()
	if err := os.WriteFile(path, []byte(`path_template = "{repo_parent}/new/{branch}"`), 0600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	nextModTime := previousModTime.Add(time.Second)
	if err := os.Chtimes(path, nextModTime, nextModTime); err != nil {
		t.Fatalf("set config mtime: %v", err)
	}

	config, _, changed, err := loadConfigIfChanged(path, previousModTime)

	if err != nil {
		t.Fatalf("loadConfigIfChanged() error = %v", err)
	}
	if !changed {
		t.Fatal("loadConfigIfChanged() changed = false, want true")
	}
	if config.PathTemplate != "{repo_parent}/new/{branch}" {
		t.Fatalf("loaded PathTemplate = %q, want new template", config.PathTemplate)
	}
}

func TestConfigReloadedMessageUpdatesCreatePathPreview(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.state.Repo.Root = "/repo/git-treehouse"
	model.createDialog.input.SetValue("feature/login")

	updated, _ := model.Update(configReloadedMsg{config: appconfig.Config{
		PathTemplate: "~/.worktrees/{repo_name}/{branch}",
	}})
	model = updated.(Model)

	output := model.renderCreateAtWidth(120)
	if !strings.Contains(output, ".worktrees/git-treehouse/feature-login") {
		t.Fatalf("renderCreateAtWidth() should use reloaded path template:\n%s", output)
	}
}

func TestPullRequestLoadStoresSessionCache(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/feature", Branch: "feature"},
	})
	pullRequests := map[string]gitdata.PullRequest{
		"feature": {Number: 42, State: "○", CI: "✓"},
	}

	updated, _ := updateModel(t, model, prLoadedMsg{
		pullRequests: pullRequests,
		enabled:      true,
		repoRoot:     model.state.Repo.Root,
		id:           model.enrichmentID,
		checkedAt:    time.Now(),
	})

	if !updated.showPR {
		t.Fatal("PR load should show PR column")
	}
	if updated.prCacheRepoRoot != "/repo/main" || updated.prCache["feature"].Number != 42 {
		t.Fatalf("PR cache = root %q data %+v, want feature #42", updated.prCacheRepoRoot, updated.prCache)
	}
	if updated.state.Rows[0].PR == nil || updated.state.Rows[0].PR.Number != 42 {
		t.Fatalf("row PR = %+v, want #42", updated.state.Rows[0].PR)
	}
}

func TestReloadAppliesSessionPullRequestCache(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/feature", Branch: "feature"},
	})
	model.prCacheRepoRoot = "/repo/main"
	model.prCache = map[string]gitdata.PullRequest{
		"feature": {Number: 42, State: "○"},
	}
	nextState := gitdata.State{
		Repo: gitdata.Repository{Root: "/repo/main", ActiveWorktree: "/repo/main"},
		Rows: []gitdata.Worktree{{Path: "/repo/feature", Branch: "feature"}},
	}

	updated, _ := updateModel(t, model, reloadMsg{state: nextState})

	if updated.state.Rows[0].PR == nil || updated.state.Rows[0].PR.Number != 42 {
		t.Fatalf("cached PR was not attached after reload: %+v", updated.state.Rows[0].PR)
	}
	if !updated.showPR {
		t.Fatal("cached PR should keep PR column visible")
	}
}

func TestReloadReservesPullRequestColumnForRemoteRepository(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/feature", Branch: "feature"},
	})
	nextState := gitdata.State{
		Repo: gitdata.Repository{
			Root:             "/repo/main",
			ActiveWorktree:   "/repo/main",
			RemoteConfigured: true,
		},
		Rows: []gitdata.Worktree{{Path: "/repo/feature", Branch: "feature"}},
	}

	updated, _ := updateModel(t, model, reloadMsg{state: nextState})

	if !updated.showPR {
		t.Fatal("remote reload should reserve PR column before PR data loads")
	}
}

func TestDisabledPullRequestLoadKeepsExistingCache(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/feature", Branch: "feature"},
	})
	model.prCacheRepoRoot = "/repo/main"
	model.prCache = map[string]gitdata.PullRequest{
		"feature": {Number: 42, State: "○"},
	}

	updated, _ := updateModel(t, model, prLoadedMsg{
		enabled:   false,
		repoRoot:  model.state.Repo.Root,
		id:        model.enrichmentID,
		checkedAt: time.Now(),
	})

	if updated.state.Rows[0].PR == nil || updated.state.Rows[0].PR.Number != 42 {
		t.Fatalf("disabled PR load should reuse cache, got %+v", updated.state.Rows[0].PR)
	}
	if !updated.showPR {
		t.Fatal("disabled PR load with cache should keep PR column visible")
	}
}

func TestStalePullRequestLoadIsIgnored(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/feature", Branch: "feature"},
	})
	model.enrichmentID = 4

	updated, _ := updateModel(t, model, prLoadedMsg{
		pullRequests: map[string]gitdata.PullRequest{"feature": {Number: 42, State: "○"}},
		enabled:      true,
		repoRoot:     model.state.Repo.Root,
		id:           3,
		checkedAt:    time.Now(),
	})

	if updated.state.Rows[0].PR != nil {
		t.Fatalf("stale PR message attached PR: %+v", updated.state.Rows[0].PR)
	}
	if updated.showPR {
		t.Fatal("stale PR message should not show PR column")
	}
}

func TestStaleSizeLoadIsIgnored(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/feature", Branch: "feature"},
	})
	model.enrichmentID = 4

	updated, _ := updateModel(t, model, sizesLoadedMsg{
		gitSizes: map[string]int64{"/repo/feature": 1024},
		repoRoot: model.state.Repo.Root,
		id:       3,
	})

	if updated.state.Rows[0].GitSizeLoaded {
		t.Fatalf("stale size message marked row loaded: %+v", updated.state.Rows[0])
	}
}

func TestDiskUsagePathsPrioritizeVisibleRows(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/one", Branch: "one"},
		{Path: "/repo/prunable", Branch: "prunable", Prunable: true},
		{Path: "/repo/visible", Branch: "visible"},
		{Path: "/repo/loaded", Branch: "loaded", GitSizeLoaded: true},
		{Path: "/repo/background", Branch: "background"},
	})
	model.width = 160
	model.height = 6
	model.selected = 2

	visible, background := model.diskUsagePaths(now)

	if got := strings.Join(visible, ","); got != "/repo/visible" {
		t.Fatalf("visible disk paths = %q, want /repo/visible", got)
	}
	if got := strings.Join(background, ","); got != "/repo/one,/repo/background" {
		t.Fatalf("background disk paths = %q, want /repo/one,/repo/background", got)
	}
}

func TestReloadCommandFetchesBeforeStableRefreshLoad(t *testing.T) {
	worktreeList := "worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n"
	runner := &recordingRunner{results: map[string]recordingResult{
		"/repo/main|git fetch --prune":                                     {},
		"/repo/main|git rev-parse --show-toplevel":                         {output: "/repo/main\n"},
		"/repo/main|git rev-parse --git-common-dir":                        {output: ".git\n"},
		"/repo/main|git rev-parse --path-format=absolute --git-common-dir": {output: "/repo/main/.git\n"},
		"/repo/main|git worktree list --porcelain":                         {output: worktreeList},
		"/repo/main|git symbolic-ref --short refs/remotes/origin/HEAD":     {err: errors.New("no origin")},
		"/repo/main|git show-ref --verify --quiet refs/heads/main":         {},
		"/repo/main|git remote":                                            {output: "origin\n"},
	}}
	cmd := reloadCmd("/repo/main", appconfig.Config{}, runner, gitdata.Repository{
		Root:             "/repo/main",
		RemoteConfigured: true,
	}, true, false, 9)

	message := cmd().(reloadMsg)

	if message.err != nil {
		t.Fatalf("reloadCmd() error = %v", message.err)
	}
	if len(runner.commands) == 0 || runner.commands[0] != "/repo/main|git fetch --prune" {
		t.Fatalf("first command = %q, want fetch: %v", runner.commands[0], runner.commands)
	}
	if len(message.state.Rows) != 1 || !message.state.Rows[0].LocalMetadataLoaded {
		t.Fatalf("reloadCmd should return stable local metadata: %+v", message.state.Rows)
	}
	worktreeListCalls := 0
	for _, command := range runner.commands {
		if strings.Contains(command, "git worktree list --porcelain") {
			worktreeListCalls++
		}
	}
	if worktreeListCalls != 1 {
		t.Fatalf("worktree list calls = %d, want 1: %v", worktreeListCalls, runner.commands)
	}
}

func TestNextClockTickDelayUsesMinuteBoundaryAfterFirstMinute(t *testing.T) {
	lastRefresh := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	if got := nextClockTickDelay(lastRefresh, lastRefresh.Add(30*time.Second)); got != time.Second {
		t.Fatalf("delay under one minute = %s, want 1s", got)
	}
	if got := nextClockTickDelay(lastRefresh, lastRefresh.Add(90*time.Second)); got != 30*time.Second {
		t.Fatalf("delay after one minute = %s, want next minute boundary", got)
	}
}

func TestAppBottomLineEmbedsStatusOnly(t *testing.T) {
	model := Model{width: 200}

	output := model.appBottomLine(200)
	plainOutput := ansi.Strip(output)

	for _, want := range []string{"╰", "─", "╯"} {
		if !strings.Contains(output, want) {
			t.Fatalf("appBottomLine() missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(plainOutput, "Esc") || strings.Contains(plainOutput, "close/clear") {
		t.Fatalf("appBottomLine() should not show default Esc hint:\n%s", output)
	}
	if strings.Contains(plainOutput, "╰─  ─") {
		t.Fatalf("appBottomLine() should not leave a blank label gap:\n%s", output)
	}
	if !strings.HasPrefix(plainOutput, "╰") || !strings.HasSuffix(plainOutput, "╯") {
		t.Fatalf("appBottomLine() should render the bottom frame rule:\n%s", output)
	}
	for _, unwanted := range []string{"g/G", "top/bottom", "h root", "m main", "a active", "Tab filter", "s search", "⌂ root", "+ staged", "~ modified", "? untracked", "remote"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("appBottomLine() should not include moved or hidden control %q:\n%s", unwanted, output)
		}
	}
	for _, unwanted := range []string{"└┘", "╰─┘", "└─╯"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("appBottomLine() should not contain cap separator %q:\n%s", unwanted, output)
		}
	}
	if width := lipgloss.Width(output); width != 200 {
		t.Fatalf("appBottomLine() width = %d, want 200:\n%s", width, output)
	}
}

func modelWithCreateDialog(bases []gitdata.BaseOption) Model {
	input := textinput.New()
	input.Prompt = ""
	input.Cursor.Style = flashStyle
	input.Focus()
	return Model{
		width:  100,
		height: 24,
		runner: testRunner{},
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/main"},
		},
		createDialog: &createDialog{
			input: input,
			bases: bases,
		},
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
			name:  "delete dialog",
			model: Model{refreshID: 7, deleteDialog: &deleteDialog{}},
		},
		{
			name:  "command palette",
			model: Model{refreshID: 7, paletteDialog: &paletteDialog{}},
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
	if updated.refreshFlash != "✓ refreshed" {
		t.Fatalf("refresh flash = %q, want refreshed badge", updated.refreshFlash)
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

	if updated.refreshFlash != "✓ refreshed" {
		t.Fatalf("manual reload refresh flash = %q, want refreshed", updated.refreshFlash)
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

func testModelWithRows(rows []gitdata.Worktree) Model {
	for index := range rows {
		rows[index].LocalMetadataLoaded = true
	}
	return New(gitdata.State{
		Repo: gitdata.Repository{
			Root:           "/repo/main",
			ActiveWorktree: "/repo/main",
		},
		Rows: rows,
	}, appconfig.Config{}, nil)
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
