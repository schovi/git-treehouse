package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"strconv"
	"time"
)

type cleanupMergedDialog struct {
	plan   cleanupMergedPlan
	result *cleanupMergedResult
	error  string
}

type cleanupMergedPlan struct {
	worktrees []cleanupMergedWorktree
	branches  []cleanupMergedBranch
	skips     []cleanupMergedSkip
}

type cleanupMergedWorktree struct {
	row              gitdata.Worktree
	deleteBranch     bool
	runBeforeDelete  bool
	beforeDeleteHook string
}

type cleanupMergedBranch struct {
	branch gitdata.Branch
}

type cleanupMergedSkip struct {
	name   string
	reason string
}

type cleanupMergedFailure struct {
	name   string
	reason string
}

type cleanupMergedResult struct {
	removedWorktrees int
	deletedBranches  int
	skipped          int
	failures         []cleanupMergedFailure
	restores         []pendingBranchRestore
}

type pendingBranchRestore struct {
	branch string
	sha    string
	short  string
}

func (model Model) openCleanupMerged() (Model, tea.Cmd) {
	plan := model.planCleanupMerged()
	if !plan.hasActions() {
		return model.setFlash("no merged worktrees or branches to clean up")
	}
	model.help = false
	model.paletteDialog = nil
	model.filterDialog = nil
	model.createDialog = nil
	model.checkoutDialog = nil
	model.branchWorktreeDialog = nil
	model.deleteDialog = nil
	model.pullRequestDialog = nil
	model.cleanupMergedDialog = &cleanupMergedDialog{plan: plan}
	return model, nil
}

func (model Model) updateCleanupMerged(message tea.KeyMsg) (Model, tea.Cmd) {
	if model.cleanupMergedInFlight {
		if message.String() == "esc" && model.actionCancel != nil {
			model.actionCancel()
		}
		return model, nil
	}
	switch message.String() {
	case "esc":
		model.cleanupMergedDialog = nil
		return model, nil
	case "u":
		if model.cleanupMergedDialog != nil && model.cleanupMergedDialog.result != nil && model.hasPendingRestore() {
			model.cleanupMergedDialog = nil
			return model.startRestore()
		}
		return model, nil
	case "enter":
		if model.cleanupMergedDialog == nil || model.cleanupMergedDialog.result != nil {
			return model, nil
		}
		return model.startCleanupMerged(model.cleanupMergedDialog.plan)
	}
	return model, nil
}

func (model Model) startCleanupMerged(plan cleanupMergedPlan) (Model, tea.Cmd) {
	if model.actionCancel != nil {
		model.actionCancel()
	}
	model = model.cancelEnrichment()
	model.enrichmentID++
	model.cleanupMergedID++
	model.cleanupMergedInFlight = true
	model.cleanupMergedSpinner = 0
	model.refreshInFlight = false
	model.refreshID++
	model.refreshAnchor = selectionAnchor{}
	model.refreshProgressVisible = false
	model = model.clearFeedback()
	if model.cleanupMergedDialog != nil {
		model.cleanupMergedDialog.error = ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), destructiveActionTimeout)
	model.actionCancel = cancel
	command := cleanupMergedAndLoadCmd(ctx, model.reloadCwd(), model.config, model.state.Repo, model.runner, model.cleanupMergedID, plan)
	return model, tea.Batch(command, cleanupMergedSpinnerTickCmd(model.cleanupMergedID))
}

func cleanupMergedAndLoadCmd(ctx context.Context, cwd string, config config.Config, repo gitdata.Repository, runner gitdata.Runner, id int, plan cleanupMergedPlan) tea.Cmd {
	return func() tea.Msg {
		result := runCleanupMerged(ctx, repo, plan, runner)
		reloadCtx, cancel := context.WithTimeout(context.Background(), destructiveActionTimeout)
		defer cancel()
		state, err := loadStableState(reloadCtx, cwd, config, runner, repo.GitVersion, nil)
		if err != nil {
			return cleanupMergedMsg{id: id, result: result, err: fmt.Errorf("cleaned up merged, but reload failed: %w", err)}
		}
		repoConfig, hooksApproved, err := loadRepoRuntimeConfig(reloadCtx, state.Repo.Root, runner)
		if err != nil {
			return cleanupMergedMsg{id: id, result: result, err: fmt.Errorf("cleaned up merged, but reload failed: %w", err)}
		}
		return cleanupMergedMsg{id: id, state: state, repoConfig: repoConfig, hooksApproved: hooksApproved, reloaded: true, result: result, completedAt: time.Now()}
	}
}

