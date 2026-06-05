# gwt

`gwt` is a fast terminal UI for working with Git worktrees from one keyboard-driven table.

It is built for the common flow of keeping several task branches checked out at once: jump between worktrees, create a new one from the current context, remove stale worktrees, and keep enough status visible to know what is safe to touch.

## Features

- Browse all real worktrees for the repository containing the current directory.
- Jump to a selected worktree through shell integration.
- Create a new worktree and branch from the focused row.
- Delete worktrees with guardrails for active, main, dirty, and unmerged branches.
- Search branches with `s` and filter states with `Tab`.
- Refresh with `git fetch --prune`.
- Open a worktree in your editor.
- Copy the selected worktree path.
- Show compact Git state: dirty counts, upstream ahead/behind, main comparison, commit age, and disk size.
- Optionally enrich rows with GitHub PR and CI status through the `gh` CLI.
- Use `gwt list` for script-friendly, non-interactive output.

## Install

From this repository:

```sh
go install ./cmd/gwt
```

Or build a local binary:

```sh
go build ./cmd/gwt
```

The binary name is `gwt`.

## Quick Start

Run the TUI from a Git repository or any of its worktrees:

```sh
gwt
```

Print a plain table without launching the TUI:

```sh
gwt list
```

Skip GitHub lookup in list mode:

```sh
gwt list --no-github
```

## Shell Integration

A child process cannot change the parent shell's current directory directly. `gwt` uses the same pattern as tools like `zoxide` and `yazi`: the TUI writes the selected path to a temporary file, and a shell wrapper reads it after `gwt` exits.

On first raw launch, `gwt` shows a setup screen when shell integration is not active. You can install the wrapper from there, skip the screen, or continue without installing.

Supported shells:

| Shell | Init command |
| --- | --- |
| zsh | `gwt init zsh` |
| bash | `gwt init bash` |
| fish | `gwt init fish` |
| sh / dash | `gwt init sh` |
| ksh | `gwt init ksh` |
| Nushell | `gwt init nushell` |
| PowerShell | `gwt init powershell` |

Load the wrapper in your current shell:

```sh
eval "$(gwt init)"
```

`gwt init` detects the current parent shell. You can pass a shell name explicitly when needed, for example `gwt init zsh` or `gwt init nushell`.

Fish uses `source` instead:

```fish
gwt init fish | source
```

Persist the wrapper for future shells:

```sh
gwt init zsh >> ~/.zshrc
gwt init bash >> ~/.bashrc
gwt init sh >> ~/.profile
gwt init ksh >> ~/.kshrc
gwt init fish >> ~/.config/fish/config.fish
gwt init nushell | save --append ~/.config/nushell/config.nu
gwt init powershell >> ~/.config/powershell/Microsoft.PowerShell_profile.ps1
```

Restart your shell or source the file. After that, pressing `Enter` on a worktree changes your shell directory to that worktree.

If selecting a worktree only prints a path, the shell wrapper is not active in that terminal. Run `eval "$(gwt init)"` for the current shell session, or add the persistent line above to your shell config.

Without the wrapper, command substitution still works:

```sh
cd "$(gwt)"
```

## Keybindings

| Key | Action |
| --- | --- |
| `↑` / `↓`, `k` / `j` | Move selection |
| `Enter` | Go to selected worktree and exit |
| `n` | Create a new worktree from the focused row |
| `d`, `Delete`, `Backspace` | Delete the focused worktree |
| `o` | Open selected worktree in editor |
| `p` | Open the selected PR or branch page with `gh` |
| `y` | Copy selected worktree path |
| `r`, `f` | Run `git fetch --prune` and reload |
| `s` | Search branches |
| `Tab` | Cycle filter: all, modified, prunable, locked, detached |
| `Esc` | Close dialog, clear filter/search, or quit |
| `?` | Toggle help |
| `q`, `Ctrl+C` | Quit |

## Configuration

Configuration is optional. Defaults work without a config file.

Create `~/.config/gwt/config.toml`:

```toml
editor = "cursor"
path_template = "{repo_parent}/{branch}"
main_branch = ""
skip_shell_integration_welcome = false
```

Options:

| Option | Default | Description |
| --- | --- | --- |
| `editor` | `$EDITOR`, then `code` | Command used by `o` |
| `path_template` | `{repo_parent}/{branch}` | New worktree path template |
| `main_branch` | auto-detected | Override main branch detection |
| `skip_shell_integration_welcome` | `false` | Do not show the first-run shell setup screen |

Path templates support:

| Placeholder | Meaning |
| --- | --- |
| `{repo}` | Main worktree path |
| `{repo_parent}` | Parent directory of the main worktree |
| `{branch}` | Branch name sanitized for paths |

Branch sanitization replaces path separators and whitespace with `-`.

## GitHub Integration

If `gh` is installed and authenticated, `gwt` loads PR data in the background:

- PR number
- PR state
- CI status
- clickable terminal hyperlink where supported

If `gh` is missing or unauthenticated, the PR column is hidden without warning noise.

## Behavior Notes

- `gwt` never runs `git fetch` on startup.
- `r` and `f` run `git fetch --prune`, then reload.
- Bare worktree entries are not navigable rows.
- The main worktree is pinned first.
- Remaining worktrees are sorted by last commit time, newest first.
- Dirty delete and unmerged branch deletion require explicit force arming.
- Active and main worktrees cannot be deleted from the TUI.

## Development

Run tests:

```sh
go test ./...
```

Build:

```sh
go build ./cmd/gwt
```

Useful smoke checks:

```sh
gwt list --no-github
COLUMNS=80 gwt list --no-github
gwt init zsh
```

## Status

`gwt` is early software. Version `v0.1.0` is the first usable release and focuses on local, single-repository worktree management.
