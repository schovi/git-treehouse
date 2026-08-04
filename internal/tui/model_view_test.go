package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	appconfig "github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/github"
	"github.com/schovi/git-treehouse/internal/listview"
	"strings"
	"testing"
	"time"
)

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
	}, appconfig.Default(), testRunner{}, false, false)
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
	}, appconfig.Default(), testRunner{}, false, false)
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

func TestNewDisablesGitHubEnrichmentFromConfigOrFlag(t *testing.T) {
	for _, test := range []struct {
		name        string
		config      appconfig.Config
		noGitHub    bool
		noGitHubSet bool
	}{
		{name: "config", config: appconfig.Config{GitHub: false}},
		{name: "flag", config: appconfig.Default(), noGitHub: true, noGitHubSet: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &recordingRunner{}
			model := New(gitdata.State{
				Repo: gitdata.Repository{Root: "/repo/main", ActiveWorktree: "/repo/main", RemoteConfigured: true},
				Rows: []gitdata.Worktree{{
					Path:                "/repo/main",
					Branch:              "main",
					LocalMetadataLoaded: true,
					GitSizeLoaded:       true,
					PR:                  &gitdata.PullRequest{Number: 42, State: "○"},
				}},
			}, test.config, runner, test.noGitHub, test.noGitHubSet)
			model.width = 160
			model.prReview = map[int]github.PullRequestReview{42: {Loaded: true, Number: 42}}

			if model.showPR || model.prLoading {
				t.Fatalf("GitHub opt-out should hide and stop PR loading: showPR=%t prLoading=%t", model.showPR, model.prLoading)
			}
			if command := model.pullRequestFetchCommand(context.Background(), model.enrichmentID); command != nil {
				t.Fatal("GitHub opt-out returned a PR fetch command")
			}
			updated, _ := model.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
			if updated.flash != "GitHub is disabled" {
				t.Fatalf("GitHub opt-out p action flash = %q, want disabled message", updated.flash)
			}
			updated, _ = model.openPullRequestCheckout()
			if updated.pullRequestDialog != nil {
				t.Fatal("GitHub opt-out opened the PR checkout dialog")
			}
			if review := model.reviewForRow(model.tableRows()[0]); review != nil {
				t.Fatalf("GitHub opt-out rendered PR review: %+v", review)
			}
			if output := ansi.Strip(model.View()); strings.Contains(output, "#42") {
				t.Fatalf("GitHub opt-out rendered PR data:\n%s", output)
			}
			for _, command := range runner.commands {
				if strings.Contains(command, "|gh ") {
					t.Fatalf("GitHub opt-out ran gh command: %s", command)
				}
			}
		})
	}
}

