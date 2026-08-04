package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/schovi/git-treehouse/internal/gitdata"
	"github.com/schovi/git-treehouse/internal/github"
)

func forceANSIProfile(t *testing.T) {
	t.Helper()
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})
}

func dirtyWorktreeRow() gitdata.Row {
	return gitdata.Row{
		Kind: gitdata.RowKindWorktree,
		Worktree: gitdata.Worktree{
			Path:   "/repo/feature",
			Branch: "feature/login",
			Status: gitdata.StatusCounts{Staged: 1, Modified: 1, Untracked: 1},
			ChangedFiles: []gitdata.ChangedFile{
				{Path: "internal/tui/model.go", IndexCode: 'M', WorkCode: ' ', Added: 42, Deleted: 3},
				{Path: "internal/gitdata/load.go", IndexCode: ' ', WorkCode: 'M', Added: 7, Deleted: 18},
				{Path: "docs/notes.md", IndexCode: '?', WorkCode: '?', Added: -1, Deleted: -1},
			},
		},
	}
}

func TestChangesFrameRendersFilesAndStats(t *testing.T) {
	forceANSIProfile(t)
	const panelWidth = 70

	frame := changesFrame(dirtyWorktreeRow(), panelWidth)
	if frame == "" {
		t.Fatal("changesFrame() returned empty for a dirty worktree")
	}

	for _, want := range []string{
		panelTitleStyle.Render("Changes"),
		inspectorCommitStyle.Render("* "), // staged marker on the staged file
		inspectorCleanStyle.Render("+42"), // additions in green
		deleteDangerStyle.Render("-3"),    // deletions in red
		hintStyle.Render("new"),           // untracked stat cell
		"model.go",                        // file path survives truncation
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("changesFrame() missing %q:\n%s", want, frame)
		}
	}

	for index, line := range strings.Split(frame, "\n") {
		if width := lipgloss.Width(line); width != panelWidth {
			t.Fatalf("changesFrame() line %d width = %d, want %d:\n%s", index, width, panelWidth, line)
		}
	}
}

func TestChangesFrameStagedSortsFirst(t *testing.T) {
	forceANSIProfile(t)
	frame := changesFrame(dirtyWorktreeRow(), 70)
	stagedAt := strings.Index(frame, "model.go")
	untrackedAt := strings.Index(frame, "notes.md")
	if stagedAt < 0 || untrackedAt < 0 || stagedAt > untrackedAt {
		t.Fatalf("expected staged file before untracked file, got staged=%d untracked=%d:\n%s", stagedAt, untrackedAt, frame)
	}
}

func TestChangesFrameHiddenStates(t *testing.T) {
	forceANSIProfile(t)

	// A clean worktree still renders the frame, saying "no changes", so it can pair
	// beneath Details rather than collapsing the column.
	clean := gitdata.Row{Kind: gitdata.RowKindWorktree, Worktree: gitdata.Worktree{Path: "/repo/clean"}}
	frame := changesFrame(clean, 70)
	if !strings.Contains(frame, panelTitleStyle.Render("Changes")) || !strings.Contains(frame, "no changes") {
		t.Fatalf("changesFrame() for clean worktree should say \"no changes\":\n%s", frame)
	}

	branch := gitdata.Row{Kind: gitdata.RowKindBranch, Branch: gitdata.Branch{Name: "feature"}}
	if frame := changesFrame(branch, 70); frame != "" {
		t.Fatalf("changesFrame() for branch row = %q, want empty", frame)
	}

	if frame := changesFrame(dirtyWorktreeRow(), changesFrameMinWidth-1); frame != "" {
		t.Fatalf("changesFrame() below min width = %q, want empty", frame)
	}
}

func TestChangesFrameOverflowCollapses(t *testing.T) {
	forceANSIProfile(t)

	row := dirtyWorktreeRow()
	files := make([]gitdata.ChangedFile, 0, 20)
	for index := 0; index < 20; index++ {
		files = append(files, gitdata.ChangedFile{
			Path:      fmt.Sprintf("file%02d.go", index),
			IndexCode: 'M', WorkCode: ' ', Added: index, Deleted: 0,
		})
	}
	row.Worktree.ChangedFiles = files

	frame := changesFrame(row, 70)
	overflow := 20 - (changesFrameMaxFiles - 1)
	want := fmt.Sprintf("+%d more files", overflow)
	if !strings.Contains(frame, want) {
		t.Fatalf("changesFrame() overflow missing %q:\n%s", want, frame)
	}
}

