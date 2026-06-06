package gitdata

import (
	"testing"
	"time"
)

func TestWorktreeMarker(t *testing.T) {
	tests := []struct {
		name     string
		worktree Worktree
		want     string
	}{
		{
			name:     "prunable wins",
			worktree: Worktree{Prunable: true, Locked: true, IsMain: true, IsActive: true},
			want:     "×",
		},
		{
			name:     "locked wins",
			worktree: Worktree{Locked: true, IsMain: true, IsActive: true},
			want:     "!",
		},
		{
			name:     "main",
			worktree: Worktree{IsMain: true},
			want:     "⌂",
		},
		{
			name:     "active",
			worktree: Worktree{IsActive: true},
			want:     "",
		},
		{
			name:     "other",
			worktree: Worktree{},
			want:     "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.worktree.Marker(); got != test.want {
				t.Fatalf("Marker() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWorktreeStatusText(t *testing.T) {
	tests := []struct {
		name     string
		worktree Worktree
		want     string
	}{
		{
			name:     "prunable",
			worktree: Worktree{Prunable: true, Status: StatusCounts{Staged: 1}},
			want:     "+1",
		},
		{
			name:     "locked",
			worktree: Worktree{Locked: true},
			want:     "✓",
		},
		{
			name:     "detached",
			worktree: Worktree{Detached: true},
			want:     "✓",
		},
		{
			name:     "upstream gone",
			worktree: Worktree{UpstreamGone: true},
			want:     "✓",
		},
		{
			name:     "clean",
			worktree: Worktree{},
			want:     "✓",
		},
		{
			name:     "dirty counts",
			worktree: Worktree{Status: StatusCounts{Staged: 2, Modified: 3, Untracked: 1}},
			want:     "+2 ~3 ?1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.worktree.StatusText(); got != test.want {
				t.Fatalf("StatusText() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWorktreeDisplayBranchIncludesLifecycleSuffixes(t *testing.T) {
	tests := []struct {
		name     string
		worktree Worktree
		want     string
	}{
		{
			name:     "branch",
			worktree: Worktree{Branch: "feature/login"},
			want:     "feature/login",
		},
		{
			name:     "locked",
			worktree: Worktree{Branch: "experiment/locked", Locked: true},
			want:     "experiment/locked locked",
		},
		{
			name:     "prunable",
			worktree: Worktree{Branch: "stale/abandoned", Prunable: true},
			want:     "stale/abandoned prunable",
		},
		{
			name:     "detached",
			worktree: Worktree{Head: "abcdef123456", Detached: true},
			want:     "abcdef1 detached",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.worktree.DisplayBranch(); got != test.want {
				t.Fatalf("DisplayBranch() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSyncStateRemoteCompact(t *testing.T) {
	tests := []struct {
		name         string
		sync         SyncState
		upstreamGone bool
		want         string
	}{
		{name: "gone", upstreamGone: true, want: "gone"},
		{name: "none", sync: SyncState{NoUpstream: true}, want: "-"},
		{name: "pending", sync: SyncState{}, want: ""},
		{name: "synced", sync: SyncState{Available: true}, want: "✓"},
		{name: "ahead", sync: SyncState{Available: true, Ahead: 2}, want: "↑2"},
		{name: "behind", sync: SyncState{Available: true, Behind: 1}, want: "↓1"},
		{name: "diverged", sync: SyncState{Available: true, Ahead: 2, Behind: 1}, want: "↑2 ↓1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.sync.RemoteCompact(test.upstreamGone); got != test.want {
				t.Fatalf("RemoteCompact() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWorktreeDetail(t *testing.T) {
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	worktree := Worktree{
		Path:          "/repo/worktree",
		Status:        StatusCounts{Staged: 1, Modified: 2, Untracked: 3},
		Upstream:      "origin/feature",
		HeadSync:      SyncState{Available: true, Ahead: 2, Behind: 1},
		CommitShort:   "abc1234",
		CommitSubject: "introduce list view",
		CommitTime:    now.Add(-3 * time.Hour),
	}

	want := "/repo/worktree · staged 1, modified 2, untracked 3 · upstream origin/feature ↑2 ↓1 · abc1234 introduce list view 3h"
	if got := worktree.Detail(now); got != want {
		t.Fatalf("Detail() = %q, want %q", got, want)
	}
}

func TestWorktreeDetailWithoutUpstream(t *testing.T) {
	worktree := Worktree{Path: "/repo/main"}

	want := "/repo/main · clean · no upstream"
	if got := worktree.Detail(time.Now()); got != want {
		t.Fatalf("Detail() = %q, want %q", got, want)
	}
}
