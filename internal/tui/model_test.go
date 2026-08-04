package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	"github.com/schovi/git-treehouse/internal/github"
	"github.com/schovi/git-treehouse/internal/listview"
)

type testRunner struct{}

func (runner testRunner) Run(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
	return nil, errors.New("unexpected command")
}

func (runner testRunner) RunWithEnv(_ context.Context, _ string, _ []string, _ string, _ ...string) ([]byte, error) {
	return nil, errors.New("unexpected command")
}

type recordingRunner struct {
	mutex       sync.Mutex
	commands    []string
	envCommands []recordedEnvCommand
	results     map[string]recordingResult
}

type recordingResult struct {
	output string
	err    error
}

type recordedEnvCommand struct {
	command string
	env     []string
}

func (runner *recordingRunner) Run(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	return runner.run(dir, nil, name, args...)
}

func (runner *recordingRunner) RunWithEnv(_ context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	return runner.run(dir, env, name, args...)
}

type cancelledHookRunner struct {
	*recordingRunner
}

func (runner cancelledHookRunner) RunWithEnv(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	if name == "sh" && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return runner.recordingRunner.RunWithEnv(ctx, dir, env, name, args...)
}

func (runner *recordingRunner) run(dir string, env []string, name string, args ...string) ([]byte, error) {
	key := dir + "|" + name + " " + strings.Join(args, " ")
	runner.mutex.Lock()
	runner.commands = append(runner.commands, key)
	if env != nil {
		runner.envCommands = append(runner.envCommands, recordedEnvCommand{command: key, env: append([]string(nil), env...)})
	}
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

func TestSelectedInspectorRendersSafeToRemoveHint(t *testing.T) {
	forceANSIProfile(t)
	model := Model{width: 100}
	tests := []struct {
		name string
		row  gitdata.Worktree
		want string
	}{
		{
			name: "merged to main",
			row:  gitdata.Worktree{Path: "/repo/merged", Branch: "merged", LocalMetadataLoaded: true, BranchMergedToMain: true},
			want: "finished: clean, merged to main — safe to remove (d)",
		},
		{
			name: "pull request closed",
			row:  gitdata.Worktree{Path: "/repo/closed", Branch: "closed", LocalMetadataLoaded: true, PR: &gitdata.PullRequest{State: "✕"}},
			want: "finished: clean, PR merged/closed — safe to remove (d)",
		},
		{
			name: "upstream gone",
			row:  gitdata.Worktree{Path: "/repo/gone", Branch: "gone", LocalMetadataLoaded: true, BranchMergedToMain: true, UpstreamGone: true},
			want: "finished: clean, merged; remote branch deleted — safe to remove (d)",
		},
		{name: "dirty", row: gitdata.Worktree{Path: "/repo/dirty", Branch: "dirty", LocalMetadataLoaded: true, BranchMergedToMain: true, Status: gitdata.StatusCounts{Modified: 1}}},
		{name: "unmerged", row: gitdata.Worktree{Path: "/repo/unmerged", Branch: "unmerged", LocalMetadataLoaded: true}},
		{name: "root", row: gitdata.Worktree{Path: "/repo/main", Branch: "main", LocalMetadataLoaded: true, IsMain: true, BranchMergedToMain: true}},
		{name: "active", row: gitdata.Worktree{Path: "/repo/active", Branch: "active", LocalMetadataLoaded: true, IsActive: true, BranchMergedToMain: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := model.selectedRowInspectorAtWidth(gitdata.Row{Kind: gitdata.RowKindWorktree, Worktree: test.row}, time.Now(), 100)
			if test.want == "" {
				if strings.Contains(ansi.Strip(output), "safe to remove") {
					t.Fatalf("safe-to-remove hint rendered unexpectedly:\n%s", output)
				}
				return
			}
			if !strings.Contains(output, inspectorCleanStyle.Render(test.want)) {
				t.Fatalf("safe-to-remove hint missing styled text %q:\n%s", test.want, output)
			}
		})
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

	for _, want := range []string{"Branch", "main", "Dirty", "none", "Delete"} {
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
	for _, unwanted := range []string{"n new worktree", "Current", "root repository", "Root repository"} {
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

func TestViewRendersNewWorktreeActionInWorktreesFooter(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true},
	})
	model.width = 120
	model.height = 24

	output := ansi.Strip(model.View())
	footerLine := ""
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "n new worktree") && strings.Contains(line, "h root") {
			footerLine = line
			break
		}
	}

	if footerLine == "" {
		t.Fatalf("View() should render new action and navigation controls in the Worktrees footer:\n%s", output)
	}
	newIndex := strings.Index(footerLine, "n new worktree")
	rootIndex := strings.Index(footerLine, "h root")
	if newIndex < 0 || rootIndex < 0 || newIndex >= rootIndex {
		t.Fatalf("Worktrees footer should place new action before right navigation controls:\n%s", footerLine)
	}
	if !strings.Contains(footerLine[newIndex:rootIndex], "─") {
		t.Fatalf("Worktrees footer should visually separate collection and navigation controls:\n%s", footerLine)
	}
}

func TestViewRendersBranchDetailActionsInDetailsFooter(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true},
	})
	model.state.Branches = []gitdata.Branch{{Name: "feature/branch"}}
	model.filter = filterBranches
	model.width = 100
	model.height = 24

	output := model.View()
	footerLine := ""
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "↵") && strings.Contains(line, "create+go") {
			footerLine = line
			break
		}
	}

	if footerLine == "" {
		t.Fatalf("View() should render branch detail actions in the Details footer:\n%s", output)
	}
	for _, want := range []string{"╰─", "↵", "create+go", "c", "checkout root", "d", "delete", "y", "name", "p", "PR", "╯"} {
		if !strings.Contains(footerLine, want) {
			t.Fatalf("Branch Details footer missing %q:\n%s", want, footerLine)
		}
	}
	for _, unwanted := range []string{"n new worktree", "o editor", "abs path"} {
		if strings.Contains(footerLine, unwanted) {
			t.Fatalf("Branch Details footer should not contain %q:\n%s", unwanted, footerLine)
		}
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

	for _, want := range []string{"Git treehouse", " · ", "main", "1 worktrees", "root:", "r", "refresh", "12 seconds ago", "?", "help", "q", "quit"} {
		if !strings.Contains(output, want) {
			t.Fatalf("titleLine() missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"n new"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("titleLine() should not contain contextual action %q:\n%s", unwanted, output)
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
	if successFeedbackTimeout != 3*time.Second {
		t.Fatalf("success feedback timeout = %s, want 3s", successFeedbackTimeout)
	}
	if restoreOfferTimeout != 10*time.Second {
		t.Fatalf("restore offer timeout = %s, want 10s", restoreOfferTimeout)
	}
}

func TestAppControlsDropRefreshAgeBeforeCoreControls(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	model := Model{lastRefreshAt: now.Add(-12 * time.Second)}

	wide := model.appControlsAtWidthAtTime(80, now)
	for _, want := range []string{"r", "refresh", "12 seconds ago", "?", "help", "q", "quit"} {
		if !strings.Contains(wide, want) {
			t.Fatalf("wide controls missing %q:\n%s", want, wide)
		}
	}
	if strings.Contains(wide, "n new") {
		t.Fatalf("wide controls should not contain contextual new action:\n%s", wide)
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
	model.feedback = successFeedback(feedbackFrameWorktrees, "refreshed")

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
	model.setFilter(filterModified)

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
	}, appconfig.Config{ShowBranches: true}, nil)

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

func TestEnterOnBranchRowOpensBranchWorktreeDialog(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})
	model.state.Branches = []gitdata.Branch{{Name: "feature/branch"}}
	model.filter = filterBranches

	model, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		t.Fatalf("Enter on branch returned a command, want nil")
	}
	if model.branchWorktreeDialog == nil {
		t.Fatal("Enter on branch should open branch worktree dialog")
	}
	if model.branchWorktreeDialog.branch.Name != "feature/branch" {
		t.Fatalf("branch worktree branch = %q, want feature/branch", model.branchWorktreeDialog.branch.Name)
	}
	if model.branchWorktreeDialog.path != "/repo/.worktrees/main/feature-branch" {
		t.Fatalf("branch worktree path = %q, want default branch path", model.branchWorktreeDialog.path)
	}
}

func TestCOnBranchRowChecksOutRootWhenRootIsClean(t *testing.T) {
	runner := &recordingRunner{}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})
	model.runner = runner
	model.state.Branches = []gitdata.Branch{{Name: "feature/branch"}}
	model.filter = filterBranches

	model, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

	if cmd == nil {
		t.Fatal("c on branch returned nil command")
	}
	if model.loading != "checking out…" {
		t.Fatalf("loading = %q, want checking out…", model.loading)
	}
	if model.checkoutDialog != nil || model.branchWorktreeDialog != nil {
		t.Fatal("clean root checkout should not open a dialog")
	}
	message := cmd().(checkoutMsg)
	if message.err != nil {
		t.Fatalf("checkout command error = %v", message.err)
	}
	if message.path != "/repo/main" {
		t.Fatalf("checkout path = %q, want root path", message.path)
	}
	if len(runner.commands) != 1 || runner.commands[0] != "/repo/main|git switch -- feature/branch" {
		t.Fatalf("commands = %v, want git switch in root", runner.commands)
	}
}

func TestCOnBranchRowShowsDirtyRootCheckoutDialog(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, Status: gitdata.StatusCounts{Modified: 1}},
	})
	model.state.Branches = []gitdata.Branch{{Name: "feature/branch"}}
	model.filter = filterBranches

	model, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

	if cmd != nil {
		t.Fatalf("dirty root checkout returned command, want nil")
	}
	if model.checkoutDialog == nil {
		t.Fatal("c on branch with dirty root should open checkout dialog")
	}
	output := ansi.Strip(model.renderCheckoutAtWidth(100))
	for _, want := range []string{"Checkout root", "Branch", "feature/branch", "Root has uncommitted changes.", "~ modified 1", "s stash", "No checkout command will run."} {
		if !strings.Contains(output, want) {
			t.Fatalf("dirty checkout dialog missing %q:\n%s", want, output)
		}
	}
}

func TestDirtyRootCheckoutRequiresStashBeforeEnter(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main", IsMain: true}})
	model.checkoutDialog = &checkoutDialog{
		branch: gitdata.Branch{Name: "feature/branch"},
		root:   gitdata.Worktree{Path: "/repo/main", Branch: "main", Status: gitdata.StatusCounts{Modified: 1}},
	}

	model, cmd := model.updateCheckout(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		t.Fatalf("Enter without stash returned command, want nil")
	}
	if model.checkoutDialog.error != "enable stash before checking out" {
		t.Fatalf("checkout dialog error = %q, want stash prompt", model.checkoutDialog.error)
	}
}

func TestDirtyRootCheckoutStashesThenSwitches(t *testing.T) {
	runner := &recordingRunner{}
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main", IsMain: true}})
	model.runner = runner
	model.checkoutDialog = &checkoutDialog{
		branch: gitdata.Branch{Name: "feature/branch"},
		root:   gitdata.Worktree{Path: "/repo/main", Branch: "main", Status: gitdata.StatusCounts{Modified: 1}},
	}

	model, cmd := model.updateCheckout(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd != nil {
		t.Fatalf("stash toggle returned command, want nil")
	}
	model, cmd = model.updateCheckout(tea.KeyMsg{Type: tea.KeyEnter})

	if model.loading != "checking out…" {
		t.Fatalf("loading = %q, want checking out…", model.loading)
	}
	if cmd == nil {
		t.Fatal("Enter with stash returned nil command")
	}
	message := cmd().(checkoutMsg)
	if message.err != nil {
		t.Fatalf("checkout command error = %v", message.err)
	}
	if message.path != "/repo/main" {
		t.Fatalf("checkout path = %q, want root path", message.path)
	}
	want := []string{
		"/repo/main|git stash push -u -m git-treehouse: before switching to feature/branch",
		"/repo/main|git switch -- feature/branch",
	}
	if got := strings.Join(runner.commands, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("commands = %v, want stash then switch", runner.commands)
	}
}

func TestSelectedCopyTextUsesBranchNameForBranchRows(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})

	text, message, ok := model.selectedCopyText()
	if !ok || text != "/repo/main" || message != "copied absolute path: /repo/main" {
		t.Fatalf("selectedCopyText() for worktree = %q, %q, %v; want path copy", text, message, ok)
	}

	model.state.Branches = []gitdata.Branch{{Name: "feature/branch"}}
	model.filter = filterBranches

	text, message, ok = model.selectedCopyText()
	if !ok || text != "feature/branch" || message != "copied branch name: feature/branch" {
		t.Fatalf("selectedCopyText() for branch = %q, %q, %v; want branch name copy", text, message, ok)
	}
}

