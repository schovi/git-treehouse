package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"strings"
	"time"
)

type deleteDialog struct {
	stage            deleteStage
	deleteWorktree   bool
	deleteBranch     bool
	forceWorktree    bool
	runBeforeDelete  bool
	beforeDeleteHook string
	error            string
}

type deleteStage int

const (
	deleteStageOptions deleteStage = iota
	deleteStagePrune
	deleteStageLocked
)

func (model Model) openDelete() (Model, tea.Cmd) {
	selected, ok := model.selectedTableRow()
	if !ok {
		return model, nil
	}
	if selected.IsBranch() {
		branch := selected.Branch
		if branch.Name == "" {
			return model.setFlash("cannot delete an unnamed branch")
		}
		if model.state.Repo.MainBranch != "" && branch.Name == model.state.Repo.MainBranch {
			return model.setFlash("cannot delete the main branch")
		}
		dialog := deleteDialog{
			stage:        deleteStageOptions,
			deleteBranch: true,
		}
		model.help = false
		model.paletteDialog = nil
		model.createDialog = nil
		model.checkoutDialog = nil
		model.branchWorktreeDialog = nil
		model.deleteDialog = &dialog
		return model, nil
	}
	row := selected.Worktree
	if row.IsActive {
		return model.setFlash("cannot delete the active worktree")
	}
	if row.IsMain {
		return model.setFlash("cannot delete the main worktree")
	}
	dialog := deleteDialog{
		stage:          deleteStageOptions,
		deleteWorktree: deleteWorktreeDefault(row),
		deleteBranch:   deleteBranchDefault(row),
		forceWorktree:  !row.Status.Clean(),
	}
	if model.hooksApproved && model.repoConfig.BeforeDelete != "" && !row.Prunable {
		dialog.runBeforeDelete = true
		dialog.beforeDeleteHook = model.repoConfig.BeforeDelete
	}
	switch {
	case row.Locked:
		dialog.stage = deleteStageLocked
		dialog.error = "cannot delete locked worktree"
		if row.LockReason != "" {
			dialog.error += ": " + row.LockReason
		}
	case row.Prunable:
		dialog.stage = deleteStagePrune
	}
	model.help = false
	model.paletteDialog = nil
	model.createDialog = nil
	model.checkoutDialog = nil
	model.branchWorktreeDialog = nil
	model.deleteDialog = &dialog
	if model.repoConfig.BeforeDelete != "" && !model.hooksApproved && !row.Prunable {
		return model.setFlash("before_delete hook not approved; run git-treehouse allow")
	}
	return model, nil
}

