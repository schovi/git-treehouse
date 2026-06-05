# Git Treehouse

Git Treehouse is a terminal UI for Git worktrees.

It is built for repositories where you keep several task branches checked out at the same time. It gives you one keyboard-driven table for jumping between worktrees, creating new ones, deleting stale ones, and seeing enough Git state to know what is safe to touch.

## What It Does

- Lists all worktrees for the repository containing your current directory.
- Jumps your shell into the selected worktree with shell integration.
- Creates a new branch and worktree from the focused row.
- Shows the exact path where a new worktree will be created.
- Deletes worktrees with guardrails for active, main, dirty, and unmerged branches.
- Shows compact status: dirty counts, upstream sync, main comparison, commit age, disk size, PR, and CI state.
- Auto-refreshes local Git state while idle.
- Runs `git fetch --prune` only when you ask for it.
- Opens a worktree in your editor.
- Copies paths.
- Supports `git-treehouse list` for non-interactive output.

## Install

Install with Homebrew:

```sh
brew install --cask schovi/tap/git-treehouse
```

Homebrew installs `git-treehouse` and the short `gth` command. With the binary on `PATH`, Git can also run it as `git treehouse`.

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

After shell integration is installed, use `gth` for the directory-changing wrapper.

Without shell integration, `git-treehouse` can still print the selected path:

```sh
cd "$(git-treehouse)"
```

## Shell Integration

A child process cannot change the parent shell directory directly. Git Treehouse solves this with a small `gth` shell wrapper: the TUI writes the selected path to a temporary file, then the wrapper reads it and runs `cd`.

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
git-treehouse init fish >> ~/.config/fish/config.fish
git-treehouse init nushell | save --append ~/.config/nushell/config.nu
git-treehouse init powershell >> ~/.config/powershell/Microsoft.PowerShell_profile.ps1
```

These manual examples append to the shell config file directly. Create the parent config directory first if your shell has not created it yet.

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

1. Open `gth` if shell integration is installed, or `git-treehouse` otherwise.
2. Move with `up` / `down`, or `k` / `j`.
3. Press `Enter`.

With shell integration installed, your shell moves into that worktree.

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
| `Tab` | Cycle notable rows |
| `Enter` | Go to selected worktree and exit |
| `n` | Create a new worktree |
| `d`, `Delete`, `Backspace` | Delete focused worktree |
| `o` | Open selected worktree in editor |
| `p` | Open selected PR or branch page with `gh` |
| `y` | Copy selected worktree absolute path |
| `r`, `f` | Fetch, prune, and reload |
| `/` | Filter branches |
| `Esc` | Close dialog or clear filter |
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