func TestSelectedPullRequestCopyReturnsPullRequestURL(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{
			Path:   "/repo/feature",
			Branch: "feature",
			PR:     &gitdata.PullRequest{URL: "https://github.com/acme/repo/pull/42"},
		},
	})

	text, message, ok := model.selectedPullRequestCopy()

	if !ok || text != "https://github.com/acme/repo/pull/42" || message != "copied PR URL: https://github.com/acme/repo/pull/42" {
		t.Fatalf("selectedPullRequestCopy() = %q, %q, %v; want PR URL copy", text, message, ok)
	}
}

func TestSelectedPullRequestCopyReturnsFalseWithoutURL(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/no-pr", Branch: "no-pr"},
	})

	text, message, ok := model.selectedPullRequestCopy()

	if ok || text != "" || message != "" {
		t.Fatalf("selectedPullRequestCopy() without PR = %q, %q, %v; want no copy", text, message, ok)
	}

	model.state.Rows[0].PR = &gitdata.PullRequest{}

	text, message, ok = model.selectedPullRequestCopy()

	if ok || text != "" || message != "" {
		t.Fatalf("selectedPullRequestCopy() without PR URL = %q, %q, %v; want no copy", text, message, ok)
	}
}

func TestNOnBranchRowShowsEnterHint(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})
	model.state.Branches = []gitdata.Branch{{Name: "feature/branch"}}
	model.filter = filterBranches

	model, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if model.branchWorktreeDialog != nil {
		t.Fatal("n on branch should not open branch worktree dialog")
	}
	if model.flash != "press Enter to create a worktree for this branch" {
		t.Fatalf("flash = %q, want branch Enter hint", model.flash)
	}
	if cmd == nil {
		t.Fatal("n on branch should return flash timeout command")
	}
}

func TestBranchWorktreeDialogAddsExistingBranchWorktree(t *testing.T) {
	runner := &recordingRunner{results: map[string]recordingResult{
		"/repo/main|git worktree add /repo/.worktrees/main/feature-branch feature/branch": {},
	}}
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main", IsMain: true}})
	model.runner = runner
	model.branchWorktreeDialog = &branchWorktreeDialog{
		branch: gitdata.Branch{Name: "feature/branch"},
		path:   "/repo/.worktrees/main/feature-branch",
	}

	model, cmd := model.updateBranchWorktree(tea.KeyMsg{Type: tea.KeyEnter})

	if model.loading != "creating…" {
		t.Fatalf("loading = %q, want creating…", model.loading)
	}
	if cmd == nil {
		t.Fatal("Enter checkout returned nil command")
	}
	message := cmd().(checkoutMsg)
	if message.err != nil {
		t.Fatalf("checkout command error = %v", message.err)
	}
	if message.path != "/repo/.worktrees/main/feature-branch" {
		t.Fatalf("checkout path = %q, want dialog path", message.path)
	}
	if len(runner.commands) != 1 || runner.commands[0] != "/repo/main|git worktree add /repo/.worktrees/main/feature-branch feature/branch" {
		t.Fatalf("commands = %v, want git worktree add existing branch", runner.commands)
	}
}

func TestCreateWorktreeCopiesFilesAndRunsApprovedPostCreate(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "main")
	if err := os.MkdirAll(repoRoot, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".env"), []byte("TOKEN=1\n"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runner := &recordingRunner{}
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.runner = runner
	model.state.Repo.Root = repoRoot
	model.state.Repo.MainBranch = "main"
	model.repoConfig = appconfig.RepoConfig{
		CopyUntracked: []string{".env"},
		PostCreate:    "npm install",
	}
	model.hooksApproved = true
	model.createDialog.input.SetValue("feature/hook")
	path := model.createPathPreview()

	model, cmd := model.updateCreate(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("create returned nil command")
	}
	message := cmd().(createMsg)
	if message.err != nil {
		t.Fatalf("create command error = %v", message.err)
	}
	if !message.created {
		t.Fatal("create command should mark worktree created")
	}
	addCommand := repoRoot + "|git worktree add -b feature/hook " + path + " main"
	hookCommand := path + "|sh -c npm install"
	addIndex := slices.Index(runner.commands, addCommand)
	hookIndex := slices.Index(runner.commands, hookCommand)
	if addIndex == -1 || hookIndex == -1 || hookIndex <= addIndex {
		t.Fatalf("commands = %v, want hook after worktree add", runner.commands)
	}
	copied, err := os.ReadFile(filepath.Join(path, ".env"))
	if err != nil {
		t.Fatalf("ReadFile(copied .env) error = %v", err)
	}
	if string(copied) != "TOKEN=1\n" {
		t.Fatalf("copied .env = %q, want TOKEN", string(copied))
	}
	if len(runner.envCommands) != 1 || runner.envCommands[0].command != hookCommand {
		t.Fatalf("env commands = %+v, want post_create hook", runner.envCommands)
	}
	for _, wantEnv := range []string{
		"GTH_EVENT=post_create",
		"GTH_WORKTREE_PATH=" + path,
		"GTH_WORKTREE_BRANCH=feature/hook",
		"GTH_REPO_ROOT=" + repoRoot,
		"GTH_MAIN_BRANCH=main",
	} {
		if !slices.Contains(runner.envCommands[0].env, wantEnv) {
			t.Fatalf("hook env missing %q: %#v", wantEnv, runner.envCommands[0].env)
		}
	}
}

func TestCreateWorktreeSkipsUnapprovedPostCreateWithWarning(t *testing.T) {
	runner := &recordingRunner{}
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.runner = runner
	model.repoConfig = appconfig.RepoConfig{PostCreate: "npm install"}
	model.hooksApproved = false
	model.createDialog.input.SetValue("feature/hook")
	path := model.createPathPreview()

	model, cmd := model.updateCreate(tea.KeyMsg{Type: tea.KeyEnter})

	message := cmd().(createMsg)
	if message.err != nil {
		t.Fatalf("create command error = %v", message.err)
	}
	if !message.created {
		t.Fatal("create command should mark worktree created")
	}
	addCommand := "/repo/main|git worktree add -b feature/hook " + path + " main"
	if !slices.Contains(runner.commands, addCommand) {
		t.Fatalf("commands = %v, want git worktree add", runner.commands)
	}
	if len(runner.envCommands) != 0 {
		t.Fatalf("env commands = %+v, want hook skipped", runner.envCommands)
	}
	if len(message.warnings) != 1 || message.warnings[0] != "post_create hook not approved; run git-treehouse allow" {
		t.Fatalf("warnings = %#v, want unapproved hook warning", message.warnings)
	}
	updated, _ := updateModel(t, model, message)
	if updated.selectedPath != path {
		t.Fatalf("selectedPath = %q, want created path", updated.selectedPath)
	}
	if !strings.Contains(updated.flash, "post_create hook not approved") {
		t.Fatalf("flash = %q, want warning", updated.flash)
	}
}

func TestCreateWorktreeHookFailureDoesNotSelectCreatedPath(t *testing.T) {
	runner := &recordingRunner{results: map[string]recordingResult{
		"/repo/.worktrees/main/feature-hook|sh -c npm install": {err: errors.New("install failed")},
	}}
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.runner = runner
	model.repoConfig = appconfig.RepoConfig{PostCreate: "npm install"}
	model.hooksApproved = true
	model.createDialog.input.SetValue("feature/hook")
	path := model.createPathPreview()

	model, cmd := model.updateCreate(tea.KeyMsg{Type: tea.KeyEnter})
	message := cmd().(createMsg)

	if message.err == nil || !message.created {
		t.Fatalf("message = %+v, want created hook failure", message)
	}
	updated, _ := updateModel(t, model, message)
	if updated.selectedPath != "" {
		t.Fatalf("selectedPath = %q, want empty after hook failure", updated.selectedPath)
	}
	if updated.createDialog == nil || !strings.Contains(updated.createDialog.error, "worktree created at "+path+", but post_create failed") {
		t.Fatalf("create dialog error = %q, want created hook failure", updated.createDialog.error)
	}
}

func TestCreateFailureAfterDialogCloseShowsFlash(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.createDialog = nil
	model.createInFlight = true

	updated, cmd := updateModel(t, model, createMsg{err: errors.New("create failed")})

	if updated.flash != "create failed" {
		t.Fatalf("flash = %q, want create failure", updated.flash)
	}
	if updated.createInFlight {
		t.Fatal("createInFlight should clear after create result")
	}
	if cmd == nil {
		t.Fatal("create failure flash should schedule clearing")
	}
}

func TestCreateDialogIgnoresEnterWhileCreateInFlight(t *testing.T) {
	runner := &recordingRunner{}
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.runner = runner
	model.createDialog.input.SetValue("feature/guard")
	path := model.createPathPreview()

	model, firstCreate := model.updateCreate(tea.KeyMsg{Type: tea.KeyEnter})
	if firstCreate == nil {
		t.Fatal("first Enter should start create")
	}
	if !model.createInFlight {
		t.Fatal("createInFlight should be set while create runs")
	}
	_, secondCreate := model.updateCreate(tea.KeyMsg{Type: tea.KeyEnter})
	if secondCreate != nil {
		t.Fatal("second Enter should not start another create")
	}
	if message := firstCreate().(createMsg); message.err != nil {
		t.Fatalf("first create command error = %v", message.err)
	}
	command := "/repo/main|git worktree add -b feature/guard " + path + " main"
	createCount := 0
	for _, recorded := range runner.commands {
		if recorded == command {
			createCount++
		}
	}
	if createCount != 1 {
		t.Fatalf("create command count = %d, want 1; commands = %v", createCount, runner.commands)
	}
}

