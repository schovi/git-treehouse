package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"

	"github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/github"
	"github.com/schovi/git-treehouse/internal/listview"
	"github.com/schovi/git-treehouse/internal/onboarding"
	"github.com/schovi/git-treehouse/internal/pathutil"
	"github.com/schovi/git-treehouse/internal/shellinit"
	"github.com/schovi/git-treehouse/internal/tui"
)

const (
	commandName         = "git-treehouse"
	defaultRepoPath     = "."
	shellIntegrationEnv = "GTH_SHELL_INTEGRATION"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	globalOptions, remaining, err := parseGlobalOptions(args)
	if err != nil {
		return err
	}
	if globalOptions.showHelp {
		fmt.Print(helpText())
		return nil
	}
	if len(remaining) > 0 {
		switch remaining[0] {
		case "help":
			return runHelp(remaining[1:])
		case "init":
			return runInit(remaining[1:])
		case "list":
			return runList(remaining[1:], globalOptions.repoPath)
		case "doctor":
			return runDoctor(remaining[1:], globalOptions.repoPath)
		case "allow":
			return runAllow(remaining[1:], globalOptions.repoPath)
		default:
			return fmt.Errorf("unknown command %q", remaining[0])
		}
	}
	return runTUI(globalOptions.cdFile, globalOptions.repoPath)
}

type globalOptions struct {
	cdFile   string
	repoPath string
	showHelp bool
}

