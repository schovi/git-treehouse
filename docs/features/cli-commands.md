# CLI commands (list, doctor)

_Behavior spec. Index: [docs/README.md](../README.md) · Code: [docs/architecture.md](../architecture.md)._

## `git-treehouse list` (non-interactive)

- Prints the same columns as the TUI, aligned, one row per worktree.
- TTY: colored, with hyperlinks. Piped: plain text, no ANSI.
- Text output only loads async data for columns that fit at the current width. The list renderer computes whether the PR and git-aware size columns survive its normal column-fitting rules, rather than using fixed width thresholds. Width is taken from the terminal, then `$COLUMNS`, defaulting to 100. Otherwise async data is included only if it resolves within one short shared budget; unresolved cells print `-`. `--no-github` skips `gh` entirely.
- TTY output is colored with OSC 8 hyperlinks; piped output is plain text with no ANSI.
- `--json` ignores column fitting and forces full enrichment (PR + git size + full disk size). It prints structured JSON with repository metadata plus worktree fields for lifecycle state, status counts, `remote_sync` and `main_sync` state, `branch_merged_to_main`, commit info, `pull_request` info when loaded, `git_size`, `full_size`, and the compatibility `size` alias for full size.
- `--repo <path>` loads the repository containing that repo or worktree path instead of the current directory.

## `git-treehouse doctor`

Prints a stdout report for local setup diagnostics:

- `git` availability and version.
- Current repository detection and main branch.
- `gh` availability/authentication for PR data.
- Config load/path.
- Repo `.worktree` file, recognized keys, and hook approval state.
- Shell integration presence for the detected shell.
- Editor command.
- Clipboard command.