func (model Model) updateDelete(message tea.KeyMsg) (Model, tea.Cmd) {
	if model.deleteInFlight {
		if message.String() == "esc" && model.actionCancel != nil {
			model.actionCancel()
		}
		return model, nil
	}
	dialog := model.deleteDialog
	switch message.String() {
	case "esc":
		model.deleteDialog = nil
		return model, nil
	case " ":
		return model.updateDelete(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	case "t":
		if dialog.stage != deleteStageOptions {
			return model, nil
		}
		row, ok := model.selectedWorktree()
		if !ok {
			return model, nil
		}
		dialog.deleteWorktree = !dialog.deleteWorktree
		if dialog.deleteWorktree {
			dialog.deleteBranch = deleteBranchDefaultWhenWorktreeRemoved(row)
		} else {
			dialog.deleteBranch = false
		}
		dialog.error = ""
		return model, nil
	case "b":
		if dialog.stage != deleteStageOptions || !deleteBranchAvailableFromModel(model) {
			return model, nil
		}
		if !dialog.deleteWorktree {
			dialog.error = "enable worktree removal before deleting the branch"
			return model, nil
		}
		dialog.deleteBranch = !dialog.deleteBranch
		dialog.error = ""
		return model, nil
	case "h":
		if dialog.stage != deleteStageOptions || dialog.beforeDeleteHook == "" {
			return model, nil
		}
		if !dialog.deleteWorktree {
			dialog.error = "enable worktree removal before running the cleanup hook"
			return model, nil
		}
		dialog.runBeforeDelete = !dialog.runBeforeDelete
		dialog.error = ""
		return model, nil
	case "enter":
		selected, ok := model.selectedTableRow()
		if !ok {
			model.deleteDialog = nil
			return model, nil
		}
		if selected.IsBranch() {
			branch := selected.Branch
			var restore *pendingBranchRestore
			if branch.Head != "" {
				restore = &pendingBranchRestore{branch: branch.Name, sha: branch.Head, short: branch.CommitShort}
			}
			repo := model.state.Repo
			runner := model.runner
			return model.startDelete("deleted branch", restore, func(ctx context.Context) error {
				return deleteBranchRow(ctx, repo, branch, runner)
			})
		}
		row := selected.Worktree
		switch dialog.stage {
		case deleteStageLocked:
			return model, nil
		case deleteStagePrune:
		}
		if row.Prunable {
			dialog.deleteBranch = false
		}
		if !dialog.deleteWorktree && dialog.deleteBranch {
			dialog.error = "enable worktree removal before deleting the branch"
			return model, nil
		}
		if !dialog.deleteWorktree && !dialog.deleteBranch && !row.Prunable {
			dialog.error = "choose at least one delete action"
			return model, nil
		}
		if row.Locked {
			dialog.stage = deleteStageLocked
			dialog.error = "cannot delete locked worktree"
			if row.LockReason != "" {
				dialog.error += ": " + row.LockReason
			}
			return model, nil
		}
		repo := model.state.Repo
		runner := model.runner
		dialogSnapshot := *dialog
		var restore *pendingBranchRestore
		if dialogSnapshot.deleteWorktree && dialogSnapshot.deleteBranch &&
			row.Branch != "" && !row.Detached && row.Head != "" {
			restore = &pendingBranchRestore{branch: row.Branch, sha: row.Head, short: row.CommitShort}
		}
		return model.startDelete("deleted worktree", restore, func(ctx context.Context) error {
			return deleteRow(ctx, repo, row, dialogSnapshot, runner)
		})
	}
	return model, nil
}

func (model Model) startDelete(text string, restore *pendingBranchRestore, action func(context.Context) error) (Model, tea.Cmd) {
	if model.actionCancel != nil {
		model.actionCancel()
	}
	model = model.cancelEnrichment()
	model.enrichmentID++
	model.deleteID++
	model.deleteInFlight = true
	model.deleteSpinnerFrame = 0
	model.refreshInFlight = false
	model.refreshID++
	model.refreshAnchor = selectionAnchor{}
	model.refreshProgressVisible = false
	model = model.clearFeedback()
	if model.deleteDialog != nil {
		model.deleteDialog.error = ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), destructiveActionTimeout)
	model.actionCancel = cancel
	command := deleteAndLoadCmd(ctx, model.reloadCwd(), model.config, model.runner, model.state.Repo.GitVersion, model.deleteID, text, restore, action)
	return model, tea.Batch(command, deleteSpinnerTickCmd(model.deleteID))
}

func deleteAndLoadCmd(ctx context.Context, cwd string, config config.Config, runner gitdata.Runner, gitVersion string, id int, text string, restore *pendingBranchRestore, action func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		actionErr := action(ctx)
		reloadCtx, cancel := context.WithTimeout(context.Background(), destructiveActionTimeout)
		defer cancel()
		state, err := loadStableState(reloadCtx, cwd, config, runner, gitVersion, nil)
		if err != nil {
			return deleteMsg{id: id, err: fmt.Errorf("%s, but reload failed: %w", text, err)}
		}
		repoConfig, hooksApproved, err := loadRepoRuntimeConfig(reloadCtx, state.Repo.Root, runner)
		if err != nil {
			return deleteMsg{id: id, err: fmt.Errorf("%s, but reload failed: %w", text, err)}
		}
		message := deleteMsg{id: id, state: state, repoConfig: repoConfig, hooksApproved: hooksApproved, reloaded: true, text: text, restore: restore, completedAt: time.Now()}
		if actionErr != nil {
			message.err = actionErr
		}
		return message
	}
}

