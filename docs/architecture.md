# Architecture

How Git Treehouse is built. For user-facing behavior, see the [behavior spec](./README.md).

Git Treehouse is a Bubble Tea TUI for managing Git worktrees. The module is `github.com/schovi/git-treehouse`, the binary is `git-treehouse`, and the shell wrapper function is `gth`. It operates on a single repository per invocation: the repo containing the current directory, or `--repo <path>`.

## The cd-on-exit mechanism (central design decision)

A child process cannot change the parent shell's cwd. The whole I/O layout follows from this:

- The TUI renders to **stderr** (`tea.WithOutput(os.Stderr)`), keeping stdout clean so `cd "$(git-treehouse)"` works.
- With `--cd-file <path>`, the selected worktree path is written to that file. The generated `gth` shell function (see `internal/shellinit`) passes a `mktemp` file, then `cd`s into its contents after exit.
- The wrapper sets `GTH_SHELL_INTEGRATION=1`; its presence suppresses the first-run onboarding screen.

Preserve this stdout/stderr split in any new output paths.

## Package map and data flow

```
cmd/git-treehouse/main.go     CLI dispatch: `init` → shellinit, `list`/`doctor`/`allow` rendered here,
                              default → TUI. Parses global flags (--repo, --cd-file) and shell detection.
internal/tui/model.go         Bubble Tea model; composes everything below. Owns all modes,
                              keybindings, dialogs, and the command palette.
internal/gitdata              Loads + parses git state (worktree list --porcelain, status, sync),
                              plus repo-scoped copy/hook support.
internal/github               PR/CI status by shelling out to `gh` CLI (not the API).
internal/listview             Pure table renderer, shared by the TUI and the `list` subcommand.
internal/pathutil             Branch sanitizing + worktree path templating.
internal/config               Global TOML at ~/.config/git-treehouse/config.toml (live-reloaded by
                              mtime) plus the repo-scoped `.worktree` config.
internal/onboarding           Separate Bubble Tea program for first-run shell setup.
internal/shellinit            Generates the gth wrapper for zsh/bash/fish/sh/dash/ksh/nushell/powershell.
```

Notable files inside `internal/`:

| File | Responsibility |
|---|---|
| `gitdata/runner.go` | The single subprocess seam (see Key patterns). |
| `gitdata/load.go` | Orchestrates the git calls that build the worktree/branch model. Status enrichment also runs `git diff --numstat HEAD` for `Worktree.ChangedFiles`; `enrichContextGraphs` runs per non-main worktree row, for `Worktree.Graph`, three `git log` calls (commits ahead of main, behind main, and on the upstream via `HEAD..@{u}`) plus `git merge-base` and a deeper `git log` from it for the fork point and the shared ancestors that pad the Git context frame to the paired left column's height; `LoadBranchContextGraph` does the same for `Branch.Graph` on a branch-only row, but runs from the repo root against the branch ref (`refs/heads/<name>` and `<name>@{u}`) since there is no checkout to log from. It is loaded lazily for the selected branch (the TUI's `selectedBranchGraphCommand`, like the PR-review and full-disk loads) rather than eagerly for every branch, because a repo can have many local branches and only the selected one is shown. Both share `loadContextGraph`. `BucketedDiskUsage` walks a worktree once, grouping bytes into `Worktree.DiskBreakdown` (loaded lazily for the selected row alongside its full size). |
| `gitdata/parse.go` | Pure string-in/struct-out parsers (no exec). `ParseStatusPorcelain` captures per-file `ChangedFile` entries; `ParseNumstat` parses line counts; `ParseGraphCommits` parses commit short/subject lines. |
| `tui/frames.go` | Auxiliary context frames below the Details panel (PR review, Changes, Git context, Disk). Pure renderers returning a bordered section box or `""`. `model.detailBlocks` composes the below-Worktrees region as two columns: the left column stacks Details + Changes + PR review (via `lipgloss.JoinVertical`), the right is the Git context frame grown to match the left column's height; the two are placed side by side with `lipgloss.JoinHorizontal` on wide panels and stacked otherwise. The Git context renderer takes a `graphSource` (built from either a worktree row or a branch-only row) so the same frame serves both row kinds. `belowDetailFrames` holds only the (currently disabled) Disk frame. The resulting block list is sized into the table height budget by `availableTableHeightForBlocks`. |
| `github/review.go` | `LoadPullRequestReview` (`gh pr view --json` plus a `gh api graphql` call for inline `reviewThreads`) + pure `ParsePullRequestReview`/`ParseChecks`/`ParseReviewThreads` for the PR review frame. The TUI loads it lazily for the selected row and caches it by PR number (storing failed attempts too, so the frame's loading state resolves). |
| `gitdata/copy.go` | Copies repo-scoped `copy_untracked` files into a new worktree. |
| `gitdata/approval.go`, `gitdata/hooks.go` | Hook hashing + approval gating for `post_create` / `before_delete`. |
| `config/config.go` | Global `config.toml`. |
| `config/repo.go` | Repo-scoped `.worktree` config. |

## Key patterns

- **`gitdata.Runner` is the single subprocess seam**: an interface (`runner.go`) wrapping `exec.CommandContext`. All git and `gh` calls go through it; tests inject fakes, so nothing in the data layer needs a real repo. New subprocess calls must use it.
- **Parsing is pure**: `gitdata/parse.go` is string-in/struct-out with no exec. Keep new parsers there and test them directly.
- **Async via typed messages**: TUI side effects are `tea.Cmd` closures returning typed msgs (`prLoadedMsg`, `reloadMsg`, `autoRefreshMsg`, ...). A monotonic `refreshID` discards stale reloads, and `selectionAnchor` re-anchors the cursor across reloads. Follow this for any new async work.
- **Enrichment is layered**: initial git load is synchronous; PR data and disk usage load afterwards as background commands so the table appears fast. `gh` being missing/unauthenticated must stay silent (columns hidden, no errors).
- **Modes are model fields**: TUI modes (filter, help, create dialog, delete dialog, checkout dialog, palette, search) are fields on the model; `Update` routes keys by active mode.

## Rendering and styling

- **One renderer for similar UI states.** `internal/listview` is the single table renderer shared by the TUI and the `list` subcommand. Sibling UI blocks (such as the delete dialog's Worktree and Branch sections) must render through one shared helper, not separate hand-assembled append paths.
- **Lip Gloss color profile.** Tests that assert ANSI output force the color profile, then restore the previous profile.

## Testing approach

- Inject fake `Runner`s to drive `gitdata` and TUI logic without a real repo.
- Test `parse.go` directly as pure functions.
- Treat the behavior spec's visual rules as test contracts: assert ANSI/SGR state or exact border geometry, not just width and string presence. Cover marker and no-marker rows, and narrow plus wide widths when layout changes.

## Build and release

- `make build` / `make test` / `make lint` / `make security` (gosec + govulncheck). CI runs `make security`.
- Releases: GoReleaser on `v*` tags builds the `git-treehouse` + `gth` binaries and publishes a Homebrew cask to `schovi/homebrew-tap`.
