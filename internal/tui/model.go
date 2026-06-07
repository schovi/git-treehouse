package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/github"
	"github.com/schovi/git-treehouse/internal/listview"
	"github.com/schovi/git-treehouse/internal/pathutil"
)

type Model struct {
	state             gitdata.State
	config            config.Config
	runner            gitdata.Runner
	width             int
	height            int
	selected          int
	filter            worktreeFilter
	searching         bool
	search            textinput.Model
	help              bool
	loading           string
	flash             string
	flashID           int
	showPR            bool
	prLoading         bool
	prCache           map[string]gitdata.PullRequest
	prCacheRepoRoot   string
	prLastCheckedAt   time.Time
	selectedPath      string
	createDialog      *createDialog
	deleteDialog      *deleteDialog
	paletteDialog     *paletteDialog
	lastRefreshAt     time.Time
	refreshInFlight   bool
	refreshID         int
	enrichmentID      int
	enrichmentContext context.Context
	enrichmentCancel  context.CancelFunc
}

var (
	appBorderStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("65"))
	panelBorderStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	panelTitleStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true)
	titleNameStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true)
	titleRepoStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	titleSeparatorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	titleMetaStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	flashStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("58"))
	paletteSelectedStyle  = lipgloss.NewStyle().Background(lipgloss.Color("62"))
	deleteDangerStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	deleteCommandStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	inspectorLabelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("67"))
	inspectorValueStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	inspectorCleanStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	inspectorWarnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	inspectorCommitStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	inspectorSubjectStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	keyStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	hintStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	helpCategoryStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	statusMessageStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

const (
	autoRefreshInterval = 30 * time.Second
	clockTickInterval   = time.Second
	prRefreshTTL        = 5 * time.Minute
	appTitle            = "Git treehouse"
)

type createDialog struct {
	input     textinput.Model
	bases     []gitdata.BaseOption
	baseIndex int
	error     string
}

type deleteDialog struct {
	stage          deleteStage
	deleteWorktree bool
	deleteBranch   bool
	forceWorktree  bool
	error          string
}

type deleteStage int

const (
	deleteStageOptions deleteStage = iota
	deleteStagePrune
	deleteStageLocked
)

type paletteDialog struct {
	input    textinput.Model
	selected int
}

type prLoadedMsg struct {
	pullRequests map[string]gitdata.PullRequest
	enabled      bool
	repoRoot     string
	id           int
	checkedAt    time.Time
}

type sizesLoadedMsg struct {
	gitSizes  map[string]int64
	fullSizes map[string]int64
	repoRoot  string
	id        int
}