func parseGlobalOptions(args []string) (globalOptions, []string, error) {
	flags := flag.NewFlagSet(commandName, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	cdFile := flags.String("cd-file", "", "write selected worktree path to file")
	repoPath := flags.String("repo", defaultRepoPath, "load repository from path")
	shortHelp := flags.Bool("h", false, "print help")
	longHelp := flags.Bool("help", false, "print help")
	if err := flags.Parse(args); err != nil {
		return globalOptions{}, nil, err
	}
	return globalOptions{
		cdFile:   *cdFile,
		repoPath: normalizeRepoPath(*repoPath),
		showHelp: *shortHelp || *longHelp,
	}, flags.Args(), nil
}

func normalizeRepoPath(repoPath string) string {
	if repoPath == "" {
		return defaultRepoPath
	}
	return pathutil.ExpandHome(repoPath)
}

func isHelpArg(arg string) bool {
	return arg == "-h" || arg == "--help" || arg == "-help"
}

func runHelp(args []string) error {
	if len(args) == 0 || len(args) == 1 && isHelpArg(args[0]) {
		fmt.Print(helpText())
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: %s help [list|init|doctor|allow]", commandName)
	}
	switch args[0] {
	case "list":
		fmt.Print(listHelpText())
		return nil
	case "init":
		fmt.Print(initHelpText())
		return nil
	case "doctor":
		fmt.Print(doctorHelpText())
		return nil
	case "allow":
		fmt.Print(allowHelpText())
		return nil
	default:
		return fmt.Errorf("usage: %s help [list|init|doctor|allow]", commandName)
	}
}

func helpText() string {
	shells := strings.Join(shellinit.SupportedShells(), "|")
	return fmt.Sprintf(`git-treehouse manages Git worktrees from a terminal UI.

Usage:
  %[1]s [--repo <path>] [--cd-file <path>]
  %[1]s list [--repo <path>] [--no-github] [--json]
  %[1]s init [%[2]s]
  %[1]s doctor [--repo <path>]
  %[1]s allow [--repo <path>]
  %[1]s help [list|init|doctor|allow]

What it can do:
  Browse worktrees with branch, dirty state, sync state, commit age, PR/CI, and size signals.
  Switch directories through gth shell integration, create worktrees, open editors, copy paths, and delete with safeguards.
  Print plain tables or JSON for scripts, generate shell integration, and diagnose setup.

Commands:
  list     Print worktrees without launching the TUI.
  init     Print shell integration that defines gth.
  doctor   Check Git, config, GitHub CLI, shell, editor, and clipboard setup.
  allow    Approve executable hooks from the repo .worktree file.
  help     Print this help.

Options:
  --repo <path>     Load a repository or worktree path. Default: current directory.
  --cd-file <path>  Write the selected worktree path for shell integration.
  -h, --help        Print this help.

Examples:
  %[1]s
  gth
  %[1]s list --no-github
  %[1]s list --json --repo ~/code/project-worktree
  eval "$(%[1]s init zsh)"
`, commandName, shells)
}

func initHelpText() string {
	shells := strings.Join(shellinit.SupportedShells(), "|")
	return fmt.Sprintf(`git-treehouse init prints shell integration that defines gth.

Usage:
  %[1]s init [%[2]s]

Examples:
  eval "$(%[1]s init zsh)"
  %[1]s init fish | source
`, commandName, shells)
}

func runInit(args []string) error {
	if len(args) == 1 && isHelpArg(args[0]) {
		fmt.Print(initHelpText())
		return nil
	}
	if len(args) > 1 {
		return fmt.Errorf("usage: %s init [%s]", commandName, strings.Join(shellinit.SupportedShells(), "|"))
	}
	shell := ""
	if len(args) == 1 {
		shell = args[0]
	} else {
		shell = currentShell()
		if shell == "" {
			return fmt.Errorf("usage: %s init %s", commandName, strings.Join(shellinit.SupportedShells(), "|"))
		}
	}
	script, err := shellinit.Script(shell)
	if err != nil {
		return err
	}
	fmt.Print(script)
	return nil
}

func runList(args []string, repoPath string) error {
	options, err := parseListOptions(args, repoPath)
	if err != nil {
		return err
	}
	if options.showHelp {
		fmt.Print(listHelpText())
		return nil
	}
	config, err := config.LoadDefault()
	if err != nil {
		return err
	}
	runner := gitdata.ExecRunner{}
	tty := stdoutIsTTY()
	width := terminalWidth(100)
	state, err := gitdata.Load(context.Background(), options.repoPath, config, runner)
	if err != nil {
		return err
	}
	enrichmentContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	showPullRequestColumn := !options.noGitHub && state.Repo.RemoteConfigured && listview.ShowsPullRequestColumn(width)
	showGitSizeColumn := listview.ShowsGitSizeColumn(width) || listview.ShowsGitSizeColumnWithPullRequests(width, showPullRequestColumn)
	showPR, prPending := enrichListState(enrichmentContext, &state, runner, listEnrichmentOptions{
		LoadPullRequests: !options.noGitHub && (options.jsonOutput || showPullRequestColumn),
		LoadGitSize:      options.jsonOutput || showGitSizeColumn,
		LoadFullSize:     options.jsonOutput,
	})
	if options.jsonOutput {
		output, err := json.MarshalIndent(listJSONFromState(state, time.Now()), "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(output))
		return nil
	}
	output := listview.Render(state, listview.Options{
		Width:      width,
		Color:      tty,
		Hyperlinks: tty,
		ShowHeader: true,
		ShowPR:     showPR,
		Pending:    listview.LoadingPlaceholder,
		PRPending:  prPending,
	}, time.Now())
	fmt.Println(output)
	return nil
}

type listOptions struct {
	noGitHub   bool
	jsonOutput bool
	repoPath   string
	showHelp   bool
}

func parseListOptions(args []string, repoPath string) (listOptions, error) {
	flags := flag.NewFlagSet(commandName+" list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	noGitHub := flags.Bool("no-github", false, "skip GitHub PR lookup")
	jsonOutput := flags.Bool("json", false, "print structured JSON")
	selectedRepoPath := flags.String("repo", repoPath, "load repository from path")
	shortHelp := flags.Bool("h", false, "print help")
	longHelp := flags.Bool("help", false, "print help")
	if err := flags.Parse(args); err != nil {
		return listOptions{}, err
	}
	return listOptions{
		noGitHub:   *noGitHub,
		jsonOutput: *jsonOutput,
		repoPath:   normalizeRepoPath(*selectedRepoPath),
		showHelp:   *shortHelp || *longHelp,
	}, nil
}

func listHelpText() string {
	return fmt.Sprintf(`git-treehouse list prints worktrees without launching the TUI.

Usage:
  %[1]s list [--repo <path>] [--no-github] [--json]

Options:
  --repo <path>  Load a repository or worktree path. Default: current directory.
  --no-github    Skip GitHub PR lookup.
  --json         Print structured JSON.
  -h, --help     Print this help.
`, commandName)
}

type listEnrichmentOptions struct {
	LoadPullRequests bool
	LoadGitSize      bool
	LoadFullSize     bool
}

type listSizeResult struct {
	path           string
	gitSize        int64
	gitSizeLoaded  bool
	fullSize       int64
	fullSizeLoaded bool
}

func enrichListState(ctx context.Context, state *gitdata.State, runner gitdata.Runner, options listEnrichmentOptions) (bool, bool) {
	type pullRequestResult struct {
		showPR       bool
		prPending    bool
		pullRequests map[string]gitdata.PullRequest
	}
	results := make(chan any, 2)
	var waitGroup sync.WaitGroup
	if options.LoadPullRequests {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			result := pullRequestResult{}
			if github.Available(ctx, state.Repo.Root, runner) {
				result.showPR = true
				pullRequests, enabled := github.LoadPullRequestsFromAuthenticatedCLI(ctx, state.Repo.Root, runner)
				if enabled {
					result.pullRequests = pullRequests
				} else {
					result.prPending = true
				}
			}
			select {
			case results <- result:
			case <-ctx.Done():
			}
		}()
	}
	if options.LoadGitSize || options.LoadFullSize {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for _, result := range loadListSizes(ctx, *state, runner, options) {
				select {
				case results <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	done := make(chan struct{})
	go func() {
		waitGroup.Wait()
		close(done)
	}()
	showPR := false
	prPending := false
	for {
		select {
		case raw := <-results:
			switch result := raw.(type) {
			case pullRequestResult:
				showPR = result.showPR
				prPending = result.prPending
				if len(result.pullRequests) > 0 {
					state.Rows = github.AttachPullRequests(state.Rows, result.pullRequests)
				}
			case listSizeResult:
				applyListSizeResult(state, result)
			}
		case <-done:
			return showPR, prPending
		case <-ctx.Done():
			return showPR, prPending
		}
	}
}

type listJSON struct {
	Repository listJSONRepository `json:"repository"`
	Worktrees  []listJSONWorktree `json:"worktrees"`
}

type listJSONRepository struct {
	Root             string `json:"root"`
	CommonGitDir     string `json:"common_git_dir"`
	Cwd              string `json:"cwd"`
	ActiveWorktree   string `json:"active_worktree"`
	MainWorktree     string `json:"main_worktree"`
	MainBranch       string `json:"main_branch"`
	Parent           string `json:"parent"`
	RemoteConfigured bool   `json:"remote_configured"`
}

type listJSONWorktree struct {
	Path               string               `json:"path"`
	GitDir             string               `json:"git_dir"`
	Head               string               `json:"head"`
	Branch             string               `json:"branch,omitempty"`
	DisplayBranch      string               `json:"display_branch"`
	Marker             string               `json:"marker,omitempty"`
	Detached           bool                 `json:"detached"`
	Locked             bool                 `json:"locked"`
	LockReason         string               `json:"lock_reason,omitempty"`
	Prunable           bool                 `json:"prunable"`
	PruneReason        string               `json:"prune_reason,omitempty"`
	Active             bool                 `json:"active"`
	Main               bool                 `json:"main"`
	Status             listJSONStatus       `json:"status"`
	Upstream           string               `json:"upstream,omitempty"`
	UpstreamGone       bool                 `json:"upstream_gone"`
	RemoteSync         listJSONSync         `json:"remote_sync"`
	MainSync           listJSONSync         `json:"main_sync"`
	Commit             listJSONCommit       `json:"commit"`
	BranchMergedToMain bool                 `json:"branch_merged_to_main"`
	PullRequest        *listJSONPullRequest `json:"pull_request,omitempty"`
	GitSize            listJSONSize         `json:"git_size"`
	FullSize           listJSONSize         `json:"full_size"`
	Size               listJSONSize         `json:"size"`
}

type listJSONStatus struct {
	Clean     bool   `json:"clean"`
	Staged    int    `json:"staged"`
	Modified  int    `json:"modified"`
	Untracked int    `json:"untracked"`
	Text      string `json:"text"`
	Compact   string `json:"compact"`
}

type listJSONSync struct {
	Available  bool   `json:"available"`
	NoUpstream bool   `json:"no_upstream"`
	Ahead      int    `json:"ahead"`
	Behind     int    `json:"behind"`
	Text       string `json:"text"`
}

type listJSONCommit struct {
	Short   string `json:"short,omitempty"`
	Subject string `json:"subject,omitempty"`
	Time    string `json:"time,omitempty"`
	Age     string `json:"age,omitempty"`
}

type listJSONPullRequest struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	CI     string `json:"ci,omitempty"`
	URL    string `json:"url,omitempty"`
	Text   string `json:"text"`
}

type listJSONSize struct {
	Loaded bool  `json:"loaded"`
	Bytes  int64 `json:"bytes,omitempty"`
}

func listJSONFromState(state gitdata.State, now time.Time) listJSON {
	output := listJSON{
		Repository: listJSONRepository{
			Root:             state.Repo.Root,
			CommonGitDir:     state.Repo.CommonGitDir,
			Cwd:              state.Repo.Cwd,
			ActiveWorktree:   state.Repo.ActiveWorktree,
			MainWorktree:     state.Repo.MainWorktree,
			MainBranch:       state.Repo.MainBranch,
			Parent:           state.Repo.Parent,
			RemoteConfigured: state.Repo.RemoteConfigured,
		},
		Worktrees: make([]listJSONWorktree, 0, len(state.Rows)),
	}
	for _, row := range state.Rows {
		worktree := listJSONWorktree{
			Path:          row.Path,
			GitDir:        row.GitDir,
			Head:          row.Head,
			Branch:        row.Branch,
			DisplayBranch: row.DisplayBranch(),
			Marker:        row.Marker(),
			Detached:      row.Detached,
			Locked:        row.Locked,
			LockReason:    row.LockReason,
			Prunable:      row.Prunable,
			PruneReason:   row.PruneReason,
			Active:        row.IsActive,
			Main:          row.IsMain,
			Status: listJSONStatus{
				Clean:     row.Status.Clean(),
				Staged:    row.Status.Staged,
				Modified:  row.Status.Modified,
				Untracked: row.Status.Untracked,
				Text:      row.Status.Text(),
				Compact:   row.StatusText(),
			},
			Upstream:           row.Upstream,
			UpstreamGone:       row.UpstreamGone,
			RemoteSync:         syncJSON(row.HeadSync, row.HeadSync.RemoteCompact(row.UpstreamGone)),
			MainSync:           syncJSON(row.MainSync, row.MainSync.Compact()),
			BranchMergedToMain: row.BranchMergedToMain,
			GitSize:            gitSizeJSON(row),
			FullSize:           fullSizeJSON(row),
			Size:               fullSizeJSON(row),
		}
		if !row.CommitTime.IsZero() {
			worktree.Commit.Time = row.CommitTime.Format(time.RFC3339)
			worktree.Commit.Age = gitdata.RelativeAge(now, row.CommitTime)
		}
		worktree.Commit.Short = row.CommitShort
		worktree.Commit.Subject = row.CommitSubject
		if row.PR != nil {
			worktree.PullRequest = &listJSONPullRequest{
				Number: row.PR.Number,
				State:  row.PR.State,
				CI:     row.PR.CI,
				URL:    row.PR.URL,
				Text:   row.PR.Text(),
			}
		}
		output.Worktrees = append(output.Worktrees, worktree)
	}
	return output
}

func gitSizeJSON(row gitdata.Worktree) listJSONSize {
	if row.GitSizeLoaded {
		return listJSONSize{Loaded: true, Bytes: row.GitSizeBytes}
	}
	if row.SizeLoaded {
		return listJSONSize{Loaded: true, Bytes: row.SizeBytes}
	}
	return listJSONSize{}
}

func fullSizeJSON(row gitdata.Worktree) listJSONSize {
	size, loaded := row.FullSize()
	if !loaded {
		return listJSONSize{}
	}
	return listJSONSize{Loaded: true, Bytes: size}
}

func syncJSON(sync gitdata.SyncState, text string) listJSONSync {
	return listJSONSync{
		Available:  sync.Available,
		NoUpstream: sync.NoUpstream,
		Ahead:      sync.Ahead,
		Behind:     sync.Behind,
		Text:       text,
	}
}

type doctorStatus string

const (
	doctorOK      doctorStatus = "ok"
	doctorWarning doctorStatus = "warn"
	doctorError   doctorStatus = "error"
)

type doctorCheck struct {
	Name    string
	Status  doctorStatus
	Message string
}

func runDoctor(args []string, repoPath string) error {
	flags := flag.NewFlagSet(commandName+" doctor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	selectedRepoPath := flags.String("repo", repoPath, "load repository from path")
	shortHelp := flags.Bool("h", false, "print help")
	longHelp := flags.Bool("help", false, "print help")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *shortHelp || *longHelp {
		fmt.Print(doctorHelpText())
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: %s doctor [--repo <path>]", commandName)
	}
	runner := gitdata.ExecRunner{}
	checks := doctorChecks(context.Background(), runner, normalizeRepoPath(*selectedRepoPath))
	fmt.Print(formatDoctorChecks(checks))
	return nil
}

func doctorHelpText() string {
	return fmt.Sprintf(`git-treehouse doctor checks Git, config, GitHub CLI, shell, editor, and clipboard setup.

Usage:
  %[1]s doctor [--repo <path>]

Options:
  --repo <path>  Load a repository or worktree path. Default: current directory.
  -h, --help     Print this help.
`, commandName)
}

func runAllow(args []string, repoPath string) error {
	flags := flag.NewFlagSet(commandName+" allow", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	selectedRepoPath := flags.String("repo", repoPath, "load repository from path")
	shortHelp := flags.Bool("h", false, "print help")
	longHelp := flags.Bool("help", false, "print help")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *shortHelp || *longHelp {
		fmt.Print(allowHelpText())
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: %s allow [--repo <path>]", commandName)
	}
	loadedConfig, err := config.LoadDefault()
	if err != nil {
		return err
	}
	return allowRepoHooks(context.Background(), gitdata.ExecRunner{}, normalizeRepoPath(*selectedRepoPath), loadedConfig, os.Stdout)
}

func allowHelpText() string {
	return fmt.Sprintf(`git-treehouse allow approves executable hooks from the repo .worktree file.

Usage:
  %[1]s allow [--repo <path>]

Options:
  --repo <path>  Load a repository or worktree path. Default: current directory.
  -h, --help     Print this help.
`, commandName)
}

func allowRepoHooks(ctx context.Context, runner gitdata.Runner, repoPath string, loadedConfig config.Config, output io.Writer) error {
	repo, err := gitdata.ResolveRepository(ctx, repoPath, loadedConfig, runner)
	if err != nil {
		return err
	}
	repoConfig, err := config.LoadRepoConfig(repo.Root)
	if err != nil {
		return err
	}
	if !repoConfig.HasHooks() {
		_, err := fmt.Fprintln(output, "no hooks defined in .worktree; nothing to approve")
		return err
	}
	if repoConfig.PostCreate != "" {
		if _, err := fmt.Fprintln(output, "post_create: "+repoConfig.PostCreate); err != nil {
			return err
		}
	}
	if repoConfig.BeforeDelete != "" {
		if _, err := fmt.Fprintln(output, "before_delete: "+repoConfig.BeforeDelete); err != nil {
			return err
		}
	}
	if err := gitdata.WriteApprovedHash(ctx, repo.Root, config.HookHash(repoConfig), runner); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, "approved hooks for "+repo.Root)
	return err
}

func doctorChecks(ctx context.Context, runner gitdata.Runner, repoPath string) []doctorCheck {
	checks := []doctorCheck{
		doctorGit(ctx, runner),
	}
	configPath, configCheck, loadedConfig := doctorConfig()
	checks = append(checks, configCheck)
	repoCheck, repo := doctorRepository(ctx, repoPath, loadedConfig, runner)
	checks = append(checks, repoCheck)
	if repo.Root != "" {
		checks = append(checks,
			doctorWorktreeFile(repo.Root),
			doctorWorktreeApproval(ctx, repo.Root, runner),
		)
	}
	checks = append(checks,
		doctorGitHub(ctx, repo.Root, runner),
		doctorShellIntegration(),
		doctorEditor(loadedConfig),
		doctorClipboard(),
	)
	if configPath != "" {
		checks = append(checks, doctorCheck{Name: "config path", Status: doctorOK, Message: configPath})
	}
	return checks
}

func doctorGit(ctx context.Context, runner gitdata.Runner) doctorCheck {
	path, err := exec.LookPath("git")
	if err != nil {
		return doctorCheck{Name: "git", Status: doctorError, Message: "git was not found on PATH"}
	}
	output, err := runner.Run(ctx, ".", "git", "--version")
	if err != nil {
		return doctorCheck{Name: "git", Status: doctorError, Message: err.Error()}
	}
	return doctorCheck{Name: "git", Status: doctorOK, Message: strings.TrimSpace(string(output)) + " at " + path}
}

func doctorConfig() (string, doctorCheck, config.Config) {
	loadedConfig := config.Default()
	path, pathErr := config.Path()
	if pathErr != nil {
		return "", doctorCheck{Name: "config", Status: doctorWarning, Message: pathErr.Error()}, loadedConfig
	}
	if _, err := os.Stat(path); err != nil {
		return path, doctorCheck{Name: "config", Status: doctorOK, Message: "using defaults"}, loadedConfig
	}
	loaded, err := config.Load(path)
	if err != nil {
		return path, doctorCheck{Name: "config", Status: doctorError, Message: err.Error()}, loadedConfig
	}
	return path, doctorCheck{Name: "config", Status: doctorOK, Message: "loaded config.toml"}, loaded
}

func doctorRepository(ctx context.Context, repoPath string, loadedConfig config.Config, runner gitdata.Runner) (doctorCheck, gitdata.Repository) {
	repo, err := gitdata.ResolveRepository(ctx, repoPath, loadedConfig, runner)
	if err != nil {
		return doctorCheck{Name: "repository", Status: doctorWarning, Message: "not inside a git repository"}, gitdata.Repository{}
	}
	return doctorCheck{Name: "repository", Status: doctorOK, Message: repo.Root + " (main: " + repo.MainBranch + ")"}, repo
}

func doctorWorktreeFile(repoRoot string) doctorCheck {
	path := filepath.Join(repoRoot, ".worktree")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return doctorCheck{Name: ".worktree", Status: doctorOK, Message: "no .worktree file"}
		}
		return doctorCheck{Name: ".worktree", Status: doctorWarning, Message: err.Error()}
	}
	repoConfig, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		return doctorCheck{Name: ".worktree", Status: doctorError, Message: err.Error()}
	}
	keys := make([]string, 0, 4)
	if repoConfig.PathTemplate != "" {
		keys = append(keys, "path_template override")
	}
	if len(repoConfig.CopyUntracked) != 0 {
		keys = append(keys, fmt.Sprintf("%d copy_untracked entries", len(repoConfig.CopyUntracked)))
	}
	if repoConfig.PostCreate != "" {
		keys = append(keys, "post_create hook")
	}
	if repoConfig.BeforeDelete != "" {
		keys = append(keys, "before_delete hook")
	}
	if len(keys) == 0 {
		keys = append(keys, "no recognized keys")
	}
	return doctorCheck{Name: ".worktree", Status: doctorOK, Message: strings.Join(keys, ", ")}
}

func doctorWorktreeApproval(ctx context.Context, repoRoot string, runner gitdata.Runner) doctorCheck {
	repoConfig, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		return doctorCheck{Name: "hook approval", Status: doctorError, Message: err.Error()}
	}
	if !repoConfig.HasHooks() {
		return doctorCheck{Name: "hook approval", Status: doctorOK, Message: "no hooks"}
	}
	approvedHash, err := gitdata.ReadApprovedHash(ctx, repoRoot, runner)
	if err != nil {
		return doctorCheck{Name: "hook approval", Status: doctorWarning, Message: err.Error()}
	}
	currentHash := config.HookHash(repoConfig)
	switch approvedHash {
	case currentHash:
		return doctorCheck{Name: "hook approval", Status: doctorOK, Message: "approved"}
	case "":
		return doctorCheck{Name: "hook approval", Status: doctorWarning, Message: "not approved, run git-treehouse allow"}
	default:
		return doctorCheck{Name: "hook approval", Status: doctorWarning, Message: "changed since approval, re-run git-treehouse allow"}
	}
}

func doctorGitHub(ctx context.Context, repoRoot string, runner gitdata.Runner) doctorCheck {
	if _, err := exec.LookPath("gh"); err != nil {
		return doctorCheck{Name: "github", Status: doctorWarning, Message: "gh was not found; PR column will stay hidden"}
	}
	dir := repoRoot
	if dir == "" {
		dir = "."
	}
	if _, err := runner.Run(ctx, dir, "gh", "auth", "status"); err != nil {
		return doctorCheck{Name: "github", Status: doctorWarning, Message: "gh is installed but not authenticated"}
	}
	return doctorCheck{Name: "github", Status: doctorOK, Message: "gh is installed and authenticated"}
}

func doctorShellIntegration() doctorCheck {
	shell := currentShell()
	if shell == "" {
		return doctorCheck{Name: "shell", Status: doctorWarning, Message: "could not detect shell"}
	}
	if shellinit.ConfigFileContainsIntegration(shell) {
		return doctorCheck{Name: "shell", Status: doctorOK, Message: shell + " integration is installed"}
	}
	return doctorCheck{Name: "shell", Status: doctorWarning, Message: shell + " integration not found; run " + shellinit.InstallCommand(shell)}
}

func doctorEditor(loadedConfig config.Config) doctorCheck {
	editor := strings.TrimSpace(loadedConfig.Editor)
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		editor = "code"
	}
	command := strings.Fields(editor)
	if len(command) == 0 {
		return doctorCheck{Name: "editor", Status: doctorWarning, Message: "editor is empty"}
	}
	if path, err := exec.LookPath(command[0]); err == nil {
		return doctorCheck{Name: "editor", Status: doctorOK, Message: editor + " at " + path}
	}
	return doctorCheck{Name: "editor", Status: doctorWarning, Message: editor + " was not found on PATH"}
}

