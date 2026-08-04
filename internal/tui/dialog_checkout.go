package tui

import (
	"context"
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/github"
	"github.com/schovi/git-treehouse/internal/pathutil"
	"os"
	"strconv"
	"strings"
	"time"
)

type checkoutDialog struct {
	branch gitdata.Branch
	root   gitdata.Worktree
	stash  bool
	error  string
}

type branchWorktreeDialog struct {
	branch      gitdata.Branch
	path        string
	destination worktreeDestination
	error       string
}

type pullRequestCheckoutDialog struct {
	input        textinput.Model
	selected     int
	loading      bool
	error        string
	summaries    []github.PullRequestSummary
	id           int
	directLookup bool
	spinnerFrame int
}

func (model Model) openPullRequestCheckout() (Model, tea.Cmd) {
	if !model.githubEnabled() {
		return model.setFlash("GitHub is disabled")
	}
	input := textinput.New()
	input.Prompt = "> "
	input.CharLimit = 200
	input.Width = 52
	input.Cursor.Style = flashStyle
	focusCmd := input.Focus()
	model.pullRequestDialogID++
	id := model.pullRequestDialogID
	model.help = false
	model.paletteDialog = nil
	model.filterDialog = nil
	model.createDialog = nil
	model.checkoutDialog = nil
	model.branchWorktreeDialog = nil
	model.deleteDialog = nil
	model.pullRequestDialog = &pullRequestCheckoutDialog{input: input, loading: true, id: id}
	repoRoot := model.state.Repo.Root
	runner := model.runner
	loadCmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		summaries, err := github.LoadPullRequestSummaries(ctx, repoRoot, runner)
		return pullRequestSummariesLoadedMsg{summaries: summaries, err: err, id: id}
	}
	return model, tea.Batch(focusCmd, loadCmd, pullRequestSpinnerTickCmd(id))
}

func (model Model) updatePullRequestCheckout(message tea.KeyMsg) (Model, tea.Cmd) {
	dialog := model.pullRequestDialog
	switch message.String() {
	case "esc":
		model.pullRequestDialog = nil
		model.loading = ""
		return model, nil
	case "up", "k":
		model.movePullRequestSelection(-1)
		return model, nil
	case "down", "j":
		model.movePullRequestSelection(1)
		return model, nil
	case "o":
		if dialog.directLookup {
			return model, nil
		}
		matches := model.matchingPullRequestSummaries()
		if len(matches) > 0 && !dialog.loading {
			selected := clamp(dialog.selected, 0, len(matches)-1)
			return model.startPullRequestOpen(strconv.Itoa(matches[selected].Number))
		}
		query := strings.TrimSpace(dialog.input.Value())
		if query == "" {
			if !dialog.loading {
				dialog.error = "No matching PR"
			}
			return model, nil
		}
		return model.startPullRequestOpen(query)
	case "enter":
		if dialog.loading || dialog.directLookup {
			return model, nil
		}
		matches := model.matchingPullRequestSummaries()
		if len(matches) > 0 {
			selected := clamp(dialog.selected, 0, len(matches)-1)
			return model.startPullRequestCheckout(matches[selected])
		}
		query := strings.TrimSpace(dialog.input.Value())
		if query == "" {
			dialog.error = "No matching PR"
			return model, nil
		}
		return model.startPullRequestLookup(query)
	}
	previousValue := dialog.input.Value()
	var cmd tea.Cmd
	dialog.input, cmd = dialog.input.Update(message)
	if dialog.input.Value() != previousValue {
		dialog.selected = 0
		dialog.error = ""
	}
	dialog.selected = clamp(dialog.selected, 0, max(0, len(model.matchingPullRequestSummaries())-1))
	return model, cmd
}

func (model *Model) movePullRequestSelection(direction int) {
	if model.pullRequestDialog == nil || direction == 0 {
		return
	}
	matches := model.matchingPullRequestSummaries()
	if len(matches) == 0 {
		model.pullRequestDialog.selected = 0
		return
	}
	model.pullRequestDialog.selected = clamp(model.pullRequestDialog.selected+direction, 0, len(matches)-1)
}

