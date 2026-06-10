# Invocation

_Behavior spec. Index: [docs/README.md](../README.md) · Code: [docs/architecture.md](../architecture.md)._

`git-treehouse` can be started from the main repo or any of its worktrees. It resolves the repo via `git rev-parse`; the full worktree set comes from `git worktree list --porcelain`. `--repo <path>` can explicitly select a repo or worktree path instead of the current directory.

- Run outside a git repo → print a one-line error to stderr, exit 1.
- Bare repos: the bare entry is not a navigable row; only real worktrees are listed.

## Subcommands

| Command | Behavior |
|---|---|
| `git-treehouse [--repo <path>]` | Launch the native TUI. It cannot change the parent shell directory by itself. |
| `git-treehouse list [--repo <path>]` | Print the table to stdout, no TUI, no ANSI when piped. For scripting. `--json` prints structured repository/worktree data. |
| `git-treehouse init <shell>` | Print shell integration functions that define `gth` (see [shell-integration.md](./shell-integration.md)) |
| `git-treehouse doctor [--repo <path>]` | Print environment diagnostics for required and optional integrations. |
| `git-treehouse allow [--repo <path>]` | Approve executable hooks from the repo `.worktree` file. |
| `git-treehouse help [list|init|doctor|allow]` | Print command help. Root and subcommands also accept `-h` and `--help`. |