func TestBranchWorktreeDialogRunsApprovedPostCreate(t *testing.T) {
	runner := &recordingRunner{}
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main", IsMain: true}})
	model.runner = runner
	model.state.Repo.MainBranch = "main"
	model.repoConfig = appconfig.RepoConfig{PostCreate: "npm install"}
	model.hooksApproved = true
	model.branchWorktreeDialog = &branchWorktreeDialog{
		branch: gitdata.Branch{Name: "feature/branch"},
		path:   "/repo/.worktrees/main/feature-branch",
	}

	model, cmd := model.updateBranchWorktree(tea.KeyMsg{Type: tea.KeyEnter})

	message := cmd().(checkoutMsg)
	if message.err != nil {
		t.Fatalf("checkout command error = %v", message.err)
	}
	want := []string{
		"/repo/main|git worktree add /repo/.worktrees/main/feature-branch feature/branch",
		"/repo/.worktrees/main/feature-branch|sh -c npm install",
	}
	if got := strings.Join(runner.commands, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("commands = %v, want %v", runner.commands, want)
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

func TestPlanCleanupMergedScansDoneRowsAndSkipsUnsafeRows(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, BranchMergedToMain: true},
		{Path: "/repo/safe", Branch: "safe", Head: "1111111111111111111111111111111111111111", CommitShort: "1111111", BranchMergedToMain: true},
		{Path: "/repo/pr-closed", Branch: "pr-closed", PR: &gitdata.PullRequest{Number: 2, State: "✕"}},
		{Path: "/repo/dirty", Branch: "dirty", Status: gitdata.StatusCounts{Modified: 1}, BranchMergedToMain: true},
		{Path: "/repo/locked", Branch: "locked", Locked: true, BranchMergedToMain: true},
		{Path: "/repo/active", Branch: "active", IsActive: true, BranchMergedToMain: true},
		{Path: "/repo/detached", Head: "abc123456", Detached: true, BranchMergedToMain: true},
		{Path: "/repo/prunable", Branch: "prunable", Prunable: true, BranchMergedToMain: true},
		{Path: "/repo/loading", Branch: "loading", BranchMergedToMain: true},
	})
	model.state.Rows[8].LocalMetadataLoaded = false
	model.state.Branches = []gitdata.Branch{
		{Name: "branch-merged", Head: "2222222222222222222222222222222222222222", CommitShort: "2222222", BranchMergedToMain: true},
		{Name: "branch-closed", PR: &gitdata.PullRequest{Number: 3, State: "✕"}},
	}
	model.search.SetValue("does-not-match")
	model.filter = filterModified

	plan := model.planCleanupMerged()

	if len(plan.worktrees) != 2 {
		t.Fatalf("worktree actions = %+v, want safe and pr-closed", plan.worktrees)
	}
	if plan.worktrees[0].row.Branch != "safe" || !plan.worktrees[0].deleteBranch {
		t.Fatalf("first worktree action = %+v, want safe branch delete", plan.worktrees[0])
	}
	if plan.worktrees[1].row.Branch != "pr-closed" || plan.worktrees[1].deleteBranch {
		t.Fatalf("second worktree action = %+v, want pr-closed without branch delete", plan.worktrees[1])
	}
	if len(plan.branches) != 1 || plan.branches[0].branch.Name != "branch-merged" {
		t.Fatalf("branch actions = %+v, want branch-merged", plan.branches)
	}
	for _, want := range []cleanupMergedSkip{
		{name: "main", reason: "main worktree"},
		{name: "dirty", reason: "uncommitted changes"},
		{name: "locked", reason: "locked worktree"},
		{name: "active", reason: "active worktree"},
		{name: "/repo/detached", reason: "detached worktree"},
		{name: "prunable", reason: "missing worktree metadata"},
		{name: "loading", reason: "status is still loading"},
		{name: "branch-closed", reason: "branch is not merged into main"},
	} {
		if !hasCleanupSkip(plan.skips, want) {
			t.Fatalf("skips missing %+v: %+v", want, plan.skips)
		}
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

func TestCommandPaletteIncludesCheckoutPullRequest(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main"}})
	model, _ = model.openPalette()
	model.paletteDialog.input.SetValue("checkout pr")

	commands := model.matchingPaletteCommands()

	if len(commands) != 1 || commands[0].id != paletteCheckoutPullRequest || commands[0].title != "Checkout PR" {
		t.Fatalf("matching palette commands = %+v, want Checkout PR", commands)
	}
	if commands[0].shortcut != "" {
		t.Fatalf("Checkout PR shortcut = %q, want palette-only command", commands[0].shortcut)
	}
}

func TestPullRequestCheckoutOpensLoadingModal(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main"}})

	model, cmd := model.openPullRequestCheckout()

	if model.pullRequestDialog == nil {
		t.Fatal("openPullRequestCheckout() did not open dialog")
	}
	if cmd == nil {
		t.Fatal("openPullRequestCheckout() should focus input and load pull requests")
	}
	if !model.pullRequestDialog.input.Focused() {
		t.Fatal("pull request input should be focused")
	}
	output := model.renderPullRequestCheckoutAtWidth(76)
	if !strings.Contains(output, "Checkout PR") || !strings.Contains(output, "loading pull requests") {
		t.Fatalf("renderPullRequestCheckoutAtWidth() should show loading state:\n%s", output)
	}
}

func TestPullRequestCheckoutLoadingSpinnerAdvances(t *testing.T) {
	model := modelWithPullRequestDialog(nil)
	model.pullRequestDialog.loading = true

	updated, cmd := updateModel(t, model, pullRequestSpinnerTickMsg{id: model.pullRequestDialog.id})

	if updated.pullRequestDialog == nil || updated.pullRequestDialog.spinnerFrame != 1 {
		t.Fatalf("spinner frame = %+v, want next frame", updated.pullRequestDialog)
	}
	if cmd == nil {
		t.Fatal("active pull request spinner should schedule the next tick")
	}
	output := updated.renderPullRequestCheckoutAtWidth(76)
	if !strings.Contains(output, refreshSpinnerFrames[1]+" loading pull requests") {
		t.Fatalf("renderPullRequestCheckoutAtWidth() should show advanced spinner:\n%s", output)
	}
}

func TestPullRequestCheckoutFiltersByNumberTitleURLAndOwner(t *testing.T) {
	summaries := []github.PullRequestSummary{
		{
			Number:              42,
			Title:               "Auth cleanup",
			URL:                 "https://github.com/acme/repo/pull/42",
			HeadRefName:         "auth-cleanup",
			HeadRepositoryOwner: "alice",
			BaseRepositoryOwner: "schovi",
		},
		{
			Number:              41,
			Title:               "Docs",
			URL:                 "https://github.com/acme/repo/pull/41",
			HeadRefName:         "docs",
			HeadRepositoryOwner: "schovi",
			BaseRepositoryOwner: "schovi",
		},
	}
	model := modelWithPullRequestDialog(summaries)

	for _, query := range []string{"42", "auth cleanup", "pull/42", "alice", "alice/auth-cleanup"} {
		model.pullRequestDialog.input.SetValue(query)
		matches := model.matchingPullRequestSummaries()
		if len(matches) != 1 || matches[0].Number != 42 {
			t.Fatalf("matches for %q = %+v, want PR 42", query, matches)
		}
	}
}

func TestPullRequestCheckoutSelectedRowUsesFullWidthHighlight(t *testing.T) {
	summary := github.PullRequestSummary{
		Number:              42,
		Title:               "Auth cleanup",
		State:               "OPEN",
		HeadRefName:         "auth-cleanup",
		HeadRepositoryOwner: "schovi",
		BaseRepositoryOwner: "schovi",
	}
	model := modelWithPullRequestDialog([]github.PullRequestSummary{summary})
	contentWidth := 72

	output := model.renderPullRequestCheckoutAtWidth(contentWidth + 4)
	line := pullRequestOptionLine("› ", summary, contentWidth)
	want := paletteSelectedStyle.Render(padStyled(line, contentWidth))

	if !strings.Contains(output, want) {
		t.Fatalf("selected PR row should use filter-style full-width highlight %q:\n%s", want, output)
	}
}

func TestPullRequestCheckoutWrapsLongErrors(t *testing.T) {
	model := modelWithPullRequestDialog(nil)
	model.pullRequestDialog.error = "gh pr list --limit 200 --state all --json number,title,state,isDraft,headRefName,headRepositoryOwner,url,reviewDecision,updatedAt failed: HTTP 504: Gateway Timeout"

	output := model.renderPullRequestCheckoutAtWidth(64)

	if !strings.Contains(output, "gh pr list") || !strings.Contains(output, "Gateway Timeout") {
		t.Fatalf("renderPullRequestCheckoutAtWidth() should show full error details:\n%s", output)
	}
}

func TestPullRequestCheckoutOpensSelectedPullRequestInBrowser(t *testing.T) {
	runner := &recordingRunner{results: map[string]recordingResult{
		"/repo/main|gh pr view 42 --web": {},
	}}
	model := modelWithPullRequestDialog([]github.PullRequestSummary{{
		Number:      42,
		Title:       "Auth cleanup",
		State:       "OPEN",
		HeadRefName: "auth-cleanup",
	}})
	model.runner = runner

	started, cmd := model.updatePullRequestCheckout(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatal("o should open selected pull request")
	}
	rawMessage := cmd()
	message, ok := rawMessage.(pullRequestOpenedMsg)
	if !ok {
		t.Fatalf("open command returned %T, want pullRequestOpenedMsg", rawMessage)
	}
	updated, _ := updateModel(t, started, message)

	if message.err != nil {
		t.Fatalf("pullRequestOpenedMsg error = %v", message.err)
	}
	if updated.pullRequestDialog == nil || updated.pullRequestDialog.error != "" {
		t.Fatalf("pull request dialog = %+v, want modal open without error", updated.pullRequestDialog)
	}
	if len(runner.commands) != 1 || runner.commands[0] != "/repo/main|gh pr view 42 --web" {
		t.Fatalf("commands = %+v, want selected PR opened", runner.commands)
	}
}

func TestPullRequestCheckoutOpensTypedPullRequestURLInBrowser(t *testing.T) {
	runner := &recordingRunner{results: map[string]recordingResult{
		"/repo/main|gh pr view https://github.com/acme/repo/pull/404 --web": {
			err: errors.New("not found"),
		},
	}}
	model := modelWithPullRequestDialog(nil)
	model.runner = runner
	model.pullRequestDialog.input.SetValue("https://github.com/acme/repo/pull/404")

	started, cmd := model.updatePullRequestCheckout(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatal("o should open typed pull request query")
	}
	rawMessage := cmd()
	message, ok := rawMessage.(pullRequestOpenedMsg)
	if !ok {
		t.Fatalf("open command returned %T, want pullRequestOpenedMsg", rawMessage)
	}
	updated, _ := updateModel(t, started, message)

	if updated.pullRequestDialog == nil || updated.pullRequestDialog.error != "not found" {
		t.Fatalf("pull request dialog error = %+v, want inline open error", updated.pullRequestDialog)
	}
}

func TestPullRequestCheckoutReusesExistingWorktree(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true},
		{Path: "/repo/pr", Branch: "feature/login"},
	})
	model.pullRequestDialog = &pullRequestCheckoutDialog{}
	summary := github.PullRequestSummary{
		Number:              42,
		HeadRefName:         "feature/login",
		HeadRepositoryOwner: "schovi",
		BaseRepositoryOwner: "schovi",
	}

	updated, cmd := model.startPullRequestCheckout(summary)

	if updated.selectedPath != "/repo/pr" {
		t.Fatalf("selectedPath = %q, want existing PR worktree", updated.selectedPath)
	}
	if cmd == nil {
		t.Fatal("existing PR worktree should quit into that path")
	}
}

func TestPullRequestCheckoutCreatesWorktreeForExistingBranch(t *testing.T) {
	runner := &recordingRunner{results: map[string]recordingResult{
		"/repo/main|git worktree add /repo/.worktrees/main/feature-login feature/login": {},
	}}
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main", IsMain: true}})
	model.runner = runner
	model.state.Branches = []gitdata.Branch{{Name: "feature/login"}}
	model.pullRequestDialog = &pullRequestCheckoutDialog{}
	summary := github.PullRequestSummary{
		Number:              42,
		HeadRefName:         "feature/login",
		HeadRepositoryOwner: "schovi",
		BaseRepositoryOwner: "schovi",
	}

	started, cmd := model.startPullRequestCheckout(summary)
	if cmd == nil {
		t.Fatal("existing branch should create a worktree")
	}
	rawMessage := cmd()
	message, ok := rawMessage.(checkoutMsg)
	if !ok {
		t.Fatalf("checkout command returned %T, want checkoutMsg", rawMessage)
	}
	if message.err != nil || !message.created {
		t.Fatalf("checkoutMsg = %+v, want created worktree", message)
	}
	updated, quitCmd := updateModel(t, started, message)

	if updated.selectedPath != "/repo/.worktrees/main/feature-login" {
		t.Fatalf("selectedPath = %q, want created branch worktree", updated.selectedPath)
	}
	if quitCmd == nil {
		t.Fatal("successful branch worktree checkout should quit")
	}
}