func (model Model) matchingPullRequestSummaries() []github.PullRequestSummary {
	if model.pullRequestDialog == nil {
		return nil
	}
	query := strings.TrimSpace(model.pullRequestDialog.input.Value())
	if query == "" {
		return model.pullRequestDialog.summaries
	}
	matches := make([]github.PullRequestSummary, 0, len(model.pullRequestDialog.summaries))
	for _, summary := range model.pullRequestDialog.summaries {
		if pullRequestSummaryMatches(summary, query) {
			matches = append(matches, summary)
		}
	}
	return matches
}

func pullRequestSummaryMatches(summary github.PullRequestSummary, query string) bool {
	haystack := strings.Join([]string{
		fmt.Sprintf("#%d", summary.Number),
		strconv.Itoa(summary.Number),
		summary.Title,
		summary.URL,
		summary.HeadRepositoryOwner,
		summary.HeadRefName,
		summary.BranchName(),
	}, " ")
	return fuzzyMatch(haystack, query)
}

func (model Model) startPullRequestOpen(query string) (Model, tea.Cmd) {
	if model.pullRequestDialog == nil {
		return model, nil
	}
	if !model.githubEnabled() {
		return model.setFlash("GitHub is disabled")
	}
	model.pullRequestDialog.error = ""
	model.loading = "opening…"
	id := model.pullRequestDialog.id
	repoRoot := model.state.Repo.Root
	runner := model.runner
	return model, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := github.OpenPullRequest(ctx, repoRoot, query, runner)
		return pullRequestOpenedMsg{err: err, id: id}
	}
}

func (model Model) startPullRequestLookup(query string) (Model, tea.Cmd) {
	if model.pullRequestDialog == nil {
		return model, nil
	}
	if !model.githubEnabled() {
		return model.setFlash("GitHub is disabled")
	}
	model.pullRequestDialog.error = ""
	model.pullRequestDialog.directLookup = true
	model.pullRequestDialog.spinnerFrame = 0
	model.loading = "checking out…"
	id := model.pullRequestDialog.id
	repoRoot := model.state.Repo.Root
	runner := model.runner
	lookupCmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		summary, err := github.LoadPullRequestSummary(ctx, repoRoot, query, runner)
		return pullRequestSummaryLoadedMsg{summary: summary, err: err, id: id}
	}
	return model, tea.Batch(lookupCmd, pullRequestSpinnerTickCmd(id))
}

func (model Model) startPullRequestCheckout(summary github.PullRequestSummary) (Model, tea.Cmd) {
	if model.pullRequestDialog == nil {
		return model, nil
	}
	branch := summary.BranchName()
	if branch == "" {
		model.pullRequestDialog.error = "pull request branch is missing"
		return model, nil
	}
	if worktree, ok := model.worktreeForBranch(branch); ok {
		if worktree.Prunable {
			model.pullRequestDialog.error = "cannot enter a prunable worktree"
			return model, nil
		}
		if worktree.IsActive {
			return model, tea.Quit
		}
		model.selectedPath = worktree.Path
		return model, tea.Quit
	}
	path := pathutil.ApplyTemplate(model.effectivePathTemplate(), model.state.Repo.Root, branch)
	if _, err := os.Stat(path); err == nil {
		model.pullRequestDialog.error = "target path already exists: " + path
		return model, nil
	}
	destination := detectedWorktreeDestination()
	repoRoot := model.state.Repo.Root
	mainBranch := model.state.Repo.MainBranch
	repoConfig := model.repoConfig
	hooksApproved := model.hooksApproved
	runner := model.runner
	model.loading = "creating…"
	if model.branchExists(branch) {
		return model, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := gitdata.CheckoutBranchWorktree(ctx, repoRoot, branch, path, runner); err != nil {
				return checkoutMsg{path: path, branch: branch, destination: destination, createsWorktree: true, err: err}
			}
			warnings, err := runPostCreateSteps(ctx, repoRoot, path, branch, mainBranch, repoConfig, hooksApproved, runner)
			return checkoutMsg{path: path, branch: branch, destination: destination, createsWorktree: true, created: true, err: err, warnings: warnings}
		}
	}
	return model, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := gitdata.CheckoutPullRequestWorktree(ctx, repoRoot, summary.Number, branch, path, runner); err != nil {
			return checkoutMsg{path: path, branch: branch, destination: destination, createsWorktree: true, err: err}
		}
		warnings, err := runPostCreateSteps(ctx, repoRoot, path, branch, mainBranch, repoConfig, hooksApproved, runner)
		return checkoutMsg{path: path, branch: branch, destination: destination, createsWorktree: true, created: true, err: err, warnings: warnings}
	}
}

