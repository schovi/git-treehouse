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
			want:     "✗",
		},
		{
			name:     "locked wins",
			worktree: Worktree{Locked: true, IsMain: true, IsActive: true},
			want:     "🔒",
		},
		{
			name:     "main active",
			worktree: Worktree{IsMain: true, IsActive: true},
			want:     "◉",
		},
		{
			name:     "main",
			worktree: Worktree{IsMain: true},
			want:     "⌂",
		},
		{
			name:     "active",
			worktree: Worktree{IsActive: true},
			want:     "●",
		},
		{
			name:     "other",
			worktree: Worktree{},
			want:     "○",
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
			want:     "prunable",
		},
		{
			name:     "locked",
			worktree: Worktree{Locked: true},
			want:     "locked",
		},
		{
			name:     "detached",
			worktree: Worktree{Detached: true},
			want:     "detached",
		},
		{
			name:     "upstream gone",
			worktree: Worktree{UpstreamGone: true},
			want:     "⚠ gone",
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