func (model Model) startRestore() (Model, tea.Cmd) {
	if len(model.pendingRestoreBatch) > 0 {
		restores := append([]pendingBranchRestore(nil), model.pendingRestoreBatch...)
		repo := model.state.Repo
		runner := model.runner
		return model.startDelete(restoredBranchesText(len(restores)), nil, func(ctx context.Context) error {
			restored := 0
			var failures []string
			for _, restore := range restores {
				if err := gitdata.CreateBranchAt(ctx, repo.Root, restore.branch, restore.sha, runner); err != nil {
					failures = append(failures, restore.branch+": "+err.Error())
					continue
				}
				restored++
			}
			if len(failures) > 0 {
				return fmt.Errorf("restored %d %s, failed %d: %s", restored, pluralize(restored, "branch", "branches"), len(failures), strings.Join(failures, "; "))
			}
			return nil
		})
	}
	repo := model.state.Repo
	runner := model.runner
	restore := *model.pendingRestore
	return model.startDelete("restored branch "+restore.branch, nil, func(ctx context.Context) error {
		return gitdata.CreateBranchAt(ctx, repo.Root, restore.branch, restore.sha, runner)
	})
}

func restoredBranchesText(count int) string {
	if count == 1 {
		return "restored branch"
	}
	return fmt.Sprintf("restored %d branches", count)
}

func deleteBranchRow(ctx context.Context, repo gitdata.Repository, branch gitdata.Branch, runner gitdata.Runner) error {
	return gitdata.DeleteBranch(ctx, repo.Root, branch.Name, !branch.BranchMergedToMain, runner)
}

func deleteRow(ctx context.Context, repo gitdata.Repository, row gitdata.Worktree, dialog deleteDialog, runner gitdata.Runner) error {
	if row.Prunable {
		if err := gitdata.PruneWorktrees(ctx, repo.Root, runner); err != nil {
			return err
		}
		return nil
	}
	if dialog.deleteWorktree {
		if dialog.runBeforeDelete && dialog.beforeDeleteHook != "" && !row.Prunable {
			if err := gitdata.RunHook(ctx, row.Path, dialog.beforeDeleteHook, hookEnv("before_delete", row.Path, row.Branch, repo.Root, repo.MainBranch), runner); err != nil {
				return err
			}
		}
		if err := gitdata.RemoveWorktree(ctx, repo.Root, row.Path, dialog.forceWorktree, runner); err != nil {
			return err
		}
	}
	if dialog.deleteWorktree && dialog.deleteBranch && row.Branch != "" && !row.Detached {
		if err := gitdata.DeleteBranch(ctx, repo.Root, row.Branch, !row.BranchMergedToMain, runner); err != nil {
			return fmt.Errorf("worktree removed; delete remaining branch %q: %w", row.Branch, err)
		}
	}
	return nil
}

func deleteBranchAvailable(row gitdata.Worktree) bool {
	return !row.Prunable && !row.Detached && row.Branch != ""
}

func deleteBranchDefault(row gitdata.Worktree) bool {
	return deleteWorktreeDefault(row) && deleteBranchDefaultWhenWorktreeRemoved(row)
}

func deleteBranchDefaultWhenWorktreeRemoved(row gitdata.Worktree) bool {
	return row.LocalMetadataLoaded && deleteBranchAvailable(row) && row.BranchMergedToMain
}

func deleteWorktreeDefault(row gitdata.Worktree) bool {
	return row.LocalMetadataLoaded && row.Status.Clean()
}

func deleteBranchAvailableFromModel(model Model) bool {
	row, ok := model.selectedWorktree()
	return ok && deleteBranchAvailable(row)
}