func (model Model) worktreeForBranch(branch string) (gitdata.Worktree, bool) {
	for _, row := range model.state.Rows {
		if !row.Detached && row.Branch == branch {
			return row, true
		}
	}
	return gitdata.Worktree{}, false
}

func (model Model) branchExists(branch string) bool {
	for _, row := range model.state.Branches {
		if row.Name == branch {
			return true
		}
	}
	return false
}

func (model Model) openBranchWorktree(branch gitdata.Branch) (Model, tea.Cmd) {
	if branch.Name == "" {
		return model.setFlash("cannot create worktree for this branch")
	}
	path := pathutil.ApplyTemplate(model.effectivePathTemplate(), model.state.Repo.Root, branch.Name)
	model.help = false
	model.paletteDialog = nil
	model.createDialog = nil
	model.checkoutDialog = nil
	model.deleteDialog = nil
	model.branchWorktreeDialog = &branchWorktreeDialog{branch: branch, path: path, destination: detectedWorktreeDestination()}
	return model, nil
}

func (model Model) updateBranchWorktree(message tea.KeyMsg) (Model, tea.Cmd) {
	dialog := model.branchWorktreeDialog
	switch message.String() {
	case "esc":
		model.branchWorktreeDialog = nil
		return model, nil
	case "enter":
		if _, err := os.Stat(dialog.path); err == nil {
			dialog.error = "target path already exists: " + dialog.path
			return model, nil
		}
		branch := dialog.branch.Name
		path := dialog.path
		destination := dialog.destination
		repoRoot := model.state.Repo.Root
		mainBranch := model.state.Repo.MainBranch
		repoConfig := model.repoConfig
		hooksApproved := model.hooksApproved
		runner := model.runner
		model.loading = "creating…"
		return model, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := gitdata.CheckoutBranchWorktree(ctx, repoRoot, branch, path, runner); err != nil {
				return checkoutMsg{path: path, branch: branch, destination: destination, createsWorktree: true, err: err}
			}
			warnings, err := runPostCreateSteps(ctx, repoRoot, path, branch, mainBranch, repoConfig, hooksApproved, runner)
			return checkoutMsg{path: path, branch: branch, destination: destination, createsWorktree: true, created: true, err: err, warnings: warnings}
		}
	}
	return model, nil
}

func (model Model) openCheckoutRoot(branch gitdata.Branch) (Model, tea.Cmd) {
	if branch.Name == "" {
		return model.setFlash("cannot checkout this branch")
	}
	root, ok := model.rootWorktree()
	if !ok || root.Path == "" {
		return model.setFlash("cannot find root worktree")
	}
	if !root.LocalMetadataLoaded {
		return model.setFlash("root status is still loading")
	}
	if root.Status.Clean() {
		model.loading = "checking out…"
		return model, model.checkoutRootBranchCmd(branch, root, false)
	}
	model.help = false
	model.paletteDialog = nil
	model.createDialog = nil
	model.branchWorktreeDialog = nil
	model.deleteDialog = nil
	model.checkoutDialog = &checkoutDialog{branch: branch, root: root}
	return model, nil
}

func (model Model) updateCheckout(message tea.KeyMsg) (Model, tea.Cmd) {
	dialog := model.checkoutDialog
	switch message.String() {
	case "esc":
		model.checkoutDialog = nil
		return model, nil
	case "s":
		dialog.stash = !dialog.stash
		dialog.error = ""
		return model, nil
	case "enter":
		if !dialog.stash {
			dialog.error = "enable stash before checking out"
			return model, nil
		}
		model.loading = "checking out…"
		return model, model.checkoutRootBranchCmd(dialog.branch, dialog.root, true)
	}
	return model, nil
}