func graphWorktreeRow() gitdata.Row {
	return gitdata.Row{
		Kind: gitdata.RowKindWorktree,
		Worktree: gitdata.Worktree{
			Path:     "/repo/feature",
			Branch:   "feature/login",
			MainSync: gitdata.SyncState{Available: true, Ahead: 1, Behind: 5},
			Graph: gitdata.ContextGraph{
				Loaded:        true,
				BranchCommits: []gitdata.GraphCommit{{Short: "91ac32f", Subject: "wire handler"}},
				MainCommits: []gitdata.GraphCommit{
					{Short: "a1b2c3d", Subject: "fix flake"},
					{Short: "9f0e1d2", Subject: "bump deps"},
					{Short: "ccc3333", Subject: "tidy"},
				},
				ForkPoint:   gitdata.GraphCommit{Short: "f0f0f0f", Subject: "shared base commit"},
				BaseCommits: []gitdata.GraphCommit{{Short: "e1e1e1e", Subject: "older shared"}},
			},
		},
	}
}

func TestGitContextFrameRendersGraph(t *testing.T) {
	forceANSIProfile(t)
	const panelWidth = 70

	frame := gitContextFrame(graphWorktreeRow(), "main", panelWidth, 0)
	if frame == "" {
		t.Fatal("gitContextFrame() returned empty for a behind/ahead worktree")
	}

	for _, want := range []string{
		panelTitleStyle.Render("Git context"),
		inspectorWarnStyle.Render("↑1"),                 // ahead arrow, amber like the table's main± column
		deleteDangerStyle.Render("↓5"),                  // behind arrow, red
		inspectorLabelStyle.Render(padRight("main", 6)), // aligned parent label
		"→ feature/login",                               // current branch named in the value
		"├─┘",                                           // branch rail folds back into the trunk
		"wire handler",
		inspectorWarnStyle.Render("← HEAD"),
		"f0f0f0f",            // fork point rendered as a real commit SHA
		"shared base commit", // ...with its subject
		inspectorLabelStyle.Render("← fork point"),
		"+3 more on main", // 5 behind, only 2 shown
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("gitContextFrame() missing %q:\n%s", want, frame)
		}
	}

	// Our branch commit (HEAD) sits on its own rail above the shared fork point.
	headAt := strings.Index(frame, "← HEAD")
	forkAt := strings.Index(frame, "← fork point")
	if headAt > forkAt {
		t.Fatalf("expected HEAD (branch commit) above the fork point:\n%s", frame)
	}

	for index, line := range strings.Split(frame, "\n") {
		if width := lipgloss.Width(line); width != panelWidth {
			t.Fatalf("gitContextFrame() line %d width = %d, want %d:\n%s", index, width, panelWidth, line)
		}
	}
}

func TestGitContextFramePadsShortGraphWithAncestors(t *testing.T) {
	forceANSIProfile(t)
	// Screenshot case: one commit ahead, none behind. Without padding the graph is
	// just HEAD + fork point; shared ancestors below the fork fill it out.
	row := gitdata.Row{
		Kind: gitdata.RowKindWorktree,
		Worktree: gitdata.Worktree{
			Branch:   "EC-2490/note-type-es-backfill",
			MainSync: gitdata.SyncState{Available: true, Ahead: 1, Behind: 0},
			Graph: gitdata.ContextGraph{
				Loaded:        true,
				BranchCommits: []gitdata.GraphCommit{{Short: "58678ca", Subject: "Backfill note type"}},
				ForkPoint:     gitdata.GraphCommit{Short: "f0f0f0f", Subject: "fork base"},
				BaseCommits: []gitdata.GraphCommit{
					{Short: "aaa1111", Subject: "ancestor one"},
					{Short: "bbb2222", Subject: "ancestor two"},
					{Short: "ccc3333", Subject: "ancestor three"},
					{Short: "ddd4444", Subject: "ancestor four (beyond min)"},
				},
			},
		},
	}
	frame := gitContextFrame(row, "master", 70, 0)
	// HEAD (1) + fork (1) = 2 nodes; padded up to minGraphCommits (5) with 3 ancestors.
	for _, want := range []string{"ancestor one", "ancestor two", "ancestor three", "← fork point"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("gitContextFrame() short graph missing %q:\n%s", want, frame)
		}
	}
	// Only as many ancestors as needed to reach the minimum; the rest stay hidden.
	if strings.Contains(frame, "ancestor four") {
		t.Fatalf("gitContextFrame() padded beyond the minimum row count:\n%s", frame)
	}
}

