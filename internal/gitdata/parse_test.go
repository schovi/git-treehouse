package gitdata

import "testing"

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
