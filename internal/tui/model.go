package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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
	state                  gitdata.State
	config                 config.Config
	repoConfig             config.RepoConfig
	hooksApproved          bool
	runner                 gitdata.Runner
	width                  int
	height                 int
	selected               int
	filter                 worktreeFilter
	showBranches           bool
	searching              bool
	search                 textinput.Model
	help                   bool
	loading                string
	flash                  string
	flashID                int
	showPR                 bool
	prLoading              bool
	prCache                map[string]gitdata.PullRequest
	prCacheRepoRoot        string
	prLastCheckedAt        time.Time
	prCIChecked            map[int]bool
	prReview               map[int]github.PullRequestReview
	prReviewChecked        map[int]bool
	branchGraphChecked     map[string]bool
	selectedPath           string
	createDialog           *createDialog
	checkoutDialog         *checkoutDialog
	branchWorktreeDialog   *branchWorktreeDialog
	deleteDialog           *deleteDialog
	cleanupMergedDialog    *cleanupMergedDialog
	actionCancel           context.CancelFunc
	createInFlight         bool
	deleteInFlight         bool
	deleteID               int
	deleteSpinnerFrame     int
	cleanupMergedInFlight  bool
	cleanupMergedID        int
	cleanupMergedSpinner   int
	paletteDialog          *paletteDialog
	filterDialog           *filterDialog
	pullRequestDialog      *pullRequestCheckoutDialog
	pullRequestDialogID    int
	lastRefreshAt          time.Time
	refreshInFlight        bool
	refreshID              int
	refreshAnchor          selectionAnchor
	refreshProgressVisible bool
	refreshSpinnerFrame    int
	feedback               transientFeedback
	feedbackID             int
	pendingRestore         *pendingBranchRestore
	pendingRestoreBatch    []pendingBranchRestore
	enrichmentID           int
	enrichmentContext      context.Context
	enrichmentCancel       context.CancelFunc
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
	mergedGlyphStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("99")) // purple, like GitHub's merged badge
	branchOnlyDetailStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	keyStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	hintStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	scrollbarArrowStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	scrollbarThumbStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	scrollbarTrackStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpCategoryStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	statusMessageStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	refreshActivityStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	refreshSuccessStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

const (
	autoRefreshInterval      = 30 * time.Second
	clockTickInterval        = time.Second
	destructiveActionTimeout = 10 * time.Minute
	refreshTickInterval      = 80 * time.Millisecond
	successFeedbackTimeout   = 3 * time.Second
	restoreOfferTimeout      = 10 * time.Second
	prRefreshTTL             = 5 * time.Minute
	prFetchTimeout           = 15 * time.Second
	prPerBranchThreshold     = 40
	scrollbarGutterWidth     = 2
	appTitle                 = "Git treehouse"
	successGlyph             = "✓"
)

var refreshSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type createDialog struct {
	input     textinput.Model
	bases     []gitdata.BaseOption
	baseIndex int
	error     string
}

type checkoutDialog struct {
	branch gitdata.Branch
	root   gitdata.Worktree
	stash  bool
	error  string
}

type branchWorktreeDialog struct {
	branch gitdata.Branch
	path   string
	error  string
}

type deleteDialog struct {
	stage            deleteStage
	deleteWorktree   bool
	deleteBranch     bool
	forceWorktree    bool
	runBeforeDelete  bool
	beforeDeleteHook string
	error            string
}

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

type feedbackFrame int

const (
	feedbackFrameWorktrees feedbackFrame = iota
)

type feedbackKind int

const (
	feedbackKindSuccess feedbackKind = iota
)

type feedbackSegment struct {
	text string
	bold bool
}

