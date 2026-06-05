package shellinit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	blockStart = "# >>> gth shell integration >>>"
	blockEnd   = "# <<< gth shell integration <<<"
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
	script, err := Script(shell)
	if err != nil {
		return InstallResult{}, err
	}
	path, err := ConfigPath(shell)
	if err != nil {
		return InstallResult{}, err
	}
	result := InstallResult{Path: path, ReloadCommand: ReloadCommand(shell, path)}
	content, err := os.ReadFile(path)
	if err == nil && strings.Contains(string(content), blockStart) {
		result.AlreadyInstalled = true
		return result, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return InstallResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return InstallResult{}, err
	}
	block := "\n" + blockStart + "\n" + strings.TrimRight(script, "\n") + "\n" + blockEnd + "\n"
	if len(content) == 0 {
		block = strings.TrimLeft(block, "\n")
	}
	if err := os.WriteFile(path, append(content, []byte(block)...), 0600); err != nil { // #nosec G703 -- ConfigPath constrains shell install targets to known user profile files.
		return InstallResult{}, err
	}
	return result, nil
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
		return filepath.Join(home, ".config", "fish", "config.fish"), nil
	case "sh", "dash":
		return filepath.Join(home, ".profile"), nil
	case "ksh":
		return filepath.Join(home, ".kshrc"), nil
	case "nushell":
		return filepath.Join(home, ".config", "nushell", "config.nu"), nil
	case "powershell":
		return filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"), nil
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
	case "nushell":
		return fmt.Sprintf("git-treehouse init nushell | save --append %s", quotePath(path))
	case "powershell":
		return fmt.Sprintf("git-treehouse init powershell >> %s", quotePath(path))
	default:
		return fmt.Sprintf("git-treehouse init %s >> %s", shell, quotePath(path))
	}
}

func ReloadCommand(shell, path string) string {
	switch Normalize(shell) {
	case "fish", "nushell":
		return "source " + quotePath(path)
	case "powershell":
		return ". " + quotePath(path)
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
