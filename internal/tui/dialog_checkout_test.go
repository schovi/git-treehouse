package tui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	appconfig "github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/github"
	"strings"
	"testing"
)

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
	t.Setenv("TMUX", "")
	t.Setenv("ZELLIJ", "")
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
	t.Setenv("TMUX", "")
	t.Setenv("ZELLIJ", "")
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