func TestGitContextFrameInSync(t *testing.T) {
	forceANSIProfile(t)
	row := gitdata.Row{
		Kind: gitdata.RowKindWorktree,
		Worktree: gitdata.Worktree{
			Path:        "/repo/synced",
			CommitShort: "7c9e1f0",
			MainSync:    gitdata.SyncState{Available: true, Ahead: 0, Behind: 0},
			Graph:       gitdata.ContextGraph{Loaded: true},
		},
	}
	frame := gitContextFrame(row, "main", 70, 0)
	if !strings.Contains(frame, "in sync · 7c9e1f0") {
		t.Fatalf("gitContextFrame() in-sync line missing:\n%s", frame)
	}
	if strings.Contains(frame, "fork point") {
		t.Fatalf("in-sync graph should not render a fork point:\n%s", frame)
	}
}

func TestGitContextFrameHiddenStates(t *testing.T) {
	forceANSIProfile(t)

	notLoaded := gitdata.Row{Kind: gitdata.RowKindWorktree, Worktree: gitdata.Worktree{Path: "/repo/x"}}
	if frame := gitContextFrame(notLoaded, "main", 70, 0); frame != "" {
		t.Fatalf("gitContextFrame() with unloaded graph = %q, want empty", frame)
	}

	branchNoGraph := gitdata.Row{Kind: gitdata.RowKindBranch, Branch: gitdata.Branch{Name: "feature"}}
	if frame := gitContextFrame(branchNoGraph, "main", 70, 0); frame != "" {
		t.Fatalf("gitContextFrame() for branch row without a loaded graph = %q, want empty", frame)
	}

	if frame := gitContextFrame(graphWorktreeRow(), "main", gitContextFrameMinWidth-1, 0); frame != "" {
		t.Fatalf("gitContextFrame() below min width = %q, want empty", frame)
	}
}

func TestGitContextFrameRendersForBranchRow(t *testing.T) {
	forceANSIProfile(t)
	const panelWidth = 70
	// A branch-only row (no worktree) carries its own loaded graph; the frame must
	// render it using the branch tip in place of HEAD, the same as a worktree row.
	row := gitdata.Row{
		Kind: gitdata.RowKindBranch,
		Branch: gitdata.Branch{
			Name:     "EC-2490/note-type-es-backfill",
			MainSync: gitdata.SyncState{Available: true, Ahead: 1, Behind: 5},
			Graph: gitdata.ContextGraph{
				Loaded:        true,
				BranchCommits: []gitdata.GraphCommit{{Short: "c8219cb", Subject: "Backfill note type"}},
				MainCommits: []gitdata.GraphCommit{
					{Short: "a1b2c3d", Subject: "fix flake"},
					{Short: "9f0e1d2", Subject: "bump deps"},
				},
				ForkPoint: gitdata.GraphCommit{Short: "f0f0f0f", Subject: "shared base commit"},
			},
		},
	}

	frame := gitContextFrame(row, "master", panelWidth, 0)
	if frame == "" {
		t.Fatal("gitContextFrame() returned empty for a branch row with a loaded graph")
	}
	for _, want := range []string{
		panelTitleStyle.Render("Git context"),
		inspectorLabelStyle.Render(padRight("master", 6)),
		"→ EC-2490/note-type-es-backfill",
		"Backfill note type",
		inspectorWarnStyle.Render("← HEAD"),
		inspectorLabelStyle.Render("← fork point"),
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("gitContextFrame() branch row missing %q:\n%s", want, frame)
		}
	}
	for index, line := range strings.Split(frame, "\n") {
		if width := lipgloss.Width(line); width != panelWidth {
			t.Fatalf("gitContextFrame() line %d width = %d, want %d:\n%s", index, width, panelWidth, line)
		}
	}
}

