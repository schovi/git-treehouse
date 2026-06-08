package listview

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"

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
		"name",
		"remote",
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

func TestRenderRowsUsesSharedLoadingPlaceholder(t *testing.T) {
	output := RenderRows([]gitdata.Worktree{
		{Branch: "feature/pr", LocalMetadataLoaded: true},
	}, Options{
		Width:      160,
		ShowHeader: true,
		ShowPR:     true,
		Pending:    LoadingPlaceholder,
		PRPending:  true,
	}, time.Now())

	if !strings.Contains(output, LoadingPlaceholder) {
		t.Fatalf("RenderRows() missing loading placeholder:\n%s", output)
	}
}

func TestColumnVisibilityThresholds(t *testing.T) {
	if ShowsPullRequestColumn(127) {
		t.Fatal("PR column should be hidden at width 127")
	}
	if !ShowsPullRequestColumn(128) {
		t.Fatal("PR column should be visible at width 128")
	}
	if ShowsGitSizeColumn(143) {
		t.Fatal("size column should be hidden at width 143")
	}
	if !ShowsGitSizeColumn(144) {
		t.Fatal("size column should be visible at width 144")
	}
}

func TestRenderRowsAlignsNameHeaderWithDisplayedName(t *testing.T) {
	output := RenderRows([]gitdata.Worktree{
		{Branch: "feature/plain"},
	}, Options{Width: 100, ShowHeader: true}, time.Now())

	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		t.Fatalf("RenderRows() lines = %d, want header and row:\n%s", len(lines), output)
	}
	headerColumn := visualIndex(lines[0], "name")
	rowColumn := visualIndex(lines[1], "feature/plain")
	if headerColumn != rowColumn {
		t.Fatalf("name header column = %d, displayed name column = %d:\n%s", headerColumn, rowColumn, output)
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

func TestRenderRowsSelectedColorCoversWholeRow(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	tests := []struct {
		name string
		row  gitdata.Worktree
	}{
		{
			name: "root",
			row:  gitdata.Worktree{Branch: "main", IsActive: true, IsMain: true, MainSync: gitdata.SyncState{Available: true, Ahead: 1}},
		},
		{
			name: "branch",
			row: gitdata.Worktree{
				Branch:   "docs/troubleshooting",
				Status:   gitdata.StatusCounts{Staged: 1, Modified: 1},
				HeadSync: gitdata.SyncState{Available: true, Ahead: 1},
				MainSync: gitdata.SyncState{Available: true, Behind: 2},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := RenderRows([]gitdata.Worktree{test.row}, Options{
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

			backgrounds := selectedBackgroundColumns(lines[1])
			if len(backgrounds) != 100 {
				t.Fatalf("selected row columns = %d, want 100:\n%q", len(backgrounds), lines[1])
			}
			for column, selected := range backgrounds {
				if !selected {
					t.Fatalf("selected background missing at column %d:\n%q", column, lines[1])
				}
			}
		})
	}
}

func TestRenderRowsUsesTypeIconBeforeBranchColumn(t *testing.T) {
	output := RenderRows([]gitdata.Worktree{
		{Branch: "main", IsActive: true, IsMain: true},
	}, Options{
		Width:      100,
		ShowHeader: true,
	}, time.Now())

	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		t.Fatalf("RenderRows() lines = %d, want header and row:\n%s", len(lines), output)
	}
	if strings.HasPrefix(lines[0], "│") || strings.HasPrefix(lines[1], "│") {
		t.Fatalf("RenderRows() should not start with a marker separator:\n%s", output)
	}
	if !strings.Contains(lines[1], "⌂ main") {
		t.Fatalf("RenderRows() should render root type icon in branch column:\n%s", output)
	}
}

func TestRenderRowsLeavesTypeIconHeaderUntitled(t *testing.T) {
	output := RenderRows([]gitdata.Worktree{
		{Branch: "feature/plain"},
	}, Options{
		Width:      100,
		ShowHeader: true,
	}, time.Now())

	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		t.Fatalf("RenderRows() lines = %d, want header and row:\n%s", len(lines), output)
	}
	headerColumn := visualIndex(lines[0], "name")
	iconColumn := visualIndex(lines[1], "▣")
	nameColumn := visualIndex(lines[1], "feature/plain")
	if nameColumn != headerColumn {
		t.Fatalf("name header column = %d, displayed name column = %d:\n%s", headerColumn, nameColumn, output)
	}
	if iconColumn != headerColumn-2 {
		t.Fatalf("name header column = %d, row type icon column = %d:\n%s", headerColumn, iconColumn, output)
	}
}

func TestRenderRowsShowsLifecycleSuffixesAndRemoteState(t *testing.T) {
	output := RenderRows([]gitdata.Worktree{
		{Branch: "experiment/locked", Locked: true},
		{Branch: "stale/abandoned", Prunable: true, UpstreamGone: true},
		{Head: "abcdef123456", Detached: true, HeadSync: gitdata.SyncState{NoUpstream: true}},
		{Branch: "feature/remote", HeadSync: gitdata.SyncState{Available: true}},
	}, Options{
		Width:      120,
		ShowHeader: true,
	}, time.Now())

	for _, want := range []string{
		"▣ experiment/locked !",
		"▣ stale/abandoned ×",
		"▣ abcdef1 detached",
		"gone",
		"✓",
		"-",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("RenderRows() missing %q:\n%s", want, output)
		}
	}
}

