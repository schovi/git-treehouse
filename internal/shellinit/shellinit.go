package shellinit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	blockStart = "# >>> gwt shell integration >>>"
	blockEnd   = "# <<< gwt shell integration <<<"
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
	if err := os.WriteFile(path, append(content, []byte(block)...), 0600); err != nil {
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
		return "gwt init fish | source"
	case "nushell":
		return "gwt init nushell | save --force /tmp/gwt.nu; source /tmp/gwt.nu"
	case "powershell":
		return `gwt init powershell | Invoke-Expression`
	default:
		return fmt.Sprintf("eval \"$(gwt init %s)\"", Normalize(shell))
	}
}

func InstallCommand(shell string) string {
	shell = Normalize(shell)
	path, err := ConfigPath(shell)
	if err != nil {
		return "gwt init " + shell
	}
	switch shell {
	case "nushell":
		return fmt.Sprintf("gwt init nushell | save --append %s", quotePath(path))
	case "powershell":
		return fmt.Sprintf("gwt init powershell >> %s", quotePath(path))
	default:
		return fmt.Sprintf("gwt init %s >> %s", shell, quotePath(path))
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
	return `gwt() {
  _gwt_cd_file="$(mktemp -t gwt.XXXXXX)" || return
  GWT_SHELL_INTEGRATION=1 command gwt --cd-file "$_gwt_cd_file" "$@"
  _gwt_status=$?
  if [ -s "$_gwt_cd_file" ]; then
    _gwt_target="$(cat "$_gwt_cd_file")"
    rm -f "$_gwt_cd_file"
    if [ -n "$_gwt_target" ]; then
      cd "$_gwt_target" || return
    fi
  else
    rm -f "$_gwt_cd_file"
  fi
  return "$_gwt_status"
}
`
}

func fishScript() string {
	return `function gwt
  set cd_file (mktemp -t gwt.XXXXXX)
  env GWT_SHELL_INTEGRATION=1 command gwt --cd-file $cd_file $argv
  set gwt_status $status
  if test -s $cd_file
    set target (cat $cd_file)
    rm -f $cd_file
    if test -n "$target"
      cd "$target"
    end
  else
    rm -f $cd_file
  end
  return $gwt_status
end
`
}

func nushellScript() string {
	return `def --env gwt [...args] {
  let cd_file = (mktemp -t gwt.XXXXXX)
  with-env { GWT_SHELL_INTEGRATION: "1" } {
    ^gwt --cd-file $cd_file ...$args
  }
  let gwt_status = $env.LAST_EXIT_CODE
  if ($cd_file | path exists) and ((open $cd_file | str trim) != "") {
    cd (open $cd_file | str trim)
  }
  rm -f $cd_file
  $gwt_status
}
`
}

func powershellScript() string {
	return `function gwt {
  $cdFile = New-TemporaryFile
  $previousIntegration = $env:GWT_SHELL_INTEGRATION
  try {
    $env:GWT_SHELL_INTEGRATION = "1"
    $gwtCommand = (Get-Command gwt -CommandType Application).Source
    & $gwtCommand --cd-file $cdFile.FullName @args
    $gwtStatus = $LASTEXITCODE
    if ((Test-Path $cdFile.FullName) -and ((Get-Item $cdFile.FullName).Length -gt 0)) {
      $target = (Get-Content -Raw $cdFile.FullName).Trim()
      if ($target.Length -gt 0) {
        Set-Location $target
      }
    }
  } finally {
    Remove-Item $cdFile.FullName -ErrorAction SilentlyContinue
    if ($null -eq $previousIntegration) {
      Remove-Item Env:GWT_SHELL_INTEGRATION -ErrorAction SilentlyContinue
    } else {
      $env:GWT_SHELL_INTEGRATION = $previousIntegration
    }
  }
  $global:LASTEXITCODE = $gwtStatus
}
`
}
