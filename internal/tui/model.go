package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/github"
	"github.com/schovi/git-treehouse/internal/listview"
	"github.com/schovi/git-treehouse/internal/pathutil"
)

type Model struct {
	state           gitdata.State
	config          config.Config
	runner          gitdata.Runner
	width           int
	height          int
	selected        int
	filter          worktreeFilter
	searching       bool
	search          textinput.Model
	help            bool
	loading         string
	flash           string
	flashID         int
	showPR          bool
	selectedPath    string
	createDialog    *createDialog
	deleteDialog    *deleteDialog
	lastRefreshAt   time.Time
	refreshInFlight bool
	refreshID       int
}

var (
	separatorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	appBorderStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("65"))
	panelBorderStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	panelTitleStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true)
	titleNameStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true)
	titleRepoStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	titleMetaStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	flashStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("58"))
	inspectorLabelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("67"))
	inspectorValueStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	inspectorCleanStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	inspectorWarnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	inspectorCommitStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	inspectorSubjectStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	keyStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	hintStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	statusMessageStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

const (
	autoRefreshInterval = 30 * time.Second
	clockTickInterval   = time.Second
	appTitle            = "treehouse"
)

type createDialog struct {
	input     textinput.Model
	bases     []gitdata.BaseOption
	baseIndex int
	error     string
}

type deleteDialog struct {
	deleteBranch bool
	force        bool
	error        string
}

type prLoadedMsg struct {
	pullRequests map[string]gitdata.PullRequest
	enabled      bool
}

type sizeLoadedMsg struct {
	path string
	size int64
}

type reloadMsg struct {
	state       gitdata.State
	err         error
	id          int
	automatic   bool
	completedAt time.Time
}

type createMsg struct {
	path string
	err  error
}

type deleteMsg struct {
	state gitdata.State
	err   error
}

type actionMsg struct {
	text string
	err  error
}

type configOpenedMsg struct {
	path    string
	modTime time.Time
	err     error
}

type configReloadedMsg struct {
	config  config.Config
	path    string
	modTime time.Time
	err     error
}

type noOpMsg struct{}

type clearFlashMsg struct {
	id int
}

type autoRefreshMsg struct{}

type clockTickMsg struct{}

type selectionAnchor struct {
	path   string
	branch string
	head   string
}

type worktreeFilter int

const (
	filterAll worktreeFilter = iota
	filterModified
	filterPrunable
	filterLocked
	filterDetached
)

var orderedFilters = []worktreeFilter{
	filterAll,
	filterModified,
	filterPrunable,
	filterLocked,
	filterDetached,
}

func (filter worktreeFilter) label() string {
	switch filter {
	case filterModified:
		return "modified"
	case filterPrunable:
		return "prunable"
	case filterLocked:
		return "locked"
	case filterDetached:
		return "detached"
	default:
		return "all"
	}
}

func (filter worktreeFilter) matches(row gitdata.Worktree) bool {
	switch filter {
	case filterModified:
		return !row.Status.Clean()
	case filterPrunable:
		return row.Prunable
	case filterLocked:
		return row.Locked
	case filterDetached:
		return row.Detached
	default:
		return true
	}
}

func New(state gitdata.State, config config.Config, runner gitdata.Runner) Model {
	search := textinput.New()
	search.Prompt = "s "
	search.CharLimit = 200
	search.Width = 40
	return Model{
		state:         state,
		config:        config,
		runner:        runner,
		width:         100,
		height:        30,
		search:        search,
		lastRefreshAt: time.Now(),
	}
}

func (model Model) Init() tea.Cmd {
	return tea.Batch(model.enrichmentCommands(), clockTickCmd(), autoRefreshTickCmd())
}

func (model Model) SelectedPath() string {
	return model.selectedPath
}

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = message.Width
		model.height = message.Height
		return model, nil
	case prLoadedMsg:
		if message.enabled {
			model.showPR = true
			model.state.Rows = github.AttachPullRequests(model.state.Rows, message.pullRequests)
		}
		return model, nil
	case sizeLoadedMsg:
		for index := range model.state.Rows {
			if model.state.Rows[index].Path == message.path {
				model.state.Rows[index].SizeBytes = message.size
				model.state.Rows[index].SizeLoaded = true
				break
			}
		}
		return model, nil
	case reloadMsg:
		if message.id != model.refreshID {
			return model, nil
		}
		if message.automatic && !model.canApplyAutoRefresh() {
			model.refreshInFlight = false
			return model, nil
		}
		anchor := model.selectionAnchor()
		model.loading = ""
		model.refreshInFlight = false
		if message.err != nil {
			if message.automatic {
				return model, nil
			}
			return model.setFlash(message.err.Error())
		}
		model.state = message.state
		model.lastRefreshAt = message.completedAt
		model.restoreSelection(anchor)
		if message.automatic {
			return model, model.enrichmentCommands()
		}
		model, flashCmd := model.setFlash("reloaded")
		return model, tea.Batch(model.enrichmentCommands(), flashCmd)
	case createMsg:
		model.loading = ""
		if message.err != nil {
			if model.createDialog != nil {
				model.createDialog.error = message.err.Error()
			}
			return model, nil
		}
		model.selectedPath = message.path
		return model, tea.Quit
	case deleteMsg:
		anchor := model.selectionAnchor()
		model.loading = ""
		model.deleteDialog = nil
		if message.err != nil {
			return model.setFlash(message.err.Error())
		}
		model.state = message.state
		model.restoreSelection(anchor)
		model, flashCmd := model.setFlash("deleted worktree")
		return model, tea.Batch(model.enrichmentCommands(), flashCmd)
	case actionMsg:
		if message.err != nil {
			return model.setFlash(message.err.Error())
		} else {
			return model.setFlash(message.text)
		}
	case configOpenedMsg:
		if message.err != nil {
			return model.setFlash(message.err.Error())
		}
		model, flashCmd := model.setFlash("opened Git Treehouse config")
		return model, tea.Batch(flashCmd, watchConfigChangeCmd(message.path, message.modTime))
	case configReloadedMsg:
		if message.err != nil {
			return model.setFlash(message.err.Error())
		}
		model.config = message.config
		model, flashCmd := model.setFlash("config reloaded")
		if model.createDialog != nil && message.path != "" {
			return model, tea.Batch(flashCmd, watchConfigChangeCmd(message.path, message.modTime))
		}
		return model, flashCmd
	case noOpMsg:
		return model, nil
	case clearFlashMsg:
		if message.id == model.flashID {
			model.flash = ""
		}
		return model, nil
	case autoRefreshMsg:
		return model.updateAutoRefresh()
	case clockTickMsg:
		return model, clockTickCmd()
	case tea.KeyMsg:
		if model.createDialog != nil {
			return model.updateCreate(message)
		}
		if model.deleteDialog != nil {
			return model.updateDelete(message)
		}
		if model.searching {
			return model.updateSearch(message)
		}
		return model.updateList(message)
	default:
		return model, nil
	}
}

