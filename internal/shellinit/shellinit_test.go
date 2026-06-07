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
		{shell: "zsh", want: []string{"gth() {", "GTH_SHELL_INTEGRATION=1", "command git-treehouse", `cd "$_gth_target" || return`}},
		{shell: "bash", want: []string{"gth() {", "GTH_SHELL_INTEGRATION=1"}},
		{shell: "sh", want: []string{"gth() {", "GTH_SHELL_INTEGRATION=1"}},
		{shell: "dash", want: []string{"gth() {", "GTH_SHELL_INTEGRATION=1"}},
		{shell: "ksh", want: []string{"gth() {", "GTH_SHELL_INTEGRATION=1"}},
		{shell: "fish", want: []string{"function gth", "env GTH_SHELL_INTEGRATION=1", "command git-treehouse", `cd "$target"`}},
		{shell: "nushell", want: []string{"def --env gth", "GTH_SHELL_INTEGRATION", "^git-treehouse", "cd (open $cd_file | str trim)"}},
		{shell: "powershell", want: []string{"function gth", "$env:GTH_SHELL_INTEGRATION", "Get-Command git-treehouse", "Set-Location $target"}},
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

func TestInstallUsesDedicatedFilesForAutoloadingShells(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		shell        string
		path         string
		want         []string
		extraPath    string
		extraContent []string
	}{
		{
			shell: "fish",
			path:  filepath.Join(home, ".config", "fish", "functions", "gth.fish"),
			want:  []string{"function gth", "command git-treehouse"},
		},
		{
			shell: "nushell",
			path:  filepath.Join(home, ".config", "nushell", "autoload", "gth.nu"),
			want:  []string{"def --env gth", "^git-treehouse"},
		},
		{
			shell:        "powershell",
			path:         filepath.Join(powerShellModuleDir(home), "GitTreehouse.psm1"),
			want:         []string{"function gth", "git-treehouse"},
			extraPath:    filepath.Join(powerShellModuleDir(home), "GitTreehouse.psd1"),
			extraContent: []string{"RootModule = 'GitTreehouse.psm1'", "FunctionsToExport = @('gth')"},
		},
	}

	for _, test := range tests {
		t.Run(test.shell, func(t *testing.T) {
			result, err := Install(test.shell)
			if err != nil {
				t.Fatalf("Install(%s) error = %v", test.shell, err)
			}
			if result.Path != test.path {
				t.Fatalf("Install(%s).Path = %q, want %q", test.shell, result.Path, test.path)
			}
			content, err := os.ReadFile(test.path)
			if err != nil {
				t.Fatalf("read %s: %v", test.path, err)
			}
			for _, want := range append([]string{blockStart, blockEnd}, test.want...) {
				if !strings.Contains(string(content), want) {
					t.Fatalf("%s missing %q:\n%s", test.path, want, content)
				}
			}
			if test.extraPath != "" {
				extraContent, err := os.ReadFile(test.extraPath)
				if err != nil {
					t.Fatalf("read %s: %v", test.extraPath, err)
				}
				for _, want := range append([]string{blockStart, blockEnd}, test.extraContent...) {
					if !strings.Contains(string(extraContent), want) {
						t.Fatalf("%s missing %q:\n%s", test.extraPath, want, extraContent)
					}
				}
			}
			if !ConfigFileContainsIntegration(test.shell) {
				t.Fatalf("ConfigFileContainsIntegration(%s) = false after install", test.shell)
			}
			result, err = Install(test.shell)
			if err != nil {
				t.Fatalf("second Install(%s) error = %v", test.shell, err)
			}
			if !result.AlreadyInstalled {
				t.Fatalf("second Install(%s) did not report AlreadyInstalled", test.shell)
			}
		})
	}
}

func TestInstallManagedFileRejectsUnmanagedContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".config", "fish", "functions", "gth.fish")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("function gth\nend\n"), 0600); err != nil {
		t.Fatalf("write unmanaged function: %v", err)
	}

	if _, err := Install("fish"); err == nil || !strings.Contains(err.Error(), "not managed by git-treehouse") {
		t.Fatalf("Install(fish) error = %v, want unmanaged target error", err)
	}
}

func TestConfigFileContainsIntegrationAcceptsManualDedicatedInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fishPath := filepath.Join(home, ".config", "fish", "functions", "gth.fish")
	if err := os.MkdirAll(filepath.Dir(fishPath), 0700); err != nil {
		t.Fatalf("mkdir fish functions: %v", err)
	}
	fishScript, err := Script("fish")
	if err != nil {
		t.Fatalf("Script(fish) error = %v", err)
	}
	if err := os.WriteFile(fishPath, []byte(fishScript), 0600); err != nil {
		t.Fatalf("write fish script: %v", err)
	}

	if !ConfigFileContainsIntegration("fish") {
		t.Fatal("ConfigFileContainsIntegration(fish) = false for manual function install")
	}
	result, err := Install("fish")
	if err != nil {
		t.Fatalf("Install(fish) error = %v", err)
	}
	if !result.AlreadyInstalled {
		t.Fatal("Install(fish) did not report manually installed function as already installed")
	}
}

