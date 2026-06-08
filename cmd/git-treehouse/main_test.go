package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/schovi/git-treehouse/internal/config"
	"github.com/schovi/git-treehouse/internal/gitdata"
)

func TestDetectShell(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "zsh", path: "/bin/zsh", want: "zsh"},
		{name: "homebrew fish", path: "/opt/homebrew/bin/fish", want: "fish"},
		{name: "bash", path: "/usr/local/bin/bash", want: "bash"},
		{name: "nushell", path: "/opt/homebrew/bin/nu", want: "nushell"},
		{name: "powershell", path: "/usr/local/bin/pwsh", want: "powershell"},
		{name: "unknown", path: "/bin/tcsh", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := detectShell(test.path); got != test.want {
				t.Fatalf("detectShell(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}

func TestPathSelectionHintExplainsShellIntegration(t *testing.T) {
	hint := pathSelectionHint("/repo/worktree", "zsh", false)

	for _, want := range []string{
		"Selected /repo/worktree",
		"cannot change your shell directory",
		"Install shell integration, then use the smart wrapper",
		`eval "$(git-treehouse init zsh)"`,
		"\n  gth",
		"git-treehouse init zsh >> ",
		"Use git-treehouse for native commands",
		"git-treehouse list",
		".zshrc",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("pathSelectionHint() missing %q:\n%s", want, hint)
		}
	}
}

func TestPathSelectionHintSuggestsGthWhenIntegrationIsInstalled(t *testing.T) {
	hint := pathSelectionHint("/repo/worktree", "zsh", true)

	for _, want := range []string{
		"Selected /repo/worktree",
		"Shell integration appears installed in your config",
		"Reload your shell if needed",
		"Run the smart wrapper instead:\n  gth",
		"Use git-treehouse for native commands",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("pathSelectionHint() missing %q:\n%s", want, hint)
		}
	}
	if strings.Contains(hint, "Run this once") || strings.Contains(hint, "Persist it with") {
		t.Fatalf("pathSelectionHint() should not show install instructions when integration is installed:\n%s", hint)
	}
}

func TestShouldShowShellWelcome(t *testing.T) {
	cfg := config.Config{}

	if !shouldShowShellWelcome("", cfg, true, "", "zsh") {
		t.Fatal("shouldShowShellWelcome() = false, want true")
	}
	if shouldShowShellWelcome("/tmp/gth", cfg, true, "", "zsh") {
		t.Fatal("shouldShowShellWelcome() should be false with --cd-file")
	}
	if shouldShowShellWelcome("", cfg, true, "1", "zsh") {
		t.Fatal("shouldShowShellWelcome() should be false with integration env")
	}
	if shouldShowShellWelcome("", cfg, false, "", "zsh") {
		t.Fatal("shouldShowShellWelcome() should be false without tty stdout")
	}
	cfg.SkipShellIntegrationWelcome = true
	if shouldShowShellWelcome("", cfg, true, "", "zsh") {
		t.Fatal("shouldShowShellWelcome() should be false after persisted skip")
	}
}

func TestParseGlobalOptionsUsesExplicitRepo(t *testing.T) {
	options, remaining, err := parseGlobalOptions([]string{"--repo", "/repo/feature", "list", "--json"})
	if err != nil {
		t.Fatalf("parseGlobalOptions() error = %v", err)
	}
	if options.repoPath != "/repo/feature" {
		t.Fatalf("repoPath = %q, want /repo/feature", options.repoPath)
	}
	if strings.Join(remaining, " ") != "list --json" {
		t.Fatalf("remaining args = %v, want list --json", remaining)
	}
}

func TestParseGlobalOptionsAcceptsHelpFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "short", args: []string{"-h"}},
		{name: "long", args: []string{"--help"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options, remaining, err := parseGlobalOptions(test.args)
			if err != nil {
				t.Fatalf("parseGlobalOptions() error = %v", err)
			}
			if !options.showHelp {
				t.Fatal("showHelp = false, want true")
			}
			if len(remaining) != 0 {
				t.Fatalf("remaining args = %v, want none", remaining)
			}
		})
	}
}

func TestHelpTextDescribesAppCommandsAndOptions(t *testing.T) {
	output := helpText()

	for _, want := range []string{
		"git-treehouse manages Git worktrees",
		"Usage:",
		"git-treehouse [--repo <path>] [--cd-file <path>]",
		"git-treehouse list [--repo <path>] [--no-github] [--json]",
		"git-treehouse init [",
		"git-treehouse doctor [--repo <path>]",
		"git-treehouse help [list|init|doctor]",
		"Browse worktrees",
		"Switch directories through gth shell integration",
		"Print plain tables or JSON",
		"list     Print worktrees",
		"init     Print shell integration",
		"doctor   Check Git",
		"-h, --help",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("helpText() missing %q:\n%s", want, output)
		}
	}
}