func (model Model) updateList(message tea.KeyMsg) (Model, tea.Cmd) {
	switch message.String() {
	case "ctrl+c", "q":
		return model, tea.Quit
	case "esc":
		if model.help {
			model.help = false
			return model, nil
		}
		if model.filter != filterAll {
			anchor := model.selectionAnchor()
			model.filter = filterAll
			if !model.restoreSelection(anchor) && len(model.visibleIndexes()) > 0 {
				model.selected = 0
			}
			return model, nil
		}
		if model.search.Value() != "" {
			model.search.SetValue("")
			model.selected = 0
			return model, nil
		}
		return model, tea.Quit
	case "up", "k":
		model.selected = clamp(model.selected-1, 0, max(0, len(model.visibleIndexes())-1))
	case "down", "j":
		model.selected = clamp(model.selected+1, 0, max(0, len(model.visibleIndexes())-1))
	case "g":
		model.selected = 0
	case "G":
		model.selected = max(0, len(model.visibleIndexes())-1)
	case "h":
		model.selectMatching(func(row gitdata.Worktree) bool { return row.IsMain })
	case "a":
		model.selectMatching(func(row gitdata.Worktree) bool { return row.IsActive })
	case "tab":
		model.cycleFilter()
	case "enter":
		row, ok := model.selectedRow()
		if !ok {
			return model, nil
		}
		if row.Prunable {
			return model.setFlash("cannot enter a prunable worktree")
		}
		if row.IsActive {
			return model, tea.Quit
		}
		model.selectedPath = row.Path
		return model, tea.Quit
	case "n":
		return model.openCreate()
	case "delete", "backspace", "d":
		return model.openDelete()
	case "o":
		row, ok := model.selectedRow()
		if !ok || row.Prunable {
			return model.setFlash("cannot open this worktree")
		}
		return model, openEditorCmd(model.config.Editor, row.Path)
	case "p":
		row, ok := model.selectedRow()
		if !ok {
			return model, nil
		}
		return model, func() tea.Msg {
			err := github.OpenPullRequestOrBranch(context.Background(), model.state.Repo.Root, row, model.runner)
			return actionMsg{text: "opened", err: err}
		}
	case "y":
		row, ok := model.selectedRow()
		if !ok {
			return model, nil
		}
		return model, copyPathCmd(row.Path)
	case "r", "f":
		model.loading = "fetching…"
		model.refreshID++
		model.refreshInFlight = true
		return model, reloadCmd(model.reloadCwd(), model.config, model.runner, true, false, model.refreshID)
	case "s":
		model.searching = true
		model.search.Focus()
	case "?":
		model.help = !model.help
	}
	return model, nil
}

func (model Model) updateAutoRefresh() (Model, tea.Cmd) {
	nextTick := autoRefreshTickCmd()
	if !model.canAutoRefresh() {
		return model, nextTick
	}
	model.refreshID++
	model.refreshInFlight = true
	refreshCmd := reloadCmd(model.reloadCwd(), model.config, model.runner, false, true, model.refreshID)
	return model, tea.Batch(nextTick, refreshCmd)
}

func (model Model) canAutoRefresh() bool {
	return !model.refreshInFlight &&
		model.canApplyAutoRefresh()
}

func (model Model) canApplyAutoRefresh() bool {
	return model.loading == "" &&
		!model.searching &&
		!model.help &&
		model.createDialog == nil &&
		model.deleteDialog == nil
}

func (model Model) reloadCwd() string {
	if model.state.Repo.ActiveWorktree != "" {
		return model.state.Repo.ActiveWorktree
	}
	return model.state.Repo.Root
}

func (model Model) updateSearch(message tea.KeyMsg) (Model, tea.Cmd) {
	switch message.String() {
	case "esc":
		model.searching = false
		model.search.SetValue("")
		model.selected = 0
		return model, nil
	case "enter":
		model.searching = false
		return model, nil
	case "tab":
		model.cycleFilter()
		return model, nil
	}
	var cmd tea.Cmd
	model.search, cmd = model.search.Update(message)
	model.selected = clamp(model.selected, 0, max(0, len(model.visibleIndexes())-1))
	return model, cmd
}

func (model Model) openCreate() (Model, tea.Cmd) {
	row, ok := model.selectedRow()
	if !ok || row.Prunable {
		return model.setFlash("cannot create from this row")
	}
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 200
	input.Width = 34
	input.Cursor.Style = flashStyle
	focusCmd := input.Focus()
	bases := gitdata.BaseOptions(context.Background(), model.state.Repo, row, model.runner)
	if len(bases) == 0 {
		return model.setFlash("no base ref available")
	}
	model.createDialog = &createDialog{input: input, bases: bases}
	return model, focusCmd
}

