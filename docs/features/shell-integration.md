# Shell integration

_Behavior spec. Index: [docs/README.md](../README.md) · Code: [docs/architecture.md](../architecture.md)._

A child process cannot change its parent shell's cwd, so Git Treehouse uses the zoxide/yazi pattern:

- The TUI writes the selected path to a file given via `--cd-file <path>` (nothing else goes to that file).
- The `gth` wrapper function (installed via `git-treehouse init fish | source` etc.) runs the binary with a temp `--cd-file`, and `cd`s to its content after the TUI exits, if non-empty.
- `git-treehouse` remains the native CLI for non-navigating commands and direct invocation. `gth` is the directory-changing command.
- Quitting without selecting writes nothing; the shell stays where it was.
- **Graceful degradation:** without `--cd-file`, the selected path is printed to stdout on exit (TUI renders on stderr/tty), so `cd (git-treehouse)` works bare.

**Supported shells** for `init`: `zsh`, `bash`, `fish`, `sh`, `dash`, `ksh`, `nushell`, `powershell`. Common aliases are normalized (`mksh` → `ksh`, `nu` → `nushell`, `pwsh`/`pwsh.exe`/`powershell.exe` → `powershell`). POSIX shells (zsh/bash/sh/dash/ksh) share one generated script. `init` with no shell argument auto-detects from the parent process, `$SHELL`, or `$PSModulePath`. The generated wrapper sets `GTH_SHELL_INTEGRATION=1`, whose presence suppresses the first-run onboarding screen.
