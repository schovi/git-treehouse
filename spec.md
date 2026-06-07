# Git Treehouse — git worktree TUI

A fast terminal UI for managing git worktrees: browse, switch, create, and delete them from one keyboard-driven table.

- **Binary name:** `git-treehouse`
- **Smart shell wrapper:** `gth`
- **Stack:** Go + Bubble Tea (with Lip Gloss for styling, Bubbles for inputs/spinners)
- **Scope:** single repo per invocation (the repo containing the cwd), single user, local tool

## 1. Invocation

`git-treehouse` can be started from the main repo or any of its worktrees. It resolves the repo via `git rev-parse`; the full worktree set comes from `git worktree list --porcelain`.

- Run outside a git repo → print a one-line error to stderr, exit 1.
- Bare repos: the bare entry is not a navigable row; only real worktrees are listed.

### Subcommands

| Command | Behavior |
|---|---|
| `git-treehouse` | Launch the native TUI. It cannot change the parent shell directory by itself. |
| `git-treehouse list` | Print the table to stdout, no TUI, no ANSI when piped. For scripting. `--json` prints structured repository/worktree data. |
| `git-treehouse init <shell>` | Print shell integration functions that define `gth` (see §2) |
| `git-treehouse doctor` | Print environment diagnostics for required and optional integrations. |

## 2. Shell integration (cd mechanism)

A child process cannot change its parent shell's cwd, so Git Treehouse uses the zoxide/yazi pattern:

- The TUI writes the selected path to a file given via `--cd-file <path>` (nothing else goes to that file).
- The `gth` wrapper function (installed via `git-treehouse init fish | source` etc.) runs the binary with a temp `--cd-file`, and `cd`s to its content after the TUI exits, if non-empty.
- `git-treehouse` remains the native CLI for non-navigating commands and direct invocation. `gth` is the directory-changing command.
- Quitting without selecting writes nothing; the shell stays where it was.
- **Graceful degradation:** without `--cd-file`, the selected path is printed to stdout on exit (TUI renders on stderr/tty), so `cd (git-treehouse)` works bare.

## 3. Main view

### 3.1 Table

Borderless table, one row per worktree. Columns left to right:

| Column | Content |
|---|---|
| marker | Row state glyph (see 3.2) |
| branch | Branch name, with lifecycle state suffixes for `locked`, `prunable`, or `detached` rows, e.g. `cd5e190 detached` |
| status | Working-tree state, compact (see 3.4) |
| remote | Ahead/behind vs upstream, e.g. `↑2 ↓1`; `✓` when synced; `gone` when upstream was deleted; `-` when no upstream |
| main± | Ahead/behind vs the local main branch, e.g. `↑5 ↓12`; blank only for rows already on the main branch |
| commit | Short SHA + truncated subject line |
| age | Relative last-commit time (`3h`, `2d`, `5w`) |
| PR | PR number + state + CI (see 3.6), rendered as a clickable OSC 8 hyperlink |
| size | Git-aware worktree size, computed lazily from tracked and unignored untracked files (see 3.7) |

Column sizing: branch and commit are elastic; commit truncates first, then size, PR, and age drop entirely on narrow terminals. Marker, branch, status, and remote survive until the minimum compact layout.

The header includes the root repository branch, e.g. `root: codex/list-rendering-polish`, because the root repository can be checked out to a non-main branch.

### 3.2 Row markers

Markers are reserved for important type or lifecycle state. Selection and current-worktree state are rendered with text style, not marker glyphs.

| Glyph | Meaning |
|---|---|
| `⌂` | Root repository, the primary checkout that owns the worktree set |
| `!` | Locked worktree |
| `×` | Prunable worktree, directory missing on disk |
| blank | Normal worktree |

The marker column is one character wide with one space before the branch name.

### 3.3 Selection and current worktree

- The current worktree, where `git-treehouse` was started, uses bold branch text.
- The selected row uses a full-row background only. Selection does not add bold text or a row marker.
- If the current row is selected, both styles apply: full-row background plus bold branch text.

### 3.4 Status column

The status column is only for working-tree file changes:

- `✓` means clean
- Counts for dirty trees: `+2 ~3 ?1` (staged / modified / untracked); only non-zero parts shown

Lifecycle and Git state words do not appear in this column. `locked`, `prunable`, and `detached` are rendered as branch suffixes. Upstream state is rendered in `remote`.

### 3.5 Remote and main sync

`remote` compares `HEAD` to the upstream ref (`@{u}`):

- `✓` means upstream exists and `HEAD` is synced with it
- `↑2` means local has two commits not on upstream
- `↓1` means upstream has one commit not in local
- `↑2 ↓1` means local and upstream diverged
- `gone` means the configured upstream branch was deleted on the remote
- `-` means no upstream, no remote, or detached `HEAD`

`main±` compares `HEAD` to the detected local main branch. This is independent from the root repository. If the root repository is currently on a feature branch, it still gets a `main±` value. The column is blank only when the row is already on the detected main branch.

### 3.6 GitHub data (PR column)

