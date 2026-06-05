package shellinit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScriptReturnsWrappersForSupportedShells(t *testing.T) {
	tests := []struct {
		shell string
		want  []string
	}{
		{shell: "zsh", want: []string{"gwt() {", "GWT_SHELL_INTEGRATION=1", `cd "$_gwt_target" || return`}},
		{shell: "bash", want: []string{"gwt() {", "GWT_SHELL_INTEGRATION=1"}},
		{shell: "sh", want: []string{"gwt() {", "GWT_SHELL_INTEGRATION=1"}},
		{shell: "dash", want: []string{"gwt() {", "GWT_SHELL_INTEGRATION=1"}},
		{shell: "ksh", want: []string{"gwt() {", "GWT_SHELL_INTEGRATION=1"}},
		{shell: "fish", want: []string{"function gwt", "env GWT_SHELL_INTEGRATION=1", `cd "$target"`}},
		{shell: "nushell", want: []string{"def --env gwt", "GWT_SHELL_INTEGRATION", "cd (open $cd_file | str trim)"}},
		{shell: "powershell", want: []string{"function gwt", "$env:GWT_SHELL_INTEGRATION", "Set-Location $target"}},
	}

	for _, test := range tests {
		t.Run(test.shell, func(t *testing.T) {
			script, err := Script(test.shell)
			if err != nil {
				t.Fatalf("Script(%q) error = %v", test.shell, err)
			}
			for _, want := range test.want {
				if !strings.Contains(script, want) {
					t.Fatalf("Script(%q) missing %q:\n%s", test.shell, want, script)
				}
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"/bin/zsh":                  "zsh",
		"/usr/local/bin/bash":       "bash",
		"/opt/homebrew/bin/fish":    "fish",
		"/usr/bin/dash":             "dash",
		"/bin/ksh":                  "ksh",
		"/opt/homebrew/bin/nu":      "nushell",
		"/usr/local/bin/pwsh":       "powershell",
		`C:\Program Files\pwsh.exe`: "powershell",
		"/bin/tcsh":                 "",
	}
	for input, want := range tests {
		if got := Normalize(input); got != want {
			t.Fatalf("Normalize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestInstallAppendsBlockOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	result, err := Install("zsh")
	if err != nil {
		t.Fatalf("Install(zsh) error = %v", err)
	}
	if result.AlreadyInstalled {
		t.Fatal("first Install(zsh) reported AlreadyInstalled")
	}
	if result.Path != filepath.Join(home, ".zshrc") {
		t.Fatalf("Install(zsh).Path = %q, want ~/.zshrc", result.Path)
	}
	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read installed file: %v", err)
	}
	if count := strings.Count(string(content), blockStart); count != 1 {
		t.Fatalf("installed block count = %d, want 1:\n%s", count, content)
	}

	result, err = Install("zsh")
	if err != nil {
		t.Fatalf("second Install(zsh) error = %v", err)
	}
	if !result.AlreadyInstalled {
		t.Fatal("second Install(zsh) did not report AlreadyInstalled")
	}
	content, err = os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read installed file: %v", err)
	}
	if count := strings.Count(string(content), blockStart); count != 1 {
		t.Fatalf("installed block count after second install = %d, want 1:\n%s", count, content)
	}
}

func TestConfigPathAndReloadCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := map[string]string{
		"zsh":        filepath.Join(home, ".zshrc"),
		"bash":       filepath.Join(home, ".bashrc"),
		"fish":       filepath.Join(home, ".config", "fish", "config.fish"),
		"sh":         filepath.Join(home, ".profile"),
		"dash":       filepath.Join(home, ".profile"),
		"ksh":        filepath.Join(home, ".kshrc"),
		"nushell":    filepath.Join(home, ".config", "nushell", "config.nu"),
		"powershell": filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"),
	}
	for shell, want := range tests {
		got, err := ConfigPath(shell)
		if err != nil {
			t.Fatalf("ConfigPath(%q) error = %v", shell, err)
		}
		if got != want {
			t.Fatalf("ConfigPath(%q) = %q, want %q", shell, got, want)
		}
		if ReloadCommand(shell, got) == "" {
			t.Fatalf("ReloadCommand(%q) is empty", shell)
		}
	}
}

func TestInstallCommandUsesShellSpecificSyntax(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got := InstallCommand("zsh"); !strings.Contains(got, "gwt init zsh >> ") || !strings.Contains(got, ".zshrc") {
		t.Fatalf("InstallCommand(zsh) = %q", got)
	}
	if got := InstallCommand("nushell"); !strings.Contains(got, "save --append") || !strings.Contains(got, "config.nu") {
		t.Fatalf("InstallCommand(nushell) = %q", got)
	}
	if got := InstallCommand("powershell"); !strings.Contains(got, "gwt init powershell >> ") || !strings.Contains(got, "Microsoft.PowerShell_profile.ps1") {
		t.Fatalf("InstallCommand(powershell) = %q", got)
	}
}

func TestQuotePath(t *testing.T) {
	if got := quotePath("/tmp/has space/it"); got != "'/tmp/has space/it'" {
		t.Fatalf("quotePath() = %q", got)
	}
	if got := quotePath("/tmp/has'quote"); got != "'/tmp/has'\\''quote'" {
		t.Fatalf("quotePath() = %q", got)
	}
}

func TestScriptRejectsUnsupportedShell(t *testing.T) {
	_, err := Script("tcsh")
	if err == nil {
		t.Fatal("Script(tcsh) error = nil, want unsupported shell error")
	}
	if !strings.Contains(err.Error(), "unsupported shell") {
		t.Fatalf("Script(tcsh) error = %q, want unsupported shell message", err.Error())
	}
}