type transientFeedback struct {
	frame    feedbackFrame
	kind     feedbackKind
	segments []feedbackSegment
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

type filterDialog struct {
	selected int
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

type filterOption struct {
	filter  worktreeFilter
	count   int
	enabled bool
}

type prLoadedMsg struct {
	pullRequests map[string]gitdata.PullRequest
	enabled      bool
	ciIncluded   bool
	repoRoot     string
	id           int
	checkedAt    time.Time
}

type prCILoadedMsg struct {
	ci       map[int]string
	repoRoot string
	id       int
}

type prReviewLoadedMsg struct {
	number   int
	review   github.PullRequestReview
	repoRoot string
	id       int
}

type branchGraphLoadedMsg struct {
	name     string
	graph    gitdata.ContextGraph
	repoRoot string
	id       int
}

type sizesLoadedMsg struct {
	gitSizes   map[string]int64
	fullSizes  map[string]int64
	breakdowns map[string]gitdata.DiskBreakdown
	repoRoot   string
	id         int
}

type localMetadataLoadedMsg struct {
	state    gitdata.State
	err      error
	repoRoot string
	id       int
}

type reloadMsg struct {
	state         gitdata.State
	repoConfig    config.RepoConfig
	hooksApproved bool
	err           error
	id            int
	automatic     bool
	completedAt   time.Time
}

type createMsg struct {
	path     string
	created  bool
	err      error
	warnings []string
}

type checkoutMsg struct {
	path     string
	created  bool
	err      error
	warnings []string
}

type deleteMsg struct {
	state         gitdata.State
	repoConfig    config.RepoConfig
	hooksApproved bool
	reloaded      bool
	err           error
	text          string
	restore       *pendingBranchRestore
	id            int
	completedAt   time.Time
}

type cleanupMergedMsg struct {
	state         gitdata.State
	repoConfig    config.RepoConfig
	hooksApproved bool
	reloaded      bool
	result        cleanupMergedResult
	err           error
	id            int
	completedAt   time.Time
}

type settingsSavedMsg struct {
	err error
}

type pullRequestSummariesLoadedMsg struct {
	summaries []github.PullRequestSummary
	err       error
	id        int
}

type pullRequestSummaryLoadedMsg struct {
	summary github.PullRequestSummary
	err     error
	id      int
}

type pullRequestOpenedMsg struct {
	err error
	id  int
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

type clearFeedbackMsg struct {
	id int
}

type autoRefreshMsg struct{}

type clockTickMsg struct{}

type refreshSpinnerTickMsg struct {
	id int
}

type deleteSpinnerTickMsg struct {
	id int
}

type cleanupMergedSpinnerTickMsg struct {
	id int
}

type pullRequestSpinnerTickMsg struct {
	id int
}

type selectionAnchor struct {
	path   string
	branch string
	head   string
}

type worktreeFilter int

const (
	filterAll worktreeFilter = iota
	filterModified
	filterBranches
	filterMerged
	filterPrunable
	filterLocked
	filterDetached
)

var orderedFilters = []worktreeFilter{
	filterAll,
	filterModified,
	filterBranches,
	filterMerged,
	filterPrunable,
	filterLocked,
	filterDetached,
}

type paletteCommandID string

const (
	paletteGoSelected          paletteCommandID = "go-selected"
	paletteCreate              paletteCommandID = "create"
	paletteDelete              paletteCommandID = "delete"
	paletteOpenEditor          paletteCommandID = "open-editor"
	paletteOpenPullRequest     paletteCommandID = "open-pull-request"
	paletteCheckoutPullRequest paletteCommandID = "checkout-pull-request"
	paletteCleanUpMerged       paletteCommandID = "clean-up-merged"
	paletteCopyPath            paletteCommandID = "copy-path"
	paletteCopyPullRequestURL  paletteCommandID = "copy-pull-request-url"
	paletteRefresh             paletteCommandID = "refresh"
	paletteSearch              paletteCommandID = "search"
	paletteJumpRoot            paletteCommandID = "jump-root"
	paletteJumpActive          paletteCommandID = "jump-active"
	paletteJumpTop             paletteCommandID = "jump-top"
	paletteJumpBottom          paletteCommandID = "jump-bottom"
	paletteCycleFilter         paletteCommandID = "cycle-filter"
	paletteFilterAll           paletteCommandID = "filter-all"
	paletteFilterModified      paletteCommandID = "filter-modified"
	paletteFilterMerged        paletteCommandID = "filter-merged"
	paletteFilterPrunable      paletteCommandID = "filter-prunable"
	paletteFilterLocked        paletteCommandID = "filter-locked"
	paletteFilterDetached      paletteCommandID = "filter-detached"
	paletteOpenConfig          paletteCommandID = "open-config"
	paletteToggleHelp          paletteCommandID = "toggle-help"
	paletteQuit                paletteCommandID = "quit"
)

type paletteCommand struct {
	id       paletteCommandID
	title    string
	shortcut string
	keywords string
}

var paletteCommands = []paletteCommand{
	{id: paletteGoSelected, title: "Go to selected row", shortcut: "Enter", keywords: "cd switch create worktree branch"},
	{id: paletteCreate, title: "Create worktree", shortcut: "n", keywords: "new branch"},
	{id: paletteDelete, title: "Delete selected row", shortcut: "d", keywords: "remove prune branch"},
	{id: paletteOpenEditor, title: "Open in editor", shortcut: "o", keywords: "code cursor"},
	{id: paletteOpenPullRequest, title: "Open PR or branch page", shortcut: "p", keywords: "github browser"},
	{id: paletteCheckoutPullRequest, title: "Checkout PR", keywords: "github pr worktree branch"},
	{id: paletteCopyPath, title: "Copy path or branch name", shortcut: "y", keywords: "clipboard branch path"},
	{id: paletteCopyPullRequestURL, title: "Copy PR URL", keywords: "clipboard pull request link github url"},
	{id: paletteCleanUpMerged, title: "Clean up merged", keywords: "done safe remove delete worktree branch cleanup"},
	{id: paletteRefresh, title: "Fetch and reload", shortcut: "r", keywords: "refresh prune"},
	{id: paletteSearch, title: "Search branches", shortcut: "s", keywords: "find filter"},
	{id: paletteJumpRoot, title: "Jump to root repository", shortcut: "h", keywords: "main"},
	{id: paletteJumpActive, title: "Jump to active worktree", shortcut: "a", keywords: "current"},
	{id: paletteJumpTop, title: "Jump to top", shortcut: "g", keywords: "first"},
	{id: paletteJumpBottom, title: "Jump to bottom", shortcut: "G", keywords: "last"},
	{id: paletteCycleFilter, title: "Open filter picker", shortcut: "Tab", keywords: "all modified branches merged prunable locked detached"},
	{id: paletteFilterAll, title: "Filter: all", keywords: "show everything"},
	{id: paletteFilterModified, title: "Filter: modified", keywords: "dirty changes"},
	{id: paletteFilterMerged, title: "Filter: merged", keywords: "done clean safe remove cleanup"},
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
	case filterBranches:
		return "branches"
	case filterMerged:
		return "merged"
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

func (filter worktreeFilter) matches(row gitdata.Row) bool {
	switch filter {
	case filterModified:
		return row.IsWorktree() && !row.Worktree.Status.Clean()
	case filterBranches:
		return row.IsBranch()
	case filterMerged:
		return mergedFilterMatches(row)
	case filterPrunable:
		return row.IsWorktree() && row.Worktree.Prunable
	case filterLocked:
		return row.IsWorktree() && row.Worktree.Locked
	case filterDetached:
		return row.IsWorktree() && row.Worktree.Detached
	default:
		return true
	}
}

func mergedFilterMatches(row gitdata.Row) bool {
	if row.IsBranch() {
		return cleanupMergedDone(row)
	}
	if !row.IsWorktree() || row.Worktree.IsMain || row.Worktree.Detached {
		return false
	}
	return row.Worktree.Status.Clean() && cleanupMergedDone(row)
}

func prMergedOrClosed(row gitdata.Row) bool {
	pr := row.PullRequest()
	if pr == nil {
		return false
	}
	return strings.EqualFold(pr.State, "⎇") || strings.EqualFold(pr.State, "✕")
}

func New(state gitdata.State, config config.Config, runner gitdata.Runner) Model {
	search := textinput.New()
	search.Prompt = "s "
	search.CharLimit = 200
	search.Width = 40
	enrichmentContext, enrichmentCancel := context.WithCancel(context.Background())
	repoConfig, hooksApproved, _ := loadRepoRuntimeConfig(context.Background(), state.Repo.Root, runner)
	return Model{
		state:             state,
		config:            config,
		repoConfig:        repoConfig,
		hooksApproved:     hooksApproved,
		runner:            runner,
		width:             100,
		height:            30,
		search:            search,
		showBranches:      config.ShowBranches,
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

func (model Model) effectivePathTemplate() string {
	return config.EffectivePathTemplate(model.config, model.repoConfig)
}

func (model Model) withCreateWarnings(warnings []string, command tea.Cmd) (Model, tea.Cmd) {
	if len(warnings) == 0 {
		return model, command
	}
	model, flashCmd := model.setFlash(strings.Join(warnings, "\n"))
	return model, tea.Batch(flashCmd, command)
}

func createdHookError(hook, path string, err error) string {
	return "worktree created at " + path + ", but " + hook + " failed:\n" + err.Error()
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
		// Branches were rebuilt fresh (graphs empty), so re-arm the lazy graph load.
		model.branchGraphChecked = map[string]bool{}
		commands := []tea.Cmd{
			model.diskUsageCommand(context.Background(), time.Now(), model.enrichmentID),
			model.selectedBranchGraphCommand(model.enrichmentID),
		}
		// PR fetch was deferred until the branch list became available.
		if model.prLoading && model.enrichmentContext != nil {
			if prCommand := model.pullRequestFetchCommand(model.enrichmentContext, model.enrichmentID); prCommand != nil {
				commands = append(commands, prCommand)
			}
		}
		return model, tea.Batch(commands...)
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
			model.prCIChecked = map[int]bool{}
			model.prReviewChecked = map[int]bool{}
			model.prReview = map[int]github.PullRequestReview{}
			if message.ciIncluded {
				for _, pullRequest := range message.pullRequests {
					model.prCIChecked[pullRequest.Number] = true
				}
			}
			model.state.Rows = github.AttachPullRequests(model.state.Rows, message.pullRequests, model.state.Repo.MainBranch)
			model.state.Branches = github.AttachBranchPullRequests(model.state.Branches, message.pullRequests, model.state.Repo.MainBranch)
		} else if len(model.prCache) > 0 && model.prCacheRepoRoot == model.state.Repo.Root {
			model.showPR = true
			model.state.Rows = github.AttachPullRequests(model.state.Rows, model.prCache, model.state.Repo.MainBranch)
			model.state.Branches = github.AttachBranchPullRequests(model.state.Branches, model.prCache, model.state.Repo.MainBranch)
		}
		ciCommand := model.pullRequestCICommand(message.id)
		reviewCommand := model.selectedReviewCommand(message.id)
		return model, tea.Batch(ciCommand, reviewCommand)
	case prCILoadedMsg:
		if message.id != model.enrichmentID || message.repoRoot != model.state.Repo.Root {
			return model, nil
		}
		if len(message.ci) == 0 {
			return model, nil
		}
		for branch, pullRequest := range model.prCache {
			if glyph, ok := message.ci[pullRequest.Number]; ok {
				pullRequest.CI = glyph
				model.prCache[branch] = pullRequest
			}
		}
		model.state.Rows = github.AttachPullRequests(model.state.Rows, model.prCache, model.state.Repo.MainBranch)
		model.state.Branches = github.AttachBranchPullRequests(model.state.Branches, model.prCache, model.state.Repo.MainBranch)
		return model, nil
	case prReviewLoadedMsg:
		if message.id != model.enrichmentID || message.repoRoot != model.state.Repo.Root {
			return model, nil
		}
		if model.prReview == nil {
			model.prReview = map[int]github.PullRequestReview{}
		}
		// Store the outcome even on failure (a non-Loaded review). The map entry
		// marks the attempt as finished, so the frame can stop showing "loading"
		// and stay silent when gh is missing or the lookup failed.
		model.prReview[message.number] = message.review
		return model, nil
	case branchGraphLoadedMsg:
		if message.id != model.enrichmentID || message.repoRoot != model.state.Repo.Root {
			return model, nil
		}
		for index := range model.state.Branches {
			if model.state.Branches[index].Name == message.name {
				model.state.Branches[index].Graph = message.graph
				break
			}
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
		for path, breakdown := range message.breakdowns {
			if index, ok := pathIndexes[path]; ok {
				model.state.Rows[index].DiskBreakdown = breakdown
			}
		}
		return model, nil
	case reloadMsg:
		if message.id != model.refreshID {
			return model, nil
		}
		if message.automatic && !model.canApplyAutoRefresh() {
			model.refreshInFlight = false
			model.refreshAnchor = selectionAnchor{}
			model.refreshProgressVisible = false
			return model, nil
		}
		anchor := model.refreshAnchor
		if anchor == (selectionAnchor{}) {
			anchor = model.selectionAnchor()
		}
		model.loading = ""
		model.refreshInFlight = false
		model.refreshAnchor = selectionAnchor{}
		model.refreshProgressVisible = false
		if message.err != nil {
			if message.automatic {
				return model, nil
			}
			return model.setFlash(message.err.Error())
		}
		model.state = message.state
		model.repoConfig = message.repoConfig
		model.hooksApproved = message.hooksApproved || !message.repoConfig.HasHooks()
		model.showPR = model.state.Repo.RemoteConfigured
		model.prLoading = false
		model.applyCachedPullRequests()
		model.lastRefreshAt = message.completedAt
		model.restoreSelection(anchor)
		model, enrichmentCmd := model.startEnrichment(!message.automatic)
		if message.automatic {
			return model, enrichmentCmd
		}
		model, flashCmd := model.setSuccessFeedbackFor(feedbackFrameWorktrees, "refreshed", successFeedbackTimeout)
		return model, tea.Batch(enrichmentCmd, flashCmd)
	case createMsg:
		model.loading = ""
		model.createInFlight = false
		if message.err != nil {
			if message.created {
				errorText := createdHookError("post_create", message.path, message.err)
				if model.createDialog != nil {
					model.createDialog.error = errorText
				} else {
					return model.setFlash(errorText)
				}
				return model, nil
			}
			if model.createDialog != nil {
				model.createDialog.error = message.err.Error()
			} else {
				return model.setFlash(message.err.Error())
			}
			return model, nil
		}
		model.selectedPath = message.path
		return model.withCreateWarnings(message.warnings, tea.Quit)
	case checkoutMsg:
		model.loading = ""
		if message.err != nil {
			if message.created {
				errorText := createdHookError("post_create", message.path, message.err)
				if model.checkoutDialog != nil {
					model.checkoutDialog.error = errorText
				} else if model.branchWorktreeDialog != nil {
					model.branchWorktreeDialog.error = errorText
				} else if model.pullRequestDialog != nil {
					model.pullRequestDialog.error = errorText
				} else {
					return model.setFlash(errorText)
				}
				return model, nil
			}
			if model.checkoutDialog != nil {
				model.checkoutDialog.error = message.err.Error()
			} else if model.branchWorktreeDialog != nil {
				model.branchWorktreeDialog.error = message.err.Error()
			} else if model.pullRequestDialog != nil {
				model.pullRequestDialog.error = message.err.Error()
			} else {
				return model.setFlash(message.err.Error())
			}
			return model, nil
		}
		model.selectedPath = message.path
		return model.withCreateWarnings(message.warnings, tea.Quit)
	case deleteMsg:
		if message.id != model.deleteID || !model.deleteInFlight {
			return model, nil
		}
		anchor := model.selectionAnchor()
		model.deleteInFlight = false
		if model.actionCancel != nil {
			model.actionCancel()
			model.actionCancel = nil
		}
		model.deleteSpinnerFrame = 0
		if message.reloaded {
			model.state = message.state
			model.repoConfig = message.repoConfig
			model.hooksApproved = message.hooksApproved || !message.repoConfig.HasHooks()
			model.showPR = model.state.Repo.RemoteConfigured
			model.prLoading = false
			model.applyCachedPullRequests()
			if !message.completedAt.IsZero() {
				model.lastRefreshAt = message.completedAt
			}
			model.restoreSelection(anchor)
		}
		if message.err != nil {
			if model.deleteDialog != nil {
				model.deleteDialog.error = "× " + message.err.Error()
				return model, nil
			}
			return model.setFlash("× " + message.err.Error())
		}
		model.deleteDialog = nil
		model, enrichmentCmd := model.startEnrichment(true)
		var flashCmd tea.Cmd
		if message.restore != nil {
			model, flashCmd = model.setRestoreOffer(*message.restore)
		} else {
			text := message.text
			if text == "" {
				text = "deleted worktree"
			}
			model, flashCmd = model.setSuccessFeedbackFor(feedbackFrameWorktrees, text, successFeedbackTimeout)
		}
		return model, tea.Batch(enrichmentCmd, flashCmd)
	case cleanupMergedMsg:
		if message.id != model.cleanupMergedID || !model.cleanupMergedInFlight {
			return model, nil
		}
		anchor := model.selectionAnchor()
		model.cleanupMergedInFlight = false
		if model.actionCancel != nil {
			model.actionCancel()
			model.actionCancel = nil
		}
		model.cleanupMergedSpinner = 0
		if message.reloaded {
			model.state = message.state
			model.repoConfig = message.repoConfig
			model.hooksApproved = message.hooksApproved || !message.repoConfig.HasHooks()
			model.showPR = model.state.Repo.RemoteConfigured
			model.prLoading = false
			model.applyCachedPullRequests()
			if !message.completedAt.IsZero() {
				model.lastRefreshAt = message.completedAt
			}
			model.restoreSelection(anchor)
		}
		if message.err != nil {
			if model.cleanupMergedDialog != nil {
				model.cleanupMergedDialog.error = "× " + message.err.Error()
				return model, nil
			}
			return model.setFlash("× " + message.err.Error())
		}
		model, enrichmentCmd := model.startEnrichment(true)
		if len(message.result.failures) > 0 {
			if model.cleanupMergedDialog == nil {
				model.cleanupMergedDialog = &cleanupMergedDialog{}
			}
			model.cleanupMergedDialog.result = &message.result
			model.cleanupMergedDialog.error = ""
			model.pendingRestoreBatch = message.result.restores
			model.pendingRestore = nil
			return model, enrichmentCmd
		}
		model.cleanupMergedDialog = nil
		var feedbackCmd tea.Cmd
		if len(message.result.restores) > 0 {
			model, feedbackCmd = model.setCleanupRestoreOffer(message.result)
		} else {
			model, feedbackCmd = model.setSuccessFeedbackFor(feedbackFrameWorktrees, cleanupMergedSummary(message.result), successFeedbackTimeout)
		}
		return model, tea.Batch(enrichmentCmd, feedbackCmd)
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
		anchor := model.selectionAnchor()
		model.config = message.config
		model.showBranches = message.config.ShowBranches
		model.restoreSelection(anchor)
		model, flashCmd := model.setFlash("config reloaded")
		if model.createDialog != nil && message.path != "" {
			return model, tea.Batch(flashCmd, watchConfigChangeCmd(message.path, message.modTime))
		}
		return model, flashCmd
	case pullRequestSummariesLoadedMsg:
		if model.pullRequestDialog == nil || message.id != model.pullRequestDialog.id {
			return model, nil
		}
		model.pullRequestDialog.loading = false
		if message.err != nil {
			model.pullRequestDialog.error = message.err.Error()
			model.pullRequestDialog.summaries = nil
			model.pullRequestDialog.selected = 0
			return model, nil
		}
		model.pullRequestDialog.error = ""
		model.pullRequestDialog.summaries = message.summaries
		model.pullRequestDialog.selected = clamp(model.pullRequestDialog.selected, 0, max(0, len(model.matchingPullRequestSummaries())-1))
		return model, nil
	case pullRequestSummaryLoadedMsg:
		model.loading = ""
		if model.pullRequestDialog == nil || message.id != model.pullRequestDialog.id {
			return model, nil
		}
		model.pullRequestDialog.directLookup = false
		if message.err != nil {
			model.pullRequestDialog.error = "No matching PR"
			return model, nil
		}
		return model.startPullRequestCheckout(message.summary)
	case pullRequestOpenedMsg:
		model.loading = ""
		if model.pullRequestDialog == nil || message.id != model.pullRequestDialog.id {
			return model, nil
		}
		if message.err != nil {
			model.pullRequestDialog.error = message.err.Error()
			return model, nil
		}
		model.pullRequestDialog.error = ""
		return model, nil
	case settingsSavedMsg:
		if message.err != nil {
			return model.setFlash(message.err.Error())
		}
		return model, nil
	case noOpMsg:
		return model, nil
	case clearFlashMsg:
		if message.id == model.flashID {
			model.flash = ""
		}
		return model, nil
	case clearFeedbackMsg:
		if message.id == model.feedbackID {
			model = model.clearFeedback()
		}
		return model, nil
	case autoRefreshMsg:
		return model.updateAutoRefresh()
	case clockTickMsg:
		return model, clockTickCmd(model.lastRefreshAt)
	case refreshSpinnerTickMsg:
		if message.id != model.refreshID || !model.refreshInFlight || !model.refreshProgressVisible {
			return model, nil
		}
		model.refreshSpinnerFrame = (model.refreshSpinnerFrame + 1) % len(refreshSpinnerFrames)
		return model, refreshSpinnerTickCmd(model.refreshID)
	case deleteSpinnerTickMsg:
		if message.id != model.deleteID || !model.deleteInFlight {
			return model, nil
		}
		model.deleteSpinnerFrame = (model.deleteSpinnerFrame + 1) % len(refreshSpinnerFrames)
		return model, deleteSpinnerTickCmd(model.deleteID)
	case cleanupMergedSpinnerTickMsg:
		if message.id != model.cleanupMergedID || !model.cleanupMergedInFlight {
			return model, nil
		}
		model.cleanupMergedSpinner = (model.cleanupMergedSpinner + 1) % len(refreshSpinnerFrames)
		return model, cleanupMergedSpinnerTickCmd(model.cleanupMergedID)
	case pullRequestSpinnerTickMsg:
		if model.pullRequestDialog == nil ||
			message.id != model.pullRequestDialog.id ||
			(!model.pullRequestDialog.loading && !model.pullRequestDialog.directLookup) {
			return model, nil
		}
		model.pullRequestDialog.spinnerFrame = (model.pullRequestDialog.spinnerFrame + 1) % len(refreshSpinnerFrames)
		return model, pullRequestSpinnerTickCmd(model.pullRequestDialog.id)
	case tea.KeyMsg:
		if message.Type == tea.KeyCtrlC {
			model = model.cancelEnrichment()
			return model, tea.Quit
		}
		if model.createDialog != nil {
			return model.updateCreate(message)
		}
		if model.checkoutDialog != nil {
			return model.updateCheckout(message)
		}
		if model.branchWorktreeDialog != nil {
			return model.updateBranchWorktree(message)
		}
		if model.deleteDialog != nil {
			return model.updateDelete(message)
		}
		if model.cleanupMergedDialog != nil {
			return model.updateCleanupMerged(message)
		}
		if model.paletteDialog != nil {
			return model.updatePalette(message)
		}
		if model.filterDialog != nil {
			return model.updateFilterDialog(message)
		}
		if model.pullRequestDialog != nil {
			return model.updatePullRequestCheckout(message)
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
	case "q":
		model = model.cancelEnrichment()
		return model, tea.Quit
	case "ctrl+p":
		return model.openPalette()
	case "ctrl+o":
		return model, openConfigCmd(model.config.Editor, model.config)
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
		model.selectMatching(func(row gitdata.Row) bool { return row.IsWorktree() && row.Worktree.IsMain })
	case "a":
		model.selectMatching(func(row gitdata.Row) bool { return row.IsWorktree() && row.Worktree.IsActive })
	case "tab":
		return model.openFilterDialog()
	case "enter":
		row, ok := model.selectedTableRow()
		if !ok {
			return model, nil
		}
		if row.IsBranch() {
			return model.openBranchWorktree(row.Branch)
		}
		if row.Worktree.Prunable {
			return model.setFlash("cannot enter a prunable worktree")
		}
		if row.Worktree.IsActive {
			return model, tea.Quit
		}
		model.selectedPath = row.Worktree.Path
		return model, tea.Quit
	case "n":
		return model.openCreate()
	case "c":
		row, ok := model.selectedTableRow()
		if !ok || !row.IsBranch() {
			return model.setFlash("checkout root is only available for branch rows")
		}
		return model.openCheckoutRoot(row.Branch)
	case "delete", "backspace", "d":
		return model.openDelete()
	case "o":
		row, ok := model.selectedWorktree()
		if !ok || row.Prunable {
			return model.setFlash("cannot open this worktree")
		}
		return model, openEditorCmd(model.config.Editor, row.Path)
	case "p":
		row, ok := model.selectedTableRow()
		if !ok {
			return model, nil
		}
		return model, func() tea.Msg {
			err := github.OpenRowPullRequestOrBranch(context.Background(), model.state.Repo.Root, row, model.runner)
			return actionMsg{text: "opened", err: err}
		}
	case "y":
		text, flash, ok := model.selectedCopyText()
		if !ok {
			return model, nil
		}
		return model, copyTextCmd(text, flash)
	case "r":
		return model.startRefresh(true, false)
	case "u":
		if !model.hasPendingRestore() || model.deleteInFlight || model.cleanupMergedInFlight {
			return model, nil
		}
		return model.startRestore()
	case "s":
		model.searching = true
		model.search.Focus()
	case "b":
		anchor := model.selectionAnchor()
		model.showBranches = !model.showBranches
		model.config.ShowBranches = model.showBranches
		if !model.restoreSelection(anchor) && len(model.visibleIndexes()) > 0 {
			model.selected = 0
		}
		return model, persistShowBranchesCmd(model.showBranches)
	case "?":
		model.help = !model.help
	}
	reviewCommand := model.selectedReviewCommand(model.enrichmentID)
	graphCommand := model.selectedBranchGraphCommand(model.enrichmentID)
	return model, tea.Batch(reviewCommand, graphCommand)
}

func (model Model) startRefresh(fetch, automatic bool) (Model, tea.Cmd) {
	if model.refreshInFlight || model.cleanupMergedInFlight {
		return model, nil
	}
	model = model.cancelEnrichment()
	model.refreshID++
	model.refreshInFlight = true
	model.refreshAnchor = model.selectionAnchor()
	model.refreshProgressVisible = !automatic
	model.refreshSpinnerFrame = 0
	model = model.clearFeedback()
	commands := []tea.Cmd{reloadCmd(model.reloadCwd(), model.config, model.runner, model.state.Repo, fetch, automatic, model.refreshID)}
	if model.refreshProgressVisible {
		commands = append(commands, refreshSpinnerTickCmd(model.refreshID))
	}
	return model, tea.Batch(commands...)
}

func (model Model) updateAutoRefresh() (Model, tea.Cmd) {
	nextTick := autoRefreshTickCmd()
	if !model.canAutoRefresh() {
		return model, nextTick
	}
	model, refreshCmd := model.startRefresh(false, true)
	return model, tea.Batch(nextTick, refreshCmd)
}

func (model Model) canAutoRefresh() bool {
	return !model.refreshInFlight &&
		model.canApplyAutoRefresh()
}

func (model Model) canApplyAutoRefresh() bool {
	return model.loading == "" &&
		!model.deleteInFlight &&
		!model.cleanupMergedInFlight &&
		!model.searching &&
		!model.help &&
		model.createDialog == nil &&
		model.checkoutDialog == nil &&
		model.branchWorktreeDialog == nil &&
		model.deleteDialog == nil &&
		model.cleanupMergedDialog == nil &&
		model.paletteDialog == nil &&
		model.filterDialog == nil &&
		model.pullRequestDialog == nil &&
		!model.hasPendingRestore()
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
		return model.openFilterDialog()
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
		row, ok := model.selectedTableRow()
		if !ok {
			return model, nil
		}
		if row.IsBranch() {
			return model.openBranchWorktree(row.Branch)
		}
		if row.Worktree.Prunable {
			return model.setFlash("cannot enter a prunable worktree")
		}
		if row.Worktree.IsActive {
			return model, tea.Quit
		}
		model.selectedPath = row.Worktree.Path
		return model, tea.Quit
	case paletteCreate:
		return model.openCreate()
	case paletteDelete:
		return model.openDelete()
	case paletteOpenEditor:
		row, ok := model.selectedWorktree()
		if !ok || row.Prunable {
			return model.setFlash("cannot open this worktree")
		}
		return model, openEditorCmd(model.config.Editor, row.Path)
	case paletteOpenPullRequest:
		row, ok := model.selectedTableRow()
		if !ok {
			return model, nil
		}
		return model, func() tea.Msg {
			err := github.OpenRowPullRequestOrBranch(context.Background(), model.state.Repo.Root, row, model.runner)
			return actionMsg{text: "opened", err: err}
		}
	case paletteCheckoutPullRequest:
		return model.openPullRequestCheckout()
	case paletteCleanUpMerged:
		return model.openCleanupMerged()
	case paletteCopyPath:
		text, flash, ok := model.selectedCopyText()
		if !ok {
			return model, nil
		}
		return model, copyTextCmd(text, flash)
	case paletteCopyPullRequestURL:
		text, flash, ok := model.selectedPullRequestCopy()
		if !ok {
			return model.setFlash("no pull request URL for this row")
		}
		return model, copyTextCmd(text, flash)
	case paletteRefresh:
		return model.startRefresh(true, false)
	case paletteSearch:
		model.searching = true
		return model, model.search.Focus()
	case paletteJumpRoot:
		model.selectMatching(func(row gitdata.Row) bool { return row.IsWorktree() && row.Worktree.IsMain })
	case paletteJumpActive:
		model.selectMatching(func(row gitdata.Row) bool { return row.IsWorktree() && row.Worktree.IsActive })
	case paletteJumpTop:
		model.selected = 0
	case paletteJumpBottom:
		model.selected = max(0, len(model.visibleIndexes())-1)
	case paletteCycleFilter:
		return model.openFilterDialog()
	case paletteFilterAll:
		model.setFilter(filterAll)
	case paletteFilterModified:
		model.setFilter(filterModified)
	case paletteFilterMerged:
		model.setFilter(filterMerged)
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

func (model Model) openFilterDialog() (Model, tea.Cmd) {
	model.help = false
	model.paletteDialog = nil
	options := model.filterOptions()
	model.filterDialog = &filterDialog{selected: selectedFilterOptionIndex(options, model.filter)}
	return model, nil
}

func (model Model) updateFilterDialog(message tea.KeyMsg) (Model, tea.Cmd) {
	switch message.String() {
	case "esc":
		model.filterDialog = nil
		return model, nil
	case "up", "k", "shift+tab":
		model.moveFilterDialogSelection(-1)
		return model, nil
	case "down", "j", "tab":
		model.moveFilterDialogSelection(1)
		return model, nil
	case "enter":
		options := model.filterOptions()
		if len(options) == 0 {
			model.filterDialog = nil
			return model, nil
		}
		selected := clamp(model.filterDialog.selected, 0, len(options)-1)
		option := options[selected]
		if !option.enabled {
			return model, nil
		}
		model.filterDialog = nil
		model.setFilter(option.filter)
		return model, nil
	}
	return model, nil
}

func (model *Model) moveFilterDialogSelection(direction int) {
	if model.filterDialog == nil || direction == 0 {
		return
	}
	options := model.filterOptions()
	if len(options) == 0 {
		model.filterDialog.selected = 0
		return
	}
	selected := clamp(model.filterDialog.selected, 0, len(options)-1)
	for offset := 1; offset <= len(options); offset++ {
		next := (selected + direction*offset) % len(options)
		if next < 0 {
			next += len(options)
		}
		if options[next].enabled {
			model.filterDialog.selected = next
			return
		}
	}
	model.filterDialog.selected = selectedFilterOptionIndex(options, model.filter)
}

func (model Model) filterOptions() []filterOption {
	options := make([]filterOption, 0, len(orderedFilters))
	for _, filter := range orderedFilters {
		count := len(model.visibleIndexesForFilter(filter))
		options = append(options, filterOption{
			filter:  filter,
			count:   count,
			enabled: filter == filterAll || count > 0,
		})
	}
	return options
}

func selectedFilterOptionIndex(options []filterOption, current worktreeFilter) int {
	for index, option := range options {
		if option.filter == current && option.enabled {
			return index
		}
	}
	for index, option := range options {
		if option.enabled {
			return index
		}
	}
	return 0
}

func (model Model) openPullRequestCheckout() (Model, tea.Cmd) {
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
				return checkoutMsg{path: path, err: err}
			}
			warnings, err := runPostCreateSteps(ctx, repoRoot, path, branch, mainBranch, repoConfig, hooksApproved, runner)
			return checkoutMsg{path: path, created: true, err: err, warnings: warnings}
		}
	}
	return model, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := gitdata.CheckoutPullRequestWorktree(ctx, repoRoot, summary.Number, branch, path, runner); err != nil {
			return checkoutMsg{path: path, err: err}
		}
		warnings, err := runPostCreateSteps(ctx, repoRoot, path, branch, mainBranch, repoConfig, hooksApproved, runner)
		return checkoutMsg{path: path, created: true, err: err, warnings: warnings}
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
	model.branchWorktreeDialog = &branchWorktreeDialog{branch: branch, path: path}
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
				return checkoutMsg{path: path, err: err}
			}
			warnings, err := runPostCreateSteps(ctx, repoRoot, path, branch, mainBranch, repoConfig, hooksApproved, runner)
			return checkoutMsg{path: path, created: true, err: err, warnings: warnings}
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
		state, err := loadStableState(reloadCtx, cwd, config, runner)
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
	command := deleteAndLoadCmd(ctx, model.reloadCwd(), model.config, model.runner, model.deleteID, text, restore, action)
	return model, tea.Batch(command, deleteSpinnerTickCmd(model.deleteID))
}

func deleteAndLoadCmd(ctx context.Context, cwd string, config config.Config, runner gitdata.Runner, id int, text string, restore *pendingBranchRestore, action func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		actionErr := action(ctx)
		reloadCtx, cancel := context.WithTimeout(context.Background(), destructiveActionTimeout)
		defer cancel()
		state, err := loadStableState(reloadCtx, cwd, config, runner)
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

type viewSnapshot struct {
	rows        []gitdata.Row
	visibleRows []gitdata.Row
	selectedRow gitdata.Row
	hasSelected bool
	// blocks are the finished, bordered boxes that stack below the Worktrees panel
	// (the Details box, possibly paired with Git context, then the secondary frames).
	blocks []string
	// reservedBlockLines is the height the detail region is sized for: the tallest
	// blocks any row would render. Shorter rows pad up to it so moving the selection
	// never resizes the list above.
	reservedBlockLines int
	start              int
	scrollbar          listScrollbar
}

type listScrollbar struct {
	total   int
	visible int
	start   int
}

func (model Model) View() string {
	now := time.Now()
	width := viewWidth(model)
	outerWidth := max(4, width)
	contentWidth := max(1, outerWidth-4)
	panelWidth := max(4, contentWidth)
	panelContentWidth := max(1, panelWidth-2)
	rowCount := model.totalRowCount()
	var blocks []string
	reservedBlockLines := 0
	lines := []string{"Loading worktrees…"}
	worktreeScrollbar := listScrollbar{}
	if model.localMetadataReady() {
		snapshot := model.viewSnapshot(now, panelContentWidth)
		rowCount = len(snapshot.rows)
		blocks = snapshot.blocks
		reservedBlockLines = snapshot.reservedBlockLines
		worktreeScrollbar = snapshot.scrollbar
		tableWidth := tableContentWidth(panelContentWidth, snapshot.scrollbar)
		table := listview.RenderMixedRows(snapshot.visibleRows, listview.Options{
			Width:             tableWidth,
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
			lines = []string{"No rows"}
		}
		lines = renderLinesWithListScrollbar(lines, panelContentWidth, snapshot.scrollbar)
	}
	leftFooter, rightFooter := model.listFooterHintsForScrollbar(worktreeScrollbar, panelContentWidth)
	parts := []string{
		model.appTopLine(rowCount, outerWidth),
		model.wrapOuter(sectionBoxWithSplitFooterTopRight("Worktrees", lines, leftFooter, rightFooter, model.worktreesFeedback(), panelWidth), outerWidth),
	}
	for _, block := range blocks {
		if block != "" {
			parts = append(parts, model.wrapOuter(block, outerWidth))
		}
	}
	// The list is sized for the tallest row's detail region, so pad a shorter one up
	// to that height; otherwise the bottom line would jump as the selection moves.
	for range max(0, reservedBlockLines-blockLinesTotal(blocks)) {
		parts = append(parts, model.wrapOuter("", outerWidth))
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
	if model.filterDialog != nil {
		output = centeredOverlay(output, model.renderFilterAtWidth(filterDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.pullRequestDialog != nil {
		output = centeredOverlay(output, model.renderPullRequestCheckoutAtWidth(pullRequestDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.deleteDialog != nil {
		output = centeredOverlay(output, model.renderDeleteAtWidth(deleteDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.cleanupMergedDialog != nil {
		output = centeredOverlay(output, model.renderCleanupMergedAtWidth(deleteDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.branchWorktreeDialog != nil {
		output = centeredOverlay(output, model.renderBranchWorktreeAtWidth(checkoutDialogWidth(outerWidth)), outerWidth, overlayHeight)
	}
	if model.checkoutDialog != nil {
		output = centeredOverlay(output, model.renderCheckoutAtWidth(checkoutDialogWidth(outerWidth)), outerWidth, overlayHeight)
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

func (model Model) totalRowCount() int {
	return len(model.tableRows())
}

func (model Model) viewSnapshot(now time.Time, panelContentWidth int) viewSnapshot {
	indexes := model.visibleIndexes()
	tableRows := model.tableRows()
	rows := make([]gitdata.Row, 0, len(indexes))
	for _, index := range indexes {
		rows = append(rows, tableRows[index])
	}
	snapshot := viewSnapshot{rows: rows}
	if len(indexes) > 0 && model.selected >= 0 && model.selected < len(indexes) {
		snapshot.selectedRow = tableRows[indexes[model.selected]]
		snapshot.hasSelected = true
		snapshot.blocks = model.detailBlocks(snapshot.selectedRow, now, panelContentWidth+2)
	}
	snapshot.reservedBlockLines = model.reservedDetailBlockLines(rows, now, panelContentWidth+2)
	availableHeight := model.availableTableHeightForBlockLines(snapshot.reservedBlockLines)
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
	snapshot.scrollbar = listScrollbar{
		total:   len(rows),
		visible: len(snapshot.visibleRows),
		start:   snapshot.start,
	}
	return snapshot
}

// detailSideBySideMinWidth is the narrowest panel at which the Details box and the
// Git context frame render side by side. Below it they stack. The threshold keeps
// each half at or above the frames' 50-column minimum plus the gap between them.
const detailSideBySideMinWidth = 104

// detailSideBySideGap is the blank gutter between the side-by-side boxes.
const detailSideBySideGap = 1

// detailBlocks renders the Details box together with the context frames as finished,
// bordered blocks ready to stack below the Worktrees panel. On a wide panel the Git
// context frame sits beside the Details box; otherwise it stacks directly beneath it.
// The remaining frames (PR review, Changes, Disk) always stack full width below.
func (model Model) detailBlocks(row gitdata.Row, now time.Time, panelWidth int) []string {
	var blocks []string
	if left, right, ok := model.detailWithGitContext(row, now, panelWidth); ok {
		gap := strings.Repeat(" ", detailSideBySideGap)
		blocks = append(blocks, lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right))
	} else {
		body := model.rowDetailPanelAtWidth(row, now, panelWidth-2)
		blocks = append(blocks, sectionBoxWithFooter(rowDetailTitle(row), strings.Split(body, "\n"), detailFooterHints(row, panelWidth), panelWidth))
		if box := changesFrame(row, panelWidth); box != "" {
			blocks = append(blocks, box)
		}
		if box := prReviewFrame(model.reviewForRow(row), model.reviewPendingNumberForRow(row), panelWidth); box != "" {
			blocks = append(blocks, box)
		}
		if box := gitContextFrame(row, model.state.Repo.MainBranch, panelWidth, 0); box != "" {
			blocks = append(blocks, box)
		}
	}
	blocks = append(blocks, belowDetailFrames(row, panelWidth)...)
	return blocks
}

// detailWithGitContext renders the Details box and the Git context frame at half
// width each so they can be joined horizontally. It reports ok=false when the panel
// is too narrow or the row has no Git context to pair with, leaving the caller to
// stack them instead.
func (model Model) detailWithGitContext(row gitdata.Row, now time.Time, panelWidth int) (left, right string, ok bool) {
	if panelWidth < detailSideBySideMinWidth {
		return "", "", false
	}
	leftWidth := (panelWidth - detailSideBySideGap) / 2
	rightWidth := panelWidth - detailSideBySideGap - leftWidth
	// The left column stacks the Details, Changes, and PR review boxes; the Git
	// context frame on the right then grows to match that combined height, keeping
	// the outer bottom borders aligned.
	body := model.rowDetailPanelAtWidth(row, now, leftWidth-2)
	left = sectionBoxWithFooter(rowDetailTitle(row), strings.Split(body, "\n"), detailFooterHints(row, leftWidth), leftWidth)
	if changes := changesFrame(row, leftWidth); changes != "" {
		left = lipgloss.JoinVertical(lipgloss.Left, left, changes)
	}
	if pr := prReviewFrame(model.reviewForRow(row), model.reviewPendingNumberForRow(row), leftWidth); pr != "" {
		left = lipgloss.JoinVertical(lipgloss.Left, left, pr)
	}
	right = gitContextFrame(row, model.state.Repo.MainBranch, rightWidth, lineCount(left))
	if right == "" {
		return "", "", false
	}
	return left, right, true
}

func tableContentWidth(width int, scrollbar listScrollbar) int {
	if scrollbar.shouldRender(width) {
		return max(1, width-scrollbarGutterWidth)
	}
	return width
}

func renderLinesWithListScrollbar(lines []string, width int, scrollbar listScrollbar) []string {
	if !scrollbar.shouldRender(width) {
		return lines
	}
	contentWidth := tableContentWidth(width, scrollbar)
	output := make([]string, len(lines))
	for index, line := range lines {
		output[index] = padStyled(truncateStyled(line, contentWidth), contentWidth) + " " + scrollbar.glyphAt(index, len(lines))
	}
	return output
}

func (scrollbar listScrollbar) shouldRender(width int) bool {
	return width > scrollbarGutterWidth && scrollbar.total > scrollbar.visible && scrollbar.visible > 0
}

func (scrollbar listScrollbar) positionText() string {
	if scrollbar.total <= 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", scrollbar.start, scrollbar.total)
}

func (scrollbar listScrollbar) glyphAt(index, height int) string {
	if index == 0 {
		return scrollbar.arrow("↑", scrollbar.start > 0)
	}
	if index == height-1 {
		return scrollbar.arrow("↓", scrollbar.start+scrollbar.visible < scrollbar.total)
	}
	if scrollbar.trackHasThumbAt(index-1, max(0, height-2)) {
		return scrollbarThumbStyle.Render("█")
	}
	return scrollbarTrackStyle.Render("│")
}

func (scrollbar listScrollbar) arrow(value string, enabled bool) string {
	if !enabled {
		return scrollbarTrackStyle.Render(value)
	}
	return scrollbarArrowStyle.Render(value)
}

func (scrollbar listScrollbar) trackHasThumbAt(index, trackHeight int) bool {
	if trackHeight <= 0 {
		return false
	}
	thumbHeight := max(1, trackHeight*scrollbar.visible/scrollbar.total)
	thumbHeight = min(trackHeight, thumbHeight)
	availablePositions := trackHeight - thumbHeight
	maxStart := max(1, scrollbar.total-scrollbar.visible)
	thumbStart := 0
	if availablePositions > 0 {
		thumbStart = (scrollbar.start*availablePositions + maxStart/2) / maxStart
	}
	return index >= thumbStart && index < thumbStart+thumbHeight
}

func (model Model) selectedInspector(row gitdata.Worktree, now time.Time) string {
	return model.selectedRowInspector(gitdata.Row{Kind: gitdata.RowKindWorktree, Worktree: row}, now)
}

func (model Model) selectedRowInspector(row gitdata.Row, now time.Time) string {
	return model.selectedRowInspectorAtWidth(row, now, viewWidth(model))
}

func (model Model) selectedRowInspectorAtWidth(row gitdata.Row, now time.Time, width int) string {
	if row.IsBranch() {
		return model.selectedBranchInspectorAtWidth(row.Branch, width)
	}
	worktree := row.Worktree
	lines := []string{
		model.inspectorRenderedFieldAtWidth("Branch", branchText(worktree), func(value string) string {
			return branchStyle(worktree).Render(value)
		}, width),
		model.inspectorRenderedFieldAtWidth("HEAD", headText(worktree), renderHeadValue, width),
		model.inspectorFieldAtWidth("Path", model.relativePath(worktree.Path), inspectorValueStyle, width),
		model.inspectorFieldAtWidth("Status", statusText(worktree), statusStyle(worktree), width),
		model.inspectorRenderedFieldAtWidth("Dirty", dirtyDetailText(worktree.Status), renderDirtyDetailValue, width),
		model.inspectorFieldAtWidth("Size", sizeText(worktree), inspectorValueStyle, width),
	}
	lines = append(lines,
		model.inspectorFieldAtWidth("Delete", deleteSafetyText(worktree), deleteSafetyStyle(worktree), width),
	)
	if hint := model.safeToRemoveDetailHint(row); hint != "" {
		lines = append(lines, inspectorCleanStyle.Render(truncatePlain(hint, width)))
	}
	return strings.Join(lines, "\n")
}

func (model Model) safeToRemoveDetailHint(row gitdata.Row) string {
	if !row.IsWorktree() || !mergedFilterMatches(row) || model.cleanupMergedWorktreeSkipReason(row.Worktree) != "" {
		return ""
	}
	switch {
	case row.Worktree.UpstreamGone:
		return "finished: clean, merged; remote branch deleted — safe to remove (d)"
	case prMergedOrClosed(row):
		return "finished: clean, PR merged/closed — safe to remove (d)"
	default:
		return "finished: clean, merged to main — safe to remove (d)"
	}
}

func (model Model) selectedBranchInspectorAtWidth(branch gitdata.Branch, width int) string {
	lines := []string{
		model.inspectorFieldAtWidth("Branch", branch.DisplayBranch(), branchOnlyDetailStyle, width),
		model.inspectorRenderedFieldAtWidth("HEAD", branchHeadText(branch), renderHeadValue, width),
		model.inspectorFieldAtWidth("Path", "not checked out", inspectorValueStyle, width),
		model.inspectorFieldAtWidth("Status", "no worktree", inspectorValueStyle, width),
		model.inspectorFieldAtWidth("Dirty", "-", inspectorValueStyle, width),
		model.inspectorFieldAtWidth("Size", "-", inspectorValueStyle, width),
		model.inspectorFieldAtWidth("Action", "create worktree; checkout root with c", inspectorCleanStyle, width),
	}
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
	return model.rowDetailPanel(gitdata.Row{Kind: gitdata.RowKindWorktree, Worktree: row}, now)
}

func (model Model) rowDetailPanel(row gitdata.Row, now time.Time) string {
	return model.rowDetailPanelAtWidth(row, now, viewWidth(model))
}

func (model Model) rowDetailPanelAtWidth(row gitdata.Row, now time.Time, width int) string {
	return model.selectedRowInspectorAtWidth(row, now, width)
}

func detailTitle(row gitdata.Worktree) string {
	return rowDetailTitle(gitdata.Row{Kind: gitdata.RowKindWorktree, Worktree: row})
}

func rowDetailTitle(row gitdata.Row) string {
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

func detailFooterHints(row gitdata.Row, width int) string {
	actionParts := []string{"↵ go", "o editor", "d delete", "y abs path", "p PR"}
	if row.IsBranch() {
		actionParts = []string{"↵ create+go", "c checkout root", "d delete", "y name", "p PR"}
	}
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

func renderHeadValue(value string) string {
	head, rest, found := strings.Cut(value, " ")
	if !found {
		return inspectorCommitStyle.Render(value)
	}
	return inspectorCommitStyle.Render(head) + inspectorSubjectStyle.Render(" "+rest)
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

func branchHeadText(branch gitdata.Branch) string {
	if branch.Head == "" {
		if branch.Name != "" {
			return "on " + branch.Name
		}
		return "-"
	}
	if branch.Name != "" {
		return shortRef(branch.Head) + " on " + branch.Name
	}
	return shortRef(branch.Head)
}

func statusText(row gitdata.Worktree) string {
	if row.Status.Clean() {
		return "clean"
	}
	return "dirty"
}

func selectionContextTitle(row gitdata.Row) string {
	if row.IsBranch() {
		return "Local branch"
	}
	worktree := row.Worktree
	switch {
	case worktree.IsActive && worktree.IsMain:
		return "Current root repository"
	case worktree.IsActive:
		return "Current worktree"
	case worktree.IsMain:
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
	return model.rowPRText(gitdata.Row{Kind: gitdata.RowKindWorktree, Worktree: row})
}

func (model Model) rowPRText(row gitdata.Row) string {
	if row.PullRequest() == nil {
		if model.pullRequestsPending() {
			return listview.LoadingPlaceholder
		}
		return "none"
	}
	text := row.PullRequest().Text()
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
	return sectionBoxWithFooterTopRight(title, bodyLines, footer, "", width)
}

func sectionBoxWithFooterTopRight(title string, bodyLines []string, footer, topRight string, width int) string {
	return sectionBoxWithSplitFooterTopRight(title, bodyLines, footer, "", topRight, width)
}

func sectionBoxWithSplitFooterTopRight(title string, bodyLines []string, leftFooter, rightFooter, topRight string, width int) string {
	if width < 4 {
		return strings.Join(bodyLines, "\n")
	}
	innerWidth := width - 2
	lines := make([]string, 0, len(bodyLines)+2)
	lines = append(lines, sectionTopLineWithRight(title, topRight, width))
	for _, line := range bodyLines {
		lines = append(lines, panelBorderStyle.Render("│")+padStyled(line, innerWidth)+panelBorderStyle.Render("│"))
	}
	lines = append(lines, sectionBottomLineSplit(leftFooter, rightFooter, width))
	return strings.Join(lines, "\n")
}

func sectionTopLine(title string, width int) string {
	return sectionTopLineWithRight(title, "", width)
}

func sectionTopLineWithRight(title, right string, width int) string {
	innerWidth := width - 2
	label := ""
	if title != "" {
		labelWidth := max(0, innerWidth-3)
		label = " " + renderSectionTitle(title, labelWidth) + " "
	}
	labelWidth := lipgloss.Width(label)
	rightLabel := ""
	if right != "" {
		candidate := " " + right + " "
		if lipgloss.Width(candidate) <= width-labelWidth-5 {
			rightLabel = candidate
		}
	}
	if rightLabel != "" {
		ruleWidth := width - 4 - labelWidth - lipgloss.Width(rightLabel)
		if ruleWidth < 1 {
			ruleWidth = 1
		}
		return panelBorderStyle.Render("╭─") + panelTitleStyle.Render(label) + panelBorderStyle.Render(strings.Repeat("─", ruleWidth)) + rightLabel + panelBorderStyle.Render("─╮")
	}
	ruleWidth := innerWidth - 1 - labelWidth
	if ruleWidth < 0 {
		ruleWidth = 0
	}
	return panelBorderStyle.Render("╭─") + panelTitleStyle.Render(label) + panelBorderStyle.Render(strings.Repeat("─", ruleWidth)+"╮")
}

func sectionBottomLine(footer string, width int) string {
	return sectionBottomLineSplit(footer, "", width)
}

func sectionBottomLineSplit(leftFooter, rightFooter string, width int) string {
	return bottomBorderLine(width, panelBorderStyle, borderControls{parts: hintParts(leftFooter)}, borderControls{parts: hintParts(rightFooter)})
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

func (model Model) listFooterLeftHints() string {
	if model.searching {
		return "search " + model.search.Value() + "▌"
	}
	return "n new worktree"
}

func (model Model) listFooterRightHints() string {
	branchHint := "b branches"
	if model.showBranches {
		branchHint = "b hide branches"
	}
	if model.searching {
		return "Esc clear · Tab filter: " + model.filter.label() + " · " + branchHint
	}
	if model.filter != filterAll {
		return "h root · a active · Tab filter: " + model.filter.label() + " · Esc clear filter · s search · " + branchHint
	}
	return "h root · a active · Tab filter: " + model.filter.label() + " · s search · " + branchHint
}

func (model Model) listFooterHintsForScrollbar(scrollbar listScrollbar, width int) (string, string) {
	if scrollbar.shouldRender(width) {
		return model.listFooterLeftHints(), scrollbar.positionText()
	}
	return model.listFooterLeftHints(), model.listFooterRightHints()
}

func (model Model) worktreesFeedback() string {
	if model.refreshInFlight && model.refreshProgressVisible {
		frame := refreshSpinnerFrames[model.refreshSpinnerFrame%len(refreshSpinnerFrames)]
		return refreshActivityStyle.Render(frame + " refreshing")
	}
	return model.feedbackFor(feedbackFrameWorktrees)
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
	count := model.rowCountText(len(model.tableRows()))
	if model.search.Value() != "" || model.filter != filterAll {
		count = fmt.Sprintf("%d/%s", visibleCount, model.rowCountText(len(model.tableRows())))
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

func (model Model) rowCountText(total int) string {
	if model.showBranches || model.filter == filterBranches {
		return fmt.Sprintf("%d rows", total)
	}
	return fmt.Sprintf("%d worktrees", len(model.state.Rows))
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
	fullWithAge := colorKeyHints(refresh+" · ? help · q quit", false)
	if lipgloss.Width(fullWithAge) <= width {
		return fullWithAge
	}
	full := colorKeyHints("r refresh · ? help · q quit", false)
	if lipgloss.Width(full) <= width {
		return full
	}
	medium := colorKeyHints("r · ? help · q quit", false)
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

func refreshSpinnerTickCmd(id int) tea.Cmd {
	return tea.Tick(refreshTickInterval, func(time.Time) tea.Msg {
		return refreshSpinnerTickMsg{id: id}
	})
}

func deleteSpinnerTickCmd(id int) tea.Cmd {
	return tea.Tick(refreshTickInterval, func(time.Time) tea.Msg {
		return deleteSpinnerTickMsg{id: id}
	})
}

func cleanupMergedSpinnerTickCmd(id int) tea.Cmd {
	return tea.Tick(refreshTickInterval, func(time.Time) tea.Msg {
		return cleanupMergedSpinnerTickMsg{id: id}
	})
}

func pullRequestSpinnerTickCmd(id int) tea.Cmd {
	return tea.Tick(refreshTickInterval, func(time.Time) tea.Msg {
		return pullRequestSpinnerTickMsg{id: id}
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

func (model Model) setSuccessFeedbackFor(frame feedbackFrame, text string, timeout time.Duration) (Model, tea.Cmd) {
	return model.setFeedbackFor(successFeedback(frame, text), timeout)
}

func (model Model) setFeedbackFor(feedback transientFeedback, timeout time.Duration) (Model, tea.Cmd) {
	model.pendingRestore = nil
	model.pendingRestoreBatch = nil
	model.feedbackID++
	model.feedback = feedback
	id := model.feedbackID
	return model, tea.Tick(timeout, func(time.Time) tea.Msg {
		return clearFeedbackMsg{id: id}
	})
}

func (model Model) clearFeedback() Model {
	model.feedbackID++
	model.feedback = transientFeedback{}
	model.pendingRestore = nil
	model.pendingRestoreBatch = nil
	return model
}

func (model Model) setRestoreOffer(restore pendingBranchRestore) (Model, tea.Cmd) {
	model, cmd := model.setFeedbackFor(restoreOfferFeedback(restore), restoreOfferTimeout)
	model.pendingRestore = &restore
	return model, cmd
}

func (model Model) setCleanupRestoreOffer(result cleanupMergedResult) (Model, tea.Cmd) {
	model, cmd := model.setFeedbackFor(cleanupRestoreOfferFeedback(result), restoreOfferTimeout)
	model.pendingRestoreBatch = append([]pendingBranchRestore(nil), result.restores...)
	return model, cmd
}

func (model Model) hasPendingRestore() bool {
	return model.pendingRestore != nil || len(model.pendingRestoreBatch) > 0
}

func restoreOfferFeedback(restore pendingBranchRestore) transientFeedback {
	return successFeedbackWithSegments(feedbackFrameWorktrees,
		feedbackSegment{text: restoreOfferPrefix(restore)},
		feedbackSegment{text: "u", bold: true},
		feedbackSegment{text: " to restore"},
	)
}

func restoreOfferPrefix(restore pendingBranchRestore) string {
	return "deleted " + restore.branch + " (" + restore.short + ") · "
}

func cleanupRestoreOfferFeedback(result cleanupMergedResult) transientFeedback {
	return successFeedbackWithSegments(feedbackFrameWorktrees,
		feedbackSegment{text: cleanupMergedSummary(result) + " · "},
		feedbackSegment{text: "u", bold: true},
		feedbackSegment{text: " to restore branches"},
	)
}

func cleanupMergedSummary(result cleanupMergedResult) string {
	parts := []string{
		fmt.Sprintf("removed %d %s", result.removedWorktrees, pluralize(result.removedWorktrees, "worktree", "worktrees")),
		fmt.Sprintf("deleted %d %s", result.deletedBranches, pluralize(result.deletedBranches, "branch", "branches")),
	}
	if len(result.failures) > 0 {
		parts = append(parts, fmt.Sprintf("failed %d", len(result.failures)))
	}
	return "cleaned up merged: " + strings.Join(parts, ", ")
}

func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func successFeedback(frame feedbackFrame, text string) transientFeedback {
	return successFeedbackWithSegments(frame, feedbackSegment{text: text})
}

func successFeedbackWithSegments(frame feedbackFrame, segments ...feedbackSegment) transientFeedback {
	allSegments := make([]feedbackSegment, 0, len(segments)+1)
	allSegments = append(allSegments, feedbackSegment{text: successGlyph + " "})
	allSegments = append(allSegments, segments...)
	return transientFeedback{frame: frame, kind: feedbackKindSuccess, segments: allSegments}
}

func (model Model) feedbackFor(frame feedbackFrame) string {
	if model.feedback.frame != frame || len(model.feedback.segments) == 0 {
		return ""
	}
	return model.feedback.render()
}

func (feedback transientFeedback) render() string {
	style := feedback.style()
	var builder strings.Builder
	for _, segment := range feedback.segments {
		segmentStyle := style
		if segment.bold {
			segmentStyle = segmentStyle.Bold(true)
		}
		builder.WriteString(segmentStyle.Render(segment.text))
	}
	return builder.String()
}

func (feedback transientFeedback) plainText() string {
	var builder strings.Builder
	for _, segment := range feedback.segments {
		builder.WriteString(segment.text)
	}
	return builder.String()
}

func (feedback transientFeedback) style() lipgloss.Style {
	switch feedback.kind {
	default:
		return refreshSuccessStyle
	}
}

func (model Model) selectionAnchor() selectionAnchor {
	row, ok := model.selectedTableRow()
	if !ok {
		return selectionAnchor{}
	}
	if row.IsBranch() {
		return selectionAnchor{branch: row.Branch.Name, head: row.Branch.Head}
	}
	return selectionAnchor{path: row.Worktree.Path, branch: row.Worktree.Branch, head: row.Worktree.Head}
}

func (model *Model) restoreSelection(anchor selectionAnchor) bool {
	indexes := model.visibleIndexes()
	rows := model.tableRows()
	if len(indexes) == 0 {
		model.selected = 0
		return false
	}
	if anchor.path != "" {
		for visibleIndex, rowIndex := range indexes {
			row := rows[rowIndex]
			if row.IsWorktree() && row.Worktree.Path == anchor.path {
				model.selected = visibleIndex
				return true
			}
		}
	}
	if anchor.branch != "" || anchor.head != "" {
		for visibleIndex, rowIndex := range indexes {
			row := rows[rowIndex]
			if anchor.branch != "" && row.BranchName() == anchor.branch {
				model.selected = visibleIndex
				return true
			}
			if anchor.head != "" && row.Head() == anchor.head {
				model.selected = visibleIndex
				return true
			}
		}
	}
	model.selected = clamp(model.selected, 0, len(indexes)-1)
	return false
}

func (model *Model) selectMatching(match func(gitdata.Row) bool) {
	rows := model.tableRows()
	for visibleIndex, rowIndex := range model.visibleIndexes() {
		if match(rows[rowIndex]) {
			model.selected = visibleIndex
			return
		}
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
	model.state.Rows = github.AttachPullRequests(model.state.Rows, model.prCache, model.state.Repo.MainBranch)
	model.state.Branches = github.AttachBranchPullRequests(model.state.Branches, model.prCache, model.state.Repo.MainBranch)
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
	helpEntryWorktree
	helpEntryBranch
	helpEntryActive
	helpEntryLocked
	helpEntryPrunable
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
				{lead: "n", description: "new worktree", kind: helpEntryKey},
				{lead: "Tab", description: "filter", kind: helpEntryKey},
				{lead: "s", description: "search", kind: helpEntryKey},
				{lead: "b", description: "branches", kind: helpEntryKey},
				{lead: "u", description: "restore deleted branch", kind: helpEntryKey},
			},
		},
		{
			title: "Worktree Detail",
			entries: []helpEntry{
				{lead: "Enter", description: "go/create", kind: helpEntryKey},
				{lead: "c", description: "checkout root", kind: helpEntryKey},
				{lead: "o", description: "editor", kind: helpEntryKey},
				{lead: "d", description: "delete", kind: helpEntryKey},
				{lead: "y", description: "copy", kind: helpEntryKey},
				{lead: "p", description: "PR/branch", kind: helpEntryKey},
			},
		},
	}
}

func helpLegendSections() []helpSection {
	return []helpSection{
		{
			title: "Row Icons",
			entries: []helpEntry{
				{lead: "⌂", description: "root", kind: helpEntryRoot},
				{lead: "⊡", description: "worktree", kind: helpEntryWorktree},
				{lead: "⎇", description: "branch", kind: helpEntryBranch},
				{lead: "!", description: "locked", kind: helpEntryLocked},
				{lead: "×", description: "prunable", kind: helpEntryPrunable},
				{lead: "bold", description: "active row", kind: helpEntryActive},
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
				{lead: "⎇", description: "merged", kind: helpEntryMerged},
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
	case helpEntryRoot:
		return listview.RootTypeIconStyle()
	case helpEntryWorktree:
		return listview.WorktreeTypeIconStyle()
	case helpEntryBranch:
		return listview.BranchTypeIconStyle()
	case helpEntryPullRequest:
		return inspectorCommitStyle
	case helpEntryMerged:
		return mergedGlyphStyle
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
	return dialogBox("New worktree", lines, colorKeyHints("Enter create + go · Esc cancel", false), width)
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

func (model Model) renderFilterAtWidth(width int) string {
	dialog := model.filterDialog
	contentWidth := max(1, width-4)
	options := model.filterOptions()
	lines := make([]string, 0, len(options))
	if len(options) == 0 {
		lines = append(lines, hintStyle.Render("No filters"))
	} else {
		selected := clamp(dialog.selected, 0, len(options)-1)
		for index, option := range options {
			prefix := "  "
			if index == selected && option.enabled {
				prefix = "› "
			}
			line := filterOptionLine(prefix, option, contentWidth)
			if !option.enabled {
				line = hintStyle.Render(line)
			}
			if index == selected && option.enabled {
				line = paletteSelectedStyle.Render(padStyled(line, contentWidth))
			}
			lines = append(lines, line)
		}
	}
	return dialogBox("Filters", lines, filterDialogHintsAtWidth(width-6), width)
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

func filterOptionLine(prefix string, option filterOption, width int) string {
	label := option.filter.label()
	count := filterCountLabel(option.count)
	labelWidth := runewidth.StringWidth(prefix + label)
	countWidth := runewidth.StringWidth(count)
	gap := max(1, width-labelWidth-countWidth)
	return truncatePlain(prefix+label+strings.Repeat(" ", gap)+count, width)
}

func filterCountLabel(count int) string {
	if count == 1 {
		return "1 row"
	}
	return fmt.Sprintf("%d rows", count)
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

func checkoutDialogWidth(viewWidth int) int {
	return modalWidth(viewWidth, 76)
}

func paletteDialogWidth(viewWidth int) int {
	return modalWidth(viewWidth, 72)
}

func filterDialogWidth(viewWidth int) int {
	return modalWidth(viewWidth, 60)
}

func pullRequestDialogWidth(viewWidth int) int {
	return modalWidth(viewWidth, 76)
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

func filterDialogHintsAtWidth(width int) string {
	full := colorKeyHints("Enter apply · ↑/↓ move · Tab next · Esc cancel", false)
	if lipgloss.Width(full) <= width {
		return full
	}
	medium := colorKeyHints("Enter apply · Tab next · Esc", false)
	if lipgloss.Width(medium) <= width {
		return medium
	}
	short := colorKeyHints("Enter · Tab · Esc", false)
	if lipgloss.Width(short) <= width {
		return short
	}
	return ""
}

func pullRequestCheckoutHintsAtWidth(width int, loading bool) string {
	content := "Enter checkout · o open · ↑/↓ move · Esc cancel"
	if loading {
		content = "↑/↓ move · Esc cancel"
	}
	full := colorKeyHints(content, false)
	if lipgloss.Width(full) <= width {
		return full
	}
	short := "Enter · o · ↑/↓ · Esc"
	if loading {
		short = "↑/↓ · Esc"
	}
	short = colorKeyHints(short, false)
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
	haloTop := top
	haloBottom := min(height, top+len(popupLines))
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
	tableRows := model.tableRows()
	rows := make([]gitdata.Row, 0, len(tableRows))
	for _, index := range model.visibleIndexes() {
		rows = append(rows, tableRows[index])
	}
	return model.availableTableHeightForBlockLines(model.reservedDetailBlockLines(rows, now, panelContentWidth+2))
}

// reviewForRow returns the loaded PR review for the row's open pull request, or
// nil when none has been fetched.
func (model Model) reviewForRow(row gitdata.Row) *github.PullRequestReview {
	pullRequest := row.PullRequest()
	if pullRequest == nil || pullRequest.Number == 0 || model.prReview == nil {
		return nil
	}
	if review, ok := model.prReview[pullRequest.Number]; ok {
		return &review
	}
	return nil
}

// reviewPendingNumberForRow returns the PR number whose review detail is still
// being fetched for the row, or 0 when nothing is pending. It powers the PR review
// frame's loading state. A finished attempt (success or failure) leaves a prReview
// map entry, so it is no longer pending; this also keeps a failed or gh-less
// lookup from showing "loading" forever.
func (model Model) reviewPendingNumberForRow(row gitdata.Row) int {
	if !model.showPR {
		return 0
	}
	pullRequest := row.PullRequest()
	if pullRequest == nil || pullRequest.Number == 0 {
		return 0
	}
	if model.prReview != nil {
		if _, attempted := model.prReview[pullRequest.Number]; attempted {
			return 0
		}
	}
	return pullRequest.Number
}

// blockLinesTotal is the rendered height of a detail region; each block already
// includes its own top/bottom borders.
func blockLinesTotal(blocks []string) int {
	total := 0
	for _, block := range blocks {
		if block != "" {
			total += lineCount(block)
		}
	}
	return total
}

// reservedDetailBlockLines is the tallest detail region any row in the list would
// render. Sizing the list against this instead of the selected row's own blocks
// keeps the visible rows fixed while navigating; shorter rows pad the gap.
func (model Model) reservedDetailBlockLines(rows []gitdata.Row, now time.Time, panelWidth int) int {
	reserved := 0
	for _, row := range rows {
		reserved = max(reserved, blockLinesTotal(model.detailBlocks(row, now, panelWidth)))
	}
	return reserved
}

func (model Model) availableTableHeightForBlockLines(blockLines int) int {
	fixedLines := 1 + 2 + 1 + blockLines + 1
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
	rows := model.tableRowsForFilter(filter)
	indexes := make([]int, 0, len(rows))
	for index, row := range rows {
		branchMatches := pattern == "" || fuzzyMatch(row.DisplayBranch(), pattern)
		if branchMatches && filter.matches(row) {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func (model Model) tableRows() []gitdata.Row {
	return model.tableRowsForFilter(model.filter)
}

func (model Model) tableRowsForFilter(filter worktreeFilter) []gitdata.Row {
	return model.state.TableRows(model.showBranches || filter == filterBranches || filter == filterMerged)
}

func (model Model) selectedTableRow() (gitdata.Row, bool) {
	indexes := model.visibleIndexes()
	if len(indexes) == 0 || model.selected < 0 || model.selected >= len(indexes) {
		return gitdata.Row{}, false
	}
	rows := model.tableRows()
	return rows[indexes[model.selected]], true
}

func (model Model) selectedWorktree() (gitdata.Worktree, bool) {
	row, ok := model.selectedTableRow()
	if !ok || !row.IsWorktree() {
		return gitdata.Worktree{}, false
	}
	return row.Worktree, true
}

func (model Model) rootWorktree() (gitdata.Worktree, bool) {
	for _, row := range model.state.Rows {
		if row.IsMain || row.Path == model.state.Repo.Root || row.Path == model.state.Repo.MainWorktree {
			return row, true
		}
	}
	return gitdata.Worktree{}, false
}

func (model Model) selectedCopyText() (string, string, bool) {
	row, ok := model.selectedTableRow()
	if !ok {
		return "", "", false
	}
	if row.IsBranch() {
		name := row.Branch.Name
		if name == "" {
			return "", "", false
		}
		return name, "copied branch name: " + name, true
	}
	if row.Worktree.Path == "" {
		return "", "", false
	}
	return row.Worktree.Path, "copied absolute path: " + row.Worktree.Path, true
}

func (model Model) selectedPullRequestCopy() (string, string, bool) {
	row, ok := model.selectedTableRow()
	if !ok {
		return "", "", false
	}
	pr := row.PullRequest()
	if pr == nil || pr.URL == "" {
		return "", "", false
	}
	return pr.URL, "copied PR URL: " + pr.URL, true
}

func (model Model) selectedRow() (gitdata.Worktree, bool) {
	return model.selectedWorktree()
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
	// The PR fetch needs the local branch list to decide between per-branch and
	// list-wide queries, so defer it to localMetadataLoadedMsg when metadata is
	// still loading (branch rows are not populated yet).
	if model.shouldLoadPullRequests(forcePullRequests, now) && model.localMetadataReady() {
		if prCommand := model.pullRequestFetchCommand(ctx, id); prCommand != nil {
			commands = append(commands, prCommand)
		}
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

// pullRequestFetchCommand picks the PR loading strategy by local branch count.
// For a manageable number of branches it queries each branch directly (mapping
// plus CI in one fast call), which is far quicker than the list-wide query on
// large repos. Above the threshold the per-branch fan-out would issue too many
// requests, so it falls back to the single list call with lazy CI.
func (model Model) pullRequestFetchCommand(ctx context.Context, id int) tea.Cmd {
	repoRoot := model.state.Repo.Root
	runner := model.runner
	branches := model.localBranchNames()
	if len(branches) > 0 && len(branches) <= prPerBranchThreshold {
		return func() tea.Msg {
			return loadPullRequestsByBranchMsg(ctx, runner, repoRoot, id, branches)
		}
	}
	return func() tea.Msg {
		prContext, cancel := context.WithTimeout(ctx, prFetchTimeout)
		defer cancel()
		pullRequests, enabled := github.LoadPullRequests(prContext, repoRoot, runner)
		return prLoadedMsg{
			pullRequests: pullRequests,
			enabled:      enabled,
			repoRoot:     repoRoot,
			id:           id,
			checkedAt:    time.Now(),
		}
	}
}

func (model Model) localBranchNames() []string {
	seen := map[string]bool{}
	names := []string{}
	mainBranch := model.state.Repo.MainBranch
	add := func(name string) {
		// The main branch is never a PR head, so querying it is wasted work and
		// any same-named old/fork PR is dropped at attach time anyway.
		if name == "" || name == mainBranch || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, row := range model.state.Rows {
		if !row.Detached {
			add(row.Branch)
		}
	}
	for _, branch := range model.state.Branches {
		add(branch.Name)
	}
	return names
}

func loadPullRequestsByBranchMsg(ctx context.Context, runner gitdata.Runner, repoRoot string, id int, branches []string) tea.Msg {
	if !github.Available(ctx, repoRoot, runner) {
		return prLoadedMsg{repoRoot: repoRoot, id: id, checkedAt: time.Now()}
	}
	pullRequests := map[string]gitdata.PullRequest{}
	result := func() tea.Msg {
		return prLoadedMsg{pullRequests: pullRequests, enabled: true, ciIncluded: true, repoRoot: repoRoot, id: id, checkedAt: time.Now()}
	}
	jobChannel := make(chan string)
	var mutex sync.Mutex
	var waitGroup sync.WaitGroup
	workerCount := min(8, len(branches))
	for range workerCount {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for branch := range jobChannel {
				callContext, cancel := context.WithTimeout(ctx, prFetchTimeout)
				pullRequest, ok := github.LoadPullRequestForBranch(callContext, repoRoot, runner, branch)
				cancel()
				if ok {
					mutex.Lock()
					pullRequests[branch] = pullRequest
					mutex.Unlock()
				}
			}
		}()
	}
	for _, branch := range branches {
		select {
		case jobChannel <- branch:
		case <-ctx.Done():
			close(jobChannel)
			waitGroup.Wait()
			return result()
		}
	}
	close(jobChannel)
	waitGroup.Wait()
	return result()
}

// pullRequestCICommand fetches CI status for open PRs attached to local rows or
// branches. Scoping to attached PRs keeps the count bounded by local branches
// rather than the full repo, so the per-PR statusCheckRollup queries stay cheap.
func (model *Model) pullRequestCICommand(id int) tea.Cmd {
	if !model.showPR || model.enrichmentContext == nil {
		return nil
	}
	if model.prCIChecked == nil {
		model.prCIChecked = map[int]bool{}
	}
	numbers := []int{}
	add := func(pullRequest *gitdata.PullRequest) {
		if pullRequest == nil || pullRequest.Number == 0 {
			return
		}
		if !pullRequest.IsOpen() || model.prCIChecked[pullRequest.Number] {
			return
		}
		model.prCIChecked[pullRequest.Number] = true
		numbers = append(numbers, pullRequest.Number)
	}
	for index := range model.state.Rows {
		add(model.state.Rows[index].PR)
	}
	for index := range model.state.Branches {
		add(model.state.Branches[index].PR)
	}
	if len(numbers) == 0 {
		return nil
	}
	ctx := model.enrichmentContext
	runner := model.runner
	repoRoot := model.state.Repo.Root
	return func() tea.Msg {
		return loadPullRequestCIMsg(ctx, runner, repoRoot, id, numbers)
	}
}

// selectedReviewCommand lazily loads the detailed PR review for the selected row
// (checks, change requests, inline comments) when it has a PR not yet fetched,
// open or merged. It is cheap to call on every navigation: it returns nil when
// there is nothing to load.
func (model *Model) selectedReviewCommand(id int) tea.Cmd {
	if !model.showPR || model.enrichmentContext == nil {
		return nil
	}
	row, ok := model.selectedTableRow()
	if !ok {
		return nil
	}
	pullRequest := row.PullRequest()
	if pullRequest == nil || pullRequest.Number == 0 {
		return nil
	}
	if model.prReviewChecked == nil {
		model.prReviewChecked = map[int]bool{}
	}
	if model.prReviewChecked[pullRequest.Number] {
		return nil
	}
	model.prReviewChecked[pullRequest.Number] = true
	number := pullRequest.Number
	ctx := model.enrichmentContext
	runner := model.runner
	repoRoot := model.state.Repo.Root
	return func() tea.Msg {
		callContext, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		review, ok := github.LoadPullRequestReview(callContext, repoRoot, runner, number)
		if !ok {
			return prReviewLoadedMsg{number: number, repoRoot: repoRoot, id: id}
		}
		return prReviewLoadedMsg{number: number, review: review, repoRoot: repoRoot, id: id}
	}
}

// selectedBranchGraphCommand lazily loads the Git context graph for the selected
// branch-only row when it has not been fetched yet. It is loaded per selected row,
// not eagerly for every branch, since a repo can have many local branches and only
// the selected one is ever shown. Cheap to call on every navigation: returns nil when
// the selection is not a branch row or its graph is already loaded.
func (model *Model) selectedBranchGraphCommand(id int) tea.Cmd {
	if model.enrichmentContext == nil {
		return nil
	}
	row, ok := model.selectedTableRow()
	if !ok || !row.IsBranch() {
		return nil
	}
	name := row.Branch.Name
	if name == "" || name == model.state.Repo.MainBranch || row.Branch.Graph.Loaded {
		return nil
	}
	if model.branchGraphChecked == nil {
		model.branchGraphChecked = map[string]bool{}
	}
	if model.branchGraphChecked[name] {
		return nil
	}
	model.branchGraphChecked[name] = true
	ctx := model.enrichmentContext
	runner := model.runner
	repo := model.state.Repo
	return func() tea.Msg {
		callContext, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		graph := gitdata.LoadBranchContextGraph(callContext, repo, name, runner)
		return branchGraphLoadedMsg{name: name, graph: graph, repoRoot: repo.Root, id: id}
	}
}

func loadPullRequestCIMsg(ctx context.Context, runner gitdata.Runner, repoRoot string, id int, numbers []int) tea.Msg {
	ci := map[int]string{}
	if len(numbers) == 0 {
		return prCILoadedMsg{ci: ci, repoRoot: repoRoot, id: id}
	}
	jobChannel := make(chan int)
	var mutex sync.Mutex
	var waitGroup sync.WaitGroup
	workerCount := min(4, len(numbers))
	for range workerCount {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for number := range jobChannel {
				callContext, cancel := context.WithTimeout(ctx, 5*time.Second)
				glyph, ok := github.LoadPullRequestCI(callContext, repoRoot, runner, number)
				cancel()
				if ok {
					mutex.Lock()
					ci[number] = glyph
					mutex.Unlock()
				}
			}
		}()
	}
	for _, number := range numbers {
		select {
		case jobChannel <- number:
		case <-ctx.Done():
			close(jobChannel)
			waitGroup.Wait()
			return prCILoadedMsg{ci: ci, repoRoot: repoRoot, id: id}
		}
	}
	close(jobChannel)
	waitGroup.Wait()
	return prCILoadedMsg{ci: ci, repoRoot: repoRoot, id: id}
}

func (model Model) diskUsageCommand(ctx context.Context, now time.Time, id int) tea.Cmd {
	if !model.localMetadataReady() {
		return nil
	}
	visiblePaths, backgroundPaths := model.diskUsagePaths(now)
	fullPath := ""
	if row, ok := model.selectedWorktree(); ok && diskUsageFullEligible(row) {
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
	if !listview.ShowsGitSizeColumnWithPullRequests(model.tableContentWidth(), model.showPR) {
		return nil, nil
	}
	visible := model.visibleTableIndexes(now)
	tableRows := model.tableRows()
	seen := map[string]bool{}
	visiblePaths := make([]string, 0, len(visible))
	for _, rowIndex := range visible {
		row := tableRows[rowIndex]
		if !row.IsWorktree() {
			continue
		}
		if !diskUsageEligible(row.Worktree) {
			continue
		}
		seen[row.Worktree.Path] = true
		visiblePaths = append(visiblePaths, row.Worktree.Path)
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
	breakdowns := map[string]gitdata.DiskBreakdown{}
	if len(jobs) == 0 {
		return sizesLoadedMsg{gitSizes: gitSizes, fullSizes: fullSizes, breakdowns: breakdowns, repoRoot: repoRoot, id: id}
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
					if breakdown, err := gitdata.BucketedDiskUsage(ctx, job.path); err == nil {
						mutex.Lock()
						fullSizes[job.path] = breakdown.Total
						breakdowns[job.path] = breakdown
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
			return sizesLoadedMsg{gitSizes: gitSizes, fullSizes: fullSizes, breakdowns: breakdowns, repoRoot: repoRoot, id: id}
		}
	}
	close(jobChannel)
	waitGroup.Wait()
	return sizesLoadedMsg{gitSizes: gitSizes, fullSizes: fullSizes, breakdowns: breakdowns, repoRoot: repoRoot, id: id}
}

func reloadCmd(cwd string, config config.Config, runner gitdata.Runner, repo gitdata.Repository, fetch, automatic bool, id int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if fetch && repo.RemoteConfigured {
			if err := gitdata.FetchPrune(ctx, repo.Root, runner); err != nil {
				return reloadMsg{id: id, automatic: automatic, completedAt: time.Now(), err: fmt.Errorf("fetch failed: %w", err)}
			}
		}
		state, err := loadStableState(ctx, cwd, config, runner)
		if err != nil {
			return reloadMsg{id: id, automatic: automatic, completedAt: time.Now(), err: err}
		}
		repoConfig, hooksApproved, err := loadRepoRuntimeConfig(ctx, state.Repo.Root, runner)
		return reloadMsg{id: id, automatic: automatic, completedAt: time.Now(), state: state, repoConfig: repoConfig, hooksApproved: hooksApproved, err: err}
	}
}

func loadStableState(ctx context.Context, cwd string, config config.Config, runner gitdata.Runner) (gitdata.State, error) {
	state, err := gitdata.LoadSkeleton(ctx, cwd, config, runner)
	if err != nil {
		return gitdata.State{}, err
	}
	return gitdata.EnrichLocalMetadata(ctx, state, runner)
}

func loadRepoRuntimeConfig(ctx context.Context, repoRoot string, runner gitdata.Runner) (config.RepoConfig, bool, error) {
	if repoRoot == "" {
		return config.RepoConfig{}, true, nil
	}
	repoConfig, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		return config.RepoConfig{}, false, err
	}
	if !repoConfig.HasHooks() {
		return repoConfig, true, nil
	}
	if runner == nil {
		return repoConfig, false, nil
	}
	approvedHash, err := gitdata.ReadApprovedHash(ctx, repoRoot, runner)
	if err != nil {
		return repoConfig, false, err
	}
	return repoConfig, approvedHash == config.HookHash(repoConfig), nil
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

func persistShowBranchesCmd(showBranches bool) tea.Cmd {
	return func() tea.Msg {
		err := config.PatchDefault(func(config *config.Config) {
			config.ShowBranches = showBranches
		})
		return settingsSavedMsg{err: err}
	}
}

func copyTextCmd(text, message string) tea.Cmd {
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
		command.Stdin = strings.NewReader(text)
		err := command.Run()
		return actionMsg{text: message, err: err}
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