Loaded via the `gh` CLI (one `gh pr list`-style call for all branches), only if `gh` exists and is authed.

- Shows: `#123` + state glyph (open/ready `○`, draft `◌`, approved `◆`, merged `⬡`, closed `✕`) + CI status (`✓` passing, `✗` failing, `●` running).
- PR number is an OSC 8 hyperlink to the PR page (clickable in supporting terminals).
- No configured remote → column hidden entirely.
- `gh` missing/unauthed → column remains reserved but empty, no error noise.

### 3.7 Data loading model

Instant app frame, async enrichment:

1. **Synchronous (must be <50ms):** repository resolution plus one `git worktree list --porcelain`. The app frame renders immediately. Worktree rows may stay in a loading skeleton until local metadata is ready enough to sort consistently.
2. **Async, streamed in as each resolves:** local metadata (dirty status, remote/main ahead-behind from already-fetched refs, commit + age), PR + CI data via `gh`, and size data. Pending cells and detail fields show a subtle `⋯`.
3. **Size data:** the table uses a fast Git-aware size from `git ls-files --cached --others --exclude-standard`. The selected-row detail panel may additionally load full filesystem size with a cancellable `du`-equivalent walk.
4. **No `git fetch` on startup.** Ahead/behind reflects the last fetch. The TUI reloads local state every 30 seconds while idle; `r` triggers `git fetch --prune`, then loads local metadata before swapping the table so the existing rows stay visible during refresh.

Each async result patches its cell in place; no full-table flicker.
Stale async results are ignored after reloads. PR data is cached for the current TUI session and refreshed less often than local Git state. Remote-configured repositories reserve the PR column from the first render. Reloads immediately reattach last-known PR data while a fresh `gh` lookup runs, so the PR column does not flicker away.
Manual refresh shows scoped feedback in the Worktrees title: an 80ms Braille spinner with `refreshing` while in flight, then `✓ refreshed` for about 3 seconds. Auto-refresh stays quiet.

### 3.8 Detail panel, local hints, and status bar

Below the table:

- **Worktrees footer:** list-local hints live in the bottom border of the Worktrees panel. In normal mode this shows `h root · a active · Tab filter: <state> · s search`. With an active filter, it also shows `Esc clear filter`. While searching, letter keys feed the live search input, so the footer shows `search <text>▌ · Esc clear · Tab filter: <state>`.
- **Detail panel:** full info for the selected row: branch name, explicit `HEAD`, root/current state, absolute path, full status counts, Git-aware and full size when loaded, upstream name and sync state, main branch comparison, full commit subject, lifecycle/delete notes. Root/current context appears next to the Details title, for example `Details · Current root repository`; selected-row actions live in the bottom border: `↵ go · o editor · d delete · y abs path · p PR`.
- **Status bar:** transient progress and flash messages only. The app frame title starts with `Git treehouse · <repo>`. The top controls show `n new`, refresh age, help, and quit. Table-scoped refresh feedback lives in the Worktrees title instead of the status bar.
- **Help overlay:** groups shortcuts by context (`Global`, `Worktree List`, `Worktree Detail`) and groups visual legends (`Worktree Markers`, `Git Status`, `Pull Requests`). Category headers are bold white. The row-state and PR legends live here instead of the status bar.
- `g/G` remains available and documented in help, but is not shown in the main view.

### 3.9 Sorting, search, and filtering

- **Order:** root repository pinned first, remaining rows by last-commit date, newest first.
- **Search:** `s` opens a fuzzy search over branch names.
- **Filter:** `Tab` cycles filters across all, modified, prunable, locked, and detached rows. Search and filters compose. `Esc` clears the current search while searching; otherwise it clears the active filter. Bare `Esc` does not quit.

## 4. Actions & keybindings

| Key | Action |
|---|---|
| `↑`/`↓`, `k`/`j` | Move selection |
| `Enter` | cd to selected worktree and exit (writes `--cd-file`). On the active row: just quit. On a prunable row: disabled (hint shown in status bar). |
| `n` | Create worktree from focused row (§5) |
| `Delete` / `Backspace` / `d` | Delete flow (§6) |
| `o` | Open selected worktree in editor (config → `$EDITOR` fallback); TUI stays open |
| `p` | Open selected row's PR in browser (`gh pr view --web`); no PR → open repo page for the branch |
| `y` | Copy selected worktree's absolute path to clipboard; brief `copied` flash in status bar |
| `r` | `git fetch --prune` + stable refresh of all rows |
| `h` | Jump to the root repository worktree |
| `a` | Jump to the active worktree |
| `s` | Fuzzy branch search |
| `Tab` | Cycle filter: all → modified → prunable → locked → detached |
| `Ctrl+P` | Open command palette |
| `Esc` | Contextual cancel or clear: close topmost dialog, clear current search, or clear active filter. Does not quit. |
| `q`, `Ctrl+C` | Quit immediately from list view (no cd) |
| `?` | Toggle a help overlay with the full key list |

No multi-select / bulk operations in v1; every action applies to the focused row.

## 5. Create flow