func TestRenderRowsPreservesLifecycleSuffixWhenNameTruncates(t *testing.T) {
	output := RenderRows([]gitdata.Worktree{
		{Branch: "experiment/locked-branch-with-a-very-long-name", Locked: true},
		{Branch: "stale/abandoned-branch-with-a-very-long-name", Prunable: true},
	}, Options{
		Width:      60,
		ShowHeader: true,
	}, time.Now())

	for _, want := range []string{" !", " ×"} {
		if !strings.Contains(output, want) {
			t.Fatalf("RenderRows() should preserve lifecycle suffix %q when truncating:\n%s", want, output)
		}
	}
}

func TestRenderMixedRowsShowsBranchOnlyRows(t *testing.T) {
	output := RenderMixedRows([]gitdata.Row{
		{
			Kind: gitdata.RowKindBranch,
			Branch: gitdata.Branch{
				Name:          "feature/list-branches",
				HeadSync:      gitdata.SyncState{Available: true},
				MainSync:      gitdata.SyncState{Available: true, Ahead: 3},
				CommitShort:   "abc1234",
				CommitSubject: "show branches",
			},
		},
	}, Options{Width: 120, ShowHeader: true}, time.Now())

	for _, want := range []string{
		"⑂ feature/list-branches",
		"-",
		"✓",
		"↑3",
		"abc1234 show branches",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("RenderMixedRows() missing %q:\n%s", want, output)
		}
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

func TestRenderRowsDoesNotContainColumnSeparators(t *testing.T) {
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
	if strings.Contains(output, "│") {
		t.Fatalf("RenderRows() should not contain column separators:\n%s", output)
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

func selectedBackgroundColumns(value string) []bool {
	columns := []bool{}
	selected := false
	for len(value) > 0 {
		if strings.HasPrefix(value, "\x1b[") {
			end := strings.IndexByte(value, 'm')
			if end < 0 {
				return columns
			}
			selected = updateSelectedBackground(selected, value[2:end])
			value = value[end+1:]
			continue
		}
		if strings.HasPrefix(value, "\x1b]") {
			end := strings.Index(value, "\x1b\\")
			if end < 0 {
				return columns
			}
			value = value[end+2:]
			continue
		}

		character, size := utf8.DecodeRuneInString(value)
		if character == utf8.RuneError && size == 0 {
			return columns
		}
		width := runewidth.RuneWidth(character)
		for range width {
			columns = append(columns, selected)
		}
		value = value[size:]
	}
	return columns
}

func updateSelectedBackground(selected bool, sequence string) bool {
	if sequence == "" {
		return false
	}
	codes := strings.Split(sequence, ";")
	for index := 0; index < len(codes); index++ {
		switch codes[index] {
		case "0":
			selected = false
		case "49":
			selected = false
		case "48":
			if index+2 < len(codes) && codes[index+1] == "5" {
				selected = codes[index+2] == "62"
				index += 2
			}
		}
	}
	return selected
}
