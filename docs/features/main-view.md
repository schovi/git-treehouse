# Main view

_Behavior spec. Index: [docs/README.md](../README.md) · Code: [docs/architecture.md](../architecture.md)._

## Table

Borderless table. By default it shows one row per worktree unless `show_branches` is enabled. Pressing `b` also shows local branches that are not checked out by any worktree. Columns left to right:

| Column | Content |
|---|---|
| name | Row-type glyph plus display name, with optional lifecycle suffix, e.g. `⌂ main`, `⊡ feature/work`, `⊡ cd5e190 detached`, `⊡ stale/abandoned ×`, or `⎇ feature/local-only` |
| status | Working-tree state, compact (see [Status column](./columns-and-data.md)); `-` for branch-only rows |
| remote | Ahead/behind vs upstream, e.g. `↑2 ↓1`; `✓` when synced; `gone` when upstream was deleted; `-` when no upstream |
| main± | Ahead/behind vs the local main branch, e.g. `↑5 ↓12`; blank only for rows already on the main branch |
| commit | Short SHA + truncated subject line |
| age | Relative last-commit time (`3h`, `2d`, `5w`) |
| PR | PR number + state + CI (see [GitHub data (PR column)](./columns-and-data.md)), rendered as a clickable OSC 8 hyperlink |
| size | Git-aware worktree size, computed lazily from tracked and unignored untracked files (see [Data loading model](./columns-and-data.md)) |

Column sizing: name and commit are elastic. On narrow terminals, size drops first, then commit truncates down to the short SHA, then PR and age drop entirely. Name, status, and remote survive until the minimum compact layout.

The header includes the root repository branch, e.g. `root: codex/list-rendering-polish`, because the root repository can be checked out to a non-main branch.

## Row lifecycle and type icons

Row-type icons live inside the name column before the displayed name. Lifecycle icons for exceptional worktree states are suffixes in the name column. Selection and current-worktree state are rendered with text style, not marker glyphs.

Lifecycle suffixes:

| Glyph | Meaning |
|---|---|
| `!` | Locked worktree |
| `×` | Prunable worktree, directory missing on disk |

Type icons:

| Glyph | Meaning |
|---|---|
| `⌂` | Root repository, the primary checkout that owns the worktree set |
| `⊡` | Checked-out worktree |
| `⎇` | Local branch without a worktree |

The name column starts with exactly one row-type glyph and one space before the displayed name. The header label `name` aligns with the displayed name, leaving the row-type glyph area untitled. Locked and prunable worktrees append one lifecycle glyph after the displayed name. Detached worktrees still show `<sha> detached`, prefixed by the worktree type icon.

## Selection and current worktree

- The current worktree, where `git-treehouse` was started, uses bold row text.
- The selected row uses a full-row background only. Selection does not add bold text or a row marker.
- If the current row is selected, both styles apply: full-row background plus bold row text.

## Detail panel, local hints, and status bar

Below the table:

- **Worktrees footer:** list-local hints live in the bottom border of the Worktrees panel. The left side contains collection actions. In normal mode this shows `n new worktree` on the left and `h root · a active · Tab filter: <state> · s search · b branches` on the right. With branches visible it shows `b hide branches`. With an active filter, it also shows `Esc clear filter`. While searching, letter keys feed the live search input, so the footer shows `search <text>▌` on the left and `Esc clear · Tab filter: <state> · b branches` on the right.
- **Detail panel:** full info for the selected row. Worktree rows show branch name, explicit `HEAD`, root/current state, absolute path, full status counts, Git-aware and full size when loaded, upstream name and sync state, main branch comparison, full commit subject, lifecycle/delete notes. Branch-only rows show branch name, explicit `HEAD`, `Path: not checked out`, `Status: no worktree`, upstream/main comparison, commit, PR, and checkout action. Root/current context appears next to the Details title, for example `Details · Current root repository`; branch-only rows use `Details · Local branch`. Selected-row actions live in the bottom border: worktrees show `↵ go · o editor · d delete · y abs path · p PR`; branch-only rows show `↵ create+go · c checkout root · d delete · y name · p PR`.
- **Status bar:** transient progress and flash messages only. The app frame title starts with `Git treehouse · <repo>`. The top controls show refresh age, help, and quit. Table-scoped refresh feedback lives in the Worktrees title instead of the status bar.
- **Help overlay:** groups shortcuts by context (`Global`, `Worktree List`, `Worktree Detail`) and groups visual legends (`Worktree Markers`, `Git Status`, `Pull Requests`). Category headers are bold white. The row lifecycle and PR legends live here instead of the status bar.
- `g/G` remains available and documented in help, but is not shown in the main view.