`n` opens a modal dialog seeded from the focused row:

```
┌ New worktree ──────────────────────┐
│ Branch name: fix-dedup-crash▏      │
│ Base: ● feature/grouping (local)   │
│       ○ origin/feature/grouping    │
│       ○ origin/main                │
│                                    │
│ Enter create · Tab switch · Esc ✕  │
└────────────────────────────────────┘
```

- **Branch name:** free text input. Validated live against `git check-ref-format` rules and existing branch names; invalid/duplicate shows inline error and blocks Enter.
- **Base picker** (Tab or ↑/↓ cycles), defaults to the first option:
  1. Focused row's branch at its **local tip** (includes unpushed work)
  2. `origin/<focused-branch>` (last fetched remote state)
  3. `origin/<main>` (last fetched; the everyday "fork off main" path)
  - Options that don't exist (no upstream, detached row) are omitted.
- **On Enter:**
  1. Compute target path from the path template (§8): default `<repo-parent>/<sanitized-branch>` (slashes → dashes).
  2. Path collision → inline error, dialog stays open.
  3. Run `git worktree add -b <name> <path> <base>`.
  4. Success → **cd into the new worktree immediately** (write `--cd-file`, exit app).
  5. Failure → git's stderr shown in the dialog, dialog stays open.

Checking out an *existing* branch into a new worktree is intentionally out of scope for v1 (the base picker covers spawn-from-existing-state).

## 6. Delete flow

`Delete`/`Backspace`/`d` on a row opens a confirmation dialog. Never on the active or root repository row (status bar explains why).

The delete flow states exactly what will happen:

- **Locked worktree:** opens a blocking modal explaining that the worktree is locked, including Git's lock reason when available. No deletion command runs.
- **Regular worktree:** opens a confirmation modal with metadata (`Path`, `Branch`, `PR`), a worktree block, and a branch block when the row has a local branch.
- **Worktree toggle:** `t` toggles worktree removal.
  - Clean worktree → checked by default and uses `git worktree remove`.
  - Dirty worktree → unchecked by default; checking it means uncommitted changes will be discarded with `git worktree remove --force`.
- **Branch toggle:** `b` toggles local branch deletion. Branch deletion is disabled while worktree removal is unchecked, because Git will not delete a branch that is checked out in a worktree.
  - Branch merged into main → checked by default and uses safe `git branch -d`.
  - Branch unmerged → unchecked by default; checking it means force delete with `git branch -D`.
  - Upstream gone (PR merged) → hint `remote branch already deleted — likely safe`.
- `Enter` executes, `Esc` cancels. Result (or git error) flashes in the status bar; table reloads.

**Prunable rows** (directory missing) open a prune-only confirmation. The dialog offers `git worktree prune`-equivalent cleanup and does not show the branch deletion checkbox.

## 7. `git-treehouse list` (non-interactive)

- Prints the same columns as the TUI, aligned, one row per worktree.
- TTY: colored, with hyperlinks. Piped: plain text, no ANSI.
- Text output only loads async data for columns visible at the current width. PR lookup is skipped below the PR threshold, and table size is skipped below the size threshold. Otherwise async data is included only if it resolves within one short shared budget; unresolved cells print `-`. `--no-github` skips `gh` entirely.
- `--json` prints structured JSON with repository metadata plus worktree fields for lifecycle state, status counts, sync state, commit info, PR info when loaded, `git_size`, `full_size`, and the compatibility `size` alias for full size.

## 8. `git-treehouse doctor`

Prints a stdout report for local setup diagnostics:

- `git` availability and version.
- Current repository detection and main branch.
- `gh` availability/authentication for PR data.
- Config load/path.
- Shell integration presence for the detected shell.
- Editor command.
- Clipboard command.

## 9. Configuration

Optional `~/.config/git-treehouse/config.toml`. Everything works with zero config.

```toml
editor = "cursor"                          # default: $EDITOR, else `code`
path_template = "{repo_parent}/{branch}"   # default; {repo}, {repo_parent}, {branch} (sanitized)
main_branch = ""                           # default: auto-detect (origin/HEAD, fallback main/master)
```

## 10. Edge cases & errors

- **Main branch detection:** `origin/HEAD` symref; fallback to local `main`, then `master`. Override via config.
- **Detached HEAD rows:** branch column shows `<sha> detached`; `remote` shows `-`; `main±` is computed against the commit; create-base option 2 omitted.
- **No remotes at all:** `remote` shows `-`, PR column hidden, create dialog offers only local bases.
- **Worktree path with uncommitted submodule/locked state:** surface git's own error verbatim in the status bar, never swallow it.
- **Terminal too narrow (<60 cols):** drop columns per 3.1 priority; below ~40 cols show marker + branch + status only.
- All git interaction shells out to `git` (no libgit2): behavior matches the user's git version and config, and porcelain formats keep parsing stable.

## 10. Out of scope (v1)

- Multi-select / bulk delete
- Checking out an existing branch or PR into a new worktree
- Multi-repo dashboard
- Renaming or moving worktrees