func largeDiskRow() gitdata.Row {
	return gitdata.Row{
		Kind: gitdata.RowKindWorktree,
		Worktree: gitdata.Worktree{
			Path: "/repo/big",
			DiskBreakdown: gitdata.DiskBreakdown{
				Loaded:           true,
				Total:            1_503_238_553,
				ReclaimableBytes: 1_174_405_120,
				Buckets: []gitdata.DiskBucket{
					{Label: "dependencies", Bytes: 1_027_604_480},
					{Label: "git data", Bytes: 230_686_720},
					{Label: "build output", Bytes: 146_800_640},
					{Label: "source", Bytes: 50_331_648},
				},
			},
		},
	}
}

func TestDiskFrameRendersBars(t *testing.T) {
	forceANSIProfile(t)
	const panelWidth = 80

	frame := diskFrame(largeDiskRow(), panelWidth)
	if frame == "" {
		t.Fatal("diskFrame() returned empty for a large worktree")
	}

	for _, want := range []string{
		panelTitleStyle.Render("Disk"),
		"dependencies",
		"▓",
		"68%", // 1027604480 * 100 / 1503238553
		"reclaimable 1.1G",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("diskFrame() missing %q:\n%s", want, frame)
		}
	}

	for index, line := range strings.Split(frame, "\n") {
		if width := lipgloss.Width(line); width != panelWidth {
			t.Fatalf("diskFrame() line %d width = %d, want %d:\n%s", index, width, panelWidth, line)
		}
	}
}

func TestDiskFrameHiddenStates(t *testing.T) {
	forceANSIProfile(t)

	notLoaded := gitdata.Row{Kind: gitdata.RowKindWorktree, Worktree: gitdata.Worktree{Path: "/repo/x"}}
	if frame := diskFrame(notLoaded, 80); frame != "" {
		t.Fatalf("diskFrame() with unloaded breakdown = %q, want empty", frame)
	}

	small := gitdata.Row{Kind: gitdata.RowKindWorktree, Worktree: gitdata.Worktree{
		DiskBreakdown: gitdata.DiskBreakdown{
			Loaded:  true,
			Total:   1_000_000, // below threshold
			Buckets: []gitdata.DiskBucket{{Label: "source", Bytes: 1_000_000}},
		},
	}}
	if frame := diskFrame(small, 80); frame != "" {
		t.Fatalf("diskFrame() below size threshold = %q, want empty", frame)
	}

	branch := gitdata.Row{Kind: gitdata.RowKindBranch, Branch: gitdata.Branch{Name: "feature"}}
	if frame := diskFrame(branch, 80); frame != "" {
		t.Fatalf("diskFrame() for branch row = %q, want empty", frame)
	}

	if frame := diskFrame(largeDiskRow(), diskFrameMinWidth-1); frame != "" {
		t.Fatalf("diskFrame() below min width = %q, want empty", frame)
	}
}

func blockedReview() github.PullRequestReview {
	return github.PullRequestReview{
		Loaded:           true,
		Number:           1,
		URL:              "https://github.com/o/r/pull/1",
		State:            "OPEN",
		Mergeable:        "MERGEABLE",
		MergeStateStatus: "BLOCKED",
		ReviewDecision:   "CHANGES_REQUESTED",
		Checks: []github.Check{
			{Name: "build", State: github.CheckPass},
			{Name: "test", State: github.CheckPass},
			{Name: "lint", State: github.CheckFail, URL: "https://github.com/o/r/runs/42"},
			{Name: "e2e", State: github.CheckRunning},
		},
		ChangeRequests: []github.ReviewNote{
			{Author: "alice", Body: "nit: wrap this section?", URL: "https://github.com/o/r/pull/1"},
			{Author: "bob", Body: "this can panic on nil runner", URL: "https://github.com/o/r/pull/1"},
		},
	}
}