func TestPullRequestCheckoutFetchesNewBranchAndRunsPostCreateHook(t *testing.T) {
	runner := &recordingRunner{results: map[string]recordingResult{
		"/repo/main|git fetch origin pull/42/head":                                                    {},
		"/repo/main|git worktree add -b alice/feature /repo/.worktrees/main/alice-feature FETCH_HEAD": {},
		"/repo/.worktrees/main/alice-feature|sh -c npm install":                                       {},
	}}
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main", IsMain: true}})
	model.runner = runner
	model.repoConfig.PostCreate = "npm install"
	model.hooksApproved = true
	model.pullRequestDialog = &pullRequestCheckoutDialog{}
	summary := github.PullRequestSummary{
		Number:              42,
		HeadRefName:         "feature",
		HeadRepositoryOwner: "alice",
		BaseRepositoryOwner: "schovi",
	}

	started, cmd := model.startPullRequestCheckout(summary)
	if cmd == nil {
		t.Fatal("new PR branch should create a worktree")
	}
	rawMessage := cmd()
	message, ok := rawMessage.(checkoutMsg)
	if !ok {
		t.Fatalf("checkout command returned %T, want checkoutMsg", rawMessage)
	}
	updated, quitCmd := updateModel(t, started, message)

	if message.err != nil || !message.created {
		t.Fatalf("checkoutMsg = %+v, want created PR worktree", message)
	}
	if updated.selectedPath != "/repo/.worktrees/main/alice-feature" {
		t.Fatalf("selectedPath = %q, want created PR worktree", updated.selectedPath)
	}
	if quitCmd == nil {
		t.Fatal("successful PR checkout should quit")
	}
	if len(runner.envCommands) != 2 ||
		runner.envCommands[0].command != "/repo/main|git fetch origin pull/42/head" ||
		runner.envCommands[1].command != "/repo/.worktrees/main/alice-feature|sh -c npm install" {
		t.Fatalf("env commands = %+v, want guarded fetch then hook in new worktree", runner.envCommands)
	}
}

func TestPullRequestCheckoutDirectLookupShowsNoMatch(t *testing.T) {
	runner := &recordingRunner{results: map[string]recordingResult{
		"/repo/main|gh repo view --json owner": {output: `{"owner":{"login":"schovi"}}`},
		"/repo/main|gh pr view https://github.com/acme/repo/pull/404 --json number,title,state,isDraft,headRefName,headRepositoryOwner,url,reviewDecision,updatedAt": {
			err: errors.New("not found"),
		},
	}}
	model := modelWithPullRequestDialog(nil)
	model.runner = runner
	model.pullRequestDialog.input.SetValue("https://github.com/acme/repo/pull/404")

	started, cmd := model.updatePullRequestCheckout(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("unmatched PR URL should trigger direct lookup")
	}
	batchMessage := cmd()
	batch, ok := batchMessage.(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("direct lookup returned %T, want batched commands", batchMessage)
	}
	rawMessage := batch[0]()
	message, ok := rawMessage.(pullRequestSummaryLoadedMsg)
	if !ok {
		t.Fatalf("direct lookup returned %T, want pullRequestSummaryLoadedMsg", rawMessage)
	}
	updated, _ := updateModel(t, started, message)

	if updated.pullRequestDialog == nil || updated.pullRequestDialog.error != "No matching PR" {
		t.Fatalf("pull request dialog error = %+v, want no match", updated.pullRequestDialog)
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

func TestViewRendersScrollbarForOverflowingWorktreeList(t *testing.T) {
	rows := make([]gitdata.Worktree, 18)
	rows[0] = gitdata.Worktree{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true}
	for index := 1; index < len(rows); index++ {
		rows[index] = gitdata.Worktree{Path: fmt.Sprintf("/repo/worktree-%d", index), Branch: fmt.Sprintf("worktree-%d", index)}
	}
	model := testModelWithRows(rows)
	model.width = 100
	model.height = 24

	output := model.View()
	plainOutput := ansi.Strip(output)

	for _, want := range []string{"↑", "█", "↓", "0/18"} {
		if !strings.Contains(plainOutput, want) {
			t.Fatalf("View() missing scrollbar element %q:\n%s", want, output)
		}
	}
	for _, line := range strings.Split(output, "\n") {
		if width := lipgloss.Width(line); width != model.width {
			t.Fatalf("View() line width = %d, want %d:\n%q\n%s", width, model.width, line, output)
		}
	}
}

func TestViewHidesScrollbarWhenWorktreeListFits(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true},
		{Path: "/repo/docs", Branch: "docs"},
	})
	model.width = 100
	model.height = 24

	plainOutput := ansi.Strip(model.View())

	for _, unwanted := range []string{"↑", "█", "↓", "0/2"} {
		if strings.Contains(plainOutput, unwanted) {
			t.Fatalf("View() should not render scrollbar element %q when rows fit:\n%s", unwanted, plainOutput)
		}
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
		"Row Icons",
		"Git Status",
		"Pull Requests",
		"ctrl+p",
		"Esc close/cancel",
		"top/bottom",
		"b branches",
		"Enter go/create",
		"c checkout root",
		"PR/branch",
		"⊡ worktree",
		"⎇ branch",
		"bold active row",
		"remote gone",
		"◌ draft",
		"○ ready/open",
		"◆ approved",
		"⎇ merged",
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
	markerOrder := []string{"⌂ root", "⊡ worktree", "⎇ branch", "! locked", "× prunable", "bold active row"}
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

func TestHelpRowIconStylesMatchListRenderer(t *testing.T) {
	tests := []struct {
		name string
		kind helpEntryKind
		want lipgloss.Style
	}{
		{name: "root", kind: helpEntryRoot, want: listview.RootTypeIconStyle()},
		{name: "worktree", kind: helpEntryWorktree, want: listview.WorktreeTypeIconStyle()},
		{name: "branch", kind: helpEntryBranch, want: listview.BranchTypeIconStyle()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := helpEntryStyle(test.kind)
			if fmt.Sprint(got.GetForeground()) != fmt.Sprint(test.want.GetForeground()) || got.GetBold() != test.want.GetBold() {
				t.Fatalf("helpEntryStyle(%s) = foreground %v bold %t, want foreground %v bold %t",
					test.name,
					got.GetForeground(),
					got.GetBold(),
					test.want.GetForeground(),
					test.want.GetBold(),
				)
			}
		})
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

func TestCreateDialogRendersLivePathCollision(t *testing.T) {
	forceANSIProfile(t)
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.runner = &recordingRunner{}
	repoRoot := filepath.Join(t.TempDir(), "git-treehouse")
	model.state.Repo.Root = repoRoot
	targetPath := filepath.Join(filepath.Dir(repoRoot), ".worktrees", "git-treehouse", "feature-login")
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", targetPath, err)
	}

	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("feature/login")})
	output := model.renderCreateAtWidth(200)
	want := "target path already exists: " + targetPath

	if !strings.Contains(output, lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(want)) {
		t.Fatalf("renderCreateAtWidth() should show styled live collision error %q:\n%s", want, output)
	}

	model, command := model.updateCreate(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("Enter should stay blocked on a path collision")
	}
	if got := model.createDialog.error; got != want {
		t.Fatalf("Enter collision error = %q, want %q", got, want)
	}
}

func TestCreateDialogDoesNotRenderCollisionForAvailablePath(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})
	model.state.Repo.Root = filepath.Join(t.TempDir(), "git-treehouse")

	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("feature/login")})
	output := model.renderCreateAtWidth(200)

	if strings.Contains(output, "target path already exists:") {
		t.Fatalf("renderCreateAtWidth() should not show collision for available path:\n%s", output)
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

func TestOpenDeleteRendersBranchOnlyDeleteDialog(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})
	model.state.Repo.MainBranch = "main"
	model.state.Branches = []gitdata.Branch{
		{
			Name:               "feature/branch",
			Head:               "abcdef123456",
			CommitShort:        "abcdef1",
			BranchMergedToMain: true,
			PR:                 &gitdata.PullRequest{Number: 42, State: "○", CI: "✓"},
		},
	}
	model.filter = filterBranches
	model.showPR = true

	model, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if cmd != nil {
		t.Fatalf("d returned command, want nil")
	}
	if model.deleteDialog == nil {
		t.Fatal("d on branch row should open delete dialog")
	}
	output := model.renderDeleteAtWidth(80)
	checkboxLine := deleteCheckboxLine(true, deleteBranchLabel(model.state.Branches[0]), false)
	if !strings.Contains(output, checkboxLine) {
		t.Fatalf("branch delete dialog checkbox = %q, want shared helper output:\n%s", checkboxLine, output)
	}
	for _, want := range []string{
		"Delete branch",
		"Branch:",
		"feature/branch",
		"HEAD:",
		"abcdef1 on feature/branch",
		"PR:",
		"#42 ○ ✓",
		"[x] delete local branch",
		"Local branch ref will be deleted. No worktree files are removed.",
		"Merged into main.",
		"git branch -d feature/branch",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("branch delete dialog missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"remove worktree", "git worktree remove"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("branch delete dialog should not contain %q:\n%s", unwanted, output)
		}
	}
}

func TestOpenDeleteRendersForceBranchOnlyDeleteDialog(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})
	model.state.Repo.MainBranch = "main"
	model.state.Branches = []gitdata.Branch{{Name: "feature/unmerged"}}
	model.filter = filterBranches

	model, _ = model.openDelete()

	output := model.renderDeleteAtWidth(80)
	for _, want := range []string{
		"[x] force delete local branch",
		"Not merged into main.",
		"git branch -D feature/unmerged",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("force branch delete dialog missing %q:\n%s", want, output)
		}
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
	dialog := deleteDialog{stage: deleteStagePrune, deleteBranch: true, runBeforeDelete: true, beforeDeleteHook: "docker compose down"}

	err := deleteRow(context.Background(), gitdata.Repository{Root: "/repo/main"}, row, dialog, runner)

	if err != nil {
		t.Fatalf("deleteRow() error = %v", err)
	}
	want := []string{"/repo/main|git worktree prune"}
	if got := strings.Join(runner.commands, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("commands = %v, want %v", runner.commands, want)
	}
}

func TestDeleteBranchRowUsesSafeDeleteForMergedBranch(t *testing.T) {
	runner := &recordingRunner{}
	branch := gitdata.Branch{Name: "feature", BranchMergedToMain: true}

	err := deleteBranchRow(context.Background(), gitdata.Repository{Root: "/repo/main"}, branch, runner)

	if err != nil {
		t.Fatalf("deleteBranchRow() error = %v", err)
	}
	want := []string{"/repo/main|git branch -d feature"}
	if got := strings.Join(runner.commands, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("commands = %v, want %v", runner.commands, want)
	}
}