func TestSubcommandHelpTextDescribesOptions(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name:   "list",
			output: listHelpText(),
			want: []string{
				"git-treehouse list prints worktrees",
				"git-treehouse list [--repo <path>] [--no-github] [--json]",
				"--no-github",
				"--json",
				"-h, --help",
			},
		},
		{
			name:   "init",
			output: initHelpText(),
			want: []string{
				"git-treehouse init prints shell integration",
				"git-treehouse init [",
				"eval \"$(git-treehouse init zsh)\"",
				"git-treehouse init fish | source",
			},
		},
		{
			name:   "doctor",
			output: doctorHelpText(),
			want: []string{
				"git-treehouse doctor checks Git",
				"git-treehouse doctor [--repo <path>]",
				"--repo <path>",
				"-h, --help",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, want := range test.want {
				if !strings.Contains(test.output, want) {
					t.Fatalf("%s help missing %q:\n%s", test.name, want, test.output)
				}
			}
		})
	}
}

func TestRunHelpFlagsAndCommandsReturnNil(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "short global", args: []string{"-h"}, want: "git-treehouse manages Git worktrees"},
		{name: "long global", args: []string{"--help"}, want: "git-treehouse manages Git worktrees"},
		{name: "help command", args: []string{"help"}, want: "git-treehouse manages Git worktrees"},
		{name: "help list", args: []string{"help", "list"}, want: "git-treehouse list prints worktrees"},
		{name: "help init", args: []string{"help", "init"}, want: "git-treehouse init prints shell integration"},
		{name: "help doctor", args: []string{"help", "doctor"}, want: "git-treehouse doctor checks Git"},
		{name: "list help", args: []string{"list", "-h"}, want: "git-treehouse list prints worktrees"},
		{name: "init help", args: []string{"init", "-h"}, want: "git-treehouse init prints shell integration"},
		{name: "doctor help", args: []string{"doctor", "-h"}, want: "git-treehouse doctor checks Git"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, err := captureStdout(t, func() error {
				return run(test.args)
			})
			if err != nil {
				t.Fatalf("run(%v) error = %v", test.args, err)
			}
			if !strings.Contains(output, test.want) {
				t.Fatalf("run(%v) output missing %q:\n%s", test.args, test.want, output)
			}
		})
	}
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	original := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = write
	defer func() {
		os.Stdout = original
	}()

	fnErr := fn()
	if err := write.Close(); err != nil {
		t.Fatalf("stdout pipe close error = %v", err)
	}
	output, readErr := io.ReadAll(read)
	if readErr != nil {
		t.Fatalf("stdout pipe read error = %v", readErr)
	}
	if err := read.Close(); err != nil {
		t.Fatalf("stdout pipe read close error = %v", err)
	}
	return string(output), fnErr
}

func TestParseListOptionsInheritsGlobalRepo(t *testing.T) {
	options, err := parseListOptions([]string{"--json"}, "/repo/feature")
	if err != nil {
		t.Fatalf("parseListOptions() error = %v", err)
	}
	if !options.jsonOutput {
		t.Fatal("jsonOutput = false, want true")
	}
	if options.repoPath != "/repo/feature" {
		t.Fatalf("repoPath = %q, want inherited /repo/feature", options.repoPath)
	}
}

func TestParseListOptionsAllowsSubcommandRepoOverride(t *testing.T) {
	options, err := parseListOptions([]string{"--repo", "/repo/docs", "--no-github"}, "/repo/feature")
	if err != nil {
		t.Fatalf("parseListOptions() error = %v", err)
	}
	if !options.noGitHub {
		t.Fatal("noGitHub = false, want true")
	}
	if options.repoPath != "/repo/docs" {
		t.Fatalf("repoPath = %q, want /repo/docs", options.repoPath)
	}
}

func TestParseListOptionsAcceptsHelpFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "short", args: []string{"-h"}},
		{name: "long", args: []string{"--help"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options, err := parseListOptions(test.args, "/repo/feature")
			if err != nil {
				t.Fatalf("parseListOptions() error = %v", err)
			}
			if !options.showHelp {
				t.Fatal("showHelp = false, want true")
			}
		})
	}
}

func TestTerminalWidthUsesColumnsFallback(t *testing.T) {
	t.Setenv("COLUMNS", "142")

	if got := terminalWidth(100); got != 142 {
		t.Fatalf("terminalWidth() = %d, want 142", got)
	}
}