func runCleanupMerged(ctx context.Context, repo gitdata.Repository, plan cleanupMergedPlan, runner gitdata.Runner) cleanupMergedResult {
	result := cleanupMergedResult{skipped: len(plan.skips)}
	for _, item := range plan.worktrees {
		if item.runBeforeDelete && item.beforeDeleteHook != "" {
			if err := gitdata.RunHook(ctx, item.row.Path, item.beforeDeleteHook, hookEnv("before_delete", item.row.Path, item.row.Branch, repo.Root, repo.MainBranch), runner); err != nil {
				result.failures = append(result.failures, cleanupMergedFailure{name: cleanupWorktreeName(item.row), reason: err.Error()})
				continue
			}
		}
		if err := gitdata.RemoveWorktree(ctx, repo.Root, item.row.Path, false, runner); err != nil {
			result.failures = append(result.failures, cleanupMergedFailure{name: cleanupWorktreeName(item.row), reason: err.Error()})
			continue
		}
		result.removedWorktrees++
		if !item.deleteBranch {
			continue
		}
		if err := gitdata.DeleteBranch(ctx, repo.Root, item.row.Branch, false, runner); err != nil {
			result.failures = append(result.failures, cleanupMergedFailure{name: item.row.Branch, reason: err.Error()})
			continue
		}
		result.deletedBranches++
		if restore, ok := restoreFromWorktree(item.row); ok {
			result.restores = append(result.restores, restore)
		}
	}
	for _, item := range plan.branches {
		if err := gitdata.DeleteBranch(ctx, repo.Root, item.branch.Name, false, runner); err != nil {
			result.failures = append(result.failures, cleanupMergedFailure{name: item.branch.Name, reason: err.Error()})
			continue
		}
		result.deletedBranches++
		if restore, ok := restoreFromBranch(item.branch); ok {
			result.restores = append(result.restores, restore)
		}
	}
	return result
}

func (model Model) planCleanupMerged() cleanupMergedPlan {
	rows := model.state.TableRows(true)
	plan := cleanupMergedPlan{}
	for _, row := range rows {
		if !cleanupMergedDone(row) {
			continue
		}
		if row.IsBranch() {
			branch := row.Branch
			switch {
			case branch.Name == "":
				plan.skips = append(plan.skips, cleanupMergedSkip{name: "(unnamed branch)", reason: "branch name is missing"})
			case model.state.Repo.MainBranch != "" && branch.Name == model.state.Repo.MainBranch:
				plan.skips = append(plan.skips, cleanupMergedSkip{name: branch.Name, reason: "main branch"})
			case !branch.BranchMergedToMain:
				plan.skips = append(plan.skips, cleanupMergedSkip{name: branch.Name, reason: "branch is not merged into " + model.deleteMainBranchName()})
			default:
				plan.branches = append(plan.branches, cleanupMergedBranch{branch: branch})
			}
			continue
		}
		worktree := row.Worktree
		if reason := model.cleanupMergedWorktreeSkipReason(worktree); reason != "" {
			plan.skips = append(plan.skips, cleanupMergedSkip{name: cleanupWorktreeName(worktree), reason: reason})
			continue
		}
		plan.worktrees = append(plan.worktrees, cleanupMergedWorktree{
			row:              worktree,
			deleteBranch:     worktree.Branch != "" && worktree.BranchMergedToMain,
			runBeforeDelete:  model.hooksApproved && model.repoConfig.BeforeDelete != "",
			beforeDeleteHook: model.repoConfig.BeforeDelete,
		})
	}
	return plan
}

