<p align="center">
  <img src="docs/assets/git-treehouse-icon-small.png" alt="Git Treehouse" width="120">
</p>

<h1 align="center">Git Treehouse</h1>

<p align="center">
  <strong>Git Treehouse - Tame AI agent worktrees</strong>
</p>

<p align="center">
  When agents create branches and worktrees faster than you can track them, Git Treehouse gives you one keyboard-driven view to inspect, jump, clean up, and create the next worktree safely.
  Use `git-treehouse` as the native CLI, or `gth` after shell integration for directory-changing workflows.
</p>

<p align="center">
  <a href="https://github.com/schovi/git-treehouse/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/schovi/git-treehouse/ci.yml?branch=main&style=flat-square" alt="Build"></a>
  <a href="https://github.com/schovi/git-treehouse/releases"><img src="https://img.shields.io/github/v/release/schovi/git-treehouse?style=flat-square" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License"></a>
</p>

<p align="center">
  <img src="docs/assets/screen-app-1.png" alt="Git Treehouse app screenshot">
</p>

<p align="center">
  More screenshots:
  <a href="docs/assets/screen-onboarding.png">onboarding</a> |
  <a href="docs/assets/screen-app-2.png">app example #2</a> |
  <a href="docs/assets/screen-new-worktree.png">create new worktree</a>
</p>

## What It Does

| | Feature |
| --- | --- |
| ✅ | List every worktree for the repository containing your current directory. |
| ✅ | Jump your shell into the selected worktree with the `gth` shell wrapper. |
| ✅ | Create a new branch and worktree from the focused row, with the target path previewed before creation. |
| ✅ | Delete worktrees with guardrails for active, main, dirty, and unmerged branches. |
| ✅ | Search branches with `s` and filter worktree states with `Tab`. |
| ✅ | See dirty counts, upstream sync, main comparison, commit age, disk size, PR, and CI state in one table. |
| ✅ | Auto-refresh local Git state while idle, with `git fetch --prune` only when you ask for it. |
| ✅ | Open a worktree in your editor, copy paths, or use `git-treehouse list` for non-interactive output. |

## Install

Install with Homebrew:

```sh
brew install --cask schovi/tap/git-treehouse
```

Homebrew installs the native `git-treehouse` binary and the short `gth` command. With the binary on `PATH`, Git can also run it as `git treehouse`.

Install from GitHub with Go:

```sh
go install github.com/schovi/git-treehouse/cmd/git-treehouse@latest
```

Or from a local checkout:

```sh
go install ./cmd/git-treehouse
```

Go installs the `git-treehouse` binary.

Optional dependency:

- `gh`, authenticated with GitHub, enables PR and CI status.

## First Run

Run `git-treehouse` inside a Git repository or any of its worktrees:

```sh
git-treehouse
```

On first launch, `git-treehouse` may show a shell integration setup screen. Install it if you want `Enter` to change your shell directory to the selected worktree.

After shell integration is installed, use `gth` for directory-changing runs. Keep using `git-treehouse` for native commands like `git-treehouse list` and `git-treehouse init`.

Without shell integration, `git-treehouse` can still print the selected path:

```sh
cd "$(git-treehouse)"
```

## Shell Integration

A child process cannot change the parent shell directory directly. Git Treehouse solves this with a small `gth` shell wrapper: the TUI writes the selected path to a temporary file, then the wrapper reads it and runs `cd`.

`git-treehouse init` installs shell functions for `gth`. It does not replace the native `git-treehouse` binary. Use `gth` when you want the selected worktree to become your current directory.

The first-run setup screen can install this for you. You can also install it manually.

Load integration for the current shell session:

```sh
eval "$(git-treehouse init)"
```

`git-treehouse init` auto-detects your shell when it can. Pass a shell name explicitly if detection is wrong or unavailable.

Fish:

```fish
git-treehouse init fish | source
```

Persistent setup examples:

