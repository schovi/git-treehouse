package listview

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/schovi/git-treehouse/internal/gitdata"
)

func TestRenderRowsOmitsAnsiAndHyperlinksWhenDisabled(t *testing.T) {
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	rows := []gitdata.Worktree{
		{
			Branch:        "feature/plain",
			Status:        gitdata.StatusCounts{Staged: 1, Modified: 2, Untracked: 3},
			HeadSync:      gitdata.SyncState{Available: true, Ahead: 1, Behind: 2},
			MainSync:      gitdata.SyncState{Available: true, Behind: 4},
			CommitShort:   "abc1234",
			CommitSubject: "render rows",
			CommitTime:    now.Add(-2 * time.Hour),
			PR: &gitdata.PullRequest{
				Number: 42,
				State:  "○",
				CI:     "✓",
				URL:    "https://example.test/pull/42",
			},
			SizeBytes:  1536,
			SizeLoaded: true,
		},
	}

	output := RenderRows(rows, Options{
		Width:      160,
		Color:      false,
		Hyperlinks: false,
		ShowHeader: true,
		ShowPR:     true,
	}, now)

	if strings.Contains(output, "\x1b") {
		t.Fatalf("RenderRows() contains ANSI or OSC escape sequence: %q", output)
	}
	if strings.Contains(output, "https://example.test/pull/42") {
		t.Fatalf("RenderRows() contains hyperlink URL when hyperlinks are disabled: %q", output)
	}
	for _, want := range []string{
		"branch",
		"feature/plain",
		"+1 ~2 ?3",
		"↑1 ↓2",
		"↓4",
		"abc1234 render rows",
		"2h",
		"#42 ○ ✓",
		"1.5K",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("RenderRows() missing %q:\n%s", want, output)
		}
	}
}

func TestRenderRowsUsesPendingPlaceholder(t *testing.T) {
	output := RenderRows([]gitdata.Worktree{
		{Branch: "main"},
	}, Options{
		Width:      160,
		ShowHeader: true,
		Pending:    "-",
	}, time.Now())

	if !strings.Contains(output, "-") {
		t.Fatalf("RenderRows() missing pending placeholder:\n%s", output)
	}
	if strings.Contains(output, "…") {
		t.Fatalf("RenderRows() contains spinner for script output:\n%s", output)
	}
}

func TestRenderRowsUsesPendingPlaceholderForUnresolvedPRs(t *testing.T) {
	output := RenderRows([]gitdata.Worktree{
		{Branch: "feature/pr"},
	}, Options{
		Width:      160,
		ShowHeader: true,
		ShowPR:     true,
		Pending:    "-",
		PRPending:  true,
	}, time.Now())

	if !strings.Contains(output, "-") {
		t.Fatalf("RenderRows() missing unresolved PR placeholder:\n%s", output)
	}
}

func TestRenderRowsAlignsBranchWithHeader(t *testing.T) {
	output := RenderRows([]gitdata.Worktree{
		{Branch: "feature/plain"},
	}, Options{Width: 100, ShowHeader: true}, time.Now())

	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		t.Fatalf("RenderRows() lines = %d, want header and row:\n%s", len(lines), output)
	}
	headerColumn := visualIndex(lines[0], "branch")
	rowColumn := visualIndex(lines[1], "feature/plain")
	if headerColumn != rowColumn {
		t.Fatalf("branch column = %d, row branch column = %d:\n%s", headerColumn, rowColumn, output)
	}
}

func TestRenderRowsSelectedColorKeepsBranchText(t *testing.T) {
	output := RenderRows([]gitdata.Worktree{
		{Branch: "main", IsActive: true, IsMain: true},
	}, Options{
		Width:             100,
		Color:             true,
		ShowHeader:        true,
		HighlightSelected: true,
		SelectedIndex:     0,
	}, time.Now())

	if !strings.Contains(output, "main") {
		t.Fatalf("selected colored row lost branch text:\n%q", output)
	}
}

func TestRenderRowsSelectedColorPadsToWidth(t *testing.T) {
	output := RenderRows([]gitdata.Worktree{
		{Branch: "main", IsActive: true, IsMain: true},
	}, Options{
		Width:             100,
		Color:             true,
		ShowHeader:        true,
		HighlightSelected: true,
		SelectedIndex:     0,
	}, time.Now())
	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		t.Fatalf("RenderRows() lines = %d, want header and row:\n%s", len(lines), output)
	}
	if width := lipgloss.Width(lines[1]); width != 100 {
		t.Fatalf("selected row width = %d, want 100:\n%q", width, lines[1])
	}
}

func TestRenderRowsCanShowSeparatorsInRows(t *testing.T) {
	output := RenderRows([]gitdata.Worktree{
		{Branch: "main", IsActive: true, IsMain: true},
	}, Options{
		Width:          100,
		ShowHeader:     true,
		ShowSeparators: true,
	}, time.Now())

	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		t.Fatalf("RenderRows() lines = %d, want header and row:\n%s", len(lines), output)
	}
	if strings.Count(lines[0], "│") == 0 || strings.Count(lines[1], "│") == 0 {
		t.Fatalf("RenderRows() should include separators in header and rows:\n%s", output)
	}
}

func TestRenderRowsFoldsMarkerIntoBranchColumn(t *testing.T) {
	output := RenderRows([]gitdata.Worktree{
		{Branch: "main", IsActive: true, IsMain: true},
	}, Options{
		Width:          100,
		ShowHeader:     true,
		ShowSeparators: true,
	}, time.Now())

	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		t.Fatalf("RenderRows() lines = %d, want header and row:\n%s", len(lines), output)
	}
	if strings.HasPrefix(lines[0], "│") || strings.HasPrefix(lines[1], "│") {
		t.Fatalf("RenderRows() should not start with a marker separator:\n%s", output)
	}
	if !strings.Contains(lines[1], "◉ main") {
		t.Fatalf("RenderRows() should render marker inside branch column:\n%s", output)
	}
}

func TestRenderRowsHeaderIsBoldWhite(t *testing.T) {
	if !headerStyle.GetBold() {
		t.Fatal("headerStyle should be bold")
	}
	if got := fmt.Sprint(headerStyle.GetForeground()); got != "255" {
		t.Fatalf("headerStyle foreground = %q, want 255", got)
	}
}

func TestRenderRowsSelectedRowDoesNotContainStyledSeparators(t *testing.T) {
	output := RenderRows([]gitdata.Worktree{
		{Branch: "main", IsActive: true, IsMain: true},
	}, Options{
		Width:             100,
		Color:             true,
		ShowHeader:        true,
		ShowSeparators:    true,
		HighlightSelected: true,
		SelectedIndex:     0,
	}, time.Now())
	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		t.Fatalf("RenderRows() lines = %d, want header and row:\n%s", len(lines), output)
	}
	styledSeparator := headerRuleStyle.Render("│")
	if styledSeparator != "│" && strings.Contains(lines[1], styledSeparator) {
		t.Fatalf("selected row should not contain independently styled separators:\n%q", lines[1])
	}
	if width := lipgloss.Width(lines[1]); width != 100 {
		t.Fatalf("selected row width = %d, want 100:\n%q", width, lines[1])
	}
}

func visualIndex(line, needle string) int {
	byteIndex := strings.Index(line, needle)
	if byteIndex < 0 {
		return -1
	}
	width := 0
	for _, character := range line[:byteIndex] {
		width += runewidth.RuneWidth(character)
	}
	return width
}