func (model Model) checkoutRootBranchCmd(branch gitdata.Branch, root gitdata.Worktree, stash bool) tea.Cmd {
	return func() tea.Msg {
		if stash {
			if err := gitdata.StashWorktreeChanges(context.Background(), root.Path, checkoutStashMessage(branch.Name), model.runner); err != nil {
				return checkoutMsg{err: err}
			}
		}
		err := gitdata.SwitchBranch(context.Background(), root.Path, branch.Name, model.runner)
		return checkoutMsg{path: root.Path, err: err}
	}
}

func checkoutStashMessage(branch string) string {
	return "git-treehouse: before switching to " + branch
}

func checkoutStashCommand(branch string) string {
	return "git stash push -u -m " + fmt.Sprintf("%q", checkoutStashMessage(branch))
}

func checkoutSwitchCommand(branch string) string {
	return "git switch -- " + branch
}

func hookEnv(event, worktreePath, branch, repoRoot, mainBranch string) []string {
	return []string{
		"GTH_EVENT=" + event,
		"GTH_WORKTREE_PATH=" + worktreePath,
		"GTH_WORKTREE_BRANCH=" + branch,
		"GTH_REPO_ROOT=" + repoRoot,
		"GTH_MAIN_BRANCH=" + mainBranch,
	}
}

func runPostCreateSteps(ctx context.Context, repoRoot, path, branch, mainBranch string, repoConfig config.RepoConfig, hooksApproved bool, runner gitdata.Runner) ([]string, error) {
	var warnings []string
	for _, err := range gitdata.CopyWorktreeFiles(repoRoot, path, repoConfig.CopyUntracked) {
		warnings = append(warnings, err.Error())
	}
	if repoConfig.PostCreate == "" {
		return warnings, nil
	}
	if !hooksApproved {
		warnings = append(warnings, "post_create hook not approved; run git-treehouse allow")
		return warnings, nil
	}
	err := gitdata.RunHook(ctx, path, repoConfig.PostCreate, hookEnv("post_create", path, branch, repoRoot, mainBranch), runner)
	return warnings, err
}

func (model Model) renderCheckoutAtWidth(width int) string {
	dialog := model.checkoutDialog
	contentWidth := max(1, width-4)
	lines := []string{
		dialogFieldLine("Branch", dialog.branch.DisplayBranch(), contentWidth),
		dialogFieldLine("Root", dialog.root.Path, contentWidth),
		dialogFieldLine("Current", dialog.root.DisplayBranch(), contentWidth),
		"",
		deleteDangerStyle.Render("Root has uncommitted changes."),
		deleteDangerStyle.Render(dirtyDetailText(dialog.root.Status)),
		"",
	}
	stashBlock := deleteToggleBlock{
		title:   "Root changes",
		key:     "s",
		enabled: true,
		checked: dialog.stash,
		label:   "stash current changes",
	}
	if dialog.stash {
		stashBlock.details = append(stashBlock.details, "Stash includes untracked files.")
		stashBlock.commands = append(stashBlock.commands,
			deleteCommand{text: checkoutStashCommand(dialog.branch.Name)},
			deleteCommand{text: checkoutSwitchCommand(dialog.branch.Name)},
		)
	} else {
		stashBlock.details = append(stashBlock.details, "Checkout is blocked until root changes are stashed.")
		stashBlock.details = append(stashBlock.details, hintStyle.Render("No checkout command will run."))
	}
	lines = append(lines, renderDeleteToggleBlock(stashBlock)...)
	if dialog.error != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(dialog.error))
	}
	for index, line := range lines {
		lines[index] = truncateStyled(line, contentWidth)
	}
	return dialogBox("Checkout root", lines, checkoutDialogHintsAtWidth(dialog.stash, width-6), width)
}

