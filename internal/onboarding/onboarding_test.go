package onboarding

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestViewExplainsShellIntegration(t *testing.T) {
	model := newModel(Info{
		Shell:             "zsh",
		ActivationCommand: `eval "$(git-treehouse init zsh)"`,
		InstallPath:       "/home/me/.zshrc",
		ReloadCommand:     ". /home/me/.zshrc",
	})

	output := model.View()

	for _, want := range []string{
		"Set up gth shell integration",
		"cannot change your",
		"current shell directory",
		`eval "$(git-treehouse init zsh)"`,
		"/home/me/.zshrc",
		"Install for me",
		"Don't show again",
		"Continue to app",
		"╭─",
		"╰",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("View() missing %q:\n%s", want, output)
		}
	}
}

func TestEnterChoosesSelectedAction(t *testing.T) {
	initial := newModel(Info{})
	initial.selected = 1

	updated, _ := initial.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(model).result.Action

	if got != ActionSkip {
		t.Fatalf("selected action = %v, want ActionSkip", got)
	}
}

func TestQContinuesWithoutPersistingSkip(t *testing.T) {
	updated, _ := newModel(Info{}).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := updated.(model).result.Action

	if got != ActionContinue {
		t.Fatalf("q action = %v, want ActionContinue", got)
	}
}

func TestFramePadsToTerminalHeight(t *testing.T) {
	model := newModel(Info{})
	model.width = 72
	model.height = 24

	output := model.View()
	lines := strings.Split(output, "\n")

	if len(lines) != model.height {
		t.Fatalf("line count = %d, want %d:\n%s", len(lines), model.height, output)
	}
	for index, line := range lines {
		if width := lipgloss.Width(line); width != model.width {
			t.Fatalf("line %d width = %d, want %d:\n%s", index, width, model.width, output)
		}
	}
}