func (model Model) renderDeleteAtWidth(width int) string {
	selected, ok := model.selectedTableRow()
	if !ok {
		return dialogBox("Delete", []string{"No row selected."}, deleteDialogHintsAtWidth("Esc cancel", width-6), width)
	}
	if selected.IsBranch() {
		return model.renderDeleteBranchAtWidth(selected.Branch, width)
	}
	row := selected.Worktree
	dialog := model.deleteDialog
	contentWidth := max(1, width-4)
	lines := model.deleteMetadataLines(row, contentWidth)
	lines = append(lines, "")
	bottom := "Esc cancel"
	switch dialog.stage {
	case deleteStageLocked:
		lines = append(lines,
			deleteDangerStyle.Render("Cannot delete locked worktree."),
			"Unlock this worktree before deleting it.",
		)
		if row.LockReason != "" {
			lines = append(lines, "Reason: "+row.LockReason)
		}
	case deleteStagePrune:
		lines = append(lines,
			deleteSectionTitle("Worktree"),
			"[x] prune missing worktree metadata",
		)
		if row.PruneReason != "" {
			lines = append(lines, "Reason: "+row.PruneReason)
		}
		bottom = "Enter prune · Esc cancel"
	default:
		if !row.Status.Clean() {
			lines = append(lines,
				deleteDangerStyle.Render("Uncommitted changes will be discarded when removing the worktree."),
				deleteDangerStyle.Render(dirtyDetailText(row.Status)),
				"",
			)
		}
		worktreeBlock := deleteToggleBlock{
			title:   "Worktree",
			key:     "t",
			enabled: true,
			checked: dialog.deleteWorktree,
			label:   "remove worktree",
		}
		if dialog.deleteWorktree {
			if dialog.forceWorktree {
				worktreeBlock.commands = append(worktreeBlock.commands, deleteCommand{text: "git worktree remove --force", danger: true})
			} else {
				worktreeBlock.commands = append(worktreeBlock.commands, deleteCommand{text: "git worktree remove"})
			}
		} else {
			worktreeBlock.details = append(worktreeBlock.details, hintStyle.Render("No worktree command will run."))
		}
		lines = append(lines, renderDeleteToggleBlock(worktreeBlock)...)
		if dialog.beforeDeleteHook != "" {
			hookEnabled := dialog.deleteWorktree
			hookBlock := deleteToggleBlock{
				title:   "Cleanup hook",
				key:     "h",
				enabled: hookEnabled,
				checked: dialog.runBeforeDelete && hookEnabled,
				label:   "run before_delete cleanup hook",
				muted:   !hookEnabled,
			}
			if !hookEnabled {
				hookBlock.details = append(hookBlock.details, hintStyle.Render("Enable worktree removal first; the hook runs before removal."))
			} else if dialog.runBeforeDelete {
				hookBlock.details = append(hookBlock.details, "Runs in the worktree before removal.")
				hookBlock.commands = append(hookBlock.commands, deleteCommand{text: "sh -c " + fmt.Sprintf("%q", dialog.beforeDeleteHook)})
			} else {
				hookBlock.details = append(hookBlock.details, hintStyle.Render("No cleanup hook will run."))
			}
			lines = append(lines, "")
			lines = append(lines, renderDeleteToggleBlock(hookBlock)...)
		}
		if deleteBranchAvailable(row) {
			branchEnabled := dialog.deleteWorktree
			branchBlock := deleteToggleBlock{
				title:   "Branch",
				key:     "b",
				enabled: branchEnabled,
				checked: dialog.deleteBranch && branchEnabled,
				muted:   !branchEnabled,
			}
			if row.BranchMergedToMain {
				branchBlock.label = "delete local branch"
				if !branchEnabled {
					branchBlock.details = append(branchBlock.details, hintStyle.Render("Enable worktree removal first; the branch is checked out here."))
				} else if dialog.deleteBranch {
					branchBlock.details = append(branchBlock.details, "Merged into "+model.deleteMainBranchName()+".")
					branchBlock.commands = append(branchBlock.commands, deleteCommand{text: "git branch -d " + row.Branch})
				} else {
					branchBlock.details = append(branchBlock.details, "Merged into "+model.deleteMainBranchName()+", branch will be kept.")
					branchBlock.details = append(branchBlock.details, hintStyle.Render("No branch command will run."))
				}
			} else {
				branchBlock.label = "force delete local branch"
				if !branchEnabled {
					branchBlock.details = append(branchBlock.details, hintStyle.Render("Enable worktree removal first; the branch is checked out here."))
				} else if dialog.deleteBranch {
					branchBlock.details = append(branchBlock.details, deleteDangerStyle.Render("Not merged into "+model.deleteMainBranchName()+"."))
					branchBlock.commands = append(branchBlock.commands, deleteCommand{text: "git branch -D " + row.Branch, danger: true})
				} else {
					branchBlock.details = append(branchBlock.details, "Not merged into "+model.deleteMainBranchName()+", branch will be kept.")
					branchBlock.details = append(branchBlock.details, hintStyle.Render("No branch command will run."))
				}
			}
			if row.UpstreamGone {
				branchBlock.details = append(branchBlock.details, "Remote branch already deleted, likely safe.")
			}
			lines = append(lines, "")
			lines = append(lines, renderDeleteToggleBlock(branchBlock)...)
			bottom = "Enter delete · Esc cancel"
		} else {
			bottom = "Enter delete · Esc cancel"
		}
	}
	if dialog.error != "" && dialog.stage != deleteStageLocked {
		lines = append(lines, "")
		lines = append(lines, deleteErrorLines(dialog.error, contentWidth)...)
	}
	lines = model.clipDialogBody(lines)
	for index, line := range lines {
		lines[index] = truncateStyled(line, contentWidth)
	}
	return dialogBox("Delete worktree", lines, model.deleteDialogBottomContent(bottom, width-6), width)
}