func (model Model) renderBranchWorktreeAtWidth(width int) string {
	dialog := model.branchWorktreeDialog
	contentWidth := max(1, width-4)
	lines := []string{
		truncatePlain("Branch: "+dialog.branch.DisplayBranch(), contentWidth),
		truncatePlain("Path: "+dialog.path, contentWidth),
	}
	if dialog.error != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(truncatePlain(dialog.error, contentWidth)))
	}
	return dialogBox("New worktree", lines, colorKeyHints(dialog.destination.createHint()+" · Esc cancel", false), width)
}

func (model Model) renderPullRequestCheckoutAtWidth(width int) string {
	dialog := model.pullRequestDialog
	contentWidth := max(1, width-4)
	input := dialog.input
	input.Width = max(1, contentWidth-runewidth.StringWidth(input.Prompt)-1)
	lines := []string{input.View(), ""}
	spinner := refreshSpinnerFrames[dialog.spinnerFrame%len(refreshSpinnerFrames)]
	switch {
	case dialog.loading:
		lines = append(lines, "  "+spinner+" loading pull requests")
	case dialog.directLookup:
		lines = append(lines, "  "+spinner+" looking up pull request")
	default:
		matches := model.matchingPullRequestSummaries()
		if len(matches) == 0 {
			lines = append(lines, hintStyle.Render("  No matching PR"))
		} else {
			selected := clamp(dialog.selected, 0, len(matches)-1)
			start, end := pullRequestWindow(selected, len(matches), 8)
			for index := start; index < end; index++ {
				prefix := "  "
				if index == selected {
					prefix = "› "
				}
				line := pullRequestOptionLine(prefix, matches[index], contentWidth)
				if index == selected {
					line = paletteSelectedStyle.Render(padStyled(line, contentWidth))
				}
				lines = append(lines, line)
			}
		}
	}
	if dialog.error != "" {
		for _, line := range pullRequestErrorLines(dialog.error, contentWidth) {
			lines = append(lines, deleteDangerStyle.Render(line))
		}
	}
	return dialogBox("Checkout PR", lines, pullRequestCheckoutHintsAtWidth(width-6, dialog.loading || dialog.directLookup), width)
}

func pullRequestErrorLines(message string, width int) []string {
	return wrapPlainWithPrefixes(strings.TrimSpace(message), "  × ", "    ", width)
}

// clipDialogBody caps the dialog body so the bordered box (body plus the two
// border lines) never grows taller than the terminal. The error block is the
// last and most variable section, so trimming the tail drops error overflow
// first; a marker line signals the cut.
func (model Model) clipDialogBody(lines []string) []string {
	if model.height <= 0 {
		return lines
	}
	maxBody := model.height - 2
	if maxBody < 1 || len(lines) <= maxBody {
		return lines
	}
	clipped := make([]string, maxBody)
	copy(clipped, lines[:maxBody])
	clipped[maxBody-1] = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("  … (resize for full message)")
	return clipped
}

// maxDeleteErrorLines keeps a verbose git failure from dominating the dialog.
// Git prints the actionable warning/error first and trailing advisory hints
// after, so the first lines carry the signal.
const maxDeleteErrorLines = 6

func pullRequestWindow(selected, count, limit int) (int, int) {
	if count <= limit {
		return 0, count
	}
	start := selected - limit/2
	if start < 0 {
		start = 0
	}
	if start+limit > count {
		start = count - limit
	}
	return start, start + limit
}

func pullRequestOptionLine(prefix string, summary github.PullRequestSummary, width int) string {
	number := fmt.Sprintf("#%d", summary.Number)
	state := summary.StateGlyph()
	title := summary.Title
	if title == "" {
		title = "(untitled)"
	}
	branch := summary.BranchName()
	fixed := prefix + padRight(number, 5) + "  " + state + "  "
	fixedWidth := runewidth.StringWidth(fixed)
	branchWidth := min(24, runewidth.StringWidth(branch))
	titleWidth := width - fixedWidth - branchWidth - 2
	if branch == "" || titleWidth < 8 {
		return truncatePlain(fixed+title, width)
	}
	titleText := padRight(truncatePlain(title, titleWidth), titleWidth)
	branchText := truncatePlain(branch, branchWidth)
	return truncatePlain(fixed+titleText+"  "+branchText, width)
}