func TestPRReviewFrameRendersBlockers(t *testing.T) {
	forceANSIProfile(t)
	const panelWidth = 70

	review := blockedReview()
	frame := prReviewFrame(&review, 0, panelWidth)
	if frame == "" {
		t.Fatal("prReviewFrame() returned empty for a loaded review")
	}

	for _, want := range []string{
		panelTitleStyle.Render("PR review"),
		"changes requested by 2",
		"2 passed · 1 failed · 1 running",
		deleteDangerStyle.Render("✗"),  // failing check glyph
		inspectorWarnStyle.Render("●"), // running check glyph
		"lint",
		inspectorCommitStyle.Render("@alice"),
		"blocked by merge requirements",                // verdict from merge state
		"\x1b]8;;https://github.com/o/r/runs/42\x1b\\", // failing check links to its detail page
		"\x1b]8;;https://github.com/o/r/pull/1\x1b\\",  // change-request preview links to the PR
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("prReviewFrame() missing %q:\n%s", want, frame)
		}
	}

	// Not a draft, so the state line must not invent a "draft no" token.
	if strings.Contains(frame, "draft no") {
		t.Fatalf("prReviewFrame() should not render \"draft no\" for a non-draft PR:\n%s", frame)
	}

	// Order is state, checks, then review (with comments under it).
	checksAt := strings.Index(frame, "2 passed")
	reviewAt := strings.Index(frame, "changes requested by 2")
	commentAt := strings.Index(frame, "@alice")
	if checksAt >= reviewAt || reviewAt >= commentAt {
		t.Fatalf("expected order checks < review < comment, got %d/%d/%d:\n%s", checksAt, reviewAt, commentAt, frame)
	}

	for index, line := range strings.Split(frame, "\n") {
		if width := lipgloss.Width(line); width != panelWidth {
			t.Fatalf("prReviewFrame() line %d width = %d, want %d:\n%s", index, width, panelWidth, line)
		}
	}
}

func TestPRReviewFrameReadyToMerge(t *testing.T) {
	forceANSIProfile(t)
	review := github.PullRequestReview{
		Loaded:           true,
		Number:           9,
		State:            "OPEN",
		Mergeable:        "MERGEABLE",
		MergeStateStatus: "CLEAN",
		ReviewDecision:   "APPROVED",
		Checks:           []github.Check{{Name: "build", State: github.CheckPass}},
	}
	frame := prReviewFrame(&review, 0, 70)
	if !strings.Contains(frame, "ready to merge") {
		t.Fatalf("prReviewFrame() ready verdict missing:\n%s", frame)
	}
}

func TestPRReviewFrameHiddenStates(t *testing.T) {
	forceANSIProfile(t)

	if frame := prReviewFrame(nil, 0, 70); frame != "" {
		t.Fatalf("prReviewFrame(nil) = %q, want empty", frame)
	}
	unloaded := github.PullRequestReview{Loaded: false}
	if frame := prReviewFrame(&unloaded, 0, 70); frame != "" {
		t.Fatalf("prReviewFrame() unloaded = %q, want empty", frame)
	}
	review := blockedReview()
	if frame := prReviewFrame(&review, 0, prReviewFrameMinWidth-1); frame != "" {
		t.Fatalf("prReviewFrame() below min width = %q, want empty", frame)
	}
}

