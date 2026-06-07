package shellinit

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	blockStart           = "# >>> gth shell integration >>>"
	blockEnd             = "# <<< gth shell integration <<<"
	powerShellModuleName = "GitTreehouse"
)

type InstallResult struct {
	Path             string
	ReloadCommand    string
	AlreadyInstalled bool
}

func Script(shell string) (string, error) {
	switch Normalize(shell) {
	case "fish":
		return fishScript(), nil
	case "zsh", "bash", "sh", "dash", "ksh":
		return posixScript(), nil
	case "nushell":
		return nushellScript(), nil
	case "powershell":
		return powershellScript(), nil
	default:
		return "", fmt.Errorf("unsupported shell %q, expected %s", shell, strings.Join(SupportedShells(), ", "))
	}
}

func Normalize(shell string) string {
	switch strings.ToLower(shellBase(shell)) {
	case "fish":
		return "fish"
	case "zsh":
		return "zsh"
	case "bash":
		return "bash"
	case "sh":
		return "sh"
	case "dash":
		return "dash"
	case "ksh", "mksh":
		return "ksh"
	case "nu", "nushell":
		return "nushell"
	case "pwsh", "powershell", "powershell.exe", "pwsh.exe":
		return "powershell"
	default:
		return ""
	}
}

func shellBase(shell string) string {
	shell = strings.TrimSpace(shell)
	if shell == "" {
		return ""
	}
	index := strings.LastIndexAny(shell, `/\`)
	if index >= 0 {
		return shell[index+1:]
	}
	return shell
}

func SupportedShells() []string {
	return []string{"zsh", "bash", "fish", "sh", "dash", "ksh", "nushell", "powershell"}
}

func Install(shell string) (InstallResult, error) {
	shell = Normalize(shell)
	if shell == "" {
		return InstallResult{}, fmt.Errorf("unsupported shell")
	}
	targets, err := installTargets(shell)
	if err != nil {
		return InstallResult{}, err
	}
	path := targets[0].path
	result := InstallResult{Path: path, ReloadCommand: ReloadCommand(shell, path)}
	if installTargetsAlreadyInstalled(targets) {
		result.AlreadyInstalled = true
		return result, nil
	}
	for _, target := range targets {
		if err := installTarget(target); err != nil {
			return InstallResult{}, err
		}
	}
	return result, nil
}

func ConfigFileContainsIntegration(shell string) bool {
	targets, err := installTargets(shell)
	if err != nil {
		return false
	}
	if installTargetsAlreadyInstalled(targets) {
		return true
	}
	legacyTarget, err := legacyInstallTarget(shell)
	if err != nil {
		return false
	}
	return installTargetsAlreadyInstalled([]installTargetFile{legacyTarget})
}

func ConfigPath(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch Normalize(shell) {
	case "zsh":
		return filepath.Join(home, ".zshrc"), nil
	case "bash":
		return filepath.Join(home, ".bashrc"), nil
	case "fish":
		return filepath.Join(home, ".config", "fish", "functions", "gth.fish"), nil
	case "sh", "dash":
		return filepath.Join(home, ".profile"), nil
	case "ksh":
		return filepath.Join(home, ".kshrc"), nil
	case "nushell":
		return filepath.Join(home, ".config", "nushell", "autoload", "gth.nu"), nil
	case "powershell":
		return filepath.Join(powerShellModuleDir(home), powerShellModuleName+".psm1"), nil
	default:
		return "", fmt.Errorf("unsupported shell %q", shell)
	}
}

func ActivationCommand(shell string) string {
	switch Normalize(shell) {
	case "fish":
		return "git-treehouse init fish | source"
	case "nushell":
		return "git-treehouse init nushell | save --force /tmp/gth.nu; source /tmp/gth.nu"
	case "powershell":
		return `git-treehouse init powershell | Invoke-Expression`
	default:
		return fmt.Sprintf("eval \"$(git-treehouse init %s)\"", Normalize(shell))
	}
}

func InstallCommand(shell string) string {
	shell = Normalize(shell)
	path, err := ConfigPath(shell)
	if err != nil {
		return "git-treehouse init " + shell
	}
	switch shell {
	case "fish":
		return fmt.Sprintf("mkdir -p %s; and git-treehouse init fish > %s", quotePath(filepath.Dir(path)), quotePath(path))
	case "nushell":
		return fmt.Sprintf("mkdir %s; git-treehouse init nushell | save --force %s", quotePath(filepath.Dir(path)), quotePath(path))
	case "powershell":
		moduleDir := filepath.Dir(path)
		return fmt.Sprintf("New-Item -ItemType Directory -Force %s | Out-Null; git-treehouse init powershell > %s; New-ModuleManifest -Path %s -RootModule %s -ModuleVersion '1.0.0' -FunctionsToExport 'gth' -CmdletsToExport @() -AliasesToExport @() -VariablesToExport @() -Force", quotePowerShellString(moduleDir), quotePowerShellString(path), quotePowerShellString(powerShellManifestPath(path)), quotePowerShellString(powerShellModuleName+".psm1"))
	default:
		return fmt.Sprintf("git-treehouse init %s >> %s", shell, quotePath(path))
	}
}

func ReloadCommand(shell, path string) string {
	switch Normalize(shell) {
	case "fish", "nushell":
		return "source " + quotePath(path)
	case "powershell":
		return "Import-Module " + quotePowerShellString(powerShellManifestPath(path)) + " -Force"
	default:
		return ". " + quotePath(path)
	}
}

func quotePath(path string) string {
	if path == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

func quotePowerShellString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

type installMode int

const (
	installModeAppend installMode = iota
	installModeManaged
)

type installTargetFile struct {
	path       string
	content    string
	mode       installMode
	indicators []string
}

func installTargets(shell string) ([]installTargetFile, error) {
	shell = Normalize(shell)
	script, err := Script(shell)
	if err != nil {
		return nil, err
	}
	path, err := ConfigPath(shell)
	if err != nil {
		return nil, err
	}
	switch shell {
	case "fish", "nushell":
		return []installTargetFile{{
			path:       path,
			content:    markedScript(script),
			mode:       installModeManaged,
			indicators: shellScriptIndicators(shell),
		}}, nil
	case "powershell":
		return []installTargetFile{
			{
				path:       path,
				content:    markedScript(script),
				mode:       installModeManaged,
				indicators: shellScriptIndicators(shell),
			},
			{
				path:       powerShellManifestPath(path),
				content:    markedScript(powerShellManifest()),
				mode:       installModeManaged,
				indicators: powerShellManifestIndicators(),
			},
		}, nil
	default:
		return []installTargetFile{{
			path:       path,
			content:    markedScript(script),
			mode:       installModeAppend,
			indicators: shellScriptIndicators(shell),
		}}, nil
	}
}

func legacyInstallTarget(shell string) (installTargetFile, error) {
	shell = Normalize(shell)
	home, err := os.UserHomeDir()
	if err != nil {
		return installTargetFile{}, err
	}
	switch shell {
	case "fish":
		return installTargetFile{
			path:       filepath.Join(home, ".config", "fish", "config.fish"),
			indicators: shellScriptIndicators(shell),
		}, nil
	case "nushell":
		return installTargetFile{
			path:       filepath.Join(home, ".config", "nushell", "config.nu"),
			indicators: shellScriptIndicators(shell),
		}, nil
	case "powershell":
		return installTargetFile{
			path:       filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"),
			indicators: shellScriptIndicators(shell),
		}, nil
	default:
		return installTargetFile{}, fmt.Errorf("no legacy install target for %q", shell)
	}
}

func installTargetsAlreadyInstalled(targets []installTargetFile) bool {
	for _, target := range targets {
		content, err := os.ReadFile(target.path)
		if err != nil || !containsIntegration(string(content), target.indicators) {
			return false
		}
	}
	return true
}

func installTarget(target installTargetFile) error {
	content, err := os.ReadFile(target.path)
	if err == nil && containsIntegration(string(content), target.indicators) {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && target.mode == installModeManaged && len(content) > 0 {
		return fmt.Errorf("%s already exists and is not managed by git-treehouse", target.path)
	}
	if err := os.MkdirAll(filepath.Dir(target.path), 0700); err != nil {
		return err
	}
	switch target.mode {
	case installModeAppend:
		content = appendIntegrationBlock(content, target.content)
	case installModeManaged:
		content = []byte(target.content)
	}
	if err := os.WriteFile(target.path, content, 0600); err != nil { // #nosec G703 -- ConfigPath constrains shell install targets to known user integration files.
		return err
	}
	return nil
}

func appendIntegrationBlock(content []byte, block string) []byte {
	if len(content) == 0 {
		return []byte(block)
	}
	return append(content, []byte("\n"+block)...)
}

func markedScript(script string) string {
	return blockStart + "\n" + strings.TrimRight(script, "\n") + "\n" + blockEnd + "\n"
}

func containsIntegration(content string, indicators []string) bool {
	if strings.Contains(content, blockStart) {
		return true
	}
	for _, indicator := range indicators {
		if !strings.Contains(content, indicator) {
			return false
		}
	}
	return len(indicators) > 0
}

func shellScriptIndicators(shell string) []string {
	switch shell {
	case "fish":
		return []string{"function gth", "GTH_SHELL_INTEGRATION=1", "git-treehouse --cd-file"}
	case "nushell":
		return []string{"def --env gth", "GTH_SHELL_INTEGRATION", "^git-treehouse --cd-file"}
	case "powershell":
		return []string{"function gth", "$env:GTH_SHELL_INTEGRATION", "git-treehouse", "--cd-file"}
	default:
		return []string{"gth() {", "GTH_SHELL_INTEGRATION=1 command git-treehouse", "--cd-file"}
	}
}

func powerShellModuleDir(home string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "Documents", "PowerShell", "Modules", powerShellModuleName)
	}
	return filepath.Join(home, ".local", "share", "powershell", "Modules", powerShellModuleName)
}

func powerShellManifestPath(modulePath string) string {
	return filepath.Join(filepath.Dir(modulePath), powerShellModuleName+".psd1")
}

func powerShellManifest() string {
	return `@{
  RootModule = 'GitTreehouse.psm1'
  ModuleVersion = '1.0.0'
  GUID = '7c59f7af-4d75-4a3c-9d13-1391d6b0ec2b'
  Author = 'Git Treehouse'
  Description = 'Git Treehouse shell integration'
  PowerShellVersion = '5.1'
  FunctionsToExport = @('gth')
  CmdletsToExport = @()
  VariablesToExport = @()
  AliasesToExport = @()
}
`
}

func powerShellManifestIndicators() []string {
	return []string{"RootModule", "GitTreehouse.psm1", "FunctionsToExport", "gth"}
}

func posixScript() string {
	return `gth() {
  _gth_cd_file="$(mktemp -t gth.XXXXXX)" || return
  GTH_SHELL_INTEGRATION=1 command git-treehouse --cd-file "$_gth_cd_file" "$@"
  _gth_status=$?
  if [ -s "$_gth_cd_file" ]; then
    _gth_target="$(cat "$_gth_cd_file")"
    rm -f "$_gth_cd_file"
    if [ -n "$_gth_target" ]; then
      cd "$_gth_target" || return
    fi
  else
    rm -f "$_gth_cd_file"
  fi
  return "$_gth_status"
}
`
}

func fishScript() string {
	return `function gth
  set cd_file (mktemp -t gth.XXXXXX)
  env GTH_SHELL_INTEGRATION=1 command git-treehouse --cd-file $cd_file $argv
  set gth_status $status
  if test -s $cd_file
    set target (cat $cd_file)
    rm -f $cd_file
    if test -n "$target"
      cd "$target"
    end
  else
    rm -f $cd_file
  end
  return $gth_status
end
`
}

func nushellScript() string {
	return `def --env gth [...args] {
  let cd_file = (mktemp -t gth.XXXXXX)
  with-env { GTH_SHELL_INTEGRATION: "1" } {
    ^git-treehouse --cd-file $cd_file ...$args
  }
  let gth_status = $env.LAST_EXIT_CODE
  if ($cd_file | path exists) and ((open $cd_file | str trim) != "") {
    cd (open $cd_file | str trim)
  }
  rm -f $cd_file
  $gth_status
}
`
}

func powershellScript() string {
	return `function gth {
  $cdFile = New-TemporaryFile
  $previousIntegration = $env:GTH_SHELL_INTEGRATION
  try {
    $env:GTH_SHELL_INTEGRATION = "1"
    $gthCommand = (Get-Command git-treehouse -CommandType Application).Source
    & $gthCommand --cd-file $cdFile.FullName @args
    $gthStatus = $LASTEXITCODE
    if ((Test-Path $cdFile.FullName) -and ((Get-Item $cdFile.FullName).Length -gt 0)) {
      $target = (Get-Content -Raw $cdFile.FullName).Trim()
      if ($target.Length -gt 0) {
        Set-Location $target
      }
    }
  } finally {
    Remove-Item $cdFile.FullName -ErrorAction SilentlyContinue
    if ($null -eq $previousIntegration) {
      Remove-Item Env:GTH_SHELL_INTEGRATION -ErrorAction SilentlyContinue
    } else {
      $env:GTH_SHELL_INTEGRATION = $previousIntegration
    }
  }
  $global:LASTEXITCODE = $gthStatus
}
`
}