func TestDeleteBranchRowUsesForceDeleteForUnmergedBranch(t *testing.T) {
	runner := &recordingRunner{}
	branch := gitdata.Branch{Name: "feature"}

	err := deleteBranchRow(context.Background(), gitdata.Repository{Root: "/repo/main"}, branch, runner)

	if err != nil {
		t.Fatalf("deleteBranchRow() error = %v", err)
	}
	want := []string{"/repo/main|git branch -D feature"}
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

func TestOpenDeleteShowsBeforeDeleteHookToggle(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
		{Path: "/repo/feature", Branch: "feature"},
	})
	model.selected = 1
	model.repoConfig = appconfig.RepoConfig{BeforeDelete: "docker compose down"}
	model.hooksApproved = true

	model, _ = model.openDelete()

	if model.deleteDialog == nil || !model.deleteDialog.runBeforeDelete {
		t.Fatalf("delete dialog = %+v, want enabled before_delete hook", model.deleteDialog)
	}
	output := ansi.Strip(model.renderDeleteAtWidth(100))
	for _, want := range []string{"Cleanup hook", "h toggle", "run before_delete cleanup hook", `sh -c "docker compose down"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("delete dialog missing %q:\n%s", want, output)
		}
	}
}

func TestOpenCleanupMergedWithoutActionsFlashes(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
		{Path: "/repo/dirty", Branch: "dirty", Status: gitdata.StatusCounts{Modified: 1}, BranchMergedToMain: true},
	})

	model, cmd := model.openCleanupMerged()

	if cmd == nil {
		t.Fatal("no cleanup actions should set a flash clear command")
	}
	if model.cleanupMergedDialog != nil {
		t.Fatal("no cleanup actions should not open dialog")
	}
	if model.flash != "no merged worktrees or branches to clean up" {
		t.Fatalf("flash = %q", model.flash)
	}
}

func TestOpenCleanupMergedRendersConfirmation(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, BranchMergedToMain: true},
		{Path: "/repo/feature", Branch: "feature", BranchMergedToMain: true},
		{Path: "/repo/closed", Branch: "closed", PR: &gitdata.PullRequest{Number: 2, State: "✕"}},
		{Path: "/repo/dirty", Branch: "dirty", Status: gitdata.StatusCounts{Modified: 1}, BranchMergedToMain: true},
	})
	model.repoConfig = appconfig.RepoConfig{BeforeDelete: "docker compose down"}
	model.hooksApproved = true
	model.state.Branches = []gitdata.Branch{{Name: "branch-only", BranchMergedToMain: true}}

	model, _ = model.openCleanupMerged()

	if model.cleanupMergedDialog == nil {
		t.Fatal("cleanup should open confirmation dialog")
	}
	output := ansi.Strip(model.renderCleanupMergedAtWidth(100))
	for _, want := range []string{
		"Worktrees:",
		"2 remove",
		"Branches:",
		"2 delete",
		"feature · remove worktree, delete branch",
		"closed · remove worktree",
		"branch-only",
		"git worktree remove /repo/feature",
		"git branch -d feature",
		"git worktree remove /repo/closed",
		"git branch -d branch-only",
		`sh -c "docker compose down"`,
		"Enter clean up",
		"Esc cancel",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("cleanup confirmation missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"Skipped:", "dirty: uncommitted changes", "git branch -d <branch>"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("cleanup confirmation should not contain %q:\n%s", unwanted, output)
		}
	}
}

func TestCleanupMergedProgressRendersInBottomBorder(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/feature", Branch: "feature", BranchMergedToMain: true},
	})
	model, _ = model.openCleanupMerged()
	model.cleanupMergedInFlight = true

	output := model.renderCleanupMergedAtWidth(80)
	outputLines := strings.Split(output, "\n")

	if !strings.Contains(outputLines[len(outputLines)-1], "⠋ cleaning") {
		t.Fatalf("cleanup modal should show progress in bottom border:\n%s", output)
	}
	if strings.Count(ansi.Strip(output), "cleaning") != 1 {
		t.Fatalf("cleanup modal should render progress once:\n%s", output)
	}
}

func TestCleanupMergedCommandRunsSafeBatchAndReloads(t *testing.T) {
	worktreeList := "worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n"
	results := stableLoadResults(worktreeList)
	results["/repo/feature|sh -c docker compose down"] = recordingResult{}
	results["/repo/main|git worktree remove /repo/feature"] = recordingResult{}
	results["/repo/main|git branch -d feature"] = recordingResult{}
	results["/repo/main|git branch -d branch-only"] = recordingResult{}
	runner := &recordingRunner{results: results}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
		{
			Path:               "/repo/feature",
			Branch:             "feature",
			Head:               "1111111111111111111111111111111111111111",
			CommitShort:        "1111111",
			BranchMergedToMain: true,
		},
	})
	model.runner = runner
	model.repoConfig = appconfig.RepoConfig{BeforeDelete: "docker compose down"}
	model.hooksApproved = true
	model.state.Branches = []gitdata.Branch{{
		Name:               "branch-only",
		Head:               "2222222222222222222222222222222222222222",
		CommitShort:        "2222222",
		BranchMergedToMain: true,
	}}
	model, _ = model.openCleanupMerged()

	started, cmd := model.updateCleanupMerged(tea.KeyMsg{Type: tea.KeyEnter})
	message := firstCleanupMergedMessage(t, cmd)

	if message.err != nil {
		t.Fatalf("cleanup command error = %v", message.err)
	}
	if message.result.removedWorktrees != 1 || message.result.deletedBranches != 2 || len(message.result.failures) != 0 {
		t.Fatalf("cleanup result = %+v, want one worktree and two branches", message.result)
	}
	for _, want := range []string{
		"/repo/feature|sh -c docker compose down",
		"/repo/main|git worktree remove /repo/feature",
		"/repo/main|git branch -d feature",
		"/repo/main|git branch -d branch-only",
	} {
		if !hasCommand(runner.commands, want) {
			t.Fatalf("commands missing %q: %v", want, runner.commands)
		}
	}
	for _, command := range runner.commands {
		if strings.Contains(command, "--force") || strings.Contains(command, "git branch -D") {
			t.Fatalf("cleanup should not run force commands: %v", runner.commands)
		}
	}
	if len(runner.envCommands) != 1 {
		t.Fatalf("env commands = %+v, want one before_delete hook", runner.envCommands)
	}
	for _, wantEnv := range []string{
		"GTH_EVENT=before_delete",
		"GTH_WORKTREE_PATH=/repo/feature",
		"GTH_WORKTREE_BRANCH=feature",
		"GTH_REPO_ROOT=/repo/main",
	} {
		if !slices.Contains(runner.envCommands[0].env, wantEnv) {
			t.Fatalf("hook env missing %q: %#v", wantEnv, runner.envCommands[0].env)
		}
	}

	updated, _ := updateModel(t, started, message)

	if updated.cleanupMergedDialog != nil {
		t.Fatal("successful cleanup should close dialog")
	}
	if len(updated.pendingRestoreBatch) != 2 {
		t.Fatalf("pending restore batch = %+v, want two branches", updated.pendingRestoreBatch)
	}
	for _, want := range []string{"cleaned up merged: removed 1 worktree, deleted 2 branches", "u to restore branches"} {
		if !strings.Contains(updated.feedback.plainText(), want) {
			t.Fatalf("cleanup feedback missing %q: %q", want, updated.feedback.plainText())
		}
	}
}

func TestCleanupMergedPartialFailureKeepsResultDialog(t *testing.T) {
	worktreeList := "worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n"
	results := stableLoadResults(worktreeList)
	results["/repo/feature|sh -c docker compose down"] = recordingResult{err: errors.New("cleanup failed")}
	results["/repo/main|git branch -d branch-only"] = recordingResult{}
	runner := &recordingRunner{results: results}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
		{Path: "/repo/feature", Branch: "feature", BranchMergedToMain: true},
	})
	model.runner = runner
	model.repoConfig = appconfig.RepoConfig{BeforeDelete: "docker compose down"}
	model.hooksApproved = true
	model.state.Branches = []gitdata.Branch{{
		Name:               "branch-only",
		Head:               "2222222222222222222222222222222222222222",
		CommitShort:        "2222222",
		BranchMergedToMain: true,
	}}
	model, _ = model.openCleanupMerged()

	started, cmd := model.updateCleanupMerged(tea.KeyMsg{Type: tea.KeyEnter})
	message := firstCleanupMergedMessage(t, cmd)
	updated, _ := updateModel(t, started, message)

	if message.result.removedWorktrees != 0 || message.result.deletedBranches != 1 || len(message.result.failures) != 1 {
		t.Fatalf("cleanup result = %+v, want partial failure", message.result)
	}
	if hasCommand(runner.commands, "/repo/main|git worktree remove /repo/feature") {
		t.Fatalf("hook failure should skip worktree removal: %v", runner.commands)
	}
	if updated.cleanupMergedDialog == nil || updated.cleanupMergedDialog.result == nil {
		t.Fatal("partial cleanup should keep result dialog open")
	}
	output := ansi.Strip(updated.renderCleanupMergedAtWidth(100))
	for _, want := range []string{"partially completed", "feature: cleanup failed", "Failures:", "1", "u restore branches", "Esc close"} {
		if !strings.Contains(output, want) {
			t.Fatalf("partial cleanup dialog missing %q:\n%s", want, output)
		}
	}
	if len(updated.pendingRestoreBatch) != 1 || updated.pendingRestoreBatch[0].branch != "branch-only" {
		t.Fatalf("pending restore batch = %+v, want branch-only", updated.pendingRestoreBatch)
	}
}

func TestCleanupMergedEscCancelsInFlightHook(t *testing.T) {
	runner := cancelledHookRunner{recordingRunner: &recordingRunner{results: stableLoadResults("worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n")}}
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main", IsMain: true}})
	model.runner = runner
	plan := cleanupMergedPlan{worktrees: []cleanupMergedWorktree{{
		row:              gitdata.Worktree{Path: "/repo/feature", Branch: "feature"},
		runBeforeDelete:  true,
		beforeDeleteHook: "sleep forever",
	}}}
	model.cleanupMergedDialog = &cleanupMergedDialog{plan: plan}

	started, command := model.startCleanupMerged(plan)
	cancelled, cancelCommand := started.updateCleanupMerged(tea.KeyMsg{Type: tea.KeyEsc})
	if cancelCommand != nil {
		t.Fatal("Esc should cancel the cleanup action without starting another command")
	}

	message := firstCleanupMergedMessage(t, command)
	if len(message.result.failures) != 1 || message.result.failures[0].reason != context.Canceled.Error() {
		t.Fatalf("cleanup result = %+v, want cancelled hook failure", message.result)
	}
	updated, _ := updateModel(t, cancelled, message)
	if updated.cleanupMergedInFlight {
		t.Fatal("cancelled cleanup should clear in-flight state")
	}
	if updated.cleanupMergedDialog == nil || updated.cleanupMergedDialog.result == nil {
		t.Fatal("cancelled cleanup should keep a failure result in the dialog")
	}
}

func TestDeleteHookToggleDisablesBeforeDelete(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
		{Path: "/repo/feature", Branch: "feature"},
	})
	model.selected = 1
	model.repoConfig = appconfig.RepoConfig{BeforeDelete: "docker compose down"}
	model.hooksApproved = true
	model, _ = model.openDelete()

	model, cmd := model.updateDelete(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	if cmd != nil {
		t.Fatalf("hook toggle returned command, want nil")
	}
	if model.deleteDialog == nil || model.deleteDialog.runBeforeDelete {
		t.Fatalf("delete dialog = %+v, want hook disabled", model.deleteDialog)
	}
	output := ansi.Strip(model.renderDeleteAtWidth(100))
	if !strings.Contains(output, "No cleanup hook will run.") {
		t.Fatalf("delete dialog should explain skipped hook:\n%s", output)
	}
}

func TestDeleteRowRunsBeforeDeleteBeforeWorktreeRemoval(t *testing.T) {
	runner := &recordingRunner{}
	row := gitdata.Worktree{Path: "/repo/feature", Branch: "feature"}
	dialog := deleteDialog{
		stage:            deleteStageOptions,
		deleteWorktree:   true,
		runBeforeDelete:  true,
		beforeDeleteHook: "docker compose down",
	}

	err := deleteRow(context.Background(), gitdata.Repository{Root: "/repo/main", MainBranch: "main"}, row, dialog, runner)

	if err != nil {
		t.Fatalf("deleteRow() error = %v", err)
	}
	want := []string{
		"/repo/feature|sh -c docker compose down",
		"/repo/main|git worktree remove /repo/feature",
	}
	if got := strings.Join(runner.commands, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("commands = %v, want %v", runner.commands, want)
	}
	if len(runner.envCommands) != 1 {
		t.Fatalf("env commands = %+v, want before_delete hook", runner.envCommands)
	}
	for _, wantEnv := range []string{
		"GTH_EVENT=before_delete",
		"GTH_WORKTREE_PATH=/repo/feature",
		"GTH_WORKTREE_BRANCH=feature",
		"GTH_REPO_ROOT=/repo/main",
		"GTH_MAIN_BRANCH=main",
	} {
		if !slices.Contains(runner.envCommands[0].env, wantEnv) {
			t.Fatalf("hook env missing %q: %#v", wantEnv, runner.envCommands[0].env)
		}
	}
}

func TestDeleteRowStopsWhenBeforeDeleteFails(t *testing.T) {
	runner := &recordingRunner{results: map[string]recordingResult{
		"/repo/feature|sh -c docker compose down": {err: errors.New("cleanup failed")},
	}}
	row := gitdata.Worktree{Path: "/repo/feature", Branch: "feature"}
	dialog := deleteDialog{
		stage:            deleteStageOptions,
		deleteWorktree:   true,
		runBeforeDelete:  true,
		beforeDeleteHook: "docker compose down",
	}

	err := deleteRow(context.Background(), gitdata.Repository{Root: "/repo/main"}, row, dialog, runner)

	if err == nil {
		t.Fatal("deleteRow() error = nil, want hook failure")
	}
	want := []string{"/repo/feature|sh -c docker compose down"}
	if got := strings.Join(runner.commands, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("commands = %v, want only hook command", runner.commands)
	}
}

func TestDeleteRowSkipsBeforeDeleteWhenToggleOff(t *testing.T) {
	runner := &recordingRunner{}
	row := gitdata.Worktree{Path: "/repo/feature", Branch: "feature"}
	dialog := deleteDialog{
		stage:            deleteStageOptions,
		deleteWorktree:   true,
		beforeDeleteHook: "docker compose down",
	}

	err := deleteRow(context.Background(), gitdata.Repository{Root: "/repo/main"}, row, dialog, runner)

	if err != nil {
		t.Fatalf("deleteRow() error = %v", err)
	}
	want := []string{"/repo/main|git worktree remove /repo/feature"}
	if got := strings.Join(runner.commands, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("commands = %v, want hook skipped", runner.commands)
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

func TestDeleteCommandReloadsStableStateBeforeSuccess(t *testing.T) {
	worktreeList := "worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n"
	runner := &recordingRunner{results: map[string]recordingResult{
		"/repo/main|git worktree remove /repo/feature":                     {},
		"/repo/main|git rev-parse --show-toplevel":                         {output: "/repo/main\n"},
		"/repo/main|git rev-parse --git-common-dir":                        {output: ".git\n"},
		"/repo/main|git rev-parse --path-format=absolute --git-common-dir": {output: "/repo/main/.git\n"},
		"/repo/main|git worktree list --porcelain":                         {output: worktreeList},
		"/repo/main|git symbolic-ref --short refs/remotes/origin/HEAD":     {err: errors.New("no origin")},
		"/repo/main|git show-ref --verify --quiet refs/heads/main":         {},
		"/repo/main|git remote":                                            {},
	}}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
		{Path: "/repo/feature", Branch: "feature"},
	})
	model.runner = runner
	model.selected = 1
	model, _ = model.openDelete()

	started, cmd := model.updateDelete(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("delete should start a command")
	}
	if started.loading != "" {
		t.Fatalf("delete should not use generic loading, got %q", started.loading)
	}
	if !started.deleteInFlight {
		t.Fatal("delete should mark an in-flight delete")
	}
	if strings.Contains(started.statusBar(), "deleting") {
		t.Fatalf("status bar should not show delete progress:\n%s", started.statusBar())
	}
	output := started.renderDeleteAtWidth(80)
	outputLines := strings.Split(output, "\n")
	if !strings.Contains(outputLines[len(outputLines)-1], "⠋ deleting") {
		t.Fatalf("delete modal should show progress in the bottom border:\n%s", output)
	}
	if strings.Count(ansi.Strip(output), "deleting") != 1 {
		t.Fatalf("delete modal should render progress once:\n%s", output)
	}
	batchMessage := cmd()
	batch, ok := batchMessage.(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("delete command returned %T, want BatchMsg with delete command", batchMessage)
	}
	firstMessage := batch[0]()
	message, ok := firstMessage.(deleteMsg)
	if !ok {
		t.Fatalf("first delete batch message = %T, want deleteMsg", firstMessage)
	}
	if message.err != nil {
		t.Fatalf("delete command error = %v", message.err)
	}
	if len(message.state.Rows) != 1 || message.state.Rows[0].Path != "/repo/main" {
		t.Fatalf("delete command state rows = %+v, want only main", message.state.Rows)
	}
	if !message.state.Rows[0].LocalMetadataLoaded {
		t.Fatalf("delete command should return stable local metadata: %+v", message.state.Rows)
	}

	updated, _ := updateModel(t, started, message)

	if updated.deleteDialog != nil {
		t.Fatal("successful delete should close the delete dialog")
	}
	if updated.deleteInFlight {
		t.Fatal("successful delete should clear in-flight state")
	}
	if updated.flash != "" {
		t.Fatalf("delete success should not use generic flash, got %q", updated.flash)
	}
	if updated.feedback.plainText() != "✓ deleted worktree" {
		t.Fatalf("delete success badge = %q, want Worktrees title success", updated.feedback.plainText())
	}
	if !updated.localMetadataReady() {
		t.Fatalf("updated state should stay locally ready: %+v", updated.state.Rows)
	}
	if output := updated.View(); strings.Contains(output, "Loading worktrees") {
		t.Fatalf("delete success should not render the loading skeleton:\n%s", output)
	}
	if got := strings.Join(visibleBranches(updated), ","); got != "main" {
		t.Fatalf("visible branches = %q, want main", got)
	}
}

func TestDeleteBranchSuccessOffersRestore(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	worktreeList := "worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n"
	results := stableLoadResults(worktreeList)
	results["/repo/main|git branch -d feature"] = recordingResult{}
	runner := &recordingRunner{results: results}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})
	model.runner = runner
	model.filter = filterBranches
	model.state.Branches = []gitdata.Branch{{
		Name:               "feature",
		Head:               sha,
		CommitShort:        "0123456",
		BranchMergedToMain: true,
	}}
	model, _ = model.openDelete()

	started, cmd := model.updateDelete(tea.KeyMsg{Type: tea.KeyEnter})
	message := firstDeleteMessage(t, cmd)

	if message.restore == nil {
		t.Fatal("branch delete success should include restore metadata")
	}
	if *message.restore != (pendingBranchRestore{branch: "feature", sha: sha, short: "0123456"}) {
		t.Fatalf("restore metadata = %+v, want feature at %s", *message.restore, sha)
	}

	updated, _ := updateModel(t, started, message)

	if updated.pendingRestore == nil {
		t.Fatal("delete success should leave pending restore available")
	}
	if *updated.pendingRestore != *message.restore {
		t.Fatalf("pending restore = %+v, want %+v", *updated.pendingRestore, *message.restore)
	}
	for _, want := range []string{"✓ deleted feature", "0123456", "u to restore"} {
		if !strings.Contains(updated.feedback.plainText(), want) {
			t.Fatalf("restore offer missing %q: %q", want, updated.feedback.plainText())
		}
	}
}

func TestDeleteWorktreeWithBranchOffersRestore(t *testing.T) {
	sha := "fedcba9876543210fedcba9876543210fedcba98"
	worktreeList := "worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n"
	results := stableLoadResults(worktreeList)
	results["/repo/main|git worktree remove /repo/feature"] = recordingResult{}
	results["/repo/main|git branch -d feature"] = recordingResult{}
	runner := &recordingRunner{results: results}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
		{
			Path:               "/repo/feature",
			Branch:             "feature",
			Head:               sha,
			CommitShort:        "fedcba9",
			BranchMergedToMain: true,
		},
	})
	model.runner = runner
	model.selected = 1
	model, _ = model.openDelete()

	_, cmd := model.updateDelete(tea.KeyMsg{Type: tea.KeyEnter})
	message := firstDeleteMessage(t, cmd)

	if message.restore == nil {
		t.Fatal("worktree and branch delete should include restore metadata")
	}
	if *message.restore != (pendingBranchRestore{branch: "feature", sha: sha, short: "fedcba9"}) {
		t.Fatalf("restore metadata = %+v, want feature at %s", *message.restore, sha)
	}
}

func TestDeleteWorktreeWithoutBranchDeleteDoesNotOfferRestore(t *testing.T) {
	worktreeList := "worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n"
	results := stableLoadResults(worktreeList)
	results["/repo/main|git worktree remove /repo/feature"] = recordingResult{}
	runner := &recordingRunner{results: results}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
		{
			Path:        "/repo/feature",
			Branch:      "feature",
			Head:        "fedcba9876543210fedcba9876543210fedcba98",
			CommitShort: "fedcba9",
		},
	})
	model.runner = runner
	model.selected = 1
	model, _ = model.openDelete()

	_, cmd := model.updateDelete(tea.KeyMsg{Type: tea.KeyEnter})
	message := firstDeleteMessage(t, cmd)

	if message.restore != nil {
		t.Fatalf("worktree-only delete restore = %+v, want nil", *message.restore)
	}
}

func TestRestoreKeyCreatesBranchAtPendingSHA(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	worktreeList := "worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n"
	results := stableLoadResults(worktreeList)
	results["/repo/main|git branch feature "+sha] = recordingResult{}
	runner := &recordingRunner{results: results}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})
	model.runner = runner
	model.pendingRestore = &pendingBranchRestore{branch: "feature", sha: sha, short: "0123456"}
	model.feedback = restoreOfferFeedback(*model.pendingRestore)

	started, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})

	if cmd == nil {
		t.Fatal("u with pending restore should start a command")
	}
	if started.pendingRestore != nil {
		t.Fatalf("pending restore after start = %+v, want nil", started.pendingRestore)
	}
	message := firstDeleteMessage(t, cmd)
	if message.err != nil {
		t.Fatalf("restore command error = %v", message.err)
	}
	if !hasCommand(runner.commands, "/repo/main|git branch feature "+sha) {
		t.Fatalf("commands = %v, want git branch restore command", runner.commands)
	}

	updated, _ := updateModel(t, started, message)

	if updated.feedback.plainText() != "✓ restored branch feature" {
		t.Fatalf("restore success flash = %q, want restored branch", updated.feedback.plainText())
	}
	if updated.pendingRestore != nil {
		t.Fatalf("pending restore after success = %+v, want nil", updated.pendingRestore)
	}
}

func TestRestoreKeyWithoutPendingRestoreIsNoOp(t *testing.T) {
	runner := &recordingRunner{}
	model := Model{runner: runner, selected: 2, feedback: successFeedback(feedbackFrameWorktrees, "kept")}

	updated, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})

	if cmd != nil {
		t.Fatal("u without pending restore returned command, want nil")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %v, want none", runner.commands)
	}
	if updated.selected != model.selected || updated.feedback.plainText() != model.feedback.plainText() || updated.pendingRestore != nil {
		t.Fatalf("model changed on restore no-op: %+v", updated)
	}
}

func TestRestoreKeyRestoresPendingBranchBatch(t *testing.T) {
	worktreeList := "worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n"
	results := stableLoadResults(worktreeList)
	results["/repo/main|git branch first 1111111111111111111111111111111111111111"] = recordingResult{}
	results["/repo/main|git branch second 2222222222222222222222222222222222222222"] = recordingResult{}
	runner := &recordingRunner{results: results}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})
	model.runner = runner
	model.pendingRestoreBatch = []pendingBranchRestore{
		{branch: "first", sha: "1111111111111111111111111111111111111111", short: "1111111"},
		{branch: "second", sha: "2222222222222222222222222222222222222222", short: "2222222"},
	}
	model.feedback = cleanupRestoreOfferFeedback(cleanupMergedResult{
		removedWorktrees: 1,
		deletedBranches:  2,
		restores:         model.pendingRestoreBatch,
	})

	started, cmd := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	message := firstDeleteMessage(t, cmd)

	if message.err != nil {
		t.Fatalf("restore batch command error = %v", message.err)
	}
	for _, want := range []string{
		"/repo/main|git branch first 1111111111111111111111111111111111111111",
		"/repo/main|git branch second 2222222222222222222222222222222222222222",
	} {
		if !hasCommand(runner.commands, want) {
			t.Fatalf("commands missing %q: %v", want, runner.commands)
		}
	}

	updated, _ := updateModel(t, started, message)

	if updated.pendingRestoreBatch != nil {
		t.Fatalf("pending restore batch after success = %+v, want nil", updated.pendingRestoreBatch)
	}
	if updated.feedback.plainText() != "✓ restored 2 branches" {
		t.Fatalf("restore success feedback = %q", updated.feedback.plainText())
	}
}

func TestRestoreBatchContinuesAfterBranchAlreadyExists(t *testing.T) {
	worktreeList := "worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n"
	results := stableLoadResults(worktreeList)
	results["/repo/main|git branch first 1111111111111111111111111111111111111111"] = recordingResult{err: errors.New("branch already exists")}
	results["/repo/main|git branch second 2222222222222222222222222222222222222222"] = recordingResult{}
	runner := &recordingRunner{results: results}
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main", IsMain: true}})
	model.runner = runner
	model.pendingRestoreBatch = []pendingBranchRestore{
		{branch: "first", sha: "1111111111111111111111111111111111111111", short: "1111111"},
		{branch: "second", sha: "2222222222222222222222222222222222222222", short: "2222222"},
	}

	started, command := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	message := firstDeleteMessage(t, command)
	if message.err == nil || !strings.Contains(message.err.Error(), "restored 1 branch, failed 1: first: branch already exists") {
		t.Fatalf("restore batch error = %v, want restored and failed counts", message.err)
	}
	if !hasCommand(runner.commands, "/repo/main|git branch second 2222222222222222222222222222222222222222") {
		t.Fatalf("restore batch should continue after first failure: %v", runner.commands)
	}
	updated, _ := updateModel(t, started, message)
	if !strings.Contains(updated.flash, "restored 1 branch, failed 1") {
		t.Fatalf("restore feedback = %q, want restored and failed counts", updated.flash)
	}
}

func TestRestoreOfferClearsWithFeedbackLifecycle(t *testing.T) {
	restore := &pendingBranchRestore{branch: "feature", sha: "0123456789abcdef0123456789abcdef01234567", short: "0123456"}
	model := Model{
		feedback:       restoreOfferFeedback(*restore),
		feedbackID:     5,
		pendingRestore: restore,
	}

	stale, _ := updateModel(t, model, clearFeedbackMsg{id: 4})
	if stale.pendingRestore == nil || stale.feedback.plainText() == "" {
		t.Fatalf("stale clear should keep offer, got flash %q restore %+v", stale.feedback.plainText(), stale.pendingRestore)
	}

	cleared, _ := updateModel(t, model, clearFeedbackMsg{id: 5})
	if cleared.pendingRestore != nil || cleared.feedback.plainText() != "" {
		t.Fatalf("matching clear should remove offer, got flash %q restore %+v", cleared.feedback.plainText(), cleared.pendingRestore)
	}

	model.pendingRestore = restore
	model.feedback = restoreOfferFeedback(*restore)
	autoRefreshed, autoRefreshCmd := updateModel(t, model, autoRefreshMsg{})
	if autoRefreshed.pendingRestore == nil || *autoRefreshed.pendingRestore != *restore || autoRefreshed.feedback.plainText() != model.feedback.plainText() {
		t.Fatalf("auto refresh should preserve offer, got feedback %q restore %+v", autoRefreshed.feedback.plainText(), autoRefreshed.pendingRestore)
	}
	if autoRefreshCmd == nil {
		t.Fatal("auto refresh should schedule the next tick while the offer is live")
	}

	model.pendingRestore = restore
	model.feedback = restoreOfferFeedback(*restore)
	refreshed, _ := model.startRefresh(false, false)
	if refreshed.pendingRestore != nil || refreshed.feedback.plainText() != "" {
		t.Fatalf("manual refresh should clear offer, got flash %q restore %+v", refreshed.feedback.plainText(), refreshed.pendingRestore)
	}

	model.pendingRestore = restore
	model.feedback = restoreOfferFeedback(*restore)
	deleting, _ := model.startDelete("deleted worktree", nil, func(context.Context) error { return nil })
	if deleting.pendingRestore != nil || deleting.feedback.plainText() != "" {
		t.Fatalf("startDelete should clear offer, got flash %q restore %+v", deleting.feedback.plainText(), deleting.pendingRestore)
	}
}

func TestRestoreOfferRendersAsRefreshSuccessFeedback(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	model, _ := Model{}.setRestoreOffer(pendingBranchRestore{branch: "feature", sha: "0123456789abcdef0123456789abcdef01234567", short: "0123456"})

	output := model.worktreesFeedback()
	want := restoreOfferFeedback(pendingBranchRestore{branch: "feature", sha: "0123456789abcdef0123456789abcdef01234567", short: "0123456"}).render()
	if output != want {
		t.Fatalf("worktreesFeedback() = %q, want %q", output, want)
	}
	if got := ansi.Strip(output); got != "✓ deleted feature (0123456) · u to restore" {
		t.Fatalf("restore offer text = %q, want success glyph and restore copy", got)
	}
	if !strings.Contains(output, "\x1b[38;5;42m") {
		t.Fatalf("restore offer should use green SGR, got %q", output)
	}
	if !strings.Contains(output, refreshSuccessStyle.Bold(true).Render("u")) {
		t.Fatalf("restore offer should bold the restore key, got %q", output)
	}
}

func TestDeleteBranchProgressRendersInBottomBorder(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})
	model.state.Branches = []gitdata.Branch{{Name: "feature"}}
	model.filter = filterBranches
	model, _ = model.openDelete()
	model.deleteInFlight = true

	output := model.renderDeleteAtWidth(80)
	outputLines := strings.Split(output, "\n")

	if !strings.Contains(outputLines[len(outputLines)-1], "⠋ deleting") {
		t.Fatalf("branch delete modal should show progress in the bottom border:\n%s", output)
	}
	if strings.Count(ansi.Strip(output), "deleting") != 1 {
		t.Fatalf("branch delete modal should render progress once:\n%s", output)
	}
}

func TestDeleteErrorStaysInDeleteModal(t *testing.T) {
	results := stableLoadResults("worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n")
	results["/repo/main|git worktree remove /repo/feature"] = recordingResult{err: errors.New("cannot remove worktree")}
	runner := &recordingRunner{results: results}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
		{Path: "/repo/feature", Branch: "feature"},
	})
	model.runner = runner
	model.selected = 1
	model, _ = model.openDelete()
	started, cmd := model.updateDelete(tea.KeyMsg{Type: tea.KeyEnter})
	batch := cmd().(tea.BatchMsg)
	message := batch[0]().(deleteMsg)

	updated, _ := updateModel(t, started, message)

	if updated.deleteDialog == nil {
		t.Fatal("delete error should keep the delete dialog open")
	}
	if updated.deleteInFlight {
		t.Fatal("delete error should clear in-flight state")
	}
	if updated.flash != "" {
		t.Fatalf("delete error should not use generic flash, got %q", updated.flash)
	}
	output := updated.renderDeleteAtWidth(80)
	if !strings.Contains(output, "× cannot remove worktree") {
		t.Fatalf("delete modal should show command error:\n%s", output)
	}
}

func TestDeleteEscCancelsInFlightAction(t *testing.T) {
	runner := &recordingRunner{results: stableLoadResults("worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n")}
	model := testModelWithRows([]gitdata.Worktree{{Path: "/repo/main", Branch: "main", IsMain: true}})
	model.runner = runner
	model.deleteDialog = &deleteDialog{}

	started, command := model.startDelete("deleted worktree", nil, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	cancelled, cancelCommand := started.updateDelete(tea.KeyMsg{Type: tea.KeyEsc})
	if cancelCommand != nil {
		t.Fatal("Esc should cancel the delete action without starting another command")
	}

	message := firstDeleteMessage(t, command)
	if !errors.Is(message.err, context.Canceled) {
		t.Fatalf("delete error = %v, want context cancellation", message.err)
	}
	updated, _ := updateModel(t, cancelled, message)
	if updated.deleteInFlight {
		t.Fatal("cancelled delete should clear in-flight state")
	}
	if !strings.Contains(updated.deleteDialog.error, context.Canceled.Error()) {
		t.Fatalf("delete dialog error = %q, want cancellation", updated.deleteDialog.error)
	}
}

func TestDeletePartialFailureReloadsStateAndNamesRemainingBranchAction(t *testing.T) {
	results := stableLoadResults("worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n")
	results["/repo/main|git worktree remove /repo/feature"] = recordingResult{}
	results["/repo/main|git branch -d feature"] = recordingResult{err: errors.New("branch deletion failed")}
	runner := &recordingRunner{results: results}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
		{Path: "/repo/feature", Branch: "feature", BranchMergedToMain: true},
	})
	model.runner = runner
	model.selected = 1
	model, _ = model.openDelete()
	started, command := model.updateDelete(tea.KeyMsg{Type: tea.KeyEnter})
	message := firstDeleteMessage(t, command)

	if !message.reloaded || len(message.state.Rows) != 1 {
		t.Fatalf("partial delete message = %+v, want reloaded main-only state", message)
	}
	updated, _ := updateModel(t, started, message)
	if len(updated.state.Rows) != 1 || updated.state.Rows[0].Path != "/repo/main" {
		t.Fatalf("rows after partial delete = %+v, want main only", updated.state.Rows)
	}
	if !strings.Contains(updated.deleteDialog.error, `delete remaining branch "feature"`) {
		t.Fatalf("partial delete dialog error = %q, want remaining branch action", updated.deleteDialog.error)
	}
}

func TestDeleteErrorWrapsWithinModal(t *testing.T) {
	longError := "warning: not deleting branch 'codex/fix-scan-hot-path-regressions' that is not yet merged to 'refs/remotes/origin/codex/fix-scan-hot-path-regressions', even though it is merged to HEAD"
	results := stableLoadResults("worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n")
	results["/repo/main|git worktree remove /repo/feature"] = recordingResult{err: errors.New(longError)}
	runner := &recordingRunner{results: results}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
		{Path: "/repo/feature", Branch: "feature"},
	})
	model.runner = runner
	model.width = 100
	model.height = 30
	model.selected = 1
	model, _ = model.openDelete()
	started, cmd := model.updateDelete(tea.KeyMsg{Type: tea.KeyEnter})
	batch := cmd().(tea.BatchMsg)
	message := batch[0]().(deleteMsg)

	updated, _ := updateModel(t, started, message)

	width := 80
	output := updated.renderDeleteAtWidth(width)
	for _, line := range strings.Split(output, "\n") {
		if lipgloss.Width(line) > width {
			t.Fatalf("delete modal line exceeds width %d: %q", width, line)
		}
	}
	if !strings.Contains(output, "not deleting branch") {
		t.Fatalf("delete modal should show wrapped error:\n%s", output)
	}
}

func TestDeleteErrorDropsGitHints(t *testing.T) {
	gitError := "warning: not deleting branch 'feature' that is not yet merged to 'origin/feature'\n" +
		"error: the branch 'feature' is not fully merged\n" +
		"hint: If you are sure you want to delete it, run 'git branch -D feature'\n" +
		"hint: Disable this message with \"git config set advice.forceDeleteBranch false\""
	lines := deleteErrorLines(gitError, 80)
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "hint:") {
		t.Fatalf("delete error should drop git hint lines:\n%s", joined)
	}
	if !strings.Contains(joined, "not fully merged") {
		t.Fatalf("delete error should keep the actionable error:\n%s", joined)
	}
	if len(lines) > maxDeleteErrorLines {
		t.Fatalf("delete error block = %d lines, want <= %d", len(lines), maxDeleteErrorLines)
	}
}

func TestDeleteErrorClipsToTerminalHeight(t *testing.T) {
	longError := strings.Repeat("warning: branch is not yet merged and cannot be deleted safely. ", 12)
	results := stableLoadResults("worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n")
	results["/repo/main|git worktree remove /repo/feature"] = recordingResult{err: errors.New(longError)}
	runner := &recordingRunner{results: results}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
		{Path: "/repo/feature", Branch: "feature"},
	})
	model.runner = runner
	model.width = 100
	model.height = 16
	model.selected = 1
	model, _ = model.openDelete()
	started, cmd := model.updateDelete(tea.KeyMsg{Type: tea.KeyEnter})
	batch := cmd().(tea.BatchMsg)
	message := batch[0]().(deleteMsg)

	updated, _ := updateModel(t, started, message)

	output := updated.renderDeleteAtWidth(60)
	if got := lineCount(output); got > updated.height {
		t.Fatalf("delete modal height %d exceeds terminal height %d:\n%s", got, updated.height, output)
	}
	if !strings.Contains(output, "resize for full message") {
		t.Fatalf("clipped delete modal should mark truncation:\n%s", output)
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

func TestConfigReloadedMessageUpdatesBranchVisibility(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})
	model.state.Branches = []gitdata.Branch{{Name: "feature/branch"}}

	updated, _ := model.Update(configReloadedMsg{config: appconfig.Config{ShowBranches: true}})
	model = updated.(Model)

	if !model.showBranches {
		t.Fatal("config reload should enable branch rows")
	}
	if got := strings.Join(visibleBranches(model), ","); got != "main,feature/branch" {
		t.Fatalf("visible branches = %q, want main,feature/branch", got)
	}

	updated, _ = model.Update(configReloadedMsg{config: appconfig.Config{ShowBranches: false}})
	model = updated.(Model)

	if model.showBranches {
		t.Fatal("config reload should disable branch rows")
	}
	if got := strings.Join(visibleBranches(model), ","); got != "main" {
		t.Fatalf("visible branches = %q, want main", got)
	}
}

func TestLocalBranchNamesDedupesWorktreeAndBranchRows(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main"},
		{Path: "/repo/feature", Branch: "feature"},
		{Path: "/repo/detached", Branch: "feature", Detached: true},
	})
	model.state.Branches = []gitdata.Branch{{Name: "feature"}, {Name: "topic"}}

	names := model.localBranchNames()

	want := []string{"main", "feature", "topic"}
	if len(names) != len(want) {
		t.Fatalf("localBranchNames() = %v, want %v", names, want)
	}
	for index, branch := range want {
		if names[index] != branch {
			t.Fatalf("localBranchNames() = %v, want %v", names, want)
		}
	}
}

func TestSelectedBranchGraphLoadsLazilyForSelectedRow(t *testing.T) {
	const root = "/repo/main"
	const ref = "refs/heads/feature/x"
	runner := &recordingRunner{results: map[string]recordingResult{
		root + "|git log -n 5 --format=%h%x1f%s refs/heads/main.." + ref: {output: "aaaaaaa\x1fwire handler\n"},
		root + "|git merge-base " + ref + " refs/heads/main":             {output: "fff0000\n"},
		root + "|git log -n 12 --format=%h%x1f%s fff0000":                 {output: "fff0000\x1ffork base\n"},
	}}
	model := testModelWithRows([]gitdata.Worktree{
		{Path: root, Branch: "main", IsMain: true},
	})
	model.runner = runner
	model.enrichmentContext = context.Background()
	model.state.Repo.MainBranch = "main"
	model.state.Branches = []gitdata.Branch{{Name: "feature/x", MainSync: gitdata.SyncState{Available: true, Ahead: 1}}}
	model.filter = filterBranches
	model.selected = 0

	command := model.selectedBranchGraphCommand(model.enrichmentID)
	if command == nil {
		t.Fatal("selectedBranchGraphCommand() = nil, want a command for the selected branch row")
	}
	message, ok := command().(branchGraphLoadedMsg)
	if !ok {
		t.Fatalf("command produced %T, want branchGraphLoadedMsg", command())
	}
	if message.name != "feature/x" || !message.graph.Loaded {
		t.Fatalf("branchGraphLoadedMsg = %+v, want a loaded graph for feature/x", message)
	}

	updated, _ := model.Update(message)
	updatedModel := updated.(Model)
	if !updatedModel.state.Branches[0].Graph.Loaded {
		t.Fatal("Update(branchGraphLoadedMsg) did not attach the graph to the branch")
	}

	// A second call is a no-op: the graph is already loaded for this selection.
	if again := updatedModel.selectedBranchGraphCommand(model.enrichmentID); again != nil {
		t.Fatal("selectedBranchGraphCommand() should return nil once the branch graph is loaded")
	}
}

func TestSelectedBranchGraphSkipsWorktreeAndMainRows(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true},
	})
	model.runner = &recordingRunner{}
	model.enrichmentContext = context.Background()
	model.selected = 0 // the main worktree row, not a branch

	if command := model.selectedBranchGraphCommand(model.enrichmentID); command != nil {
		t.Fatal("selectedBranchGraphCommand() should return nil for a worktree row")
	}
}

func TestLocalBranchNamesExcludesMainBranch(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "master", IsMain: true},
		{Path: "/repo/feature", Branch: "feature"},
	})
	model.state.Repo.MainBranch = "master"

	names := model.localBranchNames()

	for _, name := range names {
		if name == "master" {
			t.Fatalf("localBranchNames() = %v, must not query the main branch", names)
		}
	}
	if len(names) != 1 || names[0] != "feature" {
		t.Fatalf("localBranchNames() = %v, want [feature]", names)
	}
}

func TestPullRequestLoadWithIncludedCIMarksChecked(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/feature", Branch: "feature"},
	})
	updated, _ := updateModel(t, model, prLoadedMsg{
		pullRequests: map[string]gitdata.PullRequest{"feature": {Number: 42, State: "○", CI: "✓"}},
		enabled:      true,
		ciIncluded:   true,
		repoRoot:     model.state.Repo.Root,
		id:           model.enrichmentID,
		checkedAt:    time.Now(),
	})

	if !updated.prCIChecked[42] {
		t.Fatalf("prCIChecked = %v, want PR 42 marked so lazy CI is skipped", updated.prCIChecked)
	}
	if updated.state.Rows[0].PR == nil || updated.state.Rows[0].PR.CI != "✓" {
		t.Fatalf("row PR = %+v, want CI already attached", updated.state.Rows[0].PR)
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
		config: appconfig.Default(),
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

func modelWithPullRequestDialog(summaries []github.PullRequestSummary) Model {
	input := textinput.New()
	input.Prompt = "> "
	input.Cursor.Style = flashStyle
	input.Focus()
	return Model{
		width:  100,
		height: 24,
		config: appconfig.Default(),
		runner: testRunner{},
		state: gitdata.State{
			Repo: gitdata.Repository{
				Root:           "/repo/main",
				ActiveWorktree: "/repo/main",
			},
		},
		pullRequestDialog: &pullRequestCheckoutDialog{
			input:     input,
			summaries: summaries,
			id:        1,
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

	message := reloadCmd("/repo/main", appconfig.Config{}, runner, model.state.Repo, true, false, model.refreshID)().(reloadMsg)
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

func firstDeleteMessage(t *testing.T, cmd tea.Cmd) deleteMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("delete command is nil")
	}
	batchMessage := cmd()
	batch, ok := batchMessage.(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("delete command returned %T, want BatchMsg with delete command", batchMessage)
	}
	firstMessage := batch[0]()
	message, ok := firstMessage.(deleteMsg)
	if !ok {
		t.Fatalf("first delete batch message = %T, want deleteMsg", firstMessage)
	}
	return message
}

func firstCleanupMergedMessage(t *testing.T, cmd tea.Cmd) cleanupMergedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("cleanup command is nil")
	}
	batchMessage := cmd()
	batch, ok := batchMessage.(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("cleanup command returned %T, want BatchMsg with cleanup command", batchMessage)
	}
	firstMessage := batch[0]()
	message, ok := firstMessage.(cleanupMergedMsg)
	if !ok {
		t.Fatalf("first cleanup batch message = %T, want cleanupMergedMsg", firstMessage)
	}
	return message
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

func stableLoadResults(worktreeList string) map[string]recordingResult {
	return map[string]recordingResult{
		"/repo/main|git rev-parse --show-toplevel":                         {output: "/repo/main\n"},
		"/repo/main|git rev-parse --git-common-dir":                        {output: ".git\n"},
		"/repo/main|git rev-parse --path-format=absolute --git-common-dir": {output: "/repo/main/.git\n"},
		"/repo/main|git worktree list --porcelain":                         {output: worktreeList},
		"/repo/main|git symbolic-ref --short refs/remotes/origin/HEAD":     {err: errors.New("no origin")},
		"/repo/main|git show-ref --verify --quiet refs/heads/main":         {},
		"/repo/main|git remote":                                            {},
	}
}

func hasCommand(commands []string, want string) bool {
	for _, command := range commands {
		if command == want {
			return true
		}
	}
	return false
}

func hasCleanupSkip(skips []cleanupMergedSkip, want cleanupMergedSkip) bool {
	for _, skip := range skips {
		if skip.name == want.name && skip.reason == want.reason {
			return true
		}
	}
	return false
}

func visibleBranches(model Model) []string {
	indexes := model.visibleIndexes()
	rows := model.tableRows()
	branches := make([]string, 0, len(indexes))
	for _, index := range indexes {
		branches = append(branches, rows[index].DisplayBranch())
	}
	return branches
}

// The detail region below the list is taller for some rows (a dirty worktree adds a
// Changes frame) than others, so sizing the list against the selected row's own
// detail made rows appear and disappear while navigating. The list must instead be
// sized for the tallest row and stay fixed.
func TestViewKeepsListHeightStableAcrossSelection(t *testing.T) {
	rows := []gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true},
		{
			Path:   "/repo/dirty",
			Branch: "feature/dirty",
			Status: gitdata.StatusCounts{Modified: 2, Untracked: 1},
			ChangedFiles: []gitdata.ChangedFile{
				{Path: "a.go", WorkCode: 'M', Added: 5, Deleted: 1},
				{Path: "b.go", WorkCode: 'M', Added: 2},
				{Path: "c.go", WorkCode: '?', Added: -1, Deleted: -1},
			},
		},
	}
	for index := range 6 {
		rows = append(rows, gitdata.Worktree{Path: fmt.Sprintf("/repo/clean%d", index), Branch: fmt.Sprintf("feature/clean%d", index)})
	}
	model := testModelWithRows(rows)
	model.width = 100
	// A height that cannot fit every row leaves the detail region competing with the
	// list for lines, which is when the jitter showed up.
	model.height = 24
	now := time.Now()

	want := -1
	for selected := range model.totalRowCount() {
		model.selected = selected
		snapshot := model.viewSnapshot(now, max(1, model.width-6))
		if want < 0 {
			want = len(snapshot.visibleRows)
			if want >= model.totalRowCount() {
				t.Fatalf("test setup shows all %d rows at once, nothing to scroll", want)
			}
			continue
		}
		if got := len(snapshot.visibleRows); got != want {
			t.Fatalf("row %d shows %d list rows, want %d", selected, got, want)
		}
	}
}