func TestPRReviewFrameRendersInlineComments(t *testing.T) {
	forceANSIProfile(t)
	const panelWidth = 70

	review := github.PullRequestReview{
		Loaded: true, Number: 24128, State: "OPEN", MergeStateStatus: "BLOCKED",
		Threads: []github.ReviewThread{
			{Author: "alice", Body: "fixed now", Path: "internal/tui/model.go", Line: 10, URL: "https://github.com/o/r/pull/24128#discussion_r2", Resolved: true},
			{Author: "Copilot", Body: "consider a nil check here", Path: "internal/gitdata/load.go", Line: 88, URL: "https://github.com/o/r/pull/24128#discussion_r1"},
		},
	}
	frame := prReviewFrame(&review, 0, panelWidth)
	for _, want := range []string{
		"1 unresolved · 1 resolved",
		inspectorWarnStyle.Render("○"),  // unresolved glyph
		inspectorCleanStyle.Render("✓"), // resolved glyph
		"load.go:88",                    // compact file location
		"\x1b]8;;https://github.com/o/r/pull/24128#discussion_r1\x1b\\", // deep link to the comment
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("prReviewFrame() inline comments missing %q:\n%s", want, frame)
		}
	}
	// Unresolved threads list before resolved ones.
	unresolvedAt := strings.Index(frame, "load.go:88")
	resolvedAt := strings.Index(frame, "model.go:10")
	if unresolvedAt < 0 || resolvedAt < 0 || unresolvedAt > resolvedAt {
		t.Fatalf("expected unresolved comment before resolved, got %d/%d:\n%s", unresolvedAt, resolvedAt, frame)
	}
	for index, line := range strings.Split(frame, "\n") {
		if width := lipgloss.Width(line); width != panelWidth {
			t.Fatalf("prReviewFrame() comment line %d width = %d, want %d:\n%s", index, width, panelWidth, line)
		}
	}
}

func TestPRReviewFrameMergedShowsPurpleGlyph(t *testing.T) {
	forceANSIProfile(t)
	review := github.PullRequestReview{
		Loaded: true, Number: 24121, State: "MERGED", ReviewDecision: "APPROVED",
		Checks: []github.Check{{Name: "build", State: github.CheckPass}},
	}
	frame := prReviewFrame(&review, 0, 70)
	if !strings.Contains(frame, mergedGlyphStyle.Render("⎇")) {
		t.Fatalf("prReviewFrame() merged state missing purple git glyph:\n%s", frame)
	}
	if !strings.Contains(frame, "merged") {
		t.Fatalf("prReviewFrame() missing merged state text:\n%s", frame)
	}

	// Open PRs do not get the glyph.
	open := github.PullRequestReview{Loaded: true, Number: 9, State: "OPEN"}
	if strings.Contains(prReviewFrame(&open, 0, 70), mergedGlyphStyle.Render("⎇")) {
		t.Fatal("prReviewFrame() open state should not show the merged glyph")
	}
}

func TestPRReviewFrameLoadingState(t *testing.T) {
	forceANSIProfile(t)
	const panelWidth = 70

	// No review yet, but a pending open PR (#42): keep the frame with a placeholder.
	frame := prReviewFrame(nil, 42, panelWidth)
	for _, want := range []string{
		panelTitleStyle.Render("PR review"),
		"#42",
		"loading review",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("prReviewFrame() loading state missing %q:\n%s", want, frame)
		}
	}
	for index, line := range strings.Split(frame, "\n") {
		if width := lipgloss.Width(line); width != panelWidth {
			t.Fatalf("prReviewFrame() loading line %d width = %d, want %d:\n%s", index, width, panelWidth, line)
		}
	}

	// A finished attempt clears the pending number, so a failed/gh-less lookup
	// (nil review, pending 0) stays silent rather than loading forever.
	if frame := prReviewFrame(nil, 0, panelWidth); frame != "" {
		t.Fatalf("prReviewFrame() with no pending PR = %q, want empty", frame)
	}
}

