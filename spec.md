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
| `git-treehouse list` | Print the table to stdout, no TUI, no ANSI when piped. For scripting. |
| `git-treehouse init <shell>` | Print shell integration functions that define `gth` (see §2) |

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
| size | Disk usage, computed lazily (see 3.7) |

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

- Shows: `#123` + state glyph (open `○`, draft `◌`, merged `⬡`, closed `✕`) + CI status (`✓` passing, `✗` failing, `●` running).
- PR number is an OSC 8 hyperlink to the PR page (clickable in supporting terminals).
- `gh` missing/unauthed → column hidden entirely, no error noise.

### 3.7 Data loading model

Instant local render, async enrichment:

1. **Synchronous (must be <50ms):** `git worktree list`, branch names, dirty status, remote/main ahead-behind comparisons computed against already-fetched refs, commit + age. Table renders immediately.
2. **Async, streamed in as each resolves:** PR + CI data via `gh`; disk usage (`du`-equivalent walk per worktree, lowest priority). Pending cells show a subtle spinner/`…`.
3. **No `git fetch` on startup.** Ahead/behind reflects the last fetch. The TUI reloads local state every 30 seconds while idle; `r` triggers fetch + full reload.

Each async result patches its cell in place; no full-table flicker.

### 3.8 Detail panel, local hints, and status bar

Below the table:

- **Worktrees footer:** list-local hints live in the bottom border of the Worktrees panel. In normal mode this shows `h root · a active · Tab filter: <state> · s search`. While searching, letter keys feed the live search input, so the footer shows `search <text>▌ · Esc clear · Tab filter: <state>`.
- **Detail panel:** full info for the selected row: branch name, explicit `HEAD`, root/current state, absolute path, full status counts, upstream name and sync state, main branch comparison, full commit subject, lifecycle/delete notes.
- **Status bar:** context-sensitive global hints, e.g. `Esc close/clear`, plus the row-state legend. The top controls show `n new`, refresh age, help, and quit. During async loading the status bar appends a small progress note (`fetching PRs…`).
- `g/G` remains available and documented in help, but is not shown in the main view.

### 3.9 Sorting, search, and filtering

- **Order:** root repository pinned first, remaining rows by last-commit date, newest first.
- **Search:** `s` opens a fuzzy search over branch names.
- **Filter:** `Tab` cycles filters across all, modified, prunable, locked, and detached rows. Search and filters compose, and `Esc` clears the filter before the branch search.

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
| `r` / `f` | `git fetch --prune` + full reload of all rows |
| `h` | Jump to the root repository worktree |
| `a` | Jump to the active worktree |
| `s` | Fuzzy branch search |
| `Tab` | Cycle filter: all → modified → prunable → locked → detached |
| `Esc` | Ladder: close topmost dialog → clear filter → clear branch search → quit app |
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

The dialog states exactly what will happen:

- Worktree path to be removed.
- **Dirty worktree:** warning with the change counts; deletion requires confirming a `force` line (extra keystroke, e.g. typing `f` to arm), mapping to `git worktree remove --force`.
- **Branch toggle:** `[ ] also delete branch <name>` (Space toggles, off by default).
  - Branch merged into main → safe note shown.
  - Branch unmerged → warning + the same explicit force-arm requirement (`git branch -D`).
  - Upstream gone (PR merged) → hint `remote branch already deleted — likely safe`.
- `Enter` executes, `Esc` cancels. Result (or git error) flashes in the status bar; table reloads.

**Prunable rows** (directory missing) reuse this flow: the dialog offers `git worktree prune`-equivalent cleanup plus the same optional branch deletion.

## 7. `git-treehouse list` (non-interactive)

- Prints the same columns as the TUI, aligned, one row per worktree.
- TTY: colored, with hyperlinks. Piped: plain text, no ANSI.
- Includes async data only if it resolves within a short budget (~2s for `gh`); otherwise those cells print `-`. `--no-github` skips `gh` entirely.

## 8. Configuration

Optional `~/.config/git-treehouse/config.toml`. Everything works with zero config.

```toml
editor = "cursor"                          # default: $EDITOR, else `code`
path_template = "{repo_parent}/{branch}"   # default; {repo}, {repo_parent}, {branch} (sanitized)
main_branch = ""                           # default: auto-detect (origin/HEAD, fallback main/master)
```

## 9. Edge cases & errors

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
