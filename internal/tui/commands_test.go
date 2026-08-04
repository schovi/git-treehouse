package tui

import (
	"context"
	"errors"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	appconfig "github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/github"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

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

func TestWorktreeDestinationUsesOnlyUnambiguousMultiplexerEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		tmux        string
		zellij      string
		destination worktreeDestination
	}{
		{name: "tmux", tmux: "/tmp/tmux-1000/default,1,0", destination: worktreeDestinationTmux},
		{name: "zellij", zellij: "session", destination: worktreeDestinationZellij},
		{name: "neither", destination: worktreeDestinationGo},
		{name: "both", tmux: "socket", zellij: "session", destination: worktreeDestinationGo},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := worktreeDestinationFromEnvironment(test.tmux, test.zellij); got != test.destination {
				t.Fatalf("worktreeDestinationFromEnvironment(%q, %q) = %v, want %v", test.tmux, test.zellij, got, test.destination)
			}
		})
	}
}

func TestWorktreeDestinationBuildsArgvWithoutShellInterpolation(t *testing.T) {
	path := "/repo/.worktrees/repo/feature; echo nope"
	branch := "feature/with spaces"
	tests := []struct {
		destination worktreeDestination
		command     string
		arguments   []string
	}{
		{worktreeDestinationTmux, "tmux", []string{"new-window", "-c", path, "-n", branch}},
		{worktreeDestinationZellij, "zellij", []string{"action", "new-tab", "--cwd", path, "--name", branch}},
	}

	for _, test := range tests {
		command, arguments, ok := test.destination.command(path, branch)
		if !ok || command != test.command || !slices.Equal(arguments, test.arguments) {
			t.Fatalf("command() = %q, %q, %v; want %q, %q, true", command, arguments, ok, test.command, test.arguments)
		}
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
		root + "|git log -n 12 --format=%h%x1f%s fff0000":                {output: "fff0000\x1ffork base\n"},
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

func TestLoadSizesMsgLoadsSelectedWorktreeFullSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("full size"), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	message := loadSizesMsg(context.Background(), nil, "/repo", 1, nil, nil, filepath.Dir(path)).(sizesLoadedMsg)

	if got, want := message.fullSizes[filepath.Dir(path)], int64(len("full size")); got != want {
		t.Fatalf("full size = %d, want %d", got, want)
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

func TestDiskUsageCommandSkipsCachedAutomaticSizes(t *testing.T) {
	model := testModelWithRows([]gitdata.Worktree{{
		Path:           "/repo/feature",
		Branch:         "feature",
		GitSizeLoaded:  true,
		FullSizeLoaded: true,
	}})
	model.width = 160
	model.height = 24

	if command := model.diskUsageCommand(context.Background(), time.Now(), model.enrichmentID); command != nil {
		t.Fatal("cached automatic sizes should not schedule git ls-files or a filesystem walk")
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
	}, gitdata.State{}, true, false, 9)

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

func TestManualReloadBypassesPriorEnrichment(t *testing.T) {
	worktreeList := "worktree /repo/main\nHEAD aaaaaaaa\nbranch refs/heads/main\n\nworktree /repo/feature\nHEAD bbbbbbbb\nbranch refs/heads/feature\n"
	results := stableLoadResults(worktreeList)
	format := strings.Join([]string{
		"%(refname:short)",
		"%(objectname)",
		"%(objectname:short)",
		"%(committerdate:unix)",
		"%(contents:subject)",
		"%(upstream:short)",
		"%(upstream:track,nobracket)",
		"%(ahead-behind:refs/heads/main)",
	}, "%00")
	results["/repo/main|git for-each-ref --format="+format+" refs/heads"] = recordingResult{output: "main\x00aaaaaaaa\x00aaaaaaa\x001780000000\x00main commit\x00origin/main\x00\x000 0\n" +
		"feature\x00bbbbbbbb\x00bbbbbbb\x001780000100\x00feature commit\x00origin/feature\x00ahead 2, behind 1\x002 5\n"}
	runner := &recordingRunner{results: results}
	priorState := gitdata.State{
		Repo: gitdata.Repository{Root: "/repo/main", MainWorktree: "/repo/main", MainBranch: "main"},
		Rows: []gitdata.Worktree{
			{Path: "/repo/main", Head: "aaaaaaaa", Branch: "main", IsMain: true},
			{Path: "/repo/feature", Head: "bbbbbbbb", Branch: "feature", Graph: gitdata.ContextGraph{Loaded: true, BranchCommits: []gitdata.GraphCommit{{Short: "cached"}}}, GitSizeLoaded: true, FullSizeLoaded: true},
		},
	}

	message := reloadCmd("/repo/main", appconfig.Config{}, runner, priorState.Repo, priorState, false, false, 1)().(reloadMsg)

	if message.err != nil {
		t.Fatalf("reloadCmd() error = %v", message.err)
	}
	var feature gitdata.Worktree
	for _, row := range message.state.Rows {
		if row.Path == "/repo/feature" {
			feature = row
		}
	}
	if len(feature.Graph.BranchCommits) > 0 || feature.GitSizeLoaded || feature.FullSizeLoaded {
		t.Fatalf("manual reload reused automatic enrichment: %+v", feature)
	}
	for _, command := range runner.commands {
		if strings.Contains(command, "/repo/feature|git log") || strings.Contains(command, "/repo/feature|git merge-base") {
			return
		}
	}
	t.Fatalf("manual reload should fetch a fresh graph: %v", runner.commands)
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