func TestNewAllowsExplicitFalseNoGitHubToOverrideConfig(t *testing.T) {
	model := New(gitdata.State{
		Repo: gitdata.Repository{Root: "/repo/main", ActiveWorktree: "/repo/main", RemoteConfigured: true},
		Rows: []gitdata.Worktree{{Path: "/repo/main", Branch: "main", LocalMetadataLoaded: true, GitSizeLoaded: true}},
	}, appconfig.Config{GitHub: false}, &recordingRunner{}, false, true)

	if !model.showPR || !model.prLoading {
		t.Fatalf("explicit --no-github=false should enable PRs: showPR=%t prLoading=%t", model.showPR, model.prLoading)
	}
	if command := model.pullRequestFetchCommand(context.Background(), model.enrichmentID); command == nil {
		t.Fatal("explicit --no-github=false did not create a PR fetch command")
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

	for _, want := range []string{"↑", "█", "↓", "Tab", "filter:", "all", "s", "search", "0/18"} {
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

func TestViewKeepsScrollbarFooterHintsWithinNarrowWidth(t *testing.T) {
	rows := make([]gitdata.Worktree, 18)
	rows[0] = gitdata.Worktree{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true}
	for index := 1; index < len(rows); index++ {
		rows[index] = gitdata.Worktree{Path: fmt.Sprintf("/repo/worktree-%d", index), Branch: fmt.Sprintf("worktree-%d", index)}
	}
	model := testModelWithRows(rows)
	model.width = 56
	model.height = 24

	output := ansi.Strip(model.View())

	if !strings.Contains(output, "Tab filter: all · s search") {
		t.Fatalf("View() should keep the filter and search hints before dropping scroll position:\n%s", output)
	}
	_, rightFooter := model.listFooterHintsForScrollbar(listScrollbar{total: len(rows), visible: 7}, model.width-6)
	if strings.Contains(rightFooter, "…") {
		t.Fatalf("listFooterHintsForScrollbar() should drop whole hints, not truncate them: %q", rightFooter)
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

func TestViewCachesUnselectedDetailBlockHeights(t *testing.T) {
	rows := make([]gitdata.Worktree, 100)
	rows[0] = gitdata.Worktree{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true}
	rows[1] = gitdata.Worktree{Path: "/repo/finished", Branch: "finished", BranchMergedToMain: true}
	for index := 2; index < len(rows); index++ {
		rows[index] = gitdata.Worktree{Path: fmt.Sprintf("/repo/worktree-%d", index), Branch: fmt.Sprintf("worktree-%d", index)}
	}
	model := testModelWithRows(rows)
	model.width = 100
	model.height = 200

	model.View()
	if model.detailHeightCache == nil || model.detailHeightCache.input == "" {
		t.Fatal("View() did not measure and cache the visible detail height")
	}

	// A selected row must still render fresh, but the 100-row height pass must not.
	model.detailHeightCache.maxBlockLines = 1000
	model.selected = 1
	start, end := model.visibleTableWindow(time.Now())
	if end-start != 1 {
		t.Fatalf("visibleTableWindow() recomputed cached height: window = %d:%d, want one row", start, end)
	}
	output := ansi.Strip(model.View())
	if !strings.Contains(output, "finished: clean, merged to main — safe to remove (d)") {
		t.Fatalf("View() did not render the newly selected detail block:\n%s", output)
	}
	if model.detailHeightCache.maxBlockLines != 1000 {
		t.Fatalf("View() recomputed unchanged visible-row height: got %d, want cached 1000", model.detailHeightCache.maxBlockLines)
	}
}

func TestDetailHeightCacheInvalidatesForDetailInputs(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{
		{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true},
		{Path: "/repo/feature", Branch: "feature", PR: &gitdata.PullRequest{Number: 42}},
	})
	model.width = 100
	model.height = 30
	model.showPR = true
	model.prReview = map[int]github.PullRequestReview{}
	now := time.Now()

	model.viewSnapshot(now, model.width-6)
	assertRecomputed := func(name string, update func()) {
		t.Helper()
		model.detailHeightCache.maxBlockLines = -1
		update()
		model.viewSnapshot(now, model.width-6)
		if model.detailHeightCache.maxBlockLines == -1 {
			t.Fatalf("%s did not recompute cached detail height", name)
		}
	}

	assertRecomputed("visible rows", func() {
		model.search.SetValue("feature")
		model.selected = 0
	})
	assertRecomputed("enrichment", func() {
		model.state.Rows[1].Status.Modified = 1
		model.state.Rows[1].ChangedFiles = []gitdata.ChangedFile{{Path: "detail.go", WorkCode: 'M', Added: 1}}
	})
	assertRecomputed("PR review", func() {
		model.prReview[42] = github.PullRequestReview{Loaded: true, Number: 42, Checks: []github.Check{{State: github.CheckFail}}}
	})
	assertRecomputed("GitHub visibility", func() {
		model.showPR = false
	})

	model.detailHeightCache.maxBlockLines = -1
	model.viewSnapshot(now, model.width-7)
	if model.detailHeightCache.maxBlockLines == -1 {
		t.Fatal("panel width did not recompute cached detail height")
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
