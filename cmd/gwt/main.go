package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/schovi/git-worktree-tui/internal/config"
	"github.com/schovi/git-worktree-tui/internal/gitdata"
	"github.com/schovi/git-worktree-tui/internal/github"
	"github.com/schovi/git-worktree-tui/internal/listview"
	"github.com/schovi/git-worktree-tui/internal/shellinit"
	"github.com/schovi/git-worktree-tui/internal/tui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	globalFlags := flag.NewFlagSet("gwt", flag.ContinueOnError)
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
		return fmt.Errorf("usage: gwt init [fish|zsh|bash]")
	}
	shell := ""
	if len(args) == 1 {
		shell = args[0]
	} else {
		shell = detectShell(os.Getenv("SHELL"))
		if shell == "" {
			return fmt.Errorf("usage: gwt init fish|zsh|bash")
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
	flags := flag.NewFlagSet("gwt list", flag.ContinueOnError)
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
		fmt.Fprintln(os.Stderr, pathSelectionHint(selectedPath, os.Getenv("SHELL")))
		return nil
	}
	fmt.Println(selectedPath)
	return nil
}

func pathSelectionHint(selectedPath, shellPath string) string {
	shell := detectShell(shellPath)
	if shell == "" {
		return fmt.Sprintf("Selected %s\nA standalone gwt process cannot change your shell directory. Install shell integration with: gwt init fish|zsh|bash", selectedPath)
	}
	return fmt.Sprintf("Selected %s\nA standalone gwt process cannot change your shell directory. Run this once now:\n  %s\nPersist it with:\n  %s", selectedPath, shellActivationCommand(shell), shellInstallCommand(shell))
}

func detectShell(shellPath string) string {
	switch filepath.Base(shellPath) {
	case "fish":
		return "fish"
	case "zsh":
		return "zsh"
	case "bash":
		return "bash"
	default:
		return ""
	}
}

func shellActivationCommand(shell string) string {
	if shell == "fish" {
		return "gwt init fish | source"
	}
	return fmt.Sprintf("eval \"$(gwt init %s)\"", shell)
}

func shellInstallCommand(shell string) string {
	switch shell {
	case "fish":
		return "gwt init fish >> ~/.config/fish/config.fish"
	case "bash":
		return "gwt init bash >> ~/.bashrc"
	default:
		return "gwt init zsh >> ~/.zshrc"
	}
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
