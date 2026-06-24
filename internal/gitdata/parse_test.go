package gitdata

import (
	"strings"
	"testing"
	"time"
)

func TestParseWorktreeList(t *testing.T) {
	output := `worktree /repo/main
gitdir /repo/main/.git
HEAD abcdef1234567890
branch refs/heads/main

worktree /repo/feature
gitdir /repo/main/.git/worktrees/feature
HEAD 1234567890abcdef
branch refs/heads/feature/topic
locked maintenance window

worktree /repo/missing
gitdir /repo/main/.git/worktrees/missing
HEAD 234567890abcdef1
branch refs/heads/stale
prunable gitdir file points to non-existent location

worktree /repo/bare
bare

worktree /repo/detached
HEAD fedcba9876543210
detached
`

	got := ParseWorktreeList(output)
	if len(got) != 5 {
		t.Fatalf("ParseWorktreeList() len = %d, want 5", len(got))
	}
	if got[0].Path != "/repo/main" || got[0].GitDir != "/repo/main/.git" || got[0].Head != "abcdef1234567890" || got[0].Branch != "main" {
		t.Fatalf("first worktree parsed incorrectly: %+v", got[0])
	}
	if got[1].Branch != "feature/topic" || !got[1].Locked || got[1].LockReason != "maintenance window" {
		t.Fatalf("locked worktree parsed incorrectly: %+v", got[1])
	}
	if !got[2].Prunable || got[2].PruneReason != "gitdir file points to non-existent location" {
		t.Fatalf("prunable worktree parsed incorrectly: %+v", got[2])
	}
	if !got[3].Bare {
		t.Fatalf("bare worktree parsed incorrectly: %+v", got[3])
	}
	if !got[4].Detached || got[4].Branch != "" {
		t.Fatalf("detached worktree parsed incorrectly: %+v", got[4])
	}
}

func TestParseStatusPorcelainCountsAndUpstream(t *testing.T) {
	output := "## feature/topic...origin/feature/topic [ahead 2, behind 1]\n" +
		" M modified.txt\n" +
		"A  staged.txt\n" +
		"MM both.txt\n" +
		"R  old.txt -> new.txt\n" +
		"?? new.txt\n"

	got := ParseStatusPorcelain(output)

	if got.Upstream != "origin/feature/topic" {
		t.Fatalf("ParseStatusPorcelain().Upstream = %q, want origin/feature/topic", got.Upstream)
	}
	if got.UpstreamGone {
		t.Fatalf("ParseStatusPorcelain().UpstreamGone = true, want false")
	}
	if got.Counts.Staged != 3 || got.Counts.Modified != 2 || got.Counts.Untracked != 1 {
		t.Fatalf("ParseStatusPorcelain().Counts = %+v, want staged 3 modified 2 untracked 1", got.Counts)
	}
}

func TestParseStatusPorcelainGoneUpstream(t *testing.T) {
	got := ParseStatusPorcelain("## feature/topic...origin/feature/topic [gone]\n")

	if !got.UpstreamGone {
		t.Fatalf("ParseStatusPorcelain().UpstreamGone = false, want true")
	}
	if got.Upstream != "origin/feature/topic" {
		t.Fatalf("ParseStatusPorcelain().Upstream = %q, want origin/feature/topic", got.Upstream)
	}
	if !got.Counts.Clean() {
		t.Fatalf("ParseStatusPorcelain().Counts = %+v, want clean", got.Counts)
	}
}

func TestParseAheadBehind(t *testing.T) {
	ahead, behind, ok := ParseAheadBehind("3 5\n")
	if !ok || ahead != 3 || behind != 5 {
		t.Fatalf("ParseAheadBehind(valid) = %d, %d, %t, want 3, 5, true", ahead, behind, ok)
	}

	ahead, behind, ok = ParseAheadBehind("not counts")
	if ok || ahead != 0 || behind != 0 {
		t.Fatalf("ParseAheadBehind(invalid) = %d, %d, %t, want 0, 0, false", ahead, behind, ok)
	}
}

func TestParseRefMetadata(t *testing.T) {
	output := strings.Join([]string{
		"main\x00aaaaaaaa\x00aaaaaaa\x001780000000\x00main commit\x00origin/main\x00\x000 0",
		"feature\x00bbbbbbbb\x00bbbbbbb\x001780000100\x00feature commit\x00origin/feature\x00ahead 2, behind 1\x002 5",
		"gone\x00cccccccc\x00ccccccc\x001780000200\x00gone commit\x00origin/gone\x00gone\x000 3",
		"local\x00dddddddd\x00ddddddd\x001780000300\x00local commit\x00\x00\x001 0",
	}, "\n")

	got := ParseRefMetadata(output)

	feature := got["feature"]
	if feature.ObjectName != "bbbbbbbb" || feature.ObjectShort != "bbbbbbb" || feature.Subject != "feature commit" {
		t.Fatalf("feature metadata parsed incorrectly: %+v", feature)
	}
	if !feature.CommitTime.Equal(time.Unix(1780000100, 0)) {
		t.Fatalf("feature commit time = %s, want unix 1780000100", feature.CommitTime)
	}
	if feature.Upstream != "origin/feature" || !feature.HeadSync.Available || feature.HeadSync.Ahead != 2 || feature.HeadSync.Behind != 1 {
		t.Fatalf("feature upstream sync = upstream %q %+v, want origin/feature ↑2 ↓1", feature.Upstream, feature.HeadSync)
	}
	if !feature.MainSync.Available || feature.MainSync.Ahead != 2 || feature.MainSync.Behind != 5 {
		t.Fatalf("feature main sync = %+v, want ↑2 ↓5", feature.MainSync)
	}
	if !got["gone"].UpstreamGone {
		t.Fatalf("gone upstream not detected: %+v", got["gone"])
	}
	if !got["local"].HeadSync.NoUpstream {
		t.Fatalf("local branch should have no upstream: %+v", got["local"])
	}
}