```sh
git-treehouse init zsh >> ~/.zshrc
git-treehouse init bash >> ~/.bashrc
git-treehouse init sh >> ~/.profile
git-treehouse init ksh >> ~/.kshrc
```

Fish:

```fish
mkdir -p ~/.config/fish/functions
git-treehouse init fish > ~/.config/fish/functions/gth.fish
```

Nushell:

```nu
mkdir ~/.config/nushell/autoload
git-treehouse init nushell | save --force ~/.config/nushell/autoload/gth.nu
```

PowerShell:

```powershell
$module = New-Item -ItemType Directory -Force ~/.local/share/powershell/Modules/GitTreehouse
git-treehouse init powershell > (Join-Path $module.FullName GitTreehouse.psm1)
New-ModuleManifest -Path (Join-Path $module.FullName GitTreehouse.psd1) -RootModule GitTreehouse.psm1 -ModuleVersion 1.0.0 -FunctionsToExport gth -CmdletsToExport @() -AliasesToExport @() -VariablesToExport @() -Force
```

The first-run installer does this for you. Create parent config directories first if your shell has not created them yet.

Supported shells:

| Shell | Init command |
| --- | --- |
| zsh | `git-treehouse init zsh` |
| bash | `git-treehouse init bash` |
| fish | `git-treehouse init fish` |
| sh / dash | `git-treehouse init sh` |
| ksh | `git-treehouse init ksh` |
| Nushell | `git-treehouse init nushell` |
| PowerShell | `git-treehouse init powershell` |

Restart the shell, or source the config file after installation.

## Common Workflows

### Jump To A Worktree

1. Run `gth` after shell integration is installed.
2. Move with `up` / `down`, or `k` / `j`.
3. Press `Enter`.

Your shell moves into the selected worktree. Without shell integration, run `git-treehouse` as the native TUI and use the printed path with `cd`.

### Create A Worktree

1. Focus the row you want to branch from.
2. Press `n`.
3. Type a new branch name.
4. Use `Tab`, `shift+tab`, `up`, or `down` to choose the base.
5. Press `Enter`.

The popup shows the resolved path before creation.

Default path:

```text
{repo_parent}/.worktrees/{repo_name}/{branch}
```

Example:

```text
Repo:   /Users/me/work/api
Branch: feature/login
Path:   /Users/me/work/.worktrees/api/feature-login
```

To change where new worktrees go, press `ctrl+o` in the create popup. This opens `~/.config/git-treehouse/config.toml`. Save the file and `git-treehouse` reloads it while the popup is still open.

### Delete A Worktree

1. Focus a worktree row.
2. Press `d`, `Delete`, or `Backspace`.
3. Review the delete dialog.
4. Press `Enter`.

Active and main worktrees cannot be deleted from the TUI. Dirty or unmerged deletion requires explicitly arming force.

### Refresh State

`git-treehouse` refreshes local Git state automatically while idle.

Use `r` or `f` to run:

```sh
git fetch --prune
```

Then `git-treehouse` reloads the table.

## Keybindings

| Key | Action |
| --- | --- |
| `up` / `down`, `k` / `j` | Move selection |
| `g` / `G` | Jump to top / bottom |
| `m` | Jump to main worktree |
| `a` | Jump to active worktree |
| `Tab` | Cycle filter: all, modified, prunable, locked, detached |
| `Enter` | Go to selected worktree and exit |
| `n` | Create a new worktree |
| `d`, `Delete`, `Backspace` | Delete focused worktree |
| `o` | Open selected worktree in editor |
| `p` | Open selected PR or branch page with `gh` |
| `y` | Copy selected worktree absolute path |
| `r`, `f` | Run `git fetch --prune` and reload |
| `s` | Search branches |
| `Esc` | Close dialog, clear filter/search, or quit |
| `?` | Toggle help |
| `q`, `Ctrl+C` | Quit |

Create popup:

