package main

import (
	"strings"
	"testing"
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
	hint := pathSelectionHint("/repo/worktree", "/bin/zsh")

	for _, want := range []string{
		"Selected /repo/worktree",
		"cannot change your shell directory",
		`eval "$(gwt init zsh)"`,
		"gwt init zsh >> ~/.zshrc",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("pathSelectionHint() missing %q:\n%s", want, hint)
		}
	}
}
