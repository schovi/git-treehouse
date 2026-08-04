package tui

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/listview"
	"strings"
	"testing"
)

func TestHelpRendersCenteredOverlayInAppFrame(t *testing.T) {
	rows := make([]gitdata.Worktree, 24)
	rows[0] = gitdata.Worktree{Path: "/repo/main", Branch: "main", IsMain: true, IsActive: true}
	for index := 1; index < len(rows); index++ {
		rows[index] = gitdata.Worktree{Path: fmt.Sprintf("/repo/worktree-%d", index), Branch: fmt.Sprintf("worktree-%d", index)}
	}
	model := testModelWithRows(rows)
	model.width = 100
	model.height = 40
	model.help = true

	output := model.View()
	lines := strings.Split(output, "\n")
	helpLine := -1
	for index, line := range lines {
		if strings.Contains(line, "Help") {
			helpLine = index
			break
		}
	}

	if helpLine < 3 || helpLine > 12 {
		t.Fatalf("Help dialog line = %d, want centered in app frame:\n%s", helpLine, output)
	}
	if helpLine >= model.height/2 {
		t.Fatalf("Help dialog line = %d, should not be centered in terminal viewport:\n%s", helpLine, output)
	}
	plainOutput := ansi.Strip(output)
	if !strings.Contains(plainOutput, "Worktrees") || !strings.Contains(plainOutput, "ctrl+p commands") {
		t.Fatalf("Help overlay should preserve app content and show palette hint:\n%s", output)
	}
}

func TestHelpRendersGroupedKeysAndLegends(t *testing.T) {
	model := Model{}

	output := ansi.Strip(model.renderHelpAtWidth(68))

	for _, want := range []string{
		"Global",
		"Worktree List",
		"Worktree Detail",
		"Row Icons",
		"Git Status",
		"Pull Requests",
		"ctrl+p",
		"Esc close/cancel",
		"top/bottom",
		"b branches",
		"Enter go/create",
		"PR/branch",
		"⊡ worktree",
		"⎇ branch",
		"bold active row",
		"remote gone",
		"◌ draft",
		"○ ready/open",
		"◆ approved",
		"⎇ merged",
		"✗ CI error",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderHelpAtWidth() missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"checkout root", "r/f", "close/clear/quit"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("renderHelpAtWidth() should not contain %q:\n%s", unwanted, output)
		}
	}
	markerOrder := []string{"⌂ root", "⊡ worktree", "⎇ branch", "! locked", "× prunable", "bold active row"}
	previousIndex := -1
	for _, marker := range markerOrder {
		index := strings.Index(output, marker)
		if index < 0 {
			t.Fatalf("renderHelpAtWidth() missing marker %q:\n%s", marker, output)
		}
		if index <= previousIndex {
			t.Fatalf("renderHelpAtWidth() marker %q is out of order:\n%s", marker, output)
		}
		previousIndex = index
	}
}

func TestHelpCategoryStyleIsBoldWhite(t *testing.T) {
	if !helpCategoryStyle.GetBold() {
		t.Fatal("helpCategoryStyle should be bold")
	}
	if got := fmt.Sprint(helpCategoryStyle.GetForeground()); got != "255" {
		t.Fatalf("helpCategoryStyle foreground = %q, want 255", got)
	}
}

func TestHelpRowIconStylesMatchListRenderer(t *testing.T) {
	tests := []struct {
		name string
		kind helpEntryKind
		want lipgloss.Style
	}{
		{name: "root", kind: helpEntryRoot, want: listview.RootTypeIconStyle()},
		{name: "worktree", kind: helpEntryWorktree, want: listview.WorktreeTypeIconStyle()},
		{name: "branch", kind: helpEntryBranch, want: listview.BranchTypeIconStyle()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := helpEntryStyle(test.kind)
			if fmt.Sprint(got.GetForeground()) != fmt.Sprint(test.want.GetForeground()) || got.GetBold() != test.want.GetBold() {
				t.Fatalf("helpEntryStyle(%s) = foreground %v bold %t, want foreground %v bold %t",
					test.name,
					got.GetForeground(),
					got.GetBold(),
					test.want.GetForeground(),
					test.want.GetBold(),
				)
			}
		})
	}
}