func (model Model) renderDeleteBranchAtWidth(branch gitdata.Branch, width int) string {
	contentWidth := max(1, width-4)
	dialog := model.deleteDialog
	command := deleteBranchCommand(branch)
	lines := []string{
		dialogFieldLine("Branch", branch.DisplayBranch(), contentWidth),
		dialogFieldLine("HEAD", branchHeadText(branch), contentWidth),
		dialogFieldLine("PR", model.rowPRText(gitdata.Row{Kind: gitdata.RowKindBranch, Branch: branch}), contentWidth),
		"",
		deleteSectionTitle("Branch"),
		deleteCheckboxLine(true, deleteBranchLabel(branch), false),
		deleteDetailLine("Local branch ref will be deleted. No worktree files are removed."),
	}
	if branch.BranchMergedToMain {
		lines = append(lines, deleteDetailLine("Merged into "+model.deleteMainBranchName()+"."))
	} else {
		lines = append(lines, deleteDetailLine(deleteDangerStyle.Render("Not merged into "+model.deleteMainBranchName()+".")))
	}
	if branch.UpstreamGone {
		lines = append(lines, deleteDetailLine("Remote branch already deleted, likely safe."))
	}
	lines = append(lines, deleteCommandLine(command.text, command.danger))
	bottom := "Enter delete · Esc cancel"
	if dialog != nil && dialog.error != "" {
		lines = append(lines, "")
		lines = append(lines, deleteErrorLines(dialog.error, contentWidth)...)
	}
	lines = model.clipDialogBody(lines)
	for index, line := range lines {
		lines[index] = truncateStyled(line, contentWidth)
	}
	return dialogBox("Delete branch", lines, model.deleteDialogBottomContent(bottom, width-6), width)
}

func (model Model) deleteProgressText() string {
	frame := refreshSpinnerFrames[model.deleteSpinnerFrame%len(refreshSpinnerFrames)]
	return refreshActivityStyle.Render(frame + " deleting")
}

func (model Model) deleteDialogBottomContent(content string, width int) string {
	if model.deleteInFlight {
		return truncateStyled(model.deleteProgressText(), width)
	}
	return deleteDialogHintsAtWidth(content, width)
}

func (model Model) deleteMetadataLines(row gitdata.Worktree, width int) []string {
	return []string{
		dialogFieldLine("Path", row.Path, width),
		dialogFieldLine("Branch", row.DisplayBranch(), width),
		dialogFieldLine("PR", model.deletePRText(row), width),
	}
}

func (model Model) deleteMainBranchName() string {
	if model.state.Repo.MainBranch != "" {
		return model.state.Repo.MainBranch
	}
	return "main"
}

func deleteBranchLabel(branch gitdata.Branch) string {
	if branch.BranchMergedToMain {
		return "delete local branch"
	}
	return "force delete local branch"
}

func deleteBranchCommand(branch gitdata.Branch) deleteCommand {
	if branch.BranchMergedToMain {
		return deleteCommand{text: "git branch -d " + branch.Name}
	}
	return deleteCommand{text: "git branch -D " + branch.Name, danger: true}
}

func dialogFieldLine(label, value string, width int) string {
	labelWidth := 7
	separatorWidth := 2
	valueWidth := max(1, width-labelWidth-separatorWidth)
	labelText := inspectorLabelStyle.Render(padRight(label+":", labelWidth))
	return labelText + "  " + truncatePlain(value, valueWidth)
}

func deleteSectionTitle(title string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true).Render(title)
}

func deleteSectionHeader(title, key string, enabled bool) string {
	titleText := deleteSectionTitle(title)
	if key == "" {
		return titleText
	}
	description := "toggle"
	if !enabled {
		description += " disabled"
	}
	return titleText + hintStyle.Render(" · ") + keyStyle.Render(key) + hintStyle.Render(" "+description)
}