| Key | Action |
| --- | --- |
| text | Type branch name |
| `left` / `right` | Move branch-name cursor |
| `Tab`, `down` | Next base |
| `shift+tab`, `up` | Previous base |
| `ctrl+o` | Open `git-treehouse` config |
| `Enter` | Create worktree |
| `Esc` | Cancel |

## Configuration

Configuration is optional. Defaults work without a config file.

Path:

```text
~/.config/git-treehouse/config.toml
```

Example:

```toml
editor = "cursor"
path_template = "~/.worktrees/{repo_name}/{branch}"
main_branch = ""
skip_shell_integration_welcome = false
```

Options:

| Option | Default | Description |
| --- | --- | --- |
| `editor` | `$EDITOR`, then `code` | Command used by `o` and config editing |
| `path_template` | `{repo_parent}/.worktrees/{repo_name}/{branch}` | New worktree path template |
| `main_branch` | auto-detected | Override main branch detection |
| `skip_shell_integration_welcome` | `false` | Do not show first-run shell setup |

Path template placeholders:

| Placeholder | Meaning |
| --- | --- |
| `{repo}` | Main worktree path |
| `{repo_name}` | Main worktree directory name |
| `{repo_parent}` | Parent directory of the main worktree |
| `{branch}` | Branch name sanitized for paths |

Notes:

- `~` at the start of `path_template` expands to your home directory.
- Relative templates are resolved from the main worktree path.
- Branch sanitization replaces path separators and whitespace with `-`.
- If your config still contains the old exact default `{repo_parent}/{branch}`, `git-treehouse` treats it as the current default.

## Commands

Interactive TUI:

```sh
git-treehouse
gth
```

Print a plain table:

```sh
git-treehouse list
```

Skip GitHub lookup:

```sh
git-treehouse list --no-github
```

Print shell integration:

```sh
git-treehouse init
git-treehouse init fish
git-treehouse init zsh
```

## GitHub Integration

If `gh` is installed and authenticated, `git-treehouse` loads PR data in the background:

- PR number
- PR state
- CI status
- terminal hyperlinks where supported

If `gh` is missing or unauthenticated, PR columns stay hidden or pending without noisy errors.

## Troubleshooting

### Pressing Enter Only Prints A Path

Shell integration is not active in that terminal.

Run this for the current session:

```sh
eval "$(git-treehouse init)"
```

For fish:

```fish
git-treehouse init fish | source
```

Then install the integration persistently using the commands in [Shell Integration](#shell-integration).

### The First-Run Setup Screen Does Not Show

Check:

```toml
skip_shell_integration_welcome = false
```

in:

```text
~/.config/git-treehouse/config.toml
```

### New Worktree Path Looks Wrong

Open the create popup with `n`, then press `ctrl+o` to edit config. Change `path_template`, save the file, and the popup path preview reloads automatically.

Common templates:

```toml
path_template = "{repo_parent}/.worktrees/{repo_name}/{branch}"
path_template = "~/.worktrees/{repo_name}/{branch}"
path_template = "{repo}/worktrees/{branch}"
```

### PR Data Does Not Show

Check that GitHub CLI is installed and authenticated:

```sh
gh auth status
```

You can always skip PR lookup:

```sh
git-treehouse list --no-github
```

## Behavior Notes

- `git-treehouse` does not run `git fetch` on startup.
- Local Git state auto-refreshes while idle.
- `r` and `f` run `git fetch --prune`.
- Bare worktree entries are not navigable rows.
- The main worktree is pinned first.
- Remaining worktrees are sorted by last commit time, newest first.
- Active and main worktrees cannot be deleted from the TUI.
- Dirty delete and unmerged branch deletion require explicit force arming.

## Development

Run tests:

```sh
go test ./...
```

Build:

```sh
go build ./cmd/git-treehouse
```

Smoke checks:

```sh
git-treehouse list --no-github
COLUMNS=80 git-treehouse list --no-github
git-treehouse init zsh
git-treehouse init fish
```

## Status

`git-treehouse` is early software. Version `v0.1.0` is the first usable release and focuses on local, single-repository worktree management.
