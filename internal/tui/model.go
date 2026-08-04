package tui

import (
	"context"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/github"
	"strings"
	"time"
)

type Model struct {
	state                  gitdata.State
	config                 config.Config
	repoConfig             config.RepoConfig
	hooksApproved          bool
	noGitHub               bool
	noGitHubSet            bool
	runner                 gitdata.Runner
	width                  int
	height                 int
	selected               int
	detailHeightCache      *detailHeightCache
	filter                 worktreeFilter
	showBranches           bool
	searching              bool
	search                 textinput.Model
	help                   bool
	loading                string
	flash                  string
	flashID                int
	oldGitWarned           bool
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

type branchMetadataWarningMsg struct{}

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

func New(state gitdata.State, config config.Config, runner gitdata.Runner, noGitHub, noGitHubSet bool) Model {
	search := textinput.New()
	search.Prompt = "s "
	search.CharLimit = 200
	search.Width = 40
	enrichmentContext, enrichmentCancel := context.WithCancel(context.Background())
	repoConfig, hooksApproved, _ := loadRepoRuntimeConfig(context.Background(), state.Repo.Root, runner)
	githubEnabled := config.GitHub
	if noGitHubSet {
		githubEnabled = !noGitHub
	}
	return Model{
		state:             state,
		config:            config,
		repoConfig:        repoConfig,
		hooksApproved:     hooksApproved,
		noGitHub:          noGitHub,
		noGitHubSet:       noGitHubSet,
		runner:            runner,
		width:             100,
		height:            30,
		detailHeightCache: &detailHeightCache{},
		search:            search,
		showBranches:      config.ShowBranches,
		showPR:            state.Repo.RemoteConfigured && githubEnabled,
		prLoading:         state.Repo.RemoteConfigured && githubEnabled,
		lastRefreshAt:     time.Now(),
		enrichmentID:      1,
		enrichmentContext: enrichmentContext,
		enrichmentCancel:  enrichmentCancel,
	}
}

func (model Model) pullRequestsEnabled() bool {
	return model.githubEnabled() && model.state.Repo.RemoteConfigured
}

func (model Model) githubEnabled() bool {
	if model.noGitHubSet {
		return !model.noGitHub
	}
	return model.config.GitHub
}

func (model Model) Init() tea.Cmd {
	ctx := model.enrichmentContext
	if ctx == nil {
		ctx = context.Background()
	}
	commands := []tea.Cmd{model.enrichmentCommands(ctx, model.enrichmentID, false), clockTickCmd(model.lastRefreshAt), autoRefreshTickCmd()}
	if !gitdata.GitVersionSupportsBranchMetadata(model.state.Repo.GitVersion) {
		commands = append(commands, func() tea.Msg { return branchMetadataWarningMsg{} })
	}
	return tea.Batch(commands...)
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

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case branchMetadataWarningMsg:
		if model.oldGitWarned || gitdata.GitVersionSupportsBranchMetadata(model.state.Repo.GitVersion) {
			return model, nil
		}
		model.oldGitWarned = true
		return model.setFlash("Git < 2.41: branch-only rows, main sync, and merged-branch detection unavailable")
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
		if !model.githubEnabled() {
			return model, nil
		}
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
		model.showPR = model.pullRequestsEnabled()
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
			model.showPR = model.pullRequestsEnabled()
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
			model.showPR = model.pullRequestsEnabled()
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
		model.showPR = model.pullRequestsEnabled()
		model.prLoading = false
		model.applyCachedPullRequests()
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
