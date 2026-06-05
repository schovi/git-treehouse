package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	t.Setenv("EDITOR", "vim")

	got := Default()

	if got.Editor != "vim" {
		t.Fatalf("Default().Editor = %q, want %q", got.Editor, "vim")
	}
	if got.PathTemplate != "{repo_parent}/.worktrees/{repo_name}/{branch}" {
		t.Fatalf("Default().PathTemplate = %q, want default template", got.PathTemplate)
	}
	if got.MainBranch != "" {
		t.Fatalf("Default().MainBranch = %q, want empty", got.MainBranch)
	}
}

func TestDefaultEditorFallback(t *testing.T) {
	t.Setenv("EDITOR", "")

	if got := Default().Editor; got != "code" {
		t.Fatalf("Default().Editor = %q, want %q", got, "code")
	}
}

func TestLoadReadsConfigAndKeepsExplicitValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	err := os.WriteFile(path, []byte(`
editor = "cursor"
path_template = "{repo}/../{branch}"
main_branch = "trunk"
`), 0600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.Editor != "cursor" {
		t.Fatalf("Load().Editor = %q, want %q", got.Editor, "cursor")
	}
	if got.PathTemplate != "{repo}/../{branch}" {
		t.Fatalf("Load().PathTemplate = %q, want explicit template", got.PathTemplate)
	}
	if got.MainBranch != "trunk" {
		t.Fatalf("Load().MainBranch = %q, want %q", got.MainBranch, "trunk")
	}
}

func TestLoadFillsBlankValuesFromDefaults(t *testing.T) {
	t.Setenv("EDITOR", "nano")
	path := filepath.Join(t.TempDir(), "config.toml")
	err := os.WriteFile(path, []byte(`
editor = ""
path_template = ""
main_branch = "main"
`), 0600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.Editor != "nano" {
		t.Fatalf("Load().Editor = %q, want %q", got.Editor, "nano")
	}
	if got.PathTemplate != "{repo_parent}/.worktrees/{repo_name}/{branch}" {
		t.Fatalf("Load().PathTemplate = %q, want default template", got.PathTemplate)
	}
	if got.MainBranch != "main" {
		t.Fatalf("Load().MainBranch = %q, want %q", got.MainBranch, "main")
	}
}

func TestLoadTreatsLegacyDefaultTemplateAsCurrentDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	err := os.WriteFile(path, []byte(`
path_template = "{repo_parent}/{branch}"
`), 0600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.PathTemplate != "{repo_parent}/.worktrees/{repo_name}/{branch}" {
		t.Fatalf("Load().PathTemplate = %q, want current default template", got.PathTemplate)
	}
}

func TestLoadDefaultUsesHomeConfigWhenPresent(t *testing.T) {
	t.Setenv("EDITOR", "vim")
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".config", "git-treehouse")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("make config dir: %v", err)
	}
	err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(`
editor = "zed"
path_template = "{repo_parent}/git-treehouse/{branch}"
main_branch = "develop"
`), 0600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error = %v", err)
	}

	if got.Editor != "zed" {
		t.Fatalf("LoadDefault().Editor = %q, want %q", got.Editor, "zed")
	}
	if got.PathTemplate != "{repo_parent}/git-treehouse/{branch}" {
		t.Fatalf("LoadDefault().PathTemplate = %q, want configured template", got.PathTemplate)
	}
	if got.MainBranch != "develop" {
		t.Fatalf("LoadDefault().MainBranch = %q, want %q", got.MainBranch, "develop")
	}
}

func TestSaveDefaultWritesConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	err := SaveDefault(Config{
		Editor:                      "vim",
		PathTemplate:                "{repo_parent}/.worktrees/{repo_name}/{branch}",
		MainBranch:                  "main",
		SkipShellIntegrationWelcome: true,
	})
	if err != nil {
		t.Fatalf("SaveDefault() error = %v", err)
	}

	got, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error = %v", err)
	}
	if !got.SkipShellIntegrationWelcome {
		t.Fatal("LoadDefault().SkipShellIntegrationWelcome = false, want true")
	}
	if got.Editor != "vim" {
		t.Fatalf("LoadDefault().Editor = %q, want vim", got.Editor)
	}
}

func TestPathUsesXDGConfigHome(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_CONFIG_HOME", configHome)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	want := filepath.Join(configHome, "git-treehouse", "config.toml")
	if got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}
