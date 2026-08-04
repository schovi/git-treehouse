package tui

import (
	"context"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/pathutil"
	"os"
	"strings"
	"time"
)

type createDialog struct {
	input     textinput.Model
	bases     []gitdata.BaseOption
	baseIndex int
	error     string
}

func createdHookError(hook, path string, err error) string {
	return "worktree created at " + path + ", but " + hook + " failed:\n" + err.Error()
}

func (model Model) openCreate() (Model, tea.Cmd) {
	row, ok := model.selectedTableRow()
	if !ok || row.IsWorktree() && row.Worktree.Prunable {
		return model.setFlash("cannot create from this row")
	}
	if row.IsBranch() {
		return model.setFlash("press Enter to create a worktree for this branch")
	}
	baseRow := row.Worktree
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 200
	input.Width = 34
	input.Cursor.Style = flashStyle
	focusCmd := input.Focus()
	bases := gitdata.BaseOptions(context.Background(), model.state.Repo, baseRow, model.runner)
	if len(bases) == 0 {
		return model.setFlash("no base ref available")
	}
	model.help = false
	model.paletteDialog = nil
	model.checkoutDialog = nil
	model.branchWorktreeDialog = nil
	model.createDialog = &createDialog{input: input, bases: bases}
	return model, focusCmd
}

func (model Model) updateCreate(message tea.KeyMsg) (Model, tea.Cmd) {
	if model.createInFlight && message.String() == "enter" {
		return model, nil
	}
	dialog := model.createDialog
	switch message.String() {
	case "esc":
		model.createDialog = nil
		return model, nil
	case "tab", "down":
		dialog.baseIndex = (dialog.baseIndex + 1) % len(dialog.bases)
		return model, nil
	case "shift+tab", "up":
		dialog.baseIndex = (dialog.baseIndex + len(dialog.bases) - 1) % len(dialog.bases)
		return model, nil
	case "ctrl+o":
		return model, openConfigCmd(model.config.Editor, model.config)
	case "enter":
		model.validateCreate()
		if dialog.error != "" {
			return model, nil
		}
		branch := strings.TrimSpace(dialog.input.Value())
		path := model.createPath()
		if collisionError := createPathCollisionError(path); collisionError != "" {
			dialog.error = collisionError
			return model, nil
		}
		base := dialog.bases[dialog.baseIndex].Rev
		repoRoot := model.state.Repo.Root
		mainBranch := model.state.Repo.MainBranch
		repoConfig := model.repoConfig
		hooksApproved := model.hooksApproved
		runner := model.runner
		model.loading = "creating…"
		model.createInFlight = true
		return model, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := gitdata.CreateWorktree(ctx, repoRoot, branch, path, base, runner); err != nil {
				return createMsg{path: path, err: err}
			}
			warnings, err := runPostCreateSteps(ctx, repoRoot, path, branch, mainBranch, repoConfig, hooksApproved, runner)
			return createMsg{path: path, created: true, err: err, warnings: warnings}
		}
	default:
		var cmd tea.Cmd
		dialog.input, cmd = dialog.input.Update(message)
		dialog.error = ""
		return model, cmd
	}
}

func (model Model) validateCreate() {
	if model.createDialog == nil {
		return
	}
	branch := strings.TrimSpace(model.createDialog.input.Value())
	err := gitdata.ValidateBranchName(context.Background(), model.state.Repo.Root, branch, model.runner)
	if err != nil {
		model.createDialog.error = err.Error()
		return
	}
	model.createDialog.error = ""
}

func (model Model) renderCreateAtWidth(width int) string {
	dialog := model.createDialog
	contentWidth := max(1, width-4)
	input := dialog.input
	branchLabel := "Branch name: "
	input.Width = max(1, contentWidth-runewidth.StringWidth(branchLabel)-1)
	branchLine := branchLabel + input.View()
	if lipgloss.Width(branchLine) > contentWidth {
		branchLine = truncatePlain(strings.TrimSpace(branchLabel), contentWidth)
	}
	path := model.createPath()
	lines := []string{
		branchLine,
		truncatePlain("Path: "+model.createPathPreview(), contentWidth),
		"Base:",
	}
	for index, base := range dialog.bases {
		marker := "○"
		if index == dialog.baseIndex {
			marker = "●"
		}
		lines = append(lines, truncatePlain("  "+marker+" "+base.Label, contentWidth))
	}
	if collisionError := createPathCollisionError(path); collisionError != "" && dialog.error != collisionError {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(truncatePlain(collisionError, contentWidth)))
	}
	if dialog.error != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(truncatePlain(dialog.error, contentWidth)))
	}
	return dialogBox("New worktree", lines, createDialogHintsAtWidth(width-6), width)
}

func (model Model) createPathPreview() string {
	path := model.createPath()
	if path == "" {
		return "enter branch name"
	}
	return path
}

func (model Model) createPath() string {
	if model.createDialog == nil {
		return ""
	}
	branch := strings.TrimSpace(model.createDialog.input.Value())
	if branch == "" {
		return ""
	}
	return pathutil.ApplyTemplate(model.effectivePathTemplate(), model.state.Repo.Root, branch)
}

func createPathCollisionError(path string) string {
	if path == "" {
		return ""
	}
	if _, err := os.Stat(path); err == nil {
		return "target path already exists: " + path
	}
	return ""
}