type localMetadataLoadedMsg struct {
	state    gitdata.State
	err      error
	repoRoot string
	id       int
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

type paletteCommandID string

const (
	paletteGoSelected      paletteCommandID = "go-selected"
	paletteCreate          paletteCommandID = "create"
	paletteDelete          paletteCommandID = "delete"
	paletteOpenEditor      paletteCommandID = "open-editor"
	paletteOpenPullRequest paletteCommandID = "open-pull-request"
	paletteCopyPath        paletteCommandID = "copy-path"
	paletteRefresh         paletteCommandID = "refresh"
	paletteSearch          paletteCommandID = "search"
	paletteJumpRoot        paletteCommandID = "jump-root"
	paletteJumpActive      paletteCommandID = "jump-active"
	paletteJumpTop         paletteCommandID = "jump-top"
	paletteJumpBottom      paletteCommandID = "jump-bottom"
	paletteCycleFilter     paletteCommandID = "cycle-filter"
	paletteFilterAll       paletteCommandID = "filter-all"
	paletteFilterModified  paletteCommandID = "filter-modified"
	paletteFilterPrunable  paletteCommandID = "filter-prunable"
	paletteFilterLocked    paletteCommandID = "filter-locked"
	paletteFilterDetached  paletteCommandID = "filter-detached"
	paletteOpenConfig      paletteCommandID = "open-config"
	paletteToggleHelp      paletteCommandID = "toggle-help"
	paletteQuit            paletteCommandID = "quit"
)

type paletteCommand struct {
	id       paletteCommandID
	title    string
	shortcut string
	keywords string
}

var paletteCommands = []paletteCommand{
	{id: paletteGoSelected, title: "Go to selected worktree", shortcut: "Enter", keywords: "cd switch"},
	{id: paletteCreate, title: "Create worktree", shortcut: "n", keywords: "new branch"},
	{id: paletteDelete, title: "Delete selected worktree", shortcut: "d", keywords: "remove prune branch"},
	{id: paletteOpenEditor, title: "Open in editor", shortcut: "o", keywords: "code cursor"},
	{id: paletteOpenPullRequest, title: "Open PR or branch page", shortcut: "p", keywords: "github browser"},
	{id: paletteCopyPath, title: "Copy absolute path", shortcut: "y", keywords: "clipboard"},
	{id: paletteRefresh, title: "Fetch and reload", shortcut: "r", keywords: "refresh prune"},
	{id: paletteSearch, title: "Search branches", shortcut: "s", keywords: "find filter"},
	{id: paletteJumpRoot, title: "Jump to root repository", shortcut: "h", keywords: "main"},
	{id: paletteJumpActive, title: "Jump to active worktree", shortcut: "a", keywords: "current"},
	{id: paletteJumpTop, title: "Jump to top", shortcut: "g", keywords: "first"},
	{id: paletteJumpBottom, title: "Jump to bottom", shortcut: "G", keywords: "last"},
	{id: paletteCycleFilter, title: "Cycle filter", shortcut: "Tab", keywords: "all modified prunable locked detached"},
	{id: paletteFilterAll, title: "Filter: all", keywords: "show everything"},
	{id: paletteFilterModified, title: "Filter: modified", keywords: "dirty changes"},
	{id: paletteFilterPrunable, title: "Filter: prunable", keywords: "missing stale prune"},
	{id: paletteFilterLocked, title: "Filter: locked", keywords: "lock"},
	{id: paletteFilterDetached, title: "Filter: detached", keywords: "head sha"},
	{id: paletteOpenConfig, title: "Open config", shortcut: "ctrl+o", keywords: "settings toml"},
	{id: paletteToggleHelp, title: "Toggle help", shortcut: "?", keywords: "keys shortcuts"},
	{id: paletteQuit, title: "Quit", shortcut: "q", keywords: "exit"},
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
	enrichmentContext, enrichmentCancel := context.WithCancel(context.Background())
	return Model{
		state:             state,
		config:            config,
		runner:            runner,
		width:             100,
		height:            30,
		search:            search,
		showPR:            state.Repo.RemoteConfigured,
		prLoading:         state.Repo.RemoteConfigured,
		lastRefreshAt:     time.Now(),
		enrichmentID:      1,
		enrichmentContext: enrichmentContext,
		enrichmentCancel:  enrichmentCancel,
	}
}

func (model Model) Init() tea.Cmd {
	ctx := model.enrichmentContext
	if ctx == nil {
		ctx = context.Background()
	}
	return tea.Batch(model.enrichmentCommands(ctx, model.enrichmentID, false), clockTickCmd(model.lastRefreshAt), autoRefreshTickCmd())
}

func (model Model) SelectedPath() string {
	return model.selectedPath
}

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = message.Width
		model.height = message.Height
		return model, model.diskUsageCommand(context.Background(), time.Now(), model.enrichmentID)
	case localMetadataLoadedMsg:
		if message.id != model.enrichmentID || message.repoRoot != model.state.Repo.Root || message.err != nil {
			return model, nil
		}
		anchor := model.selectionAnchor()
		model.state = message.state
		model.applyCachedPullRequests()
		model.restoreSelection(anchor)
		return model, model.diskUsageCommand(context.Background(), time.Now(), model.enrichmentID)
	case prLoadedMsg:
		if message.id != model.enrichmentID || message.repoRoot != model.state.Repo.Root {
			return model, nil
		}
		model.prLoading = false
		model.prLastCheckedAt = message.checkedAt
		if message.enabled {
			model.showPR = true
			model.prCache = message.pullRequests
			model.prCacheRepoRoot = model.state.Repo.Root
			model.state.Rows = github.AttachPullRequests(model.state.Rows, message.pullRequests)
		} else if len(model.prCache) > 0 && model.prCacheRepoRoot == model.state.Repo.Root {
			model.showPR = true
			model.state.Rows = github.AttachPullRequests(model.state.Rows, model.prCache)
		}
		return model, nil
	case sizesLoadedMsg:
		if message.id != model.enrichmentID || message.repoRoot != model.state.Repo.Root {
			return model, nil
		}
		pathIndexes := make(map[string]int, len(model.state.Rows))
		for index, row := range model.state.Rows {
			pathIndexes[row.Path] = index
		}
		for path, size := range message.gitSizes {
			if index, ok := pathIndexes[path]; ok {
				model.state.Rows[index].GitSizeBytes = size
				model.state.Rows[index].GitSizeLoaded = true
			}
		}
		for path, size := range message.fullSizes {
			if index, ok := pathIndexes[path]; ok {
				model.state.Rows[index].FullSizeBytes = size
				model.state.Rows[index].FullSizeLoaded = true
				model.state.Rows[index].SizeBytes = size
				model.state.Rows[index].SizeLoaded = true
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
		model.showPR = model.state.Repo.RemoteConfigured
		model.prLoading = false
		model.applyCachedPullRequests()
		model.lastRefreshAt = message.completedAt
		model.restoreSelection(anchor)
		model, enrichmentCmd := model.startEnrichment(!message.automatic)
		if message.automatic {
			return model, enrichmentCmd
		}
		model, flashCmd := model.setFlash("reloaded")
		return model, tea.Batch(enrichmentCmd, flashCmd)
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
		model.showPR = model.state.Repo.RemoteConfigured
		model.prLoading = false
		model.applyCachedPullRequests()
		model.restoreSelection(anchor)
		model, flashCmd := model.setFlash("deleted worktree")
		model, enrichmentCmd := model.startEnrichment(true)
		return model, tea.Batch(enrichmentCmd, flashCmd)
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
		return model, clockTickCmd(model.lastRefreshAt)
	case tea.KeyMsg:
		if model.createDialog != nil {
			return model.updateCreate(message)
		}
		if model.deleteDialog != nil {
			return model.updateDelete(message)
		}
		if model.paletteDialog != nil {
			return model.updatePalette(message)
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
		model = model.cancelEnrichment()
		return model, tea.Quit
	case "ctrl+p":
		return model.openPalette()
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
		return model, nil
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
	case "r":
		model.loading = "fetching…"
		model = model.cancelEnrichment()
		model.refreshID++
		model.refreshInFlight = true
		return model, reloadCmd(model.reloadCwd(), model.config, model.runner, model.state.Repo, true, false, model.refreshID)
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
	model = model.cancelEnrichment()
	refreshCmd := reloadCmd(model.reloadCwd(), model.config, model.runner, model.state.Repo, false, true, model.refreshID)
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
		model.deleteDialog == nil &&
		model.paletteDialog == nil
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

func (model Model) openPalette() (Model, tea.Cmd) {
	input := textinput.New()
	input.Prompt = "> "
	input.CharLimit = 200
	input.Width = 36
	input.Cursor.Style = flashStyle
	focusCmd := input.Focus()
	model.help = false
	model.paletteDialog = &paletteDialog{input: input}
	return model, focusCmd
}

func (model Model) updatePalette(message tea.KeyMsg) (Model, tea.Cmd) {
	dialog := model.paletteDialog
	switch message.String() {
	case "esc", "ctrl+p":
		model.paletteDialog = nil
		return model, nil
	case "up", "k":
		dialog.selected = clamp(dialog.selected-1, 0, max(0, len(model.matchingPaletteCommands())-1))
		return model, nil
	case "down", "j":
		dialog.selected = clamp(dialog.selected+1, 0, max(0, len(model.matchingPaletteCommands())-1))
		return model, nil
	case "enter":
		commands := model.matchingPaletteCommands()
		if len(commands) == 0 {
			return model, nil
		}
		command := commands[clamp(dialog.selected, 0, len(commands)-1)]
		model.paletteDialog = nil
		return model.executePaletteCommand(command.id)
	}
	previousValue := dialog.input.Value()
	var cmd tea.Cmd
	dialog.input, cmd = dialog.input.Update(message)
	if dialog.input.Value() != previousValue {
		dialog.selected = 0
	}
	dialog.selected = clamp(dialog.selected, 0, max(0, len(model.matchingPaletteCommands())-1))
	return model, cmd
}

func (model Model) executePaletteCommand(id paletteCommandID) (Model, tea.Cmd) {
	switch id {
	case paletteGoSelected:
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
	case paletteCreate:
		return model.openCreate()
	case paletteDelete:
		return model.openDelete()
	case paletteOpenEditor:
		row, ok := model.selectedRow()
		if !ok || row.Prunable {
			return model.setFlash("cannot open this worktree")
		}
		return model, openEditorCmd(model.config.Editor, row.Path)
	case paletteOpenPullRequest:
		row, ok := model.selectedRow()
		if !ok {
			return model, nil
		}
		return model, func() tea.Msg {
			err := github.OpenPullRequestOrBranch(context.Background(), model.state.Repo.Root, row, model.runner)
			return actionMsg{text: "opened", err: err}
		}
	case paletteCopyPath:
		row, ok := model.selectedRow()
		if !ok {
			return model, nil
		}
		return model, copyPathCmd(row.Path)
	case paletteRefresh:
		model.loading = "fetching…"
		model = model.cancelEnrichment()
		model.refreshID++
		model.refreshInFlight = true
		return model, reloadCmd(model.reloadCwd(), model.config, model.runner, model.state.Repo, true, false, model.refreshID)
	case paletteSearch:
		model.searching = true
		return model, model.search.Focus()
	case paletteJumpRoot:
		model.selectMatching(func(row gitdata.Worktree) bool { return row.IsMain })
	case paletteJumpActive:
		model.selectMatching(func(row gitdata.Worktree) bool { return row.IsActive })
	case paletteJumpTop:
		model.selected = 0
	case paletteJumpBottom:
		model.selected = max(0, len(model.visibleIndexes())-1)
	case paletteCycleFilter:
		model.cycleFilter()
	case paletteFilterAll:
		model.setFilter(filterAll)
	case paletteFilterModified:
		model.setFilter(filterModified)
	case paletteFilterPrunable:
		model.setFilter(filterPrunable)
	case paletteFilterLocked:
		model.setFilter(filterLocked)
	case paletteFilterDetached:
		model.setFilter(filterDetached)
	case paletteOpenConfig:
		return model, openConfigCmd(model.config.Editor, model.config)
	case paletteToggleHelp:
		model.help = !model.help
	case paletteQuit:
		return model, tea.Quit
	}
	return model, nil
}

func (model Model) matchingPaletteCommands() []paletteCommand {
	if model.paletteDialog == nil {
		return paletteCommands
	}
	query := strings.TrimSpace(model.paletteDialog.input.Value())
	if query == "" {
		return paletteCommands
	}
	matches := make([]paletteCommand, 0, len(paletteCommands))
	for _, command := range paletteCommands {
		haystack := command.title + " " + command.shortcut + " " + command.keywords
		if fuzzyMatch(haystack, query) {
			matches = append(matches, command)
		}
	}
	return matches
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
	model.help = false
	model.paletteDialog = nil
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
	dialog := deleteDialog{
		stage:          deleteStageOptions,
		deleteWorktree: deleteWorktreeDefault(row),
		deleteBranch:   deleteBranchDefault(row),
		forceWorktree:  !row.Status.Clean(),
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
	model.deleteDialog = &dialog
	return model, nil
}

func (model Model) updateDelete(message tea.KeyMsg) (Model, tea.Cmd) {
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
		row, ok := model.selectedRow()
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
	case "enter":
		row, ok := model.selectedRow()
		if !ok {
			model.deleteDialog = nil
			return model, nil
		}
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
		model.loading = "deleting…"
		return model, func() tea.Msg {
			err := deleteRow(context.Background(), model.state.Repo, row, *dialog, model.runner)
			if err != nil {
				return deleteMsg{err: err}
			}
			state, err := gitdata.LoadSkeleton(context.Background(), model.state.Repo.ActiveWorktree, model.config, model.runner)
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
		return nil
	}
	if dialog.deleteWorktree {
		if err := gitdata.RemoveWorktree(ctx, repo.Root, row.Path, dialog.forceWorktree, runner); err != nil {
			return err
		}
	}
	if dialog.deleteWorktree && dialog.deleteBranch && row.Branch != "" && !row.Detached {
		if err := gitdata.DeleteBranch(ctx, repo.Root, row.Branch, !row.BranchMergedToMain, runner); err != nil {
			return err
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
	row, ok := model.selectedRow()
	return ok && deleteBranchAvailable(row)
}

type viewSnapshot struct {
	rows        []gitdata.Worktree
	visibleRows []gitdata.Worktree
	selectedRow gitdata.Worktree
	hasSelected bool
	detail      string
	start       int
}

func (model Model) View() string {
	now := time.Now()
	width := viewWidth(model)
	outerWidth := max(4, width)
	contentWidth := max(1, outerWidth-4)
	panelWidth := max(4, contentWidth)
	panelContentWidth := max(1, panelWidth-2)
	rowCount := len(model.state.Rows)
	detail := ""
	var detailRow gitdata.Worktree
	lines := []string{"Loading worktrees…"}
	if model.localMetadataReady() {
		snapshot := model.viewSnapshot(now, panelContentWidth)
		rowCount = len(snapshot.rows)
		detail = snapshot.detail
		detailRow = snapshot.selectedRow
		table := listview.RenderRows(snapshot.visibleRows, listview.Options{
			Width:             panelContentWidth,
			Color:             true,
			Hyperlinks:        true,
			ShowHeader:        true,
			ShowPR:            model.showPR,
			Pending:           listview.LoadingPlaceholder,
			PRPending:         model.pullRequestsPending(),
			HighlightSelected: true,
			SelectedIndex:     model.selected - snapshot.start,
		}, now)
		lines = strings.Split(table, "\n")
		if len(snapshot.rows) == 0 {
			lines = []string{"No worktrees"}
		}
	}
	parts := []string{
		model.appTopLine(rowCount, outerWidth),
		model.wrapOuter(sectionBoxWithFooter("Worktrees", lines, model.listFooterHints(), panelWidth), outerWidth),
	}
	if detail != "" {
		parts = append(parts, model.wrapOuter(sectionBoxWithFooter(detailTitle(detailRow), strings.Split(detail, "\n"), detailFooterHints(panelWidth), panelWidth), outerWidth))
	}
	if model.flash != "" {
		parts = append(parts, model.wrapOuter(model.flashLineAtWidth(panelWidth), outerWidth))
	}
	parts = append(parts, model.appBottomLine(outerWidth))
	output := strings.Join(parts, "\n")
	overlayHeight := lineCount(output)
	if model.help {
		output = centeredOverlay(output, model.renderHelpAtWidth(helpDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.paletteDialog != nil {
		output = centeredOverlay(output, model.renderPaletteAtWidth(paletteDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.deleteDialog != nil {
		output = centeredOverlay(output, model.renderDeleteAtWidth(deleteDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.createDialog != nil {
		output = centeredOverlay(output, model.renderCreateAtWidth(createDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	return model.frame(output)
}

func (model Model) localMetadataReady() bool {
	for _, row := range model.state.Rows {
		if !row.LocalMetadataLoaded {
			return false
		}
	}
	return true
}

func (model Model) viewSnapshot(now time.Time, panelContentWidth int) viewSnapshot {
	indexes := model.visibleIndexes()
	rows := make([]gitdata.Worktree, 0, len(indexes))
	for _, index := range indexes {
		rows = append(rows, model.state.Rows[index])
	}
	snapshot := viewSnapshot{rows: rows}
	if len(indexes) > 0 && model.selected >= 0 && model.selected < len(indexes) {
		snapshot.selectedRow = model.state.Rows[indexes[model.selected]]
		snapshot.hasSelected = true
		snapshot.detail = model.detailPanelAtWidth(snapshot.selectedRow, now, panelContentWidth)
	}
	availableHeight := model.availableTableHeightForDetail(snapshot.detail)
	if model.selected >= availableHeight {
		snapshot.start = model.selected - availableHeight + 1
	}
	if snapshot.start > len(rows) {
		snapshot.start = len(rows)
	}
	end := min(len(rows), snapshot.start+availableHeight)
	if snapshot.start < end {
		snapshot.visibleRows = rows[snapshot.start:end]
	}
	return snapshot
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
		model.inspectorFieldAtWidth("Size", sizeText(row), inspectorValueStyle, width),
	}
	lines = append(lines,
		model.inspectorRenderedFieldAtWidth("Remote", remoteText(row), func(value string) string {
			return syncStyle(row).Render(value)
		}, width),
		model.inspectorRenderedFieldAtWidth("Main", model.mainText(row), renderMainValue, width),
		model.inspectorRenderedFieldAtWidth("Commit", commitText(row, now), renderCommitValue, width),
		model.inspectorFieldAtWidth("PR", model.prText(row), inspectorValueStyle, width),
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
	return model.selectedInspectorAtWidth(row, now, width)
}

func detailTitle(row gitdata.Worktree) string {
	context := selectionContextTitle(row)
	if context == "" {
		return "Details"
	}
	return "Details · " + context
}

func renderSectionTitle(title string, width int) string {
	name, detail, found := strings.Cut(title, " · ")
	if !found {
		return panelTitleStyle.Render(truncatePlain(title, width))
	}
	separator := titleSeparatorStyle.Render(" · ")
	nameWidth := runewidth.StringWidth(name)
	detailWidth := max(0, width-nameWidth-lipgloss.Width(separator))
	if detailWidth <= 0 {
		return panelTitleStyle.Render(truncatePlain(name, width))
	}
	return panelTitleStyle.Render(name) + separator + titleRepoStyle.Render(truncatePlain(detail, detailWidth))
}

func detailFooterHints(width int) string {
	actionParts := []string{"↵ go", "o editor", "d delete", "y abs path", "p PR"}
	availableWidth := max(0, width-5)
	return joinPartsWithin(actionParts, availableWidth)
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

func selectionContextTitle(row gitdata.Worktree) string {
	switch {
	case row.IsActive && row.IsMain:
		return "Current root repository"
	case row.IsActive:
		return "Current worktree"
	case row.IsMain:
		return "Root repository"
	default:
		return ""
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

func (model Model) prText(row gitdata.Worktree) string {
	if row.PR == nil {
		if model.pullRequestsPending() {
			return listview.LoadingPlaceholder
		}
		return "none"
	}
	text := row.PR.Text()
	if text == "" {
		return "none"
	}
	return text
}

func (model Model) deletePRText(row gitdata.Worktree) string {
	if row.PR != nil {
		if text := row.PR.Text(); text != "" {
			return text
		}
	}
	if model.pullRequestsPending() {
		return listview.LoadingPlaceholder
	}
	if model.showPR {
		return "none"
	}
	return "unknown"
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
	case row.Locked:
		return "blocked, locked worktree"
	case !row.Status.Clean():
		return "allowed with force, dirty worktree"
	case deleteBranchDefault(row):
		return "allowed, branch deletion checked"
	default:
		return "allowed, branch deletion optional"
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

func sizeText(row gitdata.Worktree) string {
	parts := []string{}
	if row.GitSizeLoaded {
		parts = append(parts, "git "+formatByteSize(row.GitSizeBytes))
	} else {
		parts = append(parts, "git "+listview.LoadingPlaceholder)
	}
	if row.FullSizeLoaded {
		parts = append(parts, "full "+formatByteSize(row.FullSizeBytes))
	} else {
		parts = append(parts, "full "+listview.LoadingPlaceholder)
	}
	return strings.Join(parts, ", ")
}

func formatByteSize(bytes int64) string {
	units := []string{"B", "K", "M", "G", "T"}
	value := float64(bytes)
	unitIndex := 0
	for value >= 1024 && unitIndex < len(units)-1 {
		value /= 1024
		unitIndex++
	}
	if unitIndex == 0 {
		return fmt.Sprintf("%dB", bytes)
	}
	if value < 10 {
		return fmt.Sprintf("%.1f%s", value, units[unitIndex])
	}
	return fmt.Sprintf("%.0f%s", value, units[unitIndex])
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
	return bottomBorderLine(width, appBorderStyle, borderControls{
		parts:     model.statusLeftParts(),
		hasStatus: model.loading != "",
	}, borderControls{})
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
		labelWidth := max(0, innerWidth-3)
		label = " " + renderSectionTitle(title, labelWidth) + " "
	}
	labelWidth := lipgloss.Width(label)
	ruleWidth := innerWidth - 1 - labelWidth
	if ruleWidth < 0 {
		ruleWidth = 0
	}
	return panelBorderStyle.Render("╭─") + panelTitleStyle.Render(label) + panelBorderStyle.Render(strings.Repeat("─", ruleWidth)+"╮")
}

func sectionBottomLine(footer string, width int) string {
	return bottomBorderLine(width, panelBorderStyle, borderControls{parts: hintParts(footer)}, borderControls{})
}

type borderControls struct {
	parts     []string
	text      string
	hasStatus bool
}

func bottomBorderLine(width int, style lipgloss.Style, left, right borderControls) string {
	if width <= 0 {
		return ""
	}
	if width < 4 {
		return style.Render(strings.Repeat("─", width))
	}
	contentLimit := max(0, width-6)
	leftText := renderBorderControls(left, contentLimit)
	rightText := renderBorderControls(right, contentLimit)
	if leftText == "" && rightText == "" {
		return style.Render("╰" + strings.Repeat("─", max(0, width-2)) + "╯")
	}
	if leftText != "" && rightText != "" {
		ruleWidth := width - lipgloss.Width(leftText) - lipgloss.Width(rightText) - 8
		if ruleWidth >= 1 {
			return style.Render("╰─ ") + leftText + style.Render(" "+strings.Repeat("─", ruleWidth)+" ") + rightText + style.Render(" ─╯")
		}
		rightText = ""
	}
	if leftText != "" {
		ruleWidth := max(1, width-lipgloss.Width(leftText)-5)
		return style.Render("╰─ ") + leftText + style.Render(" "+strings.Repeat("─", ruleWidth)+"╯")
	}
	ruleWidth := max(1, width-lipgloss.Width(rightText)-5)
	return style.Render("╰"+strings.Repeat("─", ruleWidth)+" ") + rightText + style.Render(" ─╯")
}

func renderBorderControls(controls borderControls, width int) string {
	if width <= 0 {
		return ""
	}
	if len(controls.parts) > 0 {
		text := joinPartsWithin(controls.parts, width)
		if text == "" {
			return ""
		}
		return colorKeyHints(text, controls.hasStatus)
	}
	if lipgloss.Width(controls.text) == 0 {
		return ""
	}
	return truncateStyled(controls.text, width)
}

func hintParts(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, " · ")
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
	return colorKeyHints(leftText, model.loading != "" && strings.Contains(leftText, model.loading))
}

func (model Model) statusLeftParts() []string {
	if model.loading != "" {
		return []string{model.loading}
	}
	return nil
}

func (model Model) listFooterHints() string {
	if model.searching {
		return "search " + model.search.Value() + "▌ · Esc clear · Tab filter: " + model.filter.label()
	}
	if model.filter != filterAll {
		return "h root · a active · Tab filter: " + model.filter.label() + " · Esc clear filter · s search"
	}
	return "h root · a active · Tab filter: " + model.filter.label() + " · s search"
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
	if text == "" {
		return ""
	}
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
	title := titleNameStyle.Render(appTitle)
	separator := titleSeparatorStyle.Render(" · ")
	titleAndSeparatorWidth := runewidth.StringWidth(appTitle) + lipgloss.Width(separator)
	staticWidth := titleAndSeparatorWidth + runewidth.StringWidth("  ") + runewidth.StringWidth(count)
	if rootBranch != "" {
		staticWidth += runewidth.StringWidth("  root: ")
	}
	repoWidth := width - staticWidth
	if repoWidth < 4 {
		compactWidth := width - titleAndSeparatorWidth - runewidth.StringWidth(count)
		if compactWidth >= 0 {
			meta := titleMetaStyle.Render(count)
			return title + separator + meta
		}
		repoWidth = width - titleAndSeparatorWidth
		if repoWidth <= 0 {
			return titleNameStyle.Render(truncatePlain(appTitle, width))
		}
		return title + separator + titleRepoStyle.Render(truncatePlain(repoName, repoWidth))
	}
	repoName = truncatePlain(repoName, repoWidth)
	repo := titleRepoStyle.Render(repoName)
	meta := titleMetaStyle.Render(count)
	if rootBranch == "" {
		return title + separator + repo + "  " + meta
	}
	rootWidth := width - lipgloss.Width(title+separator+repo+"  "+meta+"  "+titleMetaStyle.Render("root: "))
	if rootWidth < 3 {
		return title + separator + repo + "  " + meta
	}
	return title + separator + repo + "  " + meta + "  " + titleMetaStyle.Render("root: ") + titleRepoStyle.Render(truncatePlain(rootBranch, rootWidth))
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

func clockTickCmd(lastRefreshAt time.Time) tea.Cmd {
	return tea.Tick(nextClockTickDelay(lastRefreshAt, time.Now()), func(time.Time) tea.Msg {
		return clockTickMsg{}
	})
}

func nextClockTickDelay(lastRefreshAt, now time.Time) time.Duration {
	if lastRefreshAt.IsZero() {
		return time.Minute
	}
	elapsed := now.Sub(lastRefreshAt)
	if elapsed < 0 || elapsed < time.Minute {
		return clockTickInterval
	}
	minutes := int(elapsed / time.Minute)
	nextBoundary := lastRefreshAt.Add(time.Duration(minutes+1) * time.Minute)
	delay := nextBoundary.Sub(now)
	if delay <= 0 {
		return time.Minute
	}
	return delay
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

func (model *Model) setFilter(filter worktreeFilter) {
	anchor := model.selectionAnchor()
	model.filter = filter
	if !model.restoreSelection(anchor) && len(model.visibleIndexes()) > 0 {
		model.selected = 0
	}
}

func (model *Model) applyCachedPullRequests() {
	if len(model.prCache) == 0 {
		return
	}
	if model.prCacheRepoRoot != model.state.Repo.Root {
		model.prCache = nil
		model.prCacheRepoRoot = ""
		model.showPR = model.state.Repo.RemoteConfigured
		return
	}
	model.showPR = true
	model.state.Rows = github.AttachPullRequests(model.state.Rows, model.prCache)
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

func (model Model) tableContentWidth() int {
	width := viewWidth(model)
	outerWidth := max(4, width)
	contentWidth := max(1, outerWidth-4)
	panelWidth := max(4, contentWidth)
	return max(1, panelWidth-2)
}

type helpEntryKind int

const (
	helpEntryKey helpEntryKind = iota
	helpEntryRoot
	helpEntryActive
	helpEntryLocked
	helpEntryPrunable
	helpEntryDetached
	helpEntryClean
	helpEntryStaged
	helpEntryModified
	helpEntryUntracked
	helpEntryRemote
	helpEntryPullRequest
	helpEntryApproved
	helpEntryMerged
	helpEntryClosed
	helpEntryRunning
	helpEntryError
)

type helpEntry struct {
	lead        string
	description string
	kind        helpEntryKind
}

type helpSection struct {
	title   string
	entries []helpEntry
}

func (model Model) renderHelpAtWidth(width int) string {
	contentWidth := max(1, width-4)
	lines := helpLinesAtWidth(contentWidth)
	return dialogBox("Help", lines, colorKeyHints("Esc close", false), width)
}

func helpLinesAtWidth(contentWidth int) []string {
	columns := helpColumnsForWidth(contentWidth)
	if len(columns) == 1 {
		return renderHelpSections(columns[0], contentWidth)
	}
	gap := "  "
	gapWidth := runewidth.StringWidth(gap) * (len(columns) - 1)
	columnWidth := max(1, (contentWidth-gapWidth)/len(columns))
	renderedColumns := make([][]string, 0, len(columns))
	height := 0
	for _, column := range columns {
		rendered := renderHelpSections(column, columnWidth)
		renderedColumns = append(renderedColumns, rendered)
		height = max(height, len(rendered))
	}
	lines := make([]string, 0, height)
	for row := 0; row < height; row++ {
		parts := make([]string, 0, len(renderedColumns))
		for _, column := range renderedColumns {
			line := ""
			if row < len(column) {
				line = column[row]
			}
			parts = append(parts, padStyled(line, columnWidth))
		}
		lines = append(lines, strings.Join(parts, gap))
	}
	return lines
}

func helpColumnsForWidth(contentWidth int) [][]helpSection {
	keySections := helpKeySections()
	legendSections := helpLegendSections()
	if contentWidth >= 62 {
		return [][]helpSection{
			{keySections[0], legendSections[0]},
			{keySections[1], legendSections[1]},
			{keySections[2], legendSections[2]},
		}
	}
	if contentWidth >= 42 {
		return [][]helpSection{
			{keySections[0], keySections[1], keySections[2]},
			{legendSections[0], legendSections[1], legendSections[2]},
		}
	}
	sections := make([]helpSection, 0, len(keySections)+len(legendSections))
	sections = append(sections, keySections...)
	sections = append(sections, legendSections...)
	return [][]helpSection{sections}
}

func helpKeySections() []helpSection {
	return []helpSection{
		{
			title: "Global",
			entries: []helpEntry{
				{lead: "n", description: "new worktree", kind: helpEntryKey},
				{lead: "r", description: "refresh", kind: helpEntryKey},
				{lead: "ctrl+p", description: "commands", kind: helpEntryKey},
				{lead: "?", description: "help", kind: helpEntryKey},
				{lead: "Esc", description: "close/cancel", kind: helpEntryKey},
				{lead: "q", description: "quit", kind: helpEntryKey},
			},
		},
		{
			title: "Worktree List",
			entries: []helpEntry{
				{lead: "↑/↓ k/j", description: "move", kind: helpEntryKey},
				{lead: "g/G", description: "top/bottom", kind: helpEntryKey},
				{lead: "h", description: "root", kind: helpEntryKey},
				{lead: "a", description: "active", kind: helpEntryKey},
				{lead: "Tab", description: "filter", kind: helpEntryKey},
				{lead: "s", description: "search", kind: helpEntryKey},
			},
		},
		{
			title: "Worktree Detail",
			entries: []helpEntry{
				{lead: "Enter", description: "go", kind: helpEntryKey},
				{lead: "o", description: "editor", kind: helpEntryKey},
				{lead: "d", description: "delete", kind: helpEntryKey},
				{lead: "y", description: "abs path", kind: helpEntryKey},
				{lead: "p", description: "PR/branch", kind: helpEntryKey},
			},
		},
	}
}

func helpLegendSections() []helpSection {
	return []helpSection{
		{
			title: "Worktree Markers",
			entries: []helpEntry{
				{lead: "⌂", description: "root", kind: helpEntryRoot},
				{lead: "!", description: "locked", kind: helpEntryLocked},
				{lead: "×", description: "prunable", kind: helpEntryPrunable},
				{lead: "bold", description: "active branch", kind: helpEntryActive},
				{lead: "detached", description: "HEAD", kind: helpEntryDetached},
			},
		},
		{
			title: "Git Status",
			entries: []helpEntry{
				{lead: "✓", description: "clean", kind: helpEntryClean},
				{lead: "+", description: "staged", kind: helpEntryStaged},
				{lead: "~", description: "modified", kind: helpEntryModified},
				{lead: "?", description: "untracked", kind: helpEntryUntracked},
				{lead: "remote ✓", description: "synced", kind: helpEntryRemote},
				{lead: "remote -", description: "none", kind: helpEntryRemote},
				{lead: "remote gone", description: "deleted", kind: helpEntryRemote},
			},
		},
		{
			title: "Pull Requests",
			entries: []helpEntry{
				{lead: "◌", description: "draft", kind: helpEntryPullRequest},
				{lead: "○", description: "ready/open", kind: helpEntryPullRequest},
				{lead: "◆", description: "approved", kind: helpEntryApproved},
				{lead: "⬡", description: "merged", kind: helpEntryMerged},
				{lead: "✕", description: "closed", kind: helpEntryClosed},
				{lead: "✓", description: "CI passed", kind: helpEntryClean},
				{lead: "✗", description: "CI error", kind: helpEntryError},
				{lead: "●", description: "CI running", kind: helpEntryRunning},
			},
		},
	}
}

func renderHelpSections(sections []helpSection, width int) []string {
	lines := make([]string, 0)
	for sectionIndex, section := range sections {
		if sectionIndex > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, truncateStyled(helpCategoryStyle.Render(section.title), width))
		for _, entry := range section.entries {
			lines = append(lines, truncateStyled(renderHelpEntry(entry), width))
		}
	}
	return lines
}

func renderHelpEntry(entry helpEntry) string {
	return helpEntryStyle(entry.kind).Render(entry.lead) + hintStyle.Render(" "+entry.description)
}

func helpEntryStyle(kind helpEntryKind) lipgloss.Style {
	switch kind {
	case helpEntryRoot, helpEntryDetached, helpEntryPullRequest, helpEntryMerged:
		return inspectorCommitStyle
	case helpEntryActive:
		return inspectorValueStyle.Bold(true)
	case helpEntryLocked, helpEntryPrunable, helpEntryModified, helpEntryClosed, helpEntryRunning, helpEntryError:
		return inspectorWarnStyle
	case helpEntryClean, helpEntryStaged, helpEntryApproved:
		return inspectorCleanStyle
	case helpEntryUntracked:
		return inspectorCommitStyle
	case helpEntryRemote:
		return keyStyle
	default:
		return keyStyle
	}
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

func (model Model) renderDeleteAtWidth(width int) string {
	row, _ := model.selectedRow()
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
		lines = append(lines, "", lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(dialog.error))
	}
	for index, line := range lines {
		lines[index] = truncateStyled(line, contentWidth)
	}
	return dialogBox("Delete worktree", lines, deleteDialogHintsAtWidth(bottom, width-6), width)
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

func (model Model) renderPaletteAtWidth(width int) string {
	dialog := model.paletteDialog
	contentWidth := max(1, width-4)
	input := dialog.input
	input.Width = max(1, contentWidth-runewidth.StringWidth(input.Prompt)-1)
	lines := []string{input.View()}
	commands := model.matchingPaletteCommands()
	if len(commands) == 0 {
		lines = append(lines, hintStyle.Render("No commands"))
	} else {
		limit := min(8, len(commands))
		selected := clamp(dialog.selected, 0, len(commands)-1)
		for index := 0; index < limit; index++ {
			command := commands[index]
			prefix := "  "
			style := inspectorValueStyle
			if index == selected {
				prefix = "› "
				style = paletteSelectedStyle
			}
			label := command.title
			if command.shortcut != "" {
				label += "  " + hintStyle.Render(command.shortcut)
			}
			line := truncateStyled(prefix+label, contentWidth)
			if index == selected {
				line = style.Render(padStyled(line, contentWidth))
			}
			lines = append(lines, line)
		}
	}
	return dialogBox("Commands", lines, paletteHintsAtWidth(width-6), width)
}

func paletteHintsAtWidth(width int) string {
	full := colorKeyHints("Enter run · ↑/↓ move · Esc cancel", false)
	if lipgloss.Width(full) <= width {
		return full
	}
	short := colorKeyHints("Enter · ↑/↓ · Esc", false)
	if lipgloss.Width(short) <= width {
		return short
	}
	return ""
}

func helpDialogWidth(viewWidth int) int {
	return modalWidth(viewWidth, 68)
}

func deleteDialogWidth(viewWidth int) int {
	return modalWidth(viewWidth, 76)
}

func paletteDialogWidth(viewWidth int) int {
	return modalWidth(viewWidth, 72)
}

func modalWidth(viewWidth, maximum int) int {
	if viewWidth <= 0 {
		return maximum
	}
	inset := 8
	if viewWidth < 48 {
		inset = 2
	}
	return max(4, min(maximum, viewWidth-inset))
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
	return bottomBorderLine(width, appBorderStyle, borderControls{text: content}, borderControls{})
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
	haloTop := max(0, top-1)
	haloBottom := min(height, top+len(popupLines)+1)
	haloLeft := max(0, left-1)
	haloRight := min(width, left+popupWidth+1)
	for index := haloTop; index < haloBottom; index++ {
		baseLine := padStyled(lines[index], width)
		leftText := ansi.Cut(baseLine, 0, haloLeft)
		rightText := ansi.Cut(baseLine, haloRight, width)
		lines[index] = padStyled(leftText, haloLeft) + strings.Repeat(" ", max(0, haloRight-haloLeft)) + padStyled(rightText, max(0, width-haloRight))
	}
	for index, line := range popupLines {
		target := top + index
		if target >= len(lines) {
			break
		}
		baseLine := padStyled(lines[target], width)
		leftText := ansi.Cut(baseLine, 0, left)
		rightStart := min(width, left+popupWidth)
		rightText := ansi.Cut(baseLine, rightStart, width)
		lines[target] = padStyled(leftText, left) + padStyled(line, popupWidth) + padStyled(rightText, max(0, width-rightStart))
	}
	return strings.Join(lines, "\n")
}

func (model Model) visibleTableWindow(now time.Time) (int, int) {
	indexes := model.visibleIndexes()
	availableHeight := model.availableTableHeight(now)
	start := 0
	if model.selected >= availableHeight {
		start = model.selected - availableHeight + 1
	}
	if start > len(indexes) {
		start = len(indexes)
	}
	return start, min(len(indexes), start+availableHeight)
}

func (model Model) visibleTableIndexes(now time.Time) []int {
	indexes := model.visibleIndexes()
	start, end := model.visibleTableWindow(now)
	if start >= end {
		return nil
	}
	return indexes[start:end]
}

func (model Model) availableTableHeight(now time.Time) int {
	width := viewWidth(model)
	outerWidth := max(4, width)
	contentWidth := max(1, outerWidth-4)
	panelWidth := max(4, contentWidth)
	panelContentWidth := max(1, panelWidth-2)
	detail := ""
	if row, ok := model.selectedRow(); ok {
		detail = model.detailPanelAtWidth(row, now, panelContentWidth)
	}
	return model.availableTableHeightForDetail(detail)
}

func (model Model) availableTableHeightForDetail(detail string) int {
	detailFixedLines := 0
	if detail != "" {
		detailFixedLines = 2 + lineCount(detail)
	}
	fixedLines := 1 + 2 + 1 + detailFixedLines + 1
	if model.flash != "" {
		fixedLines++
	}
	if model.height <= 0 {
		return 8
	}
	return max(1, model.height-fixedLines)
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

func (model Model) startEnrichment(forcePullRequests bool) (Model, tea.Cmd) {
	model = model.cancelEnrichment()
	model.enrichmentID++
	ctx, cancel := context.WithCancel(context.Background())
	model.enrichmentContext = ctx
	model.enrichmentCancel = cancel
	model.prLoading = model.shouldLoadPullRequests(forcePullRequests, time.Now())
	return model, model.enrichmentCommands(ctx, model.enrichmentID, forcePullRequests)
}

func (model Model) cancelEnrichment() Model {
	if model.enrichmentCancel != nil {
		model.enrichmentCancel()
		model.enrichmentCancel = nil
		model.enrichmentContext = nil
	}
	model.prLoading = false
	return model
}

func (model Model) enrichmentCommands(ctx context.Context, id int, forcePullRequests bool) tea.Cmd {
	commands := []tea.Cmd{}
	repoRoot := model.state.Repo.Root
	if model.needsLocalMetadata() {
		state := model.state
		runner := model.runner
		commands = append(commands, func() tea.Msg {
			enriched, err := gitdata.EnrichLocalMetadata(ctx, state, runner)
			return localMetadataLoadedMsg{state: enriched, err: err, repoRoot: repoRoot, id: id}
		})
	}
	now := time.Now()
	if model.shouldLoadPullRequests(forcePullRequests, now) {
		runner := model.runner
		commands = append(commands, func() tea.Msg {
			prContext, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			pullRequests, enabled := github.LoadPullRequests(prContext, repoRoot, runner)
			return prLoadedMsg{
				pullRequests: pullRequests,
				enabled:      enabled,
				repoRoot:     repoRoot,
				id:           id,
				checkedAt:    time.Now(),
			}
		})
	}
	if diskCommand := model.diskUsageCommand(ctx, now, id); diskCommand != nil {
		commands = append(commands, diskCommand)
	}
	return tea.Batch(commands...)
}

func (model Model) needsLocalMetadata() bool {
	for _, row := range model.state.Rows {
		if !row.LocalMetadataLoaded {
			return true
		}
	}
	return false
}

func (model Model) shouldLoadPullRequests(force bool, now time.Time) bool {
	if !model.state.Repo.RemoteConfigured {
		return false
	}
	if force || model.prLastCheckedAt.IsZero() {
		return true
	}
	return now.Sub(model.prLastCheckedAt) >= prRefreshTTL
}

func (model Model) pullRequestsPending() bool {
	return model.showPR && model.prLoading
}

func (model Model) diskUsageCommand(ctx context.Context, now time.Time, id int) tea.Cmd {
	if !model.localMetadataReady() {
		return nil
	}
	visiblePaths, backgroundPaths := model.diskUsagePaths(now)
	fullPath := ""
	if row, ok := model.selectedRow(); ok && diskUsageFullEligible(row) {
		fullPath = row.Path
	}
	if len(visiblePaths) == 0 && len(backgroundPaths) == 0 && fullPath == "" {
		return nil
	}
	runner := model.runner
	repoRoot := model.state.Repo.Root
	return func() tea.Msg {
		return loadSizesMsg(ctx, runner, repoRoot, id, visiblePaths, backgroundPaths, fullPath)
	}
}

func (model Model) diskUsagePaths(now time.Time) ([]string, []string) {
	if !listview.ShowsGitSizeColumn(model.tableContentWidth()) {
		return nil, nil
	}
	visible := model.visibleTableIndexes(now)
	seen := map[string]bool{}
	visiblePaths := make([]string, 0, len(visible))
	for _, rowIndex := range visible {
		row := model.state.Rows[rowIndex]
		if !diskUsageEligible(row) {
			continue
		}
		seen[row.Path] = true
		visiblePaths = append(visiblePaths, row.Path)
	}
	backgroundPaths := []string{}
	for _, row := range model.state.Rows {
		if !diskUsageEligible(row) || seen[row.Path] {
			continue
		}
		backgroundPaths = append(backgroundPaths, row.Path)
	}
	return visiblePaths, backgroundPaths
}

func diskUsageEligible(row gitdata.Worktree) bool {
	return !row.Prunable && !row.GitSizeLoaded
}

func diskUsageFullEligible(row gitdata.Worktree) bool {
	return !row.Prunable && !row.FullSizeLoaded
}

type sizeJob struct {
	path string
	full bool
}

func loadSizesMsg(ctx context.Context, runner gitdata.Runner, repoRoot string, id int, visiblePaths, backgroundPaths []string, fullPath string) tea.Msg {
	jobs := make([]sizeJob, 0, len(visiblePaths)+len(backgroundPaths)+1)
	for _, path := range visiblePaths {
		jobs = append(jobs, sizeJob{path: path})
	}
	if fullPath != "" {
		jobs = append(jobs, sizeJob{path: fullPath, full: true})
	}
	for _, path := range backgroundPaths {
		jobs = append(jobs, sizeJob{path: path})
	}
	gitSizes := map[string]int64{}
	fullSizes := map[string]int64{}
	if len(jobs) == 0 {
		return sizesLoadedMsg{gitSizes: gitSizes, fullSizes: fullSizes, repoRoot: repoRoot, id: id}
	}
	jobChannel := make(chan sizeJob)
	var mutex sync.Mutex
	var waitGroup sync.WaitGroup
	workerCount := min(2, len(jobs))
	for range workerCount {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for job := range jobChannel {
				if job.full {
					if size, err := gitdata.FullDiskUsage(ctx, job.path); err == nil {
						mutex.Lock()
						fullSizes[job.path] = size
						mutex.Unlock()
					}
					continue
				}
				if size, err := gitdata.GitAwareDiskUsage(ctx, job.path, runner); err == nil {
					mutex.Lock()
					gitSizes[job.path] = size
					mutex.Unlock()
				}
			}
		}()
	}
	for _, job := range jobs {
		select {
		case jobChannel <- job:
		case <-ctx.Done():
			close(jobChannel)
			waitGroup.Wait()
			return sizesLoadedMsg{gitSizes: gitSizes, fullSizes: fullSizes, repoRoot: repoRoot, id: id}
		}
	}
	close(jobChannel)
	waitGroup.Wait()
	return sizesLoadedMsg{gitSizes: gitSizes, fullSizes: fullSizes, repoRoot: repoRoot, id: id}
}

func reloadCmd(cwd string, config config.Config, runner gitdata.Runner, repo gitdata.Repository, fetch, automatic bool, id int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if fetch && repo.RemoteConfigured {
			if err := gitdata.FetchPrune(ctx, repo.Root, runner); err != nil {
				return reloadMsg{id: id, automatic: automatic, completedAt: time.Now(), err: err}
			}
		}
		state, err := gitdata.LoadSkeleton(ctx, cwd, config, runner)
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
