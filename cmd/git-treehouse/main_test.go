package main

import (
	"strings"
	"testing"

	"github.com/schovi/git-treehouse/internal/config"
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
	hint := pathSelectionHint("/repo/worktree", "zsh")

	for _, want := range []string{
		"Selected /repo/worktree",
		"cannot change your shell directory",
		`eval "$(git-treehouse init zsh)"`,
		"git-treehouse init zsh >> ",
		".zshrc",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("pathSelectionHint() missing %q:\n%s", want, hint)
		}
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