func doctorClipboard() doctorCheck {
	for _, command := range clipboardCommandsForRuntime(runtime.GOOS) {
		if path, err := exec.LookPath(command); err == nil {
			return doctorCheck{Name: "clipboard", Status: doctorOK, Message: command + " at " + path}
		}
	}
	return doctorCheck{Name: "clipboard", Status: doctorWarning, Message: "no supported clipboard command found"}
}

func clipboardCommandsForRuntime(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"pbcopy"}
	case "windows":
		return []string{"clip", "powershell.exe", "pwsh"}
	default:
		return []string{"wl-copy", "xclip", "xsel"}
	}
}

func formatDoctorChecks(checks []doctorCheck) string {
	var builder strings.Builder
	builder.WriteString("git-treehouse doctor\n")
	for _, check := range checks {
		fmt.Fprintf(&builder, "%-5s %-12s %s\n", check.Status, check.Name+":", check.Message)
	}
	return builder.String()
}

func loadListSizes(ctx context.Context, state gitdata.State, runner gitdata.Runner, options listEnrichmentOptions) []listSizeResult {
	paths := make([]string, 0, len(state.Rows))
	for _, row := range state.Rows {
		if !row.Prunable {
			paths = append(paths, row.Path)
		}
	}
	results := make(chan listSizeResult, len(paths))
	jobs := make(chan string)
	var waitGroup sync.WaitGroup
	workerCount := min(2, max(1, len(paths)))
	for range workerCount {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for path := range jobs {
				result := listSizeResult{path: path}
				if options.LoadGitSize {
					if size, err := gitdata.GitAwareDiskUsage(ctx, path, runner); err == nil {
						result.gitSize = size
						result.gitSizeLoaded = true
					}
				}
				if options.LoadFullSize {
					if size, err := gitdata.FullDiskUsage(ctx, path); err == nil {
						result.fullSize = size
						result.fullSizeLoaded = true
					}
				}
				select {
				case results <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, path := range paths {
			select {
			case jobs <- path:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		waitGroup.Wait()
		close(results)
	}()
	loaded := make([]listSizeResult, 0, len(paths))
	for {
		select {
		case result, ok := <-results:
			if !ok {
				return loaded
			}
			loaded = append(loaded, result)
		case <-ctx.Done():
			return loaded
		}
	}
}

func applyListSizeResult(state *gitdata.State, result listSizeResult) {
	for index := range state.Rows {
		if state.Rows[index].Path != result.path {
			continue
		}
		if result.gitSizeLoaded {
			state.Rows[index].GitSizeBytes = result.gitSize
			state.Rows[index].GitSizeLoaded = true
		}
		if result.fullSizeLoaded {
			state.Rows[index].FullSizeBytes = result.fullSize
			state.Rows[index].FullSizeLoaded = true
			state.Rows[index].SizeBytes = result.fullSize
			state.Rows[index].SizeLoaded = true
		}
		return
	}
}

func runTUI(cdFile, repoPath string) error {
	config, err := config.LoadDefault()
	if err != nil {
		return err
	}
	shell := currentShell()
	if shouldShowShellWelcome(cdFile, config, stdoutIsTTY(), os.Getenv(shellIntegrationEnv), shell) {
		if err := runShellWelcome(shell); err != nil {
			return err
		}
	}
	runner := gitdata.ExecRunner{}
	state, err := gitdata.LoadSkeleton(context.Background(), repoPath, config, runner)
	if err != nil {
		return err
	}
	program := tea.NewProgram(tui.New(state, config, runner), tea.WithOutput(os.Stderr), tea.WithAltScreen())
	finalModel, err := program.Run()
	if err != nil {
		return err
	}
	model, ok := finalModel.(tui.Model)
	if !ok {
		return nil
	}
	selectedPath := model.SelectedPath()
	if selectedPath == "" {
		return nil
	}
	if cdFile != "" {
		return os.WriteFile(cdFile, []byte(selectedPath), 0600) // #nosec G703 -- --cd-file is an explicit shell-integration path created by the wrapper.
	}
	if stdoutIsTTY() {
		shell := currentShell()
		fmt.Fprintln(os.Stderr, pathSelectionHint(selectedPath, shell, shellinit.ConfigFileContainsIntegration(shell))) // #nosec G705 -- This is terminal text, not HTML output.
		return nil
	}
	fmt.Println(selectedPath)
	return nil
}

func shouldShowShellWelcome(cdFile string, config config.Config, stdoutTTY bool, integration, shell string) bool {
	return cdFile == "" &&
		integration == "" &&
		stdoutTTY &&
		!config.SkipShellIntegrationWelcome &&
		shellinit.Normalize(shell) != ""
}

func runShellWelcome(shell string) error {
	shell = shellinit.Normalize(shell)
	path, err := shellinit.ConfigPath(shell)
	if err != nil {
		return err
	}
	result, err := onboarding.Run(os.Stderr, onboarding.Info{
		Shell:             shell,
		ActivationCommand: shellinit.ActivationCommand(shell),
		InstallPath:       path,
		ReloadCommand:     shellinit.ReloadCommand(shell, path),
	})
	if err != nil {
		return err
	}
	switch result.Action {
	case onboarding.ActionInstall:
		install, err := shellinit.Install(shell)
		if err != nil {
			return err
		}
		if err := config.PatchDefault(func(config *config.Config) {
			config.SkipShellIntegrationWelcome = true
		}); err != nil {
			return err
		}
		if install.AlreadyInstalled {
			fmt.Fprintf(os.Stderr, "gth shell integration is already installed in %s.\nReload with: %s\n\n", install.Path, install.ReloadCommand)
		} else {
			fmt.Fprintf(os.Stderr, "Installed gth shell integration in %s.\nReload with: %s\n\n", install.Path, install.ReloadCommand)
		}
	case onboarding.ActionSkip:
		return config.PatchDefault(func(config *config.Config) {
			config.SkipShellIntegrationWelcome = true
		})
	}
	return nil
}

func pathSelectionHint(selectedPath, shell string, integrationInstalled bool) string {
	shell = shellinit.Normalize(shell)
	const nativeCommandHint = "Use git-treehouse for native commands like:\n  git-treehouse list"
	if shell == "" {
		return fmt.Sprintf("Selected %s\nA standalone %s process cannot change your shell directory.\nUse gth for directory-changing runs after shell integration is installed.\nInstall it with: %s init %s\nThen run:\n  gth\n%s", selectedPath, commandName, commandName, strings.Join(shellinit.SupportedShells(), "|"), nativeCommandHint)
	}
	if integrationInstalled {
		return fmt.Sprintf("Selected %s\nA standalone %s process cannot change your shell directory.\nShell integration appears installed in your config. Reload your shell if needed.\nRun the smart wrapper instead:\n  gth\n%s", selectedPath, commandName, nativeCommandHint)
	}
	return fmt.Sprintf("Selected %s\nA standalone %s process cannot change your shell directory.\nInstall shell integration, then use the smart wrapper:\n  %s\n  gth\nPersist it with:\n  %s\n%s", selectedPath, commandName, shellinit.ActivationCommand(shell), shellinit.InstallCommand(shell), nativeCommandHint)
}

func detectShell(shellPath string) string {
	return shellinit.Normalize(shellPath)
}

func currentShell() string {
	if shell := detectShell(parentProcessName()); shell != "" {
		return shell
	}
	if shell := detectShell(os.Getenv("SHELL")); shell != "" {
		return shell
	}
	if os.Getenv("PSModulePath") != "" {
		return "powershell"
	}
	return ""
}

func parentProcessName() string {
	output, err := exec.Command("ps", "-p", strconv.Itoa(os.Getppid()), "-o", "comm=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func stdoutIsTTY() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func terminalWidth(fallback int) int {
	if stdoutIsTTY() {
		width, _, err := term.GetSize(os.Stdout.Fd())
		if err == nil && width > 0 {
			return width
		}
	}
	value, err := strconv.Atoi(os.Getenv("COLUMNS"))
	if err == nil && value > 0 {
		return value
	}
	return fallback
}
