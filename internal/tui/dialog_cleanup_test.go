package tui

import (
	"context"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	appconfig "github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"slices"
	"strings"
	"testing"
)

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