func TestTerminalWidthFallsBackWhenColumnsIsInvalid(t *testing.T) {
	t.Setenv("COLUMNS", "wide")

	if got := terminalWidth(100); got != 100 {
		t.Fatalf("terminalWidth() = %d, want 100", got)
	}
}

func TestTerminalWidthFallsBackWhenColumnsIsBlank(t *testing.T) {
	t.Setenv("COLUMNS", "")

	if got := terminalWidth(100); got != 100 {
		t.Fatalf("terminalWidth() = %d, want 100", got)
	}
}

func TestListJSONFromStateIncludesStructuredWorktreeData(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	state := gitdata.State{
		Repo: gitdata.Repository{
			Root:             "/repo/main",
			CommonGitDir:     "/repo/main/.git",
			Cwd:              "/repo/main",
			ActiveWorktree:   "/repo/main",
			MainWorktree:     "/repo/main",
			MainBranch:       "main",
			Parent:           "/repo",
			RemoteConfigured: true,
		},
		Rows: []gitdata.Worktree{{
			Path:               "/repo/main",
			GitDir:             "/repo/main/.git",
			Head:               "abcdef123456",
			Branch:             "main",
			IsActive:           true,
			IsMain:             true,
			Status:             gitdata.StatusCounts{Modified: 2, Untracked: 1},
			Upstream:           "origin/main",
			HeadSync:           gitdata.SyncState{Available: true, Ahead: 1},
			MainSync:           gitdata.SyncState{Available: true},
			CommitShort:        "abcdef1",
			CommitSubject:      "add json output",
			CommitTime:         now.Add(-2 * time.Hour),
			BranchMergedToMain: true,
			PR:                 &gitdata.PullRequest{Number: 42, State: "○", CI: "✓", URL: "https://example.test/pull/42"},
			GitSizeBytes:       1024,
			GitSizeLoaded:      true,
			FullSizeBytes:      2048,
			FullSizeLoaded:     true,
		}},
	}

	payload := listJSONFromState(state, now)
	bytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(listJSONFromState()) error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("Unmarshal(listJSONFromState()) error = %v", err)
	}

	if payload.Repository.Root != "/repo/main" {
		t.Fatalf("repository root = %q, want /repo/main", payload.Repository.Root)
	}
	if len(payload.Worktrees) != 1 {
		t.Fatalf("worktrees = %d, want 1", len(payload.Worktrees))
	}
	row := payload.Worktrees[0]
	if !row.Active || !row.Main || row.Status.Compact != "~2 ?1" || row.RemoteSync.Text != "↑1" {
		t.Fatalf("row JSON = %+v, want active main dirty remote sync", row)
	}
	if row.PullRequest == nil || row.PullRequest.Text != "#42 ○ ✓" {
		t.Fatalf("pull request JSON = %+v, want #42 ○ ✓", row.PullRequest)
	}
	if !row.Size.Loaded || row.Size.Bytes != 2048 {
		t.Fatalf("size JSON = %+v, want loaded 2048", row.Size)
	}
	if !row.GitSize.Loaded || row.GitSize.Bytes != 1024 {
		t.Fatalf("git_size JSON = %+v, want loaded 1024", row.GitSize)
	}
	if !row.FullSize.Loaded || row.FullSize.Bytes != 2048 {
		t.Fatalf("full_size JSON = %+v, want loaded 2048", row.FullSize)
	}
	if row.Commit.Age != "2h" || row.Commit.Time != now.Add(-2*time.Hour).Format(time.RFC3339) {
		t.Fatalf("commit JSON = %+v, want RFC3339 time and 2h age", row.Commit)
	}
}

func TestFormatDoctorChecks(t *testing.T) {
	output := formatDoctorChecks([]doctorCheck{
		{Name: "git", Status: doctorOK, Message: "git version 2.0"},
		{Name: "github", Status: doctorWarning, Message: "gh was not found"},
	})

	for _, want := range []string{
		"git-treehouse doctor",
		"ok    git:",
		"git version 2.0",
		"warn  github:",
		"gh was not found",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("formatDoctorChecks() missing %q:\n%s", want, output)
		}
	}
}

func TestClipboardCommandsForRuntime(t *testing.T) {
	tests := []struct {
		goos string
		want string
	}{
		{goos: "darwin", want: "pbcopy"},
		{goos: "windows", want: "clip"},
		{goos: "linux", want: "wl-copy"},
	}

	for _, test := range tests {
		t.Run(test.goos, func(t *testing.T) {
			commands := clipboardCommandsForRuntime(test.goos)
			if len(commands) == 0 || commands[0] != test.want {
				t.Fatalf("clipboardCommandsForRuntime(%q) = %v, want first %q", test.goos, commands, test.want)
			}
		})
	}
}
