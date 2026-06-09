package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLoadRepoConfigReadsWorktreeFile(t *testing.T) {
	repoRoot := t.TempDir()
	err := os.WriteFile(filepath.Join(repoRoot, ".worktree"), []byte(`
path_template = "{repo}/worktrees/{branch}"
copy_untracked = [".env", ".env.local"]
post_create = "scripts/setup"
before_delete = "scripts/cleanup"
`), 0600)
	if err != nil {
		t.Fatalf("write repo config: %v", err)
	}

	got, err := LoadRepoConfig(repoRoot)
	if err != nil {
		t.Fatalf("LoadRepoConfig() error = %v", err)
	}

	if got.PathTemplate != "{repo}/worktrees/{branch}" {
		t.Fatalf("LoadRepoConfig().PathTemplate = %q, want configured template", got.PathTemplate)
	}
	if !slices.Equal(got.CopyUntracked, []string{".env", ".env.local"}) {
		t.Fatalf("LoadRepoConfig().CopyUntracked = %#v, want .env files", got.CopyUntracked)
	}
	if got.PostCreate != "scripts/setup" {
		t.Fatalf("LoadRepoConfig().PostCreate = %q, want scripts/setup", got.PostCreate)
	}
	if got.BeforeDelete != "scripts/cleanup" {
		t.Fatalf("LoadRepoConfig().BeforeDelete = %q, want scripts/cleanup", got.BeforeDelete)
	}
}

func TestLoadRepoConfigMissingFileReturnsZeroConfig(t *testing.T) {
	got, err := LoadRepoConfig(t.TempDir())
	if err != nil {
		t.Fatalf("LoadRepoConfig() error = %v", err)
	}
	if got.PathTemplate != "" || len(got.CopyUntracked) != 0 || got.PostCreate != "" || got.BeforeDelete != "" {
		t.Fatalf("LoadRepoConfig() = %#v, want zero config", got)
	}
}

func TestLoadRepoConfigPartialFileKeepsZeroValues(t *testing.T) {
	repoRoot := t.TempDir()
	err := os.WriteFile(filepath.Join(repoRoot, ".worktree"), []byte(`
before_delete = "scripts/cleanup"
`), 0600)
	if err != nil {
		t.Fatalf("write repo config: %v", err)
	}

	got, err := LoadRepoConfig(repoRoot)
	if err != nil {
		t.Fatalf("LoadRepoConfig() error = %v", err)
	}

	if got.PathTemplate != "" {
		t.Fatalf("LoadRepoConfig().PathTemplate = %q, want empty", got.PathTemplate)
	}
	if len(got.CopyUntracked) != 0 {
		t.Fatalf("LoadRepoConfig().CopyUntracked = %#v, want empty", got.CopyUntracked)
	}
	if got.PostCreate != "" {
		t.Fatalf("LoadRepoConfig().PostCreate = %q, want empty", got.PostCreate)
	}
	if got.BeforeDelete != "scripts/cleanup" {
		t.Fatalf("LoadRepoConfig().BeforeDelete = %q, want scripts/cleanup", got.BeforeDelete)
	}
}

func TestRepoConfigHasHooks(t *testing.T) {
	tests := []struct {
		name   string
		config RepoConfig
		want   bool
	}{
		{name: "none"},
		{name: "post create", config: RepoConfig{PostCreate: "scripts/setup"}, want: true},
		{name: "before delete", config: RepoConfig{BeforeDelete: "scripts/cleanup"}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.config.HasHooks(); got != test.want {
				t.Fatalf("HasHooks() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRepoConfigHookHashIsStable(t *testing.T) {
	config := RepoConfig{
		PostCreate:   "scripts/setup",
		BeforeDelete: "scripts/cleanup",
	}
	want := "c27f5cdc78ef3bcd9083b918c4b14a547d300b7d812d44b09fbd5f2e71bc3124"

	if got := HookHash(config); got != want {
		t.Fatalf("HookHash() = %q, want %q", got, want)
	}
}

func TestRepoConfigHookHashDependsOnlyOnHooks(t *testing.T) {
	config := RepoConfig{
		PathTemplate:  "{repo}/worktrees/{branch}",
		CopyUntracked: []string{".env"},
		PostCreate:    "scripts/setup",
		BeforeDelete:  "scripts/cleanup",
	}
	sameHooks := RepoConfig{
		PathTemplate:  "{repo}/other/{branch}",
		CopyUntracked: []string{".env.local"},
		PostCreate:    "scripts/setup",
		BeforeDelete:  "scripts/cleanup",
	}
	differentHooks := RepoConfig{
		PathTemplate:  config.PathTemplate,
		CopyUntracked: config.CopyUntracked,
		PostCreate:    config.PostCreate,
		BeforeDelete:  "scripts/teardown",
	}

	if HookHash(config) != HookHash(sameHooks) {
		t.Fatal("HookHash() changed when non-hook fields changed")
	}
	if HookHash(config) == HookHash(differentHooks) {
		t.Fatal("HookHash() did not change when hook fields changed")
	}
}

func TestEffectivePathTemplateUsesRepoOverride(t *testing.T) {
	global := Config{PathTemplate: "{repo_parent}/global/{branch}"}
	repo := RepoConfig{PathTemplate: "{repo}/local/{branch}"}

	if got := EffectivePathTemplate(global, repo); got != repo.PathTemplate {
		t.Fatalf("EffectivePathTemplate() = %q, want repo template", got)
	}
}

func TestEffectivePathTemplateFallsBackToGlobal(t *testing.T) {
	global := Config{PathTemplate: "{repo_parent}/global/{branch}"}

	if got := EffectivePathTemplate(global, RepoConfig{}); got != global.PathTemplate {
		t.Fatalf("EffectivePathTemplate() = %q, want global template", got)
	}
}