func TestDetailBlocksSideBySideAndStacked(t *testing.T) {
	forceANSIProfile(t)
	model := Model{state: gitdata.State{Repo: gitdata.Repository{MainBranch: "main"}}}
	row := graphWorktreeRow()
	now := time.Now()

	// Wide panel: the Details box and the Git context frame join into a single
	// side-by-side block, every line padded to the full panel width.
	const widePanel = 120
	wide := model.detailBlocks(row, now, widePanel)
	if len(wide) == 0 {
		t.Fatal("detailBlocks() returned no blocks")
	}
	top := wide[0]
	// The left column stacks Details over Changes; Git context pairs on the right.
	for _, want := range []string{"Details", "Changes", "Git context"} {
		if !strings.Contains(top, want) {
			t.Fatalf("wide top block should contain %q:\n%s", want, top)
		}
	}
	for index, line := range strings.Split(top, "\n") {
		if got := lipgloss.Width(line); got != widePanel {
			t.Fatalf("wide top block line %d width = %d, want %d:\n%s", index, got, widePanel, top)
		}
	}

	// Narrow panel: the blocks stack full width, in order Details, Changes, Git context.
	const narrowPanel = 70
	narrow := model.detailBlocks(row, now, narrowPanel)
	if len(narrow) < 3 {
		t.Fatalf("narrow layout should produce at least 3 stacked blocks, got %d", len(narrow))
	}
	if strings.Contains(narrow[0], "Git context") || strings.Contains(narrow[0], "Changes") {
		t.Fatalf("narrow first block should be Details alone:\n%s", narrow[0])
	}
	if !strings.Contains(narrow[1], "Changes") {
		t.Fatalf("narrow second block should be Changes:\n%s", narrow[1])
	}
	if !strings.Contains(narrow[2], "Git context") {
		t.Fatalf("narrow third block should be Git context:\n%s", narrow[2])
	}
	for index, line := range strings.Split(narrow[0], "\n") {
		if got := lipgloss.Width(line); got != narrowPanel {
			t.Fatalf("narrow Details block line %d width = %d, want %d", index, got, narrowPanel)
		}
	}
}

func TestGitContextFrameRemoteLine(t *testing.T) {
	forceANSIProfile(t)

	synced := graphWorktreeRow()
	synced.Worktree.Upstream = "origin/feature/login"
	synced.Worktree.HeadSync = gitdata.SyncState{Available: true}
	frame := gitContextFrame(synced, "main", 70, 0)
	if !strings.Contains(frame, inspectorCleanStyle.Render("✓ synced")) {
		t.Fatalf("synced remote should show a green check:\n%s", frame)
	}
	if !strings.Contains(frame, inspectorLabelStyle.Render(padRight("origin", 6))) {
		t.Fatalf("remote line should name the remote as an aligned label:\n%s", frame)
	}

	ahead := graphWorktreeRow()
	ahead.Worktree.Upstream = "origin/feature/login"
	ahead.Worktree.HeadSync = gitdata.SyncState{Available: true, Ahead: 1}
	if frame := gitContextFrame(ahead, "main", 70, 0); !strings.Contains(frame, inspectorWarnStyle.Render("↑1")) {
		t.Fatalf("remote ahead should show an unpushed arrow:\n%s", frame)
	}

	local := graphWorktreeRow() // no upstream set
	if frame := gitContextFrame(local, "main", 70, 0); !strings.Contains(frame, "no upstream") {
		t.Fatalf("row without upstream should say so:\n%s", frame)
	}
}

func TestGitContextFrameFillsToTargetHeight(t *testing.T) {
	forceANSIProfile(t)
	row := graphWorktreeRow() // has 4 base ancestors fetched
	const target = 13         // total box height to match (e.g. a tall Details box)
	frame := gitContextFrame(row, "main", 70, target)
	if got := lineCount(frame); got != target {
		t.Fatalf("filled frame height = %d, want %d:\n%s", got, target, frame)
	}
	// The fill prefers real shared ancestors over blank padding.
	if !strings.Contains(frame, "older shared") {
		t.Fatalf("fill should show a real shared ancestor before padding blank lines:\n%s", frame)
	}
}

