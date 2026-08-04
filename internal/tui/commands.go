package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/github"
	"github.com/schovi/git-treehouse/internal/listview"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

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
	if !model.pullRequestsEnabled() {
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
	if !model.pullRequestsEnabled() {
		return nil
	}
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

func reloadCmd(cwd string, config config.Config, runner gitdata.Runner, repo gitdata.Repository, priorState gitdata.State, fetch, automatic bool, id int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if fetch && repo.RemoteConfigured {
			if err := gitdata.FetchPrune(ctx, repo.Root, runner); err != nil {
				return reloadMsg{id: id, automatic: automatic, completedAt: time.Now(), err: fmt.Errorf("fetch failed: %w", err)}
			}
		}
		var prior *gitdata.State
		if automatic {
			prior = &priorState
		}
		state, err := loadStableState(ctx, cwd, config, runner, repo.GitVersion, prior)
		if err != nil {
			return reloadMsg{id: id, automatic: automatic, completedAt: time.Now(), err: err}
		}
		repoConfig, hooksApproved, err := loadRepoRuntimeConfig(ctx, state.Repo.Root, runner)
		return reloadMsg{id: id, automatic: automatic, completedAt: time.Now(), state: state, repoConfig: repoConfig, hooksApproved: hooksApproved, err: err}
	}
}

func loadStableState(ctx context.Context, cwd string, config config.Config, runner gitdata.Runner, gitVersion string, priorState *gitdata.State) (gitdata.State, error) {
	state, err := gitdata.LoadSkeletonWithGitVersion(ctx, cwd, config, runner, gitVersion)
	if err != nil {
		return gitdata.State{}, err
	}
	if priorState != nil {
		return gitdata.EnrichLocalMetadataWithPriorState(ctx, state, *priorState, runner)
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