func cleanupMergedDone(row gitdata.Row) bool {
	if row.IsBranch() {
		return row.Branch.BranchMergedToMain || prMergedOrClosed(row)
	}
	return row.Worktree.BranchMergedToMain || prMergedOrClosed(row)
}

func (model Model) cleanupMergedWorktreeSkipReason(row gitdata.Worktree) string {
	switch {
	case row.IsMain:
		return "main worktree"
	case row.IsActive:
		return "active worktree"
	case row.Detached:
		return "detached worktree"
	case row.Prunable:
		return "missing worktree metadata"
	case row.Locked:
		return "locked worktree"
	case !row.LocalMetadataLoaded:
		return "status is still loading"
	case !row.Status.Clean():
		return "uncommitted changes"
	default:
		return ""
	}
}

func (plan cleanupMergedPlan) hasActions() bool {
	return len(plan.worktrees) > 0 || len(plan.branches) > 0
}

func restoreFromWorktree(row gitdata.Worktree) (pendingBranchRestore, bool) {
	if row.Branch == "" || row.Head == "" {
		return pendingBranchRestore{}, false
	}
	return pendingBranchRestore{branch: row.Branch, sha: row.Head, short: row.CommitShort}, true
}

func restoreFromBranch(branch gitdata.Branch) (pendingBranchRestore, bool) {
	if branch.Name == "" || branch.Head == "" {
		return pendingBranchRestore{}, false
	}
	return pendingBranchRestore{branch: branch.Name, sha: branch.Head, short: branch.CommitShort}, true
}

func cleanupWorktreeName(row gitdata.Worktree) string {
	if row.Branch != "" {
		return row.Branch
	}
	if row.Path != "" {
		return row.Path
	}
	return "(unnamed worktree)"
}

func (model Model) renderCleanupMergedAtWidth(width int) string {
	dialog := model.cleanupMergedDialog
	if dialog == nil {
		return dialogBox("Clean up merged", []string{"No cleanup in progress."}, deleteDialogHintsAtWidth("Esc close", width-6), width)
	}
	if dialog.result != nil {
		return model.renderCleanupMergedResultAtWidth(*dialog.result, dialog.error, width)
	}
	return model.renderCleanupMergedConfirmAtWidth(dialog.plan, dialog.error, width)
}

func (model Model) renderCleanupMergedConfirmAtWidth(plan cleanupMergedPlan, errorText string, width int) string {
	contentWidth := max(1, width-4)
	lines := []string{
		dialogFieldLine("Worktrees", fmt.Sprintf("%d remove", len(plan.worktrees)), contentWidth),
		dialogFieldLine("Branches", fmt.Sprintf("%d delete", plan.branchDeleteCount()), contentWidth),
		"",
	}
	if len(plan.worktrees) > 0 {
		lines = append(lines, deleteSectionTitle("Worktrees"))
		for _, item := range limitedCleanupWorktrees(plan.worktrees, 5) {
			lines = append(lines, deleteDetailLine(cleanupWorktreeLine(item.row, item.deleteBranch)))
		}
		if extra := len(plan.worktrees) - min(len(plan.worktrees), 5); extra > 0 {
			lines = append(lines, deleteDetailLine(hintStyle.Render(fmt.Sprintf("+%d more", extra))))
		}
		lines = append(lines, "")
	}
	if plan.branchOnlyDeleteCount() > 0 {
		lines = append(lines, deleteSectionTitle("Branch-only"))
		for _, item := range limitedCleanupBranches(plan.branches, 5) {
			lines = append(lines, deleteDetailLine(item.branch.DisplayBranch()))
		}
		if extra := len(plan.branches) - min(len(plan.branches), 5); extra > 0 {
			lines = append(lines, deleteDetailLine(hintStyle.Render(fmt.Sprintf("+%d more", extra))))
		}
		lines = append(lines, "")
	}
	lines = append(lines, deleteSectionTitle("Commands"))
	for _, command := range plan.cleanupCommands() {
		lines = append(lines, deleteCommandLine(command, false))
	}
	if errorText != "" {
		lines = append(lines, "", deleteDangerStyle.Render(errorText))
	}
	for index, line := range lines {
		lines[index] = truncateStyled(line, contentWidth)
	}
	return dialogBox("Clean up merged", lines, model.cleanupMergedBottomContent("Enter clean up · Esc cancel", width-6), width)
}