func TestGitContextFrameCollapsesRailWhenNotDiverged(t *testing.T) {
	forceANSIProfile(t)
	// Ahead of main but not behind: a clean fast-forward, no real divergence. The
	// side rail and ├─┘ fold would dangle, so the graph is a single straight column.
	row := gitdata.Row{
		Kind: gitdata.RowKindWorktree,
		Worktree: gitdata.Worktree{
			Branch:   "feature/login",
			MainSync: gitdata.SyncState{Available: true, Ahead: 2, Behind: 0},
			Graph: gitdata.ContextGraph{
				Loaded: true,
				BranchCommits: []gitdata.GraphCommit{
					{Short: "aaa1111", Subject: "second"},
					{Short: "bbb2222", Subject: "first"},
				},
				ForkPoint: gitdata.GraphCommit{Short: "f0f0f0f", Subject: "shared base"},
			},
		},
	}
	frame := gitContextFrame(row, "main", 70, 0)
	if strings.Contains(frame, "├─┘") {
		t.Fatalf("non-diverged graph should not draw the fold:\n%s", frame)
	}
	if strings.Contains(frame, "│ "+inspectorCommitStyle.Render("●")) {
		t.Fatalf("non-diverged graph should not draw a side rail:\n%s", frame)
	}
	for _, want := range []string{"second", "first", inspectorWarnStyle.Render("← HEAD"), inspectorLabelStyle.Render("← fork point")} {
		if !strings.Contains(frame, want) {
			t.Fatalf("non-diverged graph missing %q:\n%s", want, frame)
		}
	}
}

func TestGitContextFrameWeavesRemoteCommits(t *testing.T) {
	forceANSIProfile(t)

	// Behind the remote: the commits to pull render as real ◆ nodes above HEAD on the
	// branch rail, the newest carrying the remote ref label, connected by the fold.
	behind := gitdata.Row{
		Kind: gitdata.RowKindWorktree,
		Worktree: gitdata.Worktree{
			Branch:   "feature/behind-remote",
			Upstream: "origin/feature/behind-remote",
			HeadSync: gitdata.SyncState{Available: true, Behind: 2},
			MainSync: gitdata.SyncState{Available: true, Behind: 27},
			Graph: gitdata.ContextGraph{
				Loaded:      true,
				MainCommits: []gitdata.GraphCommit{{Short: "0c559f0", Subject: "merge"}},
				RemoteCommits: []gitdata.GraphCommit{
					{Short: "ee11ee2", Subject: "server commit B"},
					{Short: "ff22ff3", Subject: "server commit A"},
				},
				ForkPoint: gitdata.GraphCommit{Short: "91a71ab", Subject: "fork base"},
			},
		},
	}
	frame := gitContextFrame(behind, "main", 70, 0)
	for _, want := range []string{
		"server commit B", "server commit A", // remote-only commits rendered as nodes
		inspectorLabelStyle.Render("◆"),        // remote node glyph
		inspectorLabelStyle.Render("← origin"), // newest remote commit carries the ref label
		"├─┘",                                  // the remote nodes fold back into HEAD
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("behind-remote graph missing %q:\n%s", want, frame)
		}
	}

	// Ahead of the remote (unpushed): the commit the remote points to gets a ref label
	// rather than a separate node, so commits above it read as unpushed.
	aheadRow := gitdata.Row{
		Kind: gitdata.RowKindWorktree,
		Worktree: gitdata.Worktree{
			Branch:   "feature/ahead",
			Upstream: "origin/feature/ahead",
			HeadSync: gitdata.SyncState{Available: true, Ahead: 1},
			MainSync: gitdata.SyncState{Available: true, Ahead: 2, Behind: 3},
			Graph: gitdata.ContextGraph{
				Loaded:        true,
				MainCommits:   []gitdata.GraphCommit{{Short: "0c559f0", Subject: "merge"}},
				BranchCommits: []gitdata.GraphCommit{{Short: "aaaa111", Subject: "unpushed"}, {Short: "bbbb222", Subject: "pushed tip"}},
				ForkPoint:     gitdata.GraphCommit{Short: "91a71ab", Subject: "fork base"},
			},
		},
	}
	frame = gitContextFrame(aheadRow, "main", 70, 0)
	if !strings.Contains(frame, inspectorLabelStyle.Render("← origin")) {
		t.Fatalf("ahead-remote graph should label the remote tip on the branch:\n%s", frame)
	}
	headAt := strings.Index(frame, "← HEAD")
	originAt := strings.Index(frame, inspectorLabelStyle.Render("← origin"))
	if headAt < 0 || originAt < 0 || headAt > originAt {
		t.Fatalf("unpushed commits should sit above the remote label:\n%s", frame)
	}
}
