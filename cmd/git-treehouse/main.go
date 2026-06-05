package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/github"
	"github.com/schovi/git-treehouse/internal/listview"
	"github.com/schovi/git-treehouse/internal/onboarding"
	"github.com/schovi/git-treehouse/internal/shellinit"
	"github.com/schovi/git-treehouse/internal/tui"
)

const (
	commandName         = "git-treehouse"
	shellIntegrationEnv = "GTH_SHELL_INTEGRATION"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	globalFlags := flag.NewFlagSet(commandName, flag.ContinueOnError)
	globalFlags.SetOutput(os.Stderr)
	cdFile := globalFlags.String("cd-file", "", "write selected worktree path to file")
	if err := globalFlags.Parse(args); err != nil {
		return err
	}
	remaining := globalFlags.Args()
	if len(remaining) > 0 {
		switch remaining[0] {
		case "init":
			return runInit(remaining[1:])
		case "list":
			return runList(remaining[1:])
		default:
			return fmt.Errorf("unknown command %q", remaining[0])
		}
	}
	return runTUI(*cdFile)
}

func runInit(args []string) error {
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

func runList(args []string) error {
	flags := flag.NewFlagSet(commandName+" list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	noGitHub := flags.Bool("no-github", false, "skip GitHub PR lookup")
	if err := flags.Parse(args); err != nil {
		return err
	}
	config, err := config.LoadDefault()
	if err != nil {
		return err
	}
	runner := gitdata.ExecRunner{}
	state, err := gitdata.Load(context.Background(), ".", config, runner)
	if err != nil {
		return err
	}
	showPR := false
	prPending := false
	if !*noGitHub {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if github.Available(ctx, state.Repo.Root, runner) {
			showPR = true
			pullRequests, enabled := github.LoadPullRequests(ctx, state.Repo.Root, runner)
			if enabled {
				state.Rows = github.AttachPullRequests(state.Rows, pullRequests)
			} else {
				prPending = true
			}
		}
	}
	loadDiskUsageForList(&state, 2*time.Second)
	tty := stdoutIsTTY()
	output := listview.Render(state, listview.Options{
		Width:      terminalWidth(100),
		Color:      tty,
		Hyperlinks: tty,
		ShowHeader: true,
		ShowPR:     showPR,
		Pending:    "-",
		PRPending:  prPending,
	}, time.Now())
	fmt.Println(output)
	return nil
}

func loadDiskUsageForList(state *gitdata.State, budget time.Duration) {
	type result struct {
		path string
		size int64
	}
	results := make(chan result, len(state.Rows))
	var waitGroup sync.WaitGroup
	for _, row := range state.Rows {
		if row.Prunable {
			continue
		}
		path := row.Path
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			size, err := gitdata.DiskUsage(path)
			if err == nil {
				results <- result{path: path, size: size}
			}
		}()
	}
	done := make(chan struct{})
	go func() {
		waitGroup.Wait()
		close(results)
		close(done)
	}()
	deadline := time.After(budget)
	for {
		select {
		case result, ok := <-results:
			if !ok {
				return
			}
			applyDiskUsageResult(state, result.path, result.size)
		case <-done:
			for result := range results {
				applyDiskUsageResult(state, result.path, result.size)
			}
			return
		case <-deadline:
			return
		}
	}
}

func applyDiskUsageResult(state *gitdata.State, path string, size int64) {
	for index := range state.Rows {
		if state.Rows[index].Path == path {
			state.Rows[index].SizeBytes = size
			state.Rows[index].SizeLoaded = true
			return
		}
	}
}

func runTUI(cdFile string) error {
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
	state, err := gitdata.Load(context.Background(), ".", config, runner)
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
		return os.WriteFile(cdFile, []byte(selectedPath), 0600)
	}
	if stdoutIsTTY() {
		fmt.Fprintln(os.Stderr, pathSelectionHint(selectedPath, currentShell()))
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

func pathSelectionHint(selectedPath, shell string) string {
	shell = shellinit.Normalize(shell)
	if shell == "" {
		return fmt.Sprintf("Selected %s\nA standalone %s process cannot change your shell directory. Install shell integration with: %s init %s", selectedPath, commandName, commandName, strings.Join(shellinit.SupportedShells(), "|"))
	}
	return fmt.Sprintf("Selected %s\nA standalone %s process cannot change your shell directory. Run this once now:\n  %s\nPersist it with:\n  %s", selectedPath, commandName, shellinit.ActivationCommand(shell), shellinit.InstallCommand(shell))
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
	value, err := strconv.Atoi(os.Getenv("COLUMNS"))
	if err == nil && value > 0 {
		return value
	}
	return fallback
}