func (model Model) updateCreate(message tea.KeyMsg) (Model, tea.Cmd) {
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
		path := pathutil.ApplyTemplate(model.config.PathTemplate, model.state.Repo.Root, branch)
		if _, err := os.Stat(path); err == nil {
			dialog.error = "target path already exists: " + path
			return model, nil
		}
		base := dialog.bases[dialog.baseIndex].Rev
		model.loading = "creating…"
		return model, func() tea.Msg {
			err := gitdata.CreateWorktree(context.Background(), model.state.Repo.Root, branch, path, base, model.runner)
			return createMsg{path: path, err: err}
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

func (model Model) openDelete() (Model, tea.Cmd) {
	row, ok := model.selectedRow()
	if !ok {
		return model, nil
	}
	if row.IsActive {
		return model.setFlash("cannot delete the active worktree")
	}
	if row.IsMain {
		return model.setFlash("cannot delete the main worktree")
	}
	model.deleteDialog = &deleteDialog{}
	return model, nil
}

func (model Model) updateDelete(message tea.KeyMsg) (Model, tea.Cmd) {
	dialog := model.deleteDialog
	switch message.String() {
	case "esc":
		model.deleteDialog = nil
		return model, nil
	case " ":
		dialog.deleteBranch = !dialog.deleteBranch
		dialog.error = ""
		return model, nil
	case "f":
		dialog.force = !dialog.force
		dialog.error = ""
		return model, nil
	case "enter":
		row, ok := model.selectedRow()
		if !ok {
			model.deleteDialog = nil
			return model, nil
		}
		needsForce := !row.Status.Clean() || dialog.deleteBranch && !row.BranchMergedToMain
		if needsForce && !dialog.force {
			dialog.error = "press f to arm force for dirty or unmerged deletion"
			return model, nil
		}
		model.loading = "deleting…"
		return model, func() tea.Msg {
			err := deleteRow(context.Background(), model.state.Repo, row, *dialog, model.runner)
			if err != nil {
				return deleteMsg{err: err}
			}
			state, err := gitdata.Load(context.Background(), model.state.Repo.ActiveWorktree, model.config, model.runner)
			return deleteMsg{state: state, err: err}
		}
	}
	return model, nil
}

func deleteRow(ctx context.Context, repo gitdata.Repository, row gitdata.Worktree, dialog deleteDialog, runner gitdata.Runner) error {
	if row.Prunable {
		if err := gitdata.PruneWorktrees(ctx, repo.Root, runner); err != nil {
			return err
		}
	} else if err := gitdata.RemoveWorktree(ctx, repo.Root, row.Path, dialog.force, runner); err != nil {
		return err
	}
	if dialog.deleteBranch && row.Branch != "" && !row.Detached {
		if err := gitdata.DeleteBranch(ctx, repo.Root, row.Branch, dialog.force && !row.BranchMergedToMain, runner); err != nil {
			return err
		}
	}
	return nil
}

func (model Model) View() string {
	now := time.Now()
	width := viewWidth(model)
	outerWidth := max(4, width)
	contentWidth := max(1, outerWidth-4)
	panelWidth := max(4, contentWidth)
	panelContentWidth := max(1, panelWidth-2)
	indexes := model.visibleIndexes()
	rows := make([]gitdata.Worktree, 0, len(indexes))
	for _, index := range indexes {
		rows = append(rows, model.state.Rows[index])
	}
	selectedRow, hasSelectedRow := model.selectedRow()
	detail := ""
	if hasSelectedRow {
		detail = model.detailPanelAtWidth(selectedRow, now, panelContentWidth)
	}
	tableFixedLines := 1
	detailFixedLines := 0
	if detail != "" {
		detailFixedLines = 2 + lineCount(detail)
	}
	fixedLines := 1 + 2 + tableFixedLines + detailFixedLines + 1
	if model.flash != "" {
		fixedLines++
	}
	if model.help {
		fixedLines += lineCount(model.renderHelp())
	}
	if model.deleteDialog != nil {
		fixedLines += lineCount(model.renderDelete())
	}
	availableHeight := max(1, model.height-fixedLines)
	if model.height <= 0 {
		availableHeight = 8
	}
	if model.deleteDialog != nil || model.help {
		availableHeight = max(3, availableHeight-8)
	}
	start := 0
	if model.selected >= availableHeight {
		start = model.selected - availableHeight + 1
	}
	if start > len(rows) {
		start = len(rows)
	}
	end := min(len(rows), start+availableHeight)
	visibleRows := rows[start:end]
	table := listview.RenderRows(visibleRows, listview.Options{
		Width:             panelContentWidth,
		Color:             true,
		Hyperlinks:        true,
		ShowHeader:        true,
		ShowPR:            model.showPR,
		HighlightSelected: true,
		SelectedIndex:     model.selected - start,
	}, now)
	lines := strings.Split(table, "\n")
	if len(rows) == 0 {
		lines = []string{"No worktrees"}
	}
	parts := []string{
		model.appTopLine(len(rows), outerWidth),
		model.wrapOuter(sectionBoxWithFooter("Worktrees", lines, model.listFooterHints(), panelWidth), outerWidth),
	}
	if detail != "" {
		parts = append(parts, model.wrapOuter(sectionBox("Details", strings.Split(detail, "\n"), panelWidth), outerWidth))
	}
	if model.flash != "" {
		parts = append(parts, model.wrapOuter(model.flashLineAtWidth(panelWidth), outerWidth))
	}
	if model.help {
		parts = append(parts, model.wrapOuter(model.renderHelp(), outerWidth))
	}
	if model.deleteDialog != nil {
		parts = append(parts, model.wrapOuter(model.renderDelete(), outerWidth))
	}
	parts = append(parts, model.appBottomLine(outerWidth))
	output := strings.Join(parts, "\n")
	if model.createDialog != nil {
		output = centeredOverlay(output, model.renderCreateAtWidth(createDialogWidth(outerWidth)), outerWidth, lineCount(output))
	}
	return model.frame(output)
}

func (model Model) selectedInspector(row gitdata.Worktree, now time.Time) string {
	return model.selectedInspectorAtWidth(row, now, viewWidth(model))
}

func (model Model) selectedInspectorAtWidth(row gitdata.Worktree, now time.Time, width int) string {
	lines := []string{
		model.inspectorRenderedFieldAtWidth("Branch", branchText(row), func(value string) string {
			return branchStyle(row).Render(value)
		}, width),
		model.inspectorRenderedFieldAtWidth("HEAD", headText(row), renderHeadValue, width),
		model.inspectorFieldAtWidth("Path", model.relativePath(row.Path), inspectorValueStyle, width),
		model.inspectorFieldAtWidth("Status", statusText(row), statusStyle(row), width),
		model.inspectorRenderedFieldAtWidth("Dirty", dirtyDetailText(row.Status), renderDirtyDetailValue, width),
	}
	lines = append(lines,
		model.inspectorRenderedFieldAtWidth("Remote", remoteText(row), func(value string) string {
			return syncStyle(row).Render(value)
		}, width),
		model.inspectorRenderedFieldAtWidth("Main", model.mainText(row), renderMainValue, width),
		model.inspectorRenderedFieldAtWidth("Commit", commitText(row, now), renderCommitValue, width),
		model.inspectorFieldAtWidth("PR", prText(row), inspectorValueStyle, width),
		model.inspectorFieldAtWidth("Delete", deleteSafetyText(row), deleteSafetyStyle(row), width),
	)
	return strings.Join(lines, "\n")
}

func (model Model) inspectorFieldAtWidth(label, value string, style lipgloss.Style, width int) string {
	return model.inspectorRenderedFieldAtWidth(label, value, func(value string) string {
		return style.Render(value)
	}, width)
}

func (model Model) inspectorRenderedFieldAtWidth(label, value string, render func(string) string, width int) string {
	labelWidth := 8
	separatorWidth := 2
	if width <= 0 {
		width = 80
	}
	if width <= labelWidth+separatorWidth {
		return truncatePlain(label+": "+value, width)
	}
	valueWidth := width - labelWidth - separatorWidth
	labelText := padRight(label, labelWidth)
	return inspectorLabelStyle.Render(labelText) + "  " + render(truncatePlain(value, valueWidth))
}

func (model Model) detailPanel(row gitdata.Worktree, now time.Time) string {
	return model.detailPanelAtWidth(row, now, viewWidth(model))
}

func (model Model) detailPanelAtWidth(row gitdata.Worktree, now time.Time, width int) string {
	if width < 72 {
		return model.selectedInspectorAtWidth(row, now, width)
	}
	leftWidth := width * 55 / 100
	leftWidth = clamp(leftWidth, 34, width-34)
	rightWidth := width - leftWidth - 3
	leftLines := strings.Split(model.selectedInspectorAtWidth(row, now, leftWidth), "\n")
	rightLines := keybindingLines(row, rightWidth)
	lineCount := max(len(leftLines), len(rightLines))
	lines := make([]string, 0, lineCount)
	divider := separatorStyle.Render("│")
	for index := 0; index < lineCount; index++ {
		left := ""
		right := ""
		if index < len(leftLines) {
			left = leftLines[index]
		}
		if index < len(rightLines) {
			right = rightLines[index]
		}
		lines = append(lines, padStyled(left, leftWidth)+" "+divider+" "+padStyled(right, rightWidth))
	}
	return strings.Join(lines, "\n")
}

func keybindingLines(row gitdata.Worktree, width int) []string {
	items := []string{
		selectionContextText(row),
		"↵ go",
		"o editor",
		"d delete",
		"y abs path",
		"p PR",
	}
	lines := make([]string, 0, len(items))
	for index, item := range items {
		lines = append(lines, padStyled(keybindText(item, width, index == 0), width))
	}
	return lines
}

func keybindText(value string, width int, heading bool) string {
	if value == "" || width <= 0 {
		return ""
	}
	if heading {
		return inspectorLabelStyle.Bold(true).Render(truncatePlain(value, width))
	}
	key, rest, found := strings.Cut(value, " ")
	if !found {
		return keyStyle.Render(truncatePlain(value, width))
	}
	visibleRestWidth := max(0, width-runewidth.StringWidth(key)-1)
	return keyStyle.Render(key) + hintStyle.Render(" "+truncatePlain(rest, visibleRestWidth))
}

func renderDirtyDetailValue(value string) string {
	if value == "none" {
		return inspectorCleanStyle.Render(value)
	}
	parts := strings.Split(value, "  ")
	for index, part := range parts {
		key, rest, found := strings.Cut(part, " ")
		if !found {
			parts[index] = inspectorWarnStyle.Render(part)
			continue
		}
		switch key {
		case "+":
			parts[index] = inspectorCleanStyle.Render(key) + hintStyle.Render(" "+rest)
		case "~":
			parts[index] = inspectorWarnStyle.Render(key) + hintStyle.Render(" "+rest)
		case "?":
			parts[index] = inspectorCommitStyle.Render(key) + hintStyle.Render(" "+rest)
		default:
			parts[index] = inspectorWarnStyle.Render(part)
		}
	}
	return strings.Join(parts, hintStyle.Render("  "))
}

func renderCommitValue(value string) string {
	hash, rest, found := strings.Cut(value, " ")
	if !found {
		return inspectorCommitStyle.Render(value)
	}
	return inspectorCommitStyle.Render(hash) + inspectorSubjectStyle.Render(" "+rest)
}

func renderHeadValue(value string) string {
	head, rest, found := strings.Cut(value, " ")
	if !found {
		return inspectorCommitStyle.Render(value)
	}
	return inspectorCommitStyle.Render(head) + inspectorSubjectStyle.Render(" "+rest)
}

func renderMainValue(value string) string {
	if strings.HasPrefix(value, "↑") || strings.Contains(value, " ↑") || strings.Contains(value, "↓") {
		parts := strings.Split(value, " ")
		for index, part := range parts {
			switch {
			case strings.HasPrefix(part, "↑"):
				parts[index] = inspectorWarnStyle.Render(part)
			case strings.HasPrefix(part, "↓"):
				parts[index] = inspectorWarnStyle.Render(part)
			default:
				parts[index] = inspectorValueStyle.Render(part)
			}
		}
		return strings.Join(parts, " ")
	}
	return inspectorValueStyle.Render(value)
}

func statusStyle(row gitdata.Worktree) lipgloss.Style {
	if row.Status.Clean() {
		return inspectorCleanStyle
	}
	return inspectorWarnStyle
}

func branchStyle(row gitdata.Worktree) lipgloss.Style {
	if row.Detached {
		return inspectorCommitStyle
	}
	return inspectorValueStyle
}

func syncStyle(row gitdata.Worktree) lipgloss.Style {
	if row.UpstreamGone || row.HeadSync.Ahead > 0 || row.HeadSync.Behind > 0 {
		return inspectorWarnStyle
	}
	if row.Upstream != "" {
		return inspectorCleanStyle
	}
	return inspectorValueStyle
}

func remoteText(row gitdata.Worktree) string {
	if row.Upstream == "" {
		return "no upstream"
	}
	if row.UpstreamGone {
		return "Remote branch gone, likely merged or deleted"
	}
	state := row.HeadSync.Compact()
	if state == "" {
		state = "synced"
	}
	return row.Upstream + ", " + state
}

func branchText(row gitdata.Worktree) string {
	return row.DisplayBranch()
}

func headText(row gitdata.Worktree) string {
	if row.Head == "" {
		if row.Detached {
			return "detached"
		}
		if row.Branch != "" {
			return "on " + row.Branch
		}
		return "-"
	}
	if row.Detached {
		return shortRef(row.Head) + " detached"
	}
	if row.Branch != "" {
		return shortRef(row.Head) + " on " + row.Branch
	}
	return shortRef(row.Head)
}

func statusText(row gitdata.Worktree) string {
	if row.Status.Clean() {
		return "clean"
	}
	return "dirty"
}

func (model Model) mainText(row gitdata.Worktree) string {
	if model.state.Repo.MainBranch == "" {
		return "unknown"
	}
	if row.Branch == model.state.Repo.MainBranch && !row.Detached {
		return "on local " + model.state.Repo.MainBranch
	}
	state := row.MainSync.Compact()
	if state == "" {
		if row.MainSync.Available {
			return "synced with local " + model.state.Repo.MainBranch
		}
		return "- vs local " + model.state.Repo.MainBranch
	}
	return state + " vs local " + model.state.Repo.MainBranch
}

func selectionContextText(row gitdata.Worktree) string {
	switch {
	case row.IsActive && row.IsMain:
		return "Current root repository"
	case row.IsActive:
		return "Current worktree"
	case row.IsMain:
		return "Root repository"
	default:
		return "Actions"
	}
}

func dirtyDetailText(counts gitdata.StatusCounts) string {
	parts := make([]string, 0, 3)
	if counts.Staged > 0 {
		parts = append(parts, fmt.Sprintf("+ staged %d", counts.Staged))
	}
	if counts.Modified > 0 {
		parts = append(parts, fmt.Sprintf("~ modified %d", counts.Modified))
	}
	if counts.Untracked > 0 {
		parts = append(parts, fmt.Sprintf("? untracked %d", counts.Untracked))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "  ")
}

func prText(row gitdata.Worktree) string {
	if row.PR == nil {
		return "none"
	}
	text := row.PR.Text()
	if text == "" {
		return "none"
	}
	return text
}

func deleteSafetyText(row gitdata.Worktree) string {
	switch {
	case row.IsActive && row.IsMain:
		return "blocked, active root repository"
	case row.IsActive:
		return "blocked, active worktree"
	case row.IsMain:
		return "blocked, root repository"
	case row.Prunable:
		return "allowed, prunes missing worktree metadata"
	case !row.Status.Clean():
		return "allowed with force, dirty worktree"
	case row.Locked:
		return "allowed only if git unlocks it"
	default:
		return "allowed, branch kept by default"
	}
}

func deleteSafetyStyle(row gitdata.Worktree) lipgloss.Style {
	if row.IsActive || row.IsMain || row.Prunable || row.Locked || !row.Status.Clean() {
		return inspectorWarnStyle
	}
	return inspectorCleanStyle
}

func commitText(row gitdata.Worktree, now time.Time) string {
	if row.CommitShort == "" {
		return "-"
	}
	commit := row.CommitShort + " " + row.CommitSubject
	if age := gitdata.RelativeAge(now, row.CommitTime); age != "" {
		commit += ", " + age
	}
	return commit
}

func shortRef(ref string) string {
	if len(ref) <= 7 {
		return ref
	}
	return ref[:7]
}

func (model Model) relativePath(path string) string {
	base := model.state.Repo.ActiveWorktree
	if base == "" {
		base = model.state.Repo.Root
	}
	relative, err := filepath.Rel(base, path)
	if err != nil || relative == "" {
		return path
	}
	return relative
}

func (model Model) appTopLine(visibleCount, width int) string {
	return model.appTopLineAtTime(visibleCount, width, time.Now())
}

func (model Model) appTopLineAtTime(visibleCount, width int, now time.Time) string {
	if width <= 0 {
		return ""
	}
	if width < 4 {
		return appBorderStyle.Render(strings.Repeat("─", width))
	}
	innerWidth := width - 4
	right := model.appControlsAtWidthAtTime(innerWidth, now)
	if right != "" {
		right = " " + right + " "
	}
	leftMaxWidth := innerWidth - lipgloss.Width(right) - 3
	if leftMaxWidth < 3 {
		right = ""
		leftMaxWidth = innerWidth - 2
	}
	left := model.titleLeftContentAtWidth(visibleCount, leftMaxWidth)
	if left != "" {
		left = " " + left + " "
	}
	gapWidth := innerWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if gapWidth < 0 {
		gapWidth = 0
	}
	return appBorderStyle.Render("╭─") + left + appBorderStyle.Render(strings.Repeat("─", gapWidth)) + right + appBorderStyle.Render("─╮")
}

func (model Model) appBottomLine(width int) string {
	if width <= 0 {
		return ""
	}
	if width < 4 {
		return appBorderStyle.Render(strings.Repeat("─", width))
	}
	contentWidth := width - 6
	if contentWidth < 1 {
		return appBorderStyle.Render("╰" + strings.Repeat("─", width-2) + "╯")
	}
	leftParts := model.statusLeftParts()
	leftText := joinPartsWithin(leftParts, contentWidth)
	left := colorKeyHints(leftText, model.loading != "" && strings.Contains(leftText, model.loading))
	right := statusLegendForWidth(contentWidth - lipgloss.Width(left) - 3)
	if right == "" {
		return bottomLineWithContent(left, width)
	}
	return bottomLineWithSplit(left, right, width)
}

func bottomLineWithContent(content string, width int) string {
	contentWidth := width - 6
	if contentWidth < 1 {
		return appBorderStyle.Render("╰" + strings.Repeat("─", max(0, width-2)) + "╯")
	}
	return appBorderStyle.Render("╰─ ") + padStyled(content, contentWidth) + appBorderStyle.Render(" ─╯")
}

func bottomLineWithSplit(left, right string, width int) string {
	contentWidth := width - 6
	left = left + " "
	right = " " + right
	fillerWidth := contentWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if fillerWidth < 1 {
		return bottomLineWithContent(strings.TrimSpace(left), width)
	}
	return appBorderStyle.Render("╰─ ") + left + appBorderStyle.Render(strings.Repeat("─", fillerWidth)) + right + appBorderStyle.Render(" ─╯")
}

func (model Model) wrapOuter(content string, width int) string {
	if width < 4 {
		return content
	}
	innerWidth := width - 4
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		lines[index] = appBorderStyle.Render("│ ") + padStyled(line, innerWidth) + appBorderStyle.Render(" │")
	}
	return strings.Join(lines, "\n")
}

func sectionBox(title string, bodyLines []string, width int) string {
	return sectionBoxWithFooter(title, bodyLines, "", width)
}

func sectionBoxWithFooter(title string, bodyLines []string, footer string, width int) string {
	if width < 4 {
		return strings.Join(bodyLines, "\n")
	}
	innerWidth := width - 2
	lines := make([]string, 0, len(bodyLines)+2)
	lines = append(lines, sectionTopLine(title, width))
	for _, line := range bodyLines {
		lines = append(lines, panelBorderStyle.Render("│")+padStyled(line, innerWidth)+panelBorderStyle.Render("│"))
	}
	lines = append(lines, sectionBottomLine(footer, width))
	return strings.Join(lines, "\n")
}

func sectionTopLine(title string, width int) string {
	innerWidth := width - 2
	label := ""
	if title != "" {
		label = " " + title + " "
		label = truncatePlain(label, max(0, innerWidth-1))
	}
	labelWidth := runewidth.StringWidth(label)
	ruleWidth := innerWidth - 1 - labelWidth
	if ruleWidth < 0 {
		ruleWidth = 0
	}
	return panelBorderStyle.Render("╭─") + panelTitleStyle.Render(label) + panelBorderStyle.Render(strings.Repeat("─", ruleWidth)+"╮")
}

func sectionBottomLine(footer string, width int) string {
	innerWidth := width - 2
	if footer == "" {
		return panelBorderStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯")
	}
	maxLabelWidth := max(0, innerWidth-1)
	footer = truncatePlain(footer, maxLabelWidth)
	label := " " + colorKeyHints(footer, false) + " "
	labelWidth := lipgloss.Width(label)
	ruleWidth := innerWidth - 1 - labelWidth
	if ruleWidth < 0 {
		ruleWidth = 0
	}
	return panelBorderStyle.Render("╰─") + label + panelBorderStyle.Render(strings.Repeat("─", ruleWidth)+"╯")
}

func padRight(value string, width int) string {
	visible := runewidth.StringWidth(value)
	if visible >= width {
		return value
	}
	return value + strings.Repeat(" ", width-visible)
}

func padStyled(value string, width int) string {
	visible := lipgloss.Width(value)
	if visible >= width {
		return value
	}
	return value + strings.Repeat(" ", width-visible)
}

func truncatePlain(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	var builder strings.Builder
	used := 0
	for _, character := range value {
		characterWidth := runewidth.RuneWidth(character)
		if used+characterWidth > width-1 {
			break
		}
		builder.WriteRune(character)
		used += characterWidth
	}
	builder.WriteString("…")
	return builder.String()
}

func (model Model) statusBar() string {
	return model.statusBarAtWidth(viewWidth(model))
}

func (model Model) statusBarAtWidth(width int) string {
	leftParts := model.statusLeftParts()
	leftText := joinPartsWithin(leftParts, width)
	left := colorKeyHints(leftText, model.loading != "" && strings.Contains(leftText, model.loading))
	right := statusLegendForWidth(width - lipgloss.Width(left) - 2)
	if right == "" {
		return left
	}
	spacerWidth := width - lipgloss.Width(left) - lipgloss.Width(right)
	if spacerWidth < 1 {
		spacerWidth = 1
	}
	return left + strings.Repeat(" ", spacerWidth) + right
}

func (model Model) statusLeftParts() []string {
	leftParts := []string{"Esc close/clear"}
	if model.loading != "" {
		leftParts = append(leftParts, model.loading)
	}
	return leftParts
}

func (model Model) listFooterHints() string {
	if model.searching {
		return "search " + model.search.Value() + "▌ · Esc clear · Tab filter: " + model.filter.label()
	}
	return "h root · a active · Tab filter: " + model.filter.label() + " · s search"
}

func dirtyLegendParts() []string {
	return []string{"⌂ root", "! locked", "× prunable", "remote ✓/-/gone", "+ staged", "~ modified", "? untracked"}
}

func statusLegendForWidth(width int) string {
	if width < 12 {
		return ""
	}
	plain := strings.Join(dirtyLegendParts(), " · ")
	if runewidth.StringWidth(plain) > width {
		plain = truncatePlain(plain, width)
	}
	return colorDirtyLegend(plain)
}

func joinPartsWithin(parts []string, width int) string {
	if len(parts) == 0 || width <= 0 {
		return ""
	}
	for count := len(parts); count > 0; count-- {
		text := strings.Join(parts[:count], " · ")
		if runewidth.StringWidth(text) <= width {
			return text
		}
	}
	return truncatePlain(parts[0], width)
}

func colorKeyHints(text string, hasStatus bool) string {
	parts := strings.Split(text, " · ")
	for index, part := range parts {
		parts[index] = colorKeyHintPart(part, hasStatus && index == len(parts)-1)
	}
	return strings.Join(parts, hintStyle.Render(" · "))
}

func colorKeyHintPart(part string, isStatus bool) string {
	key, rest, found := strings.Cut(part, " ")
	if found && key != "" {
		return keyStyle.Render(key) + hintStyle.Render(" "+rest)
	}
	if isStatus {
		return statusMessageStyle.Render(part)
	}
	return hintStyle.Render(part)
}

func colorDirtyLegend(text string) string {
	return colorStatusBar(text, false)
}

func colorStatusBar(text string, hasStatus bool) string {
	parts := strings.Split(text, " · ")
	for index, part := range parts {
		parts[index] = colorDirtyLegendPartWithStatus(part, hasStatus && index == len(parts)-1)
	}
	return strings.Join(parts, hintStyle.Render(" · "))
}

func colorDirtyLegendPartWithStatus(part string, isStatus bool) string {
	key, rest, found := strings.Cut(part, " ")
	if key == "⌂" {
		return inspectorCommitStyle.Render(key) + hintStyle.Render(" "+rest)
	}
	if key == "!" {
		return inspectorWarnStyle.Render(key) + hintStyle.Render(" "+rest)
	}
	if key == "×" {
		return inspectorWarnStyle.Render(key) + hintStyle.Render(" "+rest)
	}
	if key == "+" {
		return inspectorCleanStyle.Render(key) + hintStyle.Render(" "+rest)
	}
	if key == "~" {
		return inspectorWarnStyle.Render(key) + hintStyle.Render(" "+rest)
	}
	if key == "?" {
		return inspectorCommitStyle.Render(key) + hintStyle.Render(" "+rest)
	}
	if found && key != "" {
		return keyStyle.Render(key) + hintStyle.Render(" "+rest)
	}
	if isStatus {
		return statusMessageStyle.Render(part)
	}
	return hintStyle.Render(part)
}

func (model Model) titleContentAtWidthAtTime(visibleCount, width int, now time.Time) string {
	if width <= 0 {
		return ""
	}
	right := model.appControlsAtWidthAtTime(width, now)
	leftWidth := width - lipgloss.Width(right) - 2
	if leftWidth < 3 {
		right = ""
		leftWidth = width
	}
	left := model.titleLeftContentAtWidth(visibleCount, leftWidth)
	if right == "" {
		return padStyled(left, width)
	}
	spacerWidth := width - lipgloss.Width(left) - lipgloss.Width(right)
	if spacerWidth < 1 {
		spacerWidth = 1
	}
	return left + strings.Repeat(" ", spacerWidth) + right
}

func (model Model) titleLeftContentAtWidth(visibleCount, width int) string {
	if width <= 0 {
		return ""
	}
	repoName := filepath.Base(model.state.Repo.Root)
	if repoName == "." || repoName == string(filepath.Separator) {
		repoName = model.state.Repo.Root
	}
	count := fmt.Sprintf("%d worktrees", len(model.state.Rows))
	if model.search.Value() != "" || model.filter != filterAll {
		count = fmt.Sprintf("%d/%d worktrees", visibleCount, len(model.state.Rows))
	}
	rootBranch := model.rootBranchTitle()
	if width <= runewidth.StringWidth(appTitle) {
		return titleNameStyle.Render(truncatePlain(appTitle, width))
	}
	staticWidth := runewidth.StringWidth(appTitle+"  ") + runewidth.StringWidth("  ") + runewidth.StringWidth(count)
	if rootBranch != "" {
		staticWidth += runewidth.StringWidth("  root: ")
	}
	repoWidth := width - staticWidth
	if repoWidth < 4 {
		compactWidth := width - runewidth.StringWidth(appTitle+"  ") - runewidth.StringWidth(count)
		if compactWidth >= 0 {
			title := titleNameStyle.Render(appTitle)
			meta := titleMetaStyle.Render(count)
			return title + "  " + meta
		}
		repoWidth = width - runewidth.StringWidth(appTitle+"  ")
		if repoWidth <= 0 {
			return titleNameStyle.Render(truncatePlain(appTitle, width))
		}
		return titleNameStyle.Render(appTitle) + "  " + titleRepoStyle.Render(truncatePlain(repoName, repoWidth))
	}
	repoName = truncatePlain(repoName, repoWidth)
	title := titleNameStyle.Render(appTitle)
	repo := titleRepoStyle.Render(repoName)
	meta := titleMetaStyle.Render(count)
	if rootBranch == "" {
		return title + "  " + repo + "  " + meta
	}
	rootWidth := width - lipgloss.Width(title+"  "+repo+"  "+meta+"  "+titleMetaStyle.Render("root: "))
	if rootWidth < 3 {
		return title + "  " + repo + "  " + meta
	}
	return title + "  " + repo + "  " + meta + "  " + titleMetaStyle.Render("root: ") + titleRepoStyle.Render(truncatePlain(rootBranch, rootWidth))
}

func (model Model) rootBranchTitle() string {
	for _, row := range model.state.Rows {
		if row.IsMain {
			return row.DisplayBranch()
		}
	}
	return ""
}

func (model Model) appControlsAtWidthAtTime(width int, now time.Time) string {
	refresh := refreshControlText(model.lastRefreshAt, now)
	fullWithAge := colorKeyHints("n new · "+refresh+" · ? help · q quit", false)
	if lipgloss.Width(fullWithAge) <= width {
		return fullWithAge
	}
	mediumWithAge := colorKeyHints(refresh+" · ? help · q quit", false)
	if lipgloss.Width(mediumWithAge) <= width {
		return mediumWithAge
	}
	full := colorKeyHints("n new · r refresh · ? help · q quit", false)
	if lipgloss.Width(full) <= width {
		return full
	}
	medium := colorKeyHints("r refresh · ? help · q quit", false)
	if lipgloss.Width(medium) <= width {
		return medium
	}
	short := colorKeyHints("? help · q quit", false)
	if lipgloss.Width(short) <= width {
		return short
	}
	tiny := colorKeyHints("? · q", false)
	if lipgloss.Width(tiny) <= width {
		return tiny
	}
	return ""
}

func refreshControlText(lastRefreshAt, now time.Time) string {
	age := refreshAgeText(lastRefreshAt, now)
	if age == "" {
		return "r refresh"
	}
	return "r refresh (" + age + ")"
}

func refreshAgeText(lastRefreshAt, now time.Time) string {
	if lastRefreshAt.IsZero() {
		return ""
	}
	elapsed := now.Sub(lastRefreshAt)
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed < time.Minute {
		return fmt.Sprintf("%d seconds ago", int(elapsed.Seconds()))
	}
	minutes := int(elapsed.Minutes())
	if minutes == 1 {
		return "1 minute ago"
	}
	return fmt.Sprintf("%d minutes ago", minutes)
}

func clockTickCmd() tea.Cmd {
	return tea.Tick(clockTickInterval, func(time.Time) tea.Msg {
		return clockTickMsg{}
	})
}

func autoRefreshTickCmd() tea.Cmd {
	return tea.Tick(autoRefreshInterval, func(time.Time) tea.Msg {
		return autoRefreshMsg{}
	})
}

func (model Model) flashLineAtWidth(width int) string {
	return flashStyle.Render(truncatePlain(model.flash, width))
}

func (model Model) setFlash(text string) (Model, tea.Cmd) {
	model.flashID++
	model.flash = text
	id := model.flashID
	return model, tea.Tick(2200*time.Millisecond, func(time.Time) tea.Msg {
		return clearFlashMsg{id: id}
	})
}

func (model Model) selectionAnchor() selectionAnchor {
	row, ok := model.selectedRow()
	if !ok {
		return selectionAnchor{}
	}
	return selectionAnchor{path: row.Path, branch: row.Branch, head: row.Head}
}

func (model *Model) restoreSelection(anchor selectionAnchor) bool {
	indexes := model.visibleIndexes()
	if len(indexes) == 0 {
		model.selected = 0
		return false
	}
	if anchor.path != "" {
		for visibleIndex, rowIndex := range indexes {
			if model.state.Rows[rowIndex].Path == anchor.path {
				model.selected = visibleIndex
				return true
			}
		}
	}
	if anchor.branch != "" || anchor.head != "" {
		for visibleIndex, rowIndex := range indexes {
			row := model.state.Rows[rowIndex]
			if anchor.branch != "" && row.Branch == anchor.branch {
				model.selected = visibleIndex
				return true
			}
			if anchor.head != "" && row.Head == anchor.head {
				model.selected = visibleIndex
				return true
			}
		}
	}
	model.selected = clamp(model.selected, 0, len(indexes)-1)
	return false
}

func (model *Model) selectMatching(match func(gitdata.Worktree) bool) {
	for visibleIndex, rowIndex := range model.visibleIndexes() {
		if match(model.state.Rows[rowIndex]) {
			model.selected = visibleIndex
			return
		}
	}
}

func (model *Model) cycleFilter() {
	anchor := model.selectionAnchor()
	currentIndex := 0
	for index, filter := range orderedFilters {
		if filter == model.filter {
			currentIndex = index
			break
		}
	}
	for offset := 1; offset <= len(orderedFilters); offset++ {
		filter := orderedFilters[(currentIndex+offset)%len(orderedFilters)]
		if filter != filterAll && len(model.visibleIndexesForFilter(filter)) == 0 {
			continue
		}
		model.filter = filter
		if !model.restoreSelection(anchor) && len(model.visibleIndexes()) > 0 {
			model.selected = 0
		}
		return
	}
}

func (model Model) frame(content string) string {
	width := viewWidth(model)
	lines := strings.Split(content, "\n")
	height := model.height
	if height <= 0 {
		return strings.Join(lines, "\n")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

func lineCount(value string) int {
	if value == "" {
		return 0
	}
	return len(strings.Split(value, "\n"))
}

func viewWidth(model Model) int {
	if model.width > 0 {
		return model.width
	}
	return 80
}

func (model Model) renderHelp() string {
	return box("Help", strings.Join([]string{
		"↑/↓ k/j move",
		"g/G jump top/bottom",
		"h jump root repository",
		"a jump active worktree",
		"Tab filter: all, modified, prunable, locked, detached",
		"Enter cd to worktree",
		"n create worktree",
		"d delete worktree",
		"o open editor",
		"p open PR or branch page",
		"y copy absolute path",
		"r/f fetch --prune and reload",
		"s search branches",
		"Esc close, clear filter/search, or quit",
	}, "\n"))
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
	if dialog.error != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(truncatePlain(dialog.error, contentWidth)))
	}
	return dialogBox("New worktree", lines, createDialogHintsAtWidth(width-6), width)
}

func (model Model) createPathPreview() string {
	if model.createDialog == nil {
		return ""
	}
	branch := strings.TrimSpace(model.createDialog.input.Value())
	if branch == "" {
		return "enter branch name"
	}
	return pathutil.ApplyTemplate(model.config.PathTemplate, model.state.Repo.Root, branch)
}

func (model Model) renderDelete() string {
	row, _ := model.selectedRow()
	dialog := model.deleteDialog
	branchToggle := "[ ]"
	if dialog.deleteBranch {
		branchToggle = "[x]"
	}
	force := "not armed"
	if dialog.force {
		force = "armed"
	}
	lines := []string{"Path: " + row.Path}
	if !row.Status.Clean() {
		lines = append(lines, "Dirty: "+row.Status.Text())
	}
	if row.Prunable {
		lines = append(lines, "Will prune missing worktree metadata.")
	} else {
		lines = append(lines, "Will remove this worktree.")
	}
	if row.Branch != "" && !row.Detached {
		lines = append(lines, branchToggle+" also delete branch "+row.Branch)
		if row.BranchMergedToMain {
			lines = append(lines, "Branch is merged into "+model.state.Repo.MainBranch+".")
		} else {
			lines = append(lines, "Branch is not merged into "+model.state.Repo.MainBranch+".")
		}
		if row.UpstreamGone {
			lines = append(lines, "Remote branch already deleted, likely safe.")
		}
	}
	lines = append(lines, "Force: "+force+" (f toggles)")
	if dialog.error != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(dialog.error))
	}
	lines = append(lines, "", "Enter delete · Space branch · f force · Esc cancel")
	return box("Delete worktree", strings.Join(lines, "\n"))
}