type deleteToggleBlock struct {
	title    string
	key      string
	enabled  bool
	checked  bool
	label    string
	muted    bool
	details  []string
	commands []deleteCommand
}

type deleteCommand struct {
	text   string
	danger bool
}

func renderDeleteToggleBlock(block deleteToggleBlock) []string {
	lines := []string{
		deleteSectionHeader(block.title, block.key, block.enabled),
		deleteCheckboxLine(block.checked, block.label, block.muted),
	}
	for _, detail := range block.details {
		lines = append(lines, deleteDetailLine(detail))
	}
	for _, command := range block.commands {
		lines = append(lines, deleteCommandLine(command.text, command.danger))
	}
	return lines
}

func deleteCheckboxLine(checked bool, label string, muted bool) string {
	marker := "[ ]"
	if checked {
		marker = "[x]"
	}
	line := marker + " " + label
	if muted {
		return hintStyle.Render(line)
	}
	return line
}

func deleteDetailLine(value string) string {
	return "    " + value
}

func deleteCommandLine(command string, danger bool) string {
	style := deleteCommandStyle
	if danger {
		style = deleteDangerStyle
	}
	return "    " + inspectorLabelStyle.Render("Command:") + " " + style.Render(command)
}

func truncateStyled(value string, width int) string {
	if lipgloss.Width(value) <= width {
		return value
	}
	return ansi.Cut(value, 0, max(0, width-1)) + "…"
}

func deleteDialogHintsAtWidth(content string, width int) string {
	full := colorKeyHints(content, false)
	if lipgloss.Width(full) <= width {
		return full
	}
	short := strings.ReplaceAll(content, " confirm", "")
	short = strings.ReplaceAll(short, " delete", "")
	short = strings.ReplaceAll(short, " prune", "")
	short = colorKeyHints(short, false)
	if lipgloss.Width(short) <= width {
		return short
	}
	return ""
}

func checkoutDialogHintsAtWidth(stash bool, width int) string {
	content := "s stash changes · Esc cancel"
	if stash {
		content = "Enter stash + checkout · s toggle · Esc cancel"
	}
	full := colorKeyHints(content, false)
	if lipgloss.Width(full) <= width {
		return full
	}
	short := "s stash · Esc"
	if stash {
		short = "Enter · s · Esc"
	}
	short = colorKeyHints(short, false)
	if lipgloss.Width(short) <= width {
		return short
	}
	return ""
}

func deleteErrorLines(message string, width int) []string {
	message = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(message), "×"))
	message = dropGitHintLines(message)
	lines := wrapPlainWithPrefixes(message, "× ", "  ", width)
	truncated := false
	if len(lines) > maxDeleteErrorLines {
		lines = lines[:maxDeleteErrorLines]
		truncated = true
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	styled := make([]string, len(lines))
	for index, line := range lines {
		styled[index] = style.Render(line)
	}
	if truncated {
		styled[len(styled)-1] = style.Render("  … (run the command for full output)")
	}
	return styled
}

func dropGitHintLines(message string) string {
	kept := make([]string, 0)
	for _, line := range strings.Split(message, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "hint:") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func wrapPlainWithPrefixes(message, firstPrefix, nextPrefix string, width int) []string {
	if message == "" {
		return nil
	}
	lines := []string{}
	prefix := firstPrefix
	for _, paragraph := range strings.Split(message, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, truncatePlain(prefix, width))
			prefix = nextPrefix
			continue
		}
		current := ""
		for _, word := range words {
			available := max(1, width-runewidth.StringWidth(prefix))
			if current == "" {
				current = truncatePlain(word, available)
				if current != word {
					lines = append(lines, truncatePlain(prefix+current, width))
					current = ""
					prefix = nextPrefix
				}
				continue
			}
			candidate := current + " " + word
			if runewidth.StringWidth(candidate) <= available {
				current = candidate
				continue
			}
			lines = append(lines, truncatePlain(prefix+current, width))
			prefix = nextPrefix
			available = max(1, width-runewidth.StringWidth(prefix))
			current = truncatePlain(word, available)
			if current != word {
				lines = append(lines, truncatePlain(prefix+current, width))
				current = ""
			}
		}
		if current != "" {
			lines = append(lines, truncatePlain(prefix+current, width))
			prefix = nextPrefix
		}
	}
	return lines
}