func TestParseUpstreamTrack(t *testing.T) {
	tests := []struct {
		input      string
		wantGone   bool
		wantAhead  int
		wantBehind int
	}{
		{input: "", wantGone: false},
		{input: "ahead 3", wantAhead: 3},
		{input: "behind 4", wantBehind: 4},
		{input: "ahead 3, behind 4", wantAhead: 3, wantBehind: 4},
		{input: "gone", wantGone: true},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			gone, sync := ParseUpstreamTrack(test.input)
			if gone != test.wantGone || sync.Ahead != test.wantAhead || sync.Behind != test.wantBehind {
				t.Fatalf("ParseUpstreamTrack(%q) = gone %v sync %+v", test.input, gone, sync)
			}
		})
	}
}

func TestParseStatusPorcelainFiles(t *testing.T) {
	output := "## feature/topic...origin/feature/topic [ahead 2, behind 1]\n" +
		" M modified.txt\n" +
		"A  staged.txt\n" +
		"MM both.txt\n" +
		"R  old.txt -> new.txt\n" +
		"?? untracked.txt\n"

	got := ParseStatusPorcelain(output)
	if len(got.Files) != 5 {
		t.Fatalf("ParseStatusPorcelain().Files len = %d, want 5: %+v", len(got.Files), got.Files)
	}

	modified := got.Files[0]
	if modified.Path != "modified.txt" || modified.Staged() || modified.Untracked() || modified.Glyph() != 'M' {
		t.Fatalf("modified entry parsed incorrectly: %+v glyph=%c staged=%v", modified, modified.Glyph(), modified.Staged())
	}
	staged := got.Files[1]
	if staged.Path != "staged.txt" || !staged.Staged() || staged.Glyph() != 'A' {
		t.Fatalf("staged entry parsed incorrectly: %+v glyph=%c staged=%v", staged, staged.Glyph(), staged.Staged())
	}
	rename := got.Files[3]
	if rename.Path != "new.txt" || rename.OrigPath != "old.txt" || rename.Glyph() != 'R' {
		t.Fatalf("rename entry parsed incorrectly: %+v glyph=%c", rename, rename.Glyph())
	}
	untracked := got.Files[4]
	if untracked.Path != "untracked.txt" || !untracked.Untracked() || untracked.Glyph() != '?' {
		t.Fatalf("untracked entry parsed incorrectly: %+v glyph=%c", untracked, untracked.Glyph())
	}
	for _, file := range got.Files {
		if file.HasStats() {
			t.Fatalf("expected no stats before numstat enrichment, got %+v", file)
		}
	}
}

func TestParseNumstat(t *testing.T) {
	output := "42\t3\tmodified.txt\n" +
		"-\t-\timage.png\n" +
		"5\t0\told.go => new.go\n"

	stats := ParseNumstat(output)
	if got, ok := stats["modified.txt"]; !ok || got.Added != 42 || got.Deleted != 3 {
		t.Fatalf("ParseNumstat()[modified.txt] = %+v, ok=%v, want {42 3}", got, ok)
	}
	if _, ok := stats["image.png"]; ok {
		t.Fatalf("ParseNumstat() should skip binary files, got entry for image.png")
	}
	if got, ok := stats["new.go"]; !ok || got.Added != 5 || got.Deleted != 0 {
		t.Fatalf("ParseNumstat() rename keyed by new path = %+v, ok=%v, want {5 0}", got, ok)
	}
}

func TestParseGraphCommits(t *testing.T) {
	output := "9c429f5\x1fdocs: describe copy-path action\n" +
		"40aa71a\x1flocal-side commit\n" +
		"\n"

	commits := ParseGraphCommits(output)
	if len(commits) != 2 {
		t.Fatalf("ParseGraphCommits() len = %d, want 2: %+v", len(commits), commits)
	}
	if commits[0].Short != "9c429f5" || commits[0].Subject != "docs: describe copy-path action" {
		t.Fatalf("first commit parsed incorrectly: %+v", commits[0])
	}
	if commits[1].Short != "40aa71a" || commits[1].Subject != "local-side commit" {
		t.Fatalf("second commit parsed incorrectly: %+v", commits[1])
	}
}
