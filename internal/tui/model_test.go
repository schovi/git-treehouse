package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/schovi/git-worktree-tui/internal/gitdata"
)

func TestSelectedInspectorUsesLabeledRelativeFields(t *testing.T) {
	model := Model{
		width: 80,
		state: gitdata.State{
			Repo: gitdata.Repository{
				Root:           "/repo/main",
				ActiveWorktree: "/repo/main",
			},
		},
	}
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	row := gitdata.Worktree{
		Path:          "/repo/main",
		Branch:        "main",
		Status:        gitdata.StatusCounts{Modified: 2, Untracked: 1},
		Upstream:      "origin/main",
		HeadSync:      gitdata.SyncState{Available: true},
		CommitShort:   "c00b701",
		CommitSubject: "Add strategy-review analysis artifacts",
		CommitTime:    now.Add(-4 * time.Hour),
	}

	output := model.selectedInspector(row, now)

	for _, want := range []string{
		"Branch",
		"main",
		"Path",
		".",
		"Status",
		"dirty",
		"Dirty",
		"~ modified 2  ? untracked 1",
		"Sync",
		"origin/main, synced",
		"Commit",
		"c00b701 Add strategy-review analysis artifacts, 4h",
		"PR",
		"none",
		"Delete",
		"allowed with force, dirty worktree",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("selectedInspector() missing %q:\n%s", want, output)
		}
	}
}

func TestSelectedInspectorKeepsDirtyFieldForCleanRows(t *testing.T) {
	model := Model{
		width: 80,
		state: gitdata.State{
			Repo: gitdata.Repository{
				Root:           "/repo/main",
				ActiveWorktree: "/repo/main",
			},
		},
	}
	row := gitdata.Worktree{
		Path:          "/repo/main",
		Branch:        "main",
		CommitShort:   "abc1234",
		CommitSubject: "clean row",
	}

	output := model.selectedInspector(row, time.Now())

	if !strings.Contains(output, "Dirty") || !strings.Contains(output, "none") {
		t.Fatalf("selectedInspector() should keep clean dirty field:\n%s", output)
	}
}

func TestDetailPanelSplitsInspectorAndKeybindings(t *testing.T) {
	model := Model{
		width: 100,
		state: gitdata.State{
			Repo: gitdata.Repository{
				Root:           "/repo/main",
				ActiveWorktree: "/repo/main",
			},
		},
	}
	row := gitdata.Worktree{
		Path:   "/repo/main",
		Branch: "main",
	}

	output := model.detailPanel(row, time.Now())

	for _, want := range []string{"Branch", "main", "│", "Current", "↵", "go", "o", "editor", "d", "delete", "y", "abs path", "p", "PR", "Dirty", "none"} {
		if !strings.Contains(output, want) {
			t.Fatalf("detailPanel() missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"? help", "q quit", "/ filter", "g/G top/bottom", "Esc close/clear", "r refresh", "n new", "m main", "a active", "Tab special", "Tab notable"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("detailPanel() should not contain app control %q:\n%s", unwanted, output)
		}
	}
}

func TestTitleLineIncludesHelpAndQuit(t *testing.T) {
	model := Model{
		width: 80,
		state: gitdata.State{
			Repo: gitdata.Repository{Root: "/repo/main"},
			Rows: []gitdata.Worktree{{Branch: "main"}},
		},
	}

	output := model.titleLine(1)

	for _, want := range []string{"gwt", "main", "1 worktrees", "n", "new", "r", "refresh", "?", "help", "q", "quit"} {
		if !strings.Contains(output, want) {
			t.Fatalf("titleLine() missing %q:\n%s", want, output)
		}
	}
}

func TestStatusBarSplitsAppControlsAndDirtyLegend(t *testing.T) {
	model := Model{width: 120}

	output := model.statusBar()

	for _, want := range []string{"g/G", "top/bottom", "m", "main", "a", "active", "Tab", "notable", "/", "filter", "Esc", "close/clear", "+", "staged", "~", "modified", "untracked"} {
		if !strings.Contains(output, want) {
			t.Fatalf("statusBar() missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"help", "quit"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("statusBar() should not include header controls %q:\n%s", unwanted, output)
		}
	}
	if strings.Contains(output, "delete") || strings.Contains(output, "editor") {
		t.Fatalf("statusBar() should not contain persistent keybindings:\n%s", output)
	}
}

func TestFramePadsToViewportHeight(t *testing.T) {
	model := Model{width: 12, height: 4}

	output := model.frame("one\ntwo")
	lines := strings.Split(output, "\n")

	if len(lines) != 4 {
		t.Fatalf("frame() line count = %d, want 4:\n%q", len(lines), output)
	}
	if lines[2] != strings.Repeat(" ", 12) || lines[3] != strings.Repeat(" ", 12) {
		t.Fatalf("frame() did not pad blank lines to viewport width:\n%q", output)
	}
}