func box(title, body string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Render(title + "\n" + body)
}

func createDialogWidth(viewWidth int) int {
	if viewWidth <= 0 {
		return 72
	}
	inset := 8
	if viewWidth < 48 {
		inset = 2
	}
	return max(4, min(72, viewWidth-inset))
}

func dialogBox(title string, bodyLines []string, bottomContent string, width int) string {
	width = max(4, width)
	contentWidth := max(1, width-4)
	lines := make([]string, 0, len(bodyLines)+2)
	lines = append(lines, dialogTopLine(title, width))
	for _, line := range bodyLines {
		lines = append(lines, appBorderStyle.Render("│ ")+padStyled(line, contentWidth)+appBorderStyle.Render(" │"))
	}
	lines = append(lines, dialogBottomLine(bottomContent, width))
	return strings.Join(lines, "\n")
}

func dialogTopLine(title string, width int) string {
	innerWidth := width - 2
	label := ""
	if title != "" {
		label = " " + title + " "
		label = truncatePlain(label, max(0, innerWidth-1))
	}
	labelWidth := runewidth.StringWidth(label)
	ruleWidth := max(0, innerWidth-1-labelWidth)
	return appBorderStyle.Render("╭─") + panelTitleStyle.Render(label) + appBorderStyle.Render(strings.Repeat("─", ruleWidth)+"╮")
}