func TestConfigFileContainsIntegrationAcceptsManualPowerShellModule(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	modulePath := filepath.Join(powerShellModuleDir(home), "GitTreehouse.psm1")
	if err := os.MkdirAll(filepath.Dir(modulePath), 0700); err != nil {
		t.Fatalf("mkdir PowerShell module: %v", err)
	}
	script, err := Script("powershell")
	if err != nil {
		t.Fatalf("Script(powershell) error = %v", err)
	}
	if err := os.WriteFile(modulePath, []byte(script), 0600); err != nil {
		t.Fatalf("write PowerShell module: %v", err)
	}
	manifest := "RootModule = 'GitTreehouse.psm1'\nFunctionsToExport = 'gth'\n"
	if err := os.WriteFile(powerShellManifestPath(modulePath), []byte(manifest), 0600); err != nil {
		t.Fatalf("write PowerShell manifest: %v", err)
	}

	if !ConfigFileContainsIntegration("powershell") {
		t.Fatal("ConfigFileContainsIntegration(powershell) = false for manual module install")
	}
	result, err := Install("powershell")
	if err != nil {
		t.Fatalf("Install(powershell) error = %v", err)
	}
	if !result.AlreadyInstalled {
		t.Fatal("Install(powershell) did not report manually installed module as already installed")
	}
}

func TestConfigFileContainsIntegrationAcceptsLegacyProfileInstalls(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		shell string
		path  string
	}{
		{shell: "fish", path: filepath.Join(home, ".config", "fish", "config.fish")},
		{shell: "nushell", path: filepath.Join(home, ".config", "nushell", "config.nu")},
		{shell: "powershell", path: filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1")},
	}

	for _, test := range tests {
		t.Run(test.shell, func(t *testing.T) {
			if err := os.MkdirAll(filepath.Dir(test.path), 0700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			script, err := Script(test.shell)
			if err != nil {
				t.Fatalf("Script(%s) error = %v", test.shell, err)
			}
			if err := os.WriteFile(test.path, []byte(script), 0600); err != nil {
				t.Fatalf("write legacy install: %v", err)
			}
			if !ConfigFileContainsIntegration(test.shell) {
				t.Fatalf("ConfigFileContainsIntegration(%s) = false for legacy profile install", test.shell)
			}
		})
	}
}

func TestConfigFileContainsIntegration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if ConfigFileContainsIntegration("zsh") {
		t.Fatal("ConfigFileContainsIntegration(zsh) = true before install")
	}
	if _, err := Install("zsh"); err != nil {
		t.Fatalf("Install(zsh) error = %v", err)
	}
	if !ConfigFileContainsIntegration("zsh") {
		t.Fatal("ConfigFileContainsIntegration(zsh) = false after install")
	}
}

func TestConfigPathAndReloadCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := map[string]string{
		"zsh":        filepath.Join(home, ".zshrc"),
		"bash":       filepath.Join(home, ".bashrc"),
		"fish":       filepath.Join(home, ".config", "fish", "functions", "gth.fish"),
		"sh":         filepath.Join(home, ".profile"),
		"dash":       filepath.Join(home, ".profile"),
		"ksh":        filepath.Join(home, ".kshrc"),
		"nushell":    filepath.Join(home, ".config", "nushell", "autoload", "gth.nu"),
		"powershell": filepath.Join(powerShellModuleDir(home), "GitTreehouse.psm1"),
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

	if got := InstallCommand("zsh"); !strings.Contains(got, "git-treehouse init zsh >> ") || !strings.Contains(got, ".zshrc") {
		t.Fatalf("InstallCommand(zsh) = %q", got)
	}
	if got := InstallCommand("fish"); !strings.Contains(got, "functions") || !strings.Contains(got, "gth.fish") || !strings.Contains(got, "> ") {
		t.Fatalf("InstallCommand(fish) = %q", got)
	}
	if got := InstallCommand("nushell"); !strings.Contains(got, "autoload") || !strings.Contains(got, "gth.nu") || !strings.Contains(got, "save --force") {
		t.Fatalf("InstallCommand(nushell) = %q", got)
	}
	if got := InstallCommand("powershell"); !strings.Contains(got, "New-ModuleManifest") || !strings.Contains(got, "GitTreehouse.psm1") || !strings.Contains(got, "FunctionsToExport 'gth'") {
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
