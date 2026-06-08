# Changelog

Release notes for Git Treehouse.

Dates use the GitHub release publication date for published releases. Entries without a GitHub release use the annotated tag date.

## v0.6.0 - 2026-06-08

### New Features

#### Command Help

`git-treehouse help`, `git-treehouse help list`, `git-treehouse help init`, and `git-treehouse help doctor` now print command-specific help. Root and subcommands also accept `-h` and `--help`.

#### Explicit Repository Selection

`git-treehouse`, `git-treehouse list`, and `git-treehouse doctor` can now load a repository or worktree path with `--repo <path>`, including `~` expansion.

#### Shell Integration Install Targets

The first-run installer now installs Fish, Nushell, and PowerShell integration into dedicated autoload or module files: `~/.config/fish/functions/gth.fish`, `~/.config/nushell/autoload/gth.nu`, and the `GitTreehouse` PowerShell module. Existing legacy profile installs are still detected.

### Bug Fixes

- Tightened modal overlay spacing so background content above and below centered popups is preserved.

### Documentation

- Added the project changelog.

## v0.5.0 - 2026-06-07

### New Features

#### Faster TUI Loading and Enrichment

Git Treehouse now opens the app frame before slower metadata finishes loading. Local metadata, pull request status, and worktree size data fill in asynchronously so large repos feel less blocked.

#### Refresh Feedback

Pressing `r` now runs `git fetch --prune`, keeps the current selection and visible rows stable, and shows refresh progress in the Worktrees title. Automatic refreshes stay quieter.

#### JSON Size Fields

`git-treehouse list --json` now includes `git_size` and `full_size`. The existing `size` field remains as a compatibility alias.

### Improvements

- Reworked the help overlay with grouped shortcuts and marker, status, and PR legends.
- Cleaned up Details and Worktrees panel titles, border hints, and selected-row actions.
- Added an approved pull request indicator and reduced PR-column flicker while GitHub data loads.
- Changed `Esc` in list view to cancel or clear context. Use `q` or `Ctrl+C` to exit.

## v0.4.1 - 2026-06-07

- Restored full-row selection highlight.

## v0.4.0 - 2026-06-07

- Added structured `list --json` output and `git-treehouse doctor` checks.
- Added the command palette, PR session cache, and visible-row-first disk usage.
- Replaced delete confirmation with a single structured modal and shared toggle block renderer.
- Fixed modal overlay spacing, footer borders, and help modal centering.
- Covered new CLI and TUI behavior with focused tests and spec updates.

## v0.3.1 - 2026-06-06

- Refined worktree footer controls.

## v0.3.0 - 2026-06-06

- Polished worktree list rendering.
- Fixed the icon.

## v0.2.4 - 2026-06-05

- Clarified `gth` shell integration guidance.

## v0.2.3 - 2026-06-05

- Improved the README.
- Cleared quarantine for Homebrew cask binaries.

## v0.2.2 - 2026-06-05

- Added worktree filters and branch search.
- Added AI instructions.
- Merged `codex/tab-filter-search`.

## v0.2.1 - 2026-06-05

- Simplified the generated Homebrew cask.
- Stabilized CI validation.

## v0.2.0 - 2026-06-05

- Added configurable worktree path previews.
- Added shell integration onboarding.
- Added smart TUI auto-refresh.
- Polished table UI and shell integration hints.
- Rebranded project as Git Treehouse.
- Polished new worktree popup input.
- Included two snapshot commits with no further release-note detail.

## v0.1.0 - 2026-06-04

- Added the initial Git worktree TUI application.
- Added CLI commands for the TUI, list output, and shell wrapper generation.
- Implemented git worktree loading, status parsing, create/delete flows, and optional GitHub enrichment.
- Added responsive table rendering, selected-worktree inspector, configuration, docs, and focused tests.
