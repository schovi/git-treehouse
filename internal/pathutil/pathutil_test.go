package pathutil

import (
	"path/filepath"
	"testing"
)

func TestSanitizeBranch(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		want   string
	}{
		{
			name:   "trims refs spaces and separators",
			branch: " refs/heads/feature/new UI ",
			want:   "feature-new-UI",
		},
		{
			name:   "collapses repeated separators",
			branch: "bugfix///nested\\branch   name",
			want:   "bugfix-nested-branch-name",
		},
		{
			name:   "trims generated dashes",
			branch: " /release/next/ ",
			want:   "release-next",
		},
		{
			name:   "preserves non separator characters",
			branch: "team/JIRA-123_fix.v2",
			want:   "team-JIRA-123_fix.v2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := SanitizeBranch(test.branch); got != test.want {
				t.Fatalf("SanitizeBranch(%q) = %q, want %q", test.branch, got, test.want)
			}
		})
	}
}

func TestApplyTemplate(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")

	tests := []struct {
		name     string
		template string
		branch   string
		want     string
	}{
		{
			name:     "default uses repo parent and sanitized branch",
			template: "",
			branch:   "feature/new UI",
			want:     filepath.Join(filepath.Dir(repoRoot), "feature-new-UI"),
		},
		{
			name:     "relative template is rooted in repo",
			template: "worktrees/{branch}",
			branch:   "bug/fix",
			want:     filepath.Join(repoRoot, "worktrees", "bug-fix"),
		},
		{
			name:     "expands repo placeholders and cleans path",
			template: "{repo}/../siblings/{branch}",
			branch:   "team\\branch",
			want:     filepath.Join(filepath.Dir(repoRoot), "siblings", "team-branch"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ApplyTemplate(test.template, repoRoot, test.branch); got != test.want {
				t.Fatalf("ApplyTemplate(%q, %q, %q) = %q, want %q", test.template, repoRoot, test.branch, got, test.want)
			}
		})
	}
}
