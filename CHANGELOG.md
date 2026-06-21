# Changelog

Release notes for Git Treehouse.

Dates use the GitHub release publication date for published releases. Entries without a GitHub release use the annotated tag date.

## v0.11.0 - 2026-06-21

### New Features

#### Clean Up Merged Command

Added a palette-only batch command, `Clean up merged`. It scans all done rows regardless of the current filter or search: worktrees or branch-only rows merged into main, plus worktrees whose PR is merged or closed. It opens a confirmation dialog with counts for worktrees to remove and branches to delete, representative affected rows, and the exact commands that will run.

The batch stays conservative: worktree removal uses only `git worktree remove` (never `--force`), branch deletion uses only `git branch -d` (never `-D`). Only clean, unlocked, non-root, non-active worktrees are removed; dirty, locked, active, root, detached, prunable, and not-yet-loaded rows are ignored. Worktree branches are deleted only when merged into main, while PR merged/closed worktrees not merged into main are removed but keep their local branch. Approved `before_delete` hooks run once per removed worktree; a hook failure counts as that row's failure and skips its removal while the rest of the batch continues. Full success shows a summary message; partial failure keeps a result dialog open with counts and reasons. When the batch deletes branch refs with known SHAs, the success message offers `u` to restore all deleted refs.

### Bug Fixes

- Fixed the PR column staying silently empty on large repos. The single `gh pr list` call requesting `statusCheckRollup` forced GitHub's GraphQL API to resolve CI for all 200 PRs, returning HTTP 504. The fetch is now split: a fast branch→PR mapping without `statusCheckRollup`, and CI status fetched lazily per open PR.
- Fixed the PR fetch being cancelled by a hard 2s timeout on large repos (listing 200 PRs takes ~4-5s). The timeout is now a named 15s constant.
- Fixed delete dialog errors overflowing the frame. Git delete failures now wrap across lines instead of hard-truncating, advisory git hint lines are dropped, the error block is capped, and the dialog body is clipped to the terminal height.

### Improvements

- Cut initial PR latency on normal repos. When the local branch count is small (≤40), each branch is queried with `gh pr list --head` in a parallel worker pool, so number, state, and CI all arrive together in ~1s instead of ~5s+. Above the threshold, the single list call with lazy CI is used to avoid a large request fan-out.

## v0.10.0 - 2026-06-10

### New Features

#### Checkout PR Into A Worktree

Added a palette-only `Checkout PR` command. Open the command palette (`Ctrl+P`) and run `Checkout PR` to open a centered picker that loads up to 200 pull requests via `gh pr list --state all`, sorted by most recently updated (open, draft, merged, and closed). Type to filter by PR number, title, URL, owner, head branch, or local branch name; `↑`/`↓` (or `k`/`j`) move the selection. `Enter` checks out the highlighted PR into a worktree: it cd's into an existing non-prunable worktree, runs `git worktree add` for an existing local branch, or fetches `pull/<number>/head` and creates a new worktree branch for a new PR. Same-repo PRs use `headRefName`; fork PRs use `<owner>/<headRefName>`. On success the app cd's into the worktree and exits. `o` opens the highlighted PR in the browser; if the input is a PR URL or number not in the list, `Enter`/`o` act on it directly. `Esc` closes the picker, and there is no global keybinding for this flow.

## v0.9.0 - 2026-06-10

### New Features

#### Repo-Scoped `.worktree` Configuration

Added a committed repo-level `.worktree` TOML schema for path template overrides, named-file copy behavior, and approved lifecycle hooks. Declarative settings apply immediately, while `post_create` and `before_delete` hooks run only after `git-treehouse allow` records the current hook hash in repo-local Git config. `doctor` now reports `.worktree` keys and hook approval state.

#### Filter Picker Modal

`Tab` now opens a filter picker modal instead of cycling filters in place (the palette entry is renamed from "Cycle filter" to "Open filter picker"). The modal lists every filter with its matching row count; empty filters are disabled (`all` is always available). Inside the picker, `↑`/`↓` (or `k`/`j`) move the selection, `Tab` jumps to the next enabled filter and `Shift+Tab` to the previous, `Enter` applies, and `Esc` closes.

#### Merged Worktree Filter

Added a `merged` filter to the cycle (`all → modified → branches → merged → prunable → locked → detached`) and a `filter-merged` palette command. It surfaces rows that are safe to clean up: clean worktrees whose branch is merged to main or whose PR is merged/closed, plus merged branch-only rows. The root repository and detached worktrees never match.

#### Restore Deleted Branches

After deleting a branch ref (from a branch-only row, or a combined worktree + branch delete), a green success offer appears for about 10 seconds: `✓ deleted <name> (<short-sha>) · u to restore`. Pressing `u` recreates the ref with `git branch <name> <sha>`. Only the branch ref is restored, not worktree files or uncommitted changes. The offer is superseded by the next delete or refresh.

#### Copy PR URL Command

Added a palette-only command, "Copy PR URL" (`copy-pull-request-url`), that copies the selected row's pull request URL to the clipboard with a `copied PR URL: <url>` confirmation. Rows without a PR flash `no pull request URL for this row`.

### Improvements

- Responsive list columns now drop and shrink more gracefully on narrow terminals (in both the TUI and the `list` subcommand): size drops first, then commit truncates to the short SHA, then PR and age drop, while name, status, and remote always survive. The PR and size columns size dynamically to their content.

## v0.8.0 - 2026-06-09

### New Features

#### Scrollbar for the Worktree List

The worktree list now renders a scrollbar in a right-hand gutter when the rows overflow the visible height. The gutter shows `↑`/`↓` arrows when there is more content above or below, a `█` thumb that tracks the scroll position, and a `│` track elsewhere. The footer shows the current position as `start/total`. The scrollbar only appears when the list is taller than the viewport, so narrow or short lists are unaffected.

## v0.7.0 - 2026-06-09

### New Features

#### Local Branches in the List

The table can now show local branches that are not checked out by any worktree. Press `b` to toggle branch-only rows; the preference persists to config as `show_branches` (default `false`). The `Tab` filter cycle gains a `branches` state that surfaces branch-only rows even when the toggle is off.

```toml
show_branches = false   # default: hide branch-only rows until `b` is pressed
```

#### Create a Worktree from an Existing Branch

`Enter` on a branch-only row opens a New worktree confirmation that uses your `path_template`, runs `git worktree add <path> <branch>`, and `cd`s into the new worktree on success.

#### Checkout a Branch in the Root Worktree

`c` on a branch-only row checks the branch out in the root worktree, then `cd`s into root. A clean root runs `git switch -- <branch>` directly. A dirty root opens a confirmation that blocks checkout until you enable stashing with `s`, which runs `git stash push -u` before switching. No force checkout or discard happens.

#### Branch-Only Delete

`d` on a branch-only row deletes just the local branch ref and never touches worktree files. Merged branches use safe `git branch -d`; unmerged branches require an explicit force delete with `git branch -D`.

### Improvements

- Reworked row glyphs into a single name column with row-type icons (`⌂` root, `⊡` worktree, `⎇` local branch) plus lifecycle suffixes (`!` locked, `×` prunable). The standalone marker column is gone.
- The active worktree row is now bold, and row icons use softer accent colors.
- `y` on a branch-only row copies the branch name; worktree rows still copy the absolute path.
- The detail panel and footer adapt to branch-only rows with branch-specific metadata and actions.

### Bug Fixes

- Fixed the delete flow feedback: deletion now shows in-progress spinner feedback, keeps the dialog open and shows the git error inline on failure, and ignores stale delete results.

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