func dialogBottomLine(content string, width int) string {
	contentWidth := width - 6
	if contentWidth < 1 || content == "" {
		return appBorderStyle.Render("╰" + strings.Repeat("─", max(0, width-2)) + "╯")
	}
	return appBorderStyle.Render("╰─ ") + padStyled(content, contentWidth) + appBorderStyle.Render(" ─╯")
}

func createDialogHintsAtWidth(width int) string {
	full := colorKeyHints("Enter create · Tab switch base · ctrl+o config · Esc cancel", false)
	if lipgloss.Width(full) <= width {
		return full
	}
	medium := colorKeyHints("Enter create · Tab base · ctrl+o config · Esc cancel", false)
	if lipgloss.Width(medium) <= width {
		return medium
	}
	short := colorKeyHints("Enter · Tab · ctrl+o · Esc", false)
	if lipgloss.Width(short) <= width {
		return short
	}
	return ""
}

func centeredOverlay(base, popup string, width, height int) string {
	lines := strings.Split(base, "\n")
	if height <= 0 {
		height = len(lines)
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	popupLines := strings.Split(popup, "\n")
	popupWidth := 0
	for _, line := range popupLines {
		popupWidth = max(popupWidth, lipgloss.Width(line))
	}
	top := max(0, (height-len(popupLines))/2)
	left := max(0, (width-popupWidth)/2)
	for index, line := range popupLines {
		target := top + index
		if target >= len(lines) {
			break
		}
		lines[target] = strings.Repeat(" ", left) + padStyled(line, popupWidth) + strings.Repeat(" ", max(0, width-left-popupWidth))
	}
	return strings.Join(lines, "\n")
}

func (model Model) visibleIndexes() []int {
	return model.visibleIndexesForFilter(model.filter)
}

func (model Model) visibleIndexesForFilter(filter worktreeFilter) []int {
	pattern := model.search.Value()
	indexes := make([]int, 0, len(model.state.Rows))
	for index, row := range model.state.Rows {
		branchMatches := pattern == "" || fuzzyMatch(row.DisplayBranch(), pattern)
		if branchMatches && filter.matches(row) {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func (model Model) selectedRow() (gitdata.Worktree, bool) {
	indexes := model.visibleIndexes()
	if len(indexes) == 0 || model.selected < 0 || model.selected >= len(indexes) {
		return gitdata.Worktree{}, false
	}
	return model.state.Rows[indexes[model.selected]], true
}

func (model Model) enrichmentCommands() tea.Cmd {
	commands := []tea.Cmd{
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			pullRequests, enabled := github.LoadPullRequests(ctx, model.state.Repo.Root, model.runner)
			return prLoadedMsg{pullRequests: pullRequests, enabled: enabled}
		},
	}
	for _, row := range model.state.Rows {
		if row.Prunable {
			continue
		}
		path := row.Path
		commands = append(commands, func() tea.Msg {
			size, _ := gitdata.DiskUsage(path)
			return sizeLoadedMsg{path: path, size: size}
		})
	}
	return tea.Batch(commands...)
}

func reloadCmd(cwd string, config config.Config, runner gitdata.Runner, fetch, automatic bool, id int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if fetch {
			if state, err := gitdata.Load(ctx, cwd, config, runner); err == nil && state.Repo.RemoteConfigured {
				if err := gitdata.FetchPrune(ctx, state.Repo.Root, runner); err != nil {
					return reloadMsg{id: id, automatic: automatic, completedAt: time.Now(), err: err}
				}
			}
		}
		state, err := gitdata.Load(ctx, cwd, config, runner)
		return reloadMsg{id: id, automatic: automatic, completedAt: time.Now(), state: state, err: err}
	}
}

func openEditorCmd(editor, path string) tea.Cmd {
	return func() tea.Msg {
		if editor == "" {
			editor = "code"
		}
		err := exec.Command("sh", "-c", shellQuoteCommand(editor, path)).Start()
		return actionMsg{text: "opened editor", err: err}
	}
}

func openConfigCmd(editor string, currentConfig config.Config) tea.Cmd {
	return func() tea.Msg {
		path, err := config.Path()
		if err != nil {
			return configOpenedMsg{err: err}
		}
		if _, err := os.Stat(path); err != nil {
			if !os.IsNotExist(err) {
				return configOpenedMsg{err: err}
			}
			if err := config.SaveDefault(currentConfig); err != nil {
				return configOpenedMsg{err: err}
			}
		}
		info, err := os.Stat(path)
		if err != nil {
			return configOpenedMsg{err: err}
		}
		if editor == "" {
			editor = "code"
		}
		err = exec.Command("sh", "-c", shellQuoteCommand(editor, path)).Start()
		return configOpenedMsg{path: path, modTime: info.ModTime(), err: err}
	}
}

func watchConfigChangeCmd(path string, previousModTime time.Time) tea.Cmd {
	return func() tea.Msg {
		deadline := time.Now().Add(2 * time.Minute)
		for time.Now().Before(deadline) {
			time.Sleep(500 * time.Millisecond)
			config, modTime, changed, err := loadConfigIfChanged(path, previousModTime)
			if err != nil {
				return configReloadedMsg{err: err}
			}
			if changed {
				return configReloadedMsg{config: config, path: path, modTime: modTime}
			}
		}
		return noOpMsg{}
	}
}

func loadConfigIfChanged(path string, previousModTime time.Time) (config.Config, time.Time, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return config.Config{}, time.Time{}, false, err
	}
	if info.ModTime().Equal(previousModTime) {
		return config.Config{}, info.ModTime(), false, nil
	}
	loadedConfig, err := config.Load(path)
	if err != nil {
		return loadedConfig, info.ModTime(), true, err
	}
	return loadedConfig, info.ModTime(), true, nil
}

func copyPathCmd(path string) tea.Cmd {
	return func() tea.Msg {
		var command *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			command = exec.Command("pbcopy")
		case "windows":
			command = exec.Command("clip")
		default:
			if _, err := exec.LookPath("wl-copy"); err == nil {
				command = exec.Command("wl-copy")
			} else {
				command = exec.Command("xclip", "-selection", "clipboard")
			}
		}
		command.Stdin = strings.NewReader(path)
		err := command.Run()
		return actionMsg{text: "copied absolute path: " + path, err: err}
	}
}

func shellQuoteCommand(command, argument string) string {
	return command + " " + shellQuote(argument)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func fuzzyMatch(text, pattern string) bool {
	textRunes := []rune(strings.ToLower(text))
	pattern = strings.ToLower(pattern)
	if pattern == "" {
		return true
	}
	textIndex := 0
	for _, character := range pattern {
		found := false
		for textIndex < len(textRunes) {
			if textRunes[textIndex] == character {
				found = true
				textIndex++
				break
			}
			textIndex++
		}
		if !found {
			return false
		}
	}
	return true
}

func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func FormatExitError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
