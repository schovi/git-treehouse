package tui

import (
	"context"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	appconfig "github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"slices"
	"strings"
	"testing"
)

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
