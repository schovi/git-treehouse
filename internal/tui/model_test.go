package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	appconfig "github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
)

type testRunner struct{}

func (runner testRunner) Run(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
	return nil, errors.New("unexpected command")
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
		Path:     "/repo/main",
		Branch:   "main",
		IsActive: true,
		IsMain:   true,
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

	for _, want := range []string{"treehouse", "main", "1 worktrees", "root:", "n", "new", "r", "refresh", "12 seconds ago", "?", "help", "q", "quit"} {
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
			Repo: gitdata.Repository{Root: "/repo/git-treehouse"},
			Rows: []gitdata.Worktree{{Branch: "main", IsMain: true}},
		},
	}

	output := model.appTopLineAtTime(1, 120, now)

	for _, want := range []string{"╭─", "treehouse", "r", "refresh", "12 seconds ago", "─╮"} {
		if !strings.Contains(output, want) {
			t.Fatalf("appTopLineAtTime() missing %q:\n%s", want, output)
		}
	}
	if width := lipgloss.Width(output); width != 120 {
		t.Fatalf("appTopLineAtTime() width = %d, want 120:\n%s", width, output)
	}
}

func TestStatusBarSplitsAppControlsAndDirtyLegend(t *testing.T) {
	model := Model{width: 180}

	output := model.statusBar()

	for _, want := range []string{"m", "main", "a", "active", "Esc", "close/clear", "⌂", "root", "!", "locked", "×", "prunable", "remote", "+", "staged", "~", "modified", "untracked"} {
		if !strings.Contains(output, want) {
			t.Fatalf("statusBar() missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"help", "quit", "g/G", "top/bottom", "Tab", "filter:", "s search"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("statusBar() should not include hidden or moved controls %q:\n%s", unwanted, output)
		}
	}
	if strings.Contains(output, "delete") || strings.Contains(output, "editor") {
		t.Fatalf("statusBar() should not contain persistent keybindings:\n%s", output)
	}
}

func TestSearchInputRendersInWorktreesFooter(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true},
	})
	model.width = 120
	model.height = 14
	model.searching = true
	model.search.SetValue("docs")

	output := model.View()

	for _, want := range []string{"search", "docs", "Esc", "clear", "Tab", "filter:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("View() missing search footer element %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Enter apply") {
		t.Fatalf("View() should not show apply for live search:\n%s", output)
	}
	status := model.statusBar()
	for _, unwanted := range []string{"search docs", "Enter apply", "Esc clear"} {
		if strings.Contains(status, unwanted) {
			t.Fatalf("statusBar() should not include search footer element %q:\n%s", unwanted, status)
		}
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

	for _, want := range []string{"treehouse", "Worktrees", "Details", "╭─", "╰", "Current", "Tab", "filter:", "s", "search", " · "} {
		if !strings.Contains(output, want) {
			t.Fatalf("View() missing boxed app element %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "g/G") || strings.Contains(output, "top/bottom") {
		t.Fatalf("View() should hide top/bottom hint outside help:\n%s", output)
	}
	for _, unwanted := range []string{"╭─┐", "└┘", "└─╯"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("View() should not contain cap separator %q:\n%s", unwanted, output)
		}
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
				Path:          "/repo/main",
				Branch:        "main",
				IsMain:        true,
				IsActive:      true,
				CommitShort:   "abc1234",
				CommitSubject: "boxed app",
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

func TestAppBottomLineEmbedsStatusWithDotSeparators(t *testing.T) {
	model := Model{width: 200}

	output := model.appBottomLine(200)

	for _, want := range []string{"╰─ ", " · ", "m", "main", "+", "staged", "~", "modified", "? untracked", " ─╯"} {
		if !strings.Contains(output, want) {
			t.Fatalf("appBottomLine() missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"g/G", "top/bottom", "Tab filter", "s search"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("appBottomLine() should not include moved or hidden control %q:\n%s", unwanted, output)
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

	for _, want := range []string{"╭─", "treehouse", "─╮"} {
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

func testModelWithRows(rows []gitdata.Worktree) Model {
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