func (model Model) renderCleanupMergedResultAtWidth(result cleanupMergedResult, errorText string, width int) string {
	contentWidth := max(1, width-4)
	lines := []string{
		deleteDangerStyle.Render("Clean up partially completed."),
		"",
		dialogFieldLine("Worktrees", strconv.Itoa(result.removedWorktrees), contentWidth),
		dialogFieldLine("Branches", strconv.Itoa(result.deletedBranches), contentWidth),
		dialogFieldLine("Failures", strconv.Itoa(len(result.failures)), contentWidth),
		"",
		deleteSectionTitle("Failures"),
	}
	for _, failure := range limitedCleanupFailures(result.failures, 6) {
		lines = append(lines, deleteDetailLine(failure.name+": "+failure.reason))
	}
	if extra := len(result.failures) - min(len(result.failures), 6); extra > 0 {
		lines = append(lines, deleteDetailLine(hintStyle.Render(fmt.Sprintf("+%d more", extra))))
	}
	if errorText != "" {
		lines = append(lines, "", deleteDangerStyle.Render(errorText))
	}
	for index, line := range lines {
		lines[index] = truncateStyled(line, contentWidth)
	}
	footer := "Esc close"
	if len(result.restores) > 0 {
		footer = "u restore branches · Esc close"
	}
	return dialogBox("Clean up merged", lines, deleteDialogHintsAtWidth(footer, width-6), width)
}

func (model Model) cleanupMergedProgressText() string {
	frame := refreshSpinnerFrames[model.cleanupMergedSpinner%len(refreshSpinnerFrames)]
	return refreshActivityStyle.Render(frame + " cleaning")
}

func (model Model) cleanupMergedBottomContent(content string, width int) string {
	if model.cleanupMergedInFlight {
		return truncateStyled(model.cleanupMergedProgressText(), width)
	}
	return deleteDialogHintsAtWidth(content, width)
}

func cleanupWorktreeLine(row gitdata.Worktree, deleteBranch bool) string {
	action := "remove worktree"
	if deleteBranch {
		action += ", delete branch"
	}
	return cleanupWorktreeName(row) + " · " + action
}

func limitedCleanupWorktrees(items []cleanupMergedWorktree, limit int) []cleanupMergedWorktree {
	return items[:min(len(items), limit)]
}

func limitedCleanupBranches(items []cleanupMergedBranch, limit int) []cleanupMergedBranch {
	return items[:min(len(items), limit)]
}

func limitedCleanupFailures(items []cleanupMergedFailure, limit int) []cleanupMergedFailure {
	return items[:min(len(items), limit)]
}

func (plan cleanupMergedPlan) branchDeleteCount() int {
	count := len(plan.branches)
	for _, item := range plan.worktrees {
		if item.deleteBranch {
			count++
		}
	}
	return count
}

func (plan cleanupMergedPlan) branchOnlyDeleteCount() int {
	return len(plan.branches)
}

func (plan cleanupMergedPlan) cleanupCommands() []string {
	commands := []string{}
	for _, item := range plan.worktrees {
		if item.runBeforeDelete && item.beforeDeleteHook != "" {
			commands = append(commands, "sh -c "+fmt.Sprintf("%q", item.beforeDeleteHook))
		}
		commands = append(commands, "git worktree remove "+item.row.Path)
		if item.deleteBranch {
			commands = append(commands, "git branch -d "+item.row.Branch)
		}
	}
	for _, item := range plan.branches {
		commands = append(commands, "git branch -d "+item.branch.Name)
	}
	return commands
}
