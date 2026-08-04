package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"strings"
	"testing"
)

func TestCreateDialogBaseNavigationKeys(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{
		{Label: "main (local)", Rev: "main"},
		{Label: "origin/main", Rev: "origin/main"},
		{Label: "feature (local)", Rev: "feature"},
	})

	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyTab})
	if got := model.createDialog.baseIndex; got != 1 {
		t.Fatalf("tab base index = %d, want 1", got)
	}
	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyDown})
	if got := model.createDialog.baseIndex; got != 2 {
		t.Fatalf("down base index = %d, want 2", got)
	}
	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := model.createDialog.baseIndex; got != 1 {
		t.Fatalf("shift+tab base index = %d, want 1", got)
	}
	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyUp})
	if got := model.createDialog.baseIndex; got != 0 {
		t.Fatalf("up base index = %d, want 0", got)
	}
}

func TestCreateDialogValidatesOnlyOnSubmit(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})

	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if model.createDialog.error != "" {
		t.Fatalf("typing should not validate immediately, got error %q", model.createDialog.error)
	}

	model.createDialog.input.SetValue("")
	model, _ = model.updateCreate(tea.KeyMsg{Type: tea.KeyEnter})
	if got := model.createDialog.error; got != "branch name is required" {
		t.Fatalf("submit validation error = %q, want branch name is required", got)
	}
}

func TestCreateDialogRendersCenteredOverlay(t *testing.T) {
	input := textinput.New()
	input.Prompt = ""
	model := Model{
		width:  100,
		height: 40,
		state: gitdata.State{
			Repo: gitdata.Repository{
				Root:           "/repo/main",
				ActiveWorktree: "/repo/main",
			},
			Rows: []gitdata.Worktree{{
				Path:                "/repo/main",
				Branch:              "main",
				IsMain:              true,
				IsActive:            true,
				LocalMetadataLoaded: true,
				CommitShort:         "abc1234",
				CommitSubject:       "boxed app",
			}},
		},
		createDialog: &createDialog{
			input: input,
			bases: []gitdata.BaseOption{{Label: "main (local)", Rev: "main"}},
		},
	}

	output := model.View()
	lines := strings.Split(output, "\n")
	dialogLine := -1
	for index, line := range lines {
		if strings.Contains(line, "New worktree") {
			dialogLine = index
			break
		}
	}

	if len(lines) != model.height {
		t.Fatalf("View() line count = %d, want %d:\n%s", len(lines), model.height, output)
	}
	if dialogLine < 5 || dialogLine > 12 {
		t.Fatalf("New worktree dialog line = %d, want centered in app frame:\n%s", dialogLine, output)
	}
	if dialogLine >= model.height/2 {
		t.Fatalf("New worktree dialog line = %d, should not be centered in terminal viewport:\n%s", dialogLine, output)
	}
}

func TestCreateDialogRendersColoredBorderAndBottomHints(t *testing.T) {
	model := modelWithCreateDialog([]gitdata.BaseOption{{Label: "main (local)", Rev: "main"}})

	output := model.renderCreateAtWidth(72)

	for _, want := range []string{
		appBorderStyle.Render("╭─"),
		appBorderStyle.Render("│ "),
		appBorderStyle.Render("╰─ "),
		colorKeyHints("Enter create + go · Tab switch base · ctrl+o config · Esc cancel", false),
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderCreateAtWidth() missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Tab switch · Esc cancel") {
		t.Fatalf("renderCreateAtWidth() should use Tab switch base:\n%s", output)
	}
}

func TestDialogBottomLineKeepsRightBorderRuleAfterHints(t *testing.T) {
	line := dialogBottomLine(colorKeyHints("Enter delete · Esc cancel", false), 80)

	if width := lipgloss.Width(line); width != 80 {
		t.Fatalf("dialogBottomLine() width = %d, want 80:\n%s", width, line)
	}
	if !strings.Contains(line, appBorderStyle.Render(" ─")) {
		t.Fatalf("dialogBottomLine() should render a horizontal rule after hints:\n%s", line)
	}
	if strings.Contains(line, "     "+appBorderStyle.Render("─╯")) {
		t.Fatalf("dialogBottomLine() should not leave blank padding before the right corner:\n%s", line)
	}
}

func TestBottomBorderLineHandlesEmptyStyledControls(t *testing.T) {
	line := bottomBorderLine(40, appBorderStyle, borderControls{text: colorKeyHints("", false)}, borderControls{})
	plainLine := ansi.Strip(line)

	if strings.Contains(plainLine, "╰─  ─") {
		t.Fatalf("bottomBorderLine() should not reserve space for zero-width styled content:\n%s", line)
	}
	if plainLine != "╰"+strings.Repeat("─", 38)+"╯" {
		t.Fatalf("bottomBorderLine() = %q, want solid border", plainLine)
	}
}

func TestBottomBorderLinePositionsLeftAndRightControls(t *testing.T) {
	line := bottomBorderLine(72, appBorderStyle,
		borderControls{parts: []string{"Enter run", "Esc cancel"}},
		borderControls{parts: []string{"q quit"}},
	)
	plainLine := ansi.Strip(line)

	for _, want := range []string{"Enter run · Esc cancel", "q quit"} {
		if !strings.Contains(plainLine, want) {
			t.Fatalf("bottomBorderLine() missing %q:\n%s", want, line)
		}
	}
	if !strings.Contains(plainLine, "─ q quit ─╯") {
		t.Fatalf("bottomBorderLine() should right-align right controls inside the border:\n%s", line)
	}
	if width := lipgloss.Width(line); width != 72 {
		t.Fatalf("bottomBorderLine() width = %d, want 72:\n%s", width, line)
	}
}

func TestSectionBottomLineKeepsWidthWithLongFooter(t *testing.T) {
	line := sectionBottomLine("↵ go · o editor · d delete · y abs path · p PR", 32)

	if width := lipgloss.Width(line); width != 32 {
		t.Fatalf("sectionBottomLine() width = %d, want 32:\n%s", width, line)
	}
	if !strings.Contains(line, "╰─") || !strings.Contains(line, "╯") {
		t.Fatalf("sectionBottomLine() should keep bottom border geometry:\n%s", line)
	}
}

func TestCenteredOverlayPreservesBackgroundOutsidePopupHalo(t *testing.T) {
	output := centeredOverlay("01234567890123456789012345", "POP", 26, 1)

	if output != "0123456789 POP 56789012345" {
		t.Fatalf("centeredOverlay() = %q, want background preserved outside one-cell popup halo", output)
	}
}

func TestCenteredOverlayPreservesBackgroundAboveAndBelowPopup(t *testing.T) {
	output := centeredOverlay("aaaaaaaaaa\nbbbbbbbbbb\ncccccccccc", "XX", 10, 3)
	want := "aaaaaaaaaa\nbbb XX bbb\ncccccccccc"

	if output != want {
		t.Fatalf("centeredOverlay() = %q, want %q", output, want)
	}
}
