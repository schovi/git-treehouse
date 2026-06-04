# gwt — git worktree TUI

A fast terminal UI for managing git worktrees: browse, switch, create, and delete them from one keyboard-driven table.

- **Binary + wrapper name:** `gwt`
- **Stack:** Go + Bubble Tea (with Lip Gloss for styling, Bubbles for inputs/spinners)
- **Scope:** single repo per invocation (the repo containing the cwd), single user, local tool

## 1. Invocation

`gwt` can be started from the main repo or any of its worktrees. It resolves the repo via `git rev-parse`; the full worktree set comes from `git worktree list --porcelain`.

- Run outside a git repo → print a one-line error to stderr, exit 1.
- Bare repos: the bare entry is not a navigable row; only real worktrees are listed.

### Subcommands

| Command | Behavior |
|---|---|
| `gwt` | Launch the TUI |
| `gwt list` | Print the table to stdout, no TUI, no ANSI when piped. For scripting. |
| `gwt init <shell>` | Print the shell wrapper function for `fish`, `zsh`, or `bash` (see §2) |

## 2. Shell integration (cd mechanism)

A child process cannot change its parent shell's cwd, so `gwt` uses the zoxide/yazi pattern:

- The TUI writes the selected path to a file given via `--cd-file <path>` (nothing else goes to that file).
- The wrapper function (installed via `gwt init fish | source` etc.) runs the binary with a temp `--cd-file`, and `cd`s to its content after the TUI exits, if non-empty.
- Quitting without selecting writes nothing; the shell stays where it was.
- **Graceful degradation:** without `--cd-file`, the selected path is printed to stdout on exit (TUI renders on stderr/tty), so `cd (gwt)` works bare.

## 3. Main view

### 3.1 Table

Borderless table, one row per worktree. Columns left to right:

| Column | Content |
|---|---|
| marker | Row state glyph (see 3.2) |
| branch | Branch name; `(detached)` + short SHA when detached |
| status | Working-tree state, compact (see 3.3) |
| head± | Ahead/behind vs upstream, e.g. `↑2 ↓1`; blank when synced; `–` when no upstream |
| main± | Ahead/behind vs the main branch, e.g. `↑5 ↓12`; blank for the main row |
| commit | Short SHA + truncated subject line |
| age | Relative last-commit time (`3h`, `2d`, `5w`) |
| PR | PR number + state + CI (see 3.4), rendered as a clickable OSC 8 hyperlink |
| size | Disk usage, computed lazily (see 3.5) |

Column sizing: branch and commit are elastic; commit truncates first, then size, PR, and age drop entirely on narrow terminals. Marker, branch, and status always survive.

There was a dedicated `remote` column in the original sketch; it is dropped — upstream state folds into the status column (3.3).

### 3.2 Row markers

| Glyph | Meaning |
|---|---|
| `●` | Active worktree (where `gwt` was started) |
| `○` | Other worktree |
| `⌂` | Main worktree (combined with active state: `◉` if main is also active) |
| `✗` | Prunable: directory missing on disk |
| `🔒` (`⊘` fallback) | Locked worktree |

### 3.3 Status column

Compact, info-dense:

- `✓` — clean
- Counts for dirty trees: `+2 ~3 ?1` (staged / modified / untracked); only non-zero parts shown
- `⚠ gone` — upstream branch deleted on remote (typical after PR merge)
- `detached`, `locked`, `prunable` — special states as words when they apply

The **selected row's full detail** is rendered in the detail line above the status bar (see 3.6) — the table stays compact, nothing is hidden.

### 3.4 GitHub data (PR column)

Loaded via the `gh` CLI (one `gh pr list`-style call for all branches), only if `gh` exists and is authed.

- Shows: `#123` + state glyph (open `○`, draft `◌`, merged `⬡`, closed `✕`) + CI status (`✓` passing, `✗` failing, `●` running).
- PR number is an OSC 8 hyperlink to the PR page (clickable in supporting terminals).
- `gh` missing/unauthed → column hidden entirely, no error noise.

### 3.5 Data loading model

Instant local render, async enrichment:

1. **Synchronous (must be <50ms):** `git worktree list`, branch names, dirty status, local ahead/behind (computed against already-fetched refs), commit + age. Table renders immediately.
2. **Async, streamed in as each resolves:** PR + CI data via `gh`; disk usage (`du`-equivalent walk per worktree, lowest priority). Pending cells show a subtle spinner/`…`.
3. **No `git fetch` on startup.** Ahead/behind reflects the last fetch. `r` triggers fetch + full reload (3.7).

Each async result patches its cell in place; no full-table flicker.

### 3.6 Detail line + status bar

Below the table:

- **Detail line:** full info for the selected row — absolute path, full status counts, upstream name and sync state, full commit subject.
- **Status bar:** context-sensitive key hints, e.g. `↵ go · n new · d delete · o editor · p PR · y path · r refresh · / filter · q quit`. During async loading it appends a small progress note (`fetching PRs…`).

### 3.7 Sorting & filtering

- **Order:** main worktree pinned first, remaining rows by last-commit date, newest first.
- **Filter:** `/` opens a fuzzy filter over branch names; the list narrows live. `Esc` clears it. Filtering preserves the pin-main-first rule when main matches.

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
| `/` | Fuzzy filter |
| `Esc` | Ladder: close topmost dialog → clear filter → quit app |
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

`Delete`/`Backspace`/`d` on a row opens a confirmation dialog. Never on the active or main row (status bar explains why).

The dialog states exactly what will happen:

- Worktree path to be removed.
- **Dirty worktree:** warning with the change counts; deletion requires confirming a `force` line (extra keystroke, e.g. typing `f` to arm), mapping to `git worktree remove --force`.
- **Branch toggle:** `[ ] also delete branch <name>` (Space toggles, off by default).
  - Branch merged into main → safe note shown.
  - Branch unmerged → warning + the same explicit force-arm requirement (`git branch -D`).
  - Upstream gone (PR merged) → hint `remote branch already deleted — likely safe`.
- `Enter` executes, `Esc` cancels. Result (or git error) flashes in the status bar; table reloads.

**Prunable rows** (directory missing) reuse this flow: the dialog offers `git worktree prune`-equivalent cleanup plus the same optional branch deletion.

## 7. `gwt list` (non-interactive)

- Prints the same columns as the TUI, aligned, one row per worktree.
- TTY: colored, with hyperlinks. Piped: plain text, no ANSI.
- Includes async data only if it resolves within a short budget (~2s for `gh`); otherwise those cells print `-`. `--no-github` skips `gh` entirely.

## 8. Configuration

Optional `~/.config/gwt/config.toml`. Everything works with zero config.

```toml
editor = "cursor"                          # default: $EDITOR, else `code`
path_template = "{repo_parent}/{branch}"   # default; {repo}, {repo_parent}, {branch} (sanitized)
main_branch = ""                           # default: auto-detect (origin/HEAD, fallback main/master)
```

## 9. Edge cases & errors

- **Main branch detection:** `origin/HEAD` symref; fallback to local `main`, then `master`. Override via config.
- **Detached HEAD rows:** branch column shows `(detached) <sha>`; head±/main± computed against the commit; create-base option 2 omitted.
- **No remotes at all:** head± shows `–`, PR column hidden, create dialog offers only local bases.
- **Worktree path with uncommitted submodule/locked state:** surface git's own error verbatim in the status bar, never swallow it.
- **Terminal too narrow (<60 cols):** drop columns per 3.1 priority; below ~40 cols show branch + status only.
- All git interaction shells out to `git` (no libgit2): behavior matches the user's git version and config, and porcelain formats keep parsing stable.

## 10. Out of scope (v1)

- Multi-select / bulk delete
- Checking out an existing branch or PR into a new worktree
- Multi-repo dashboard
- Background auto-refresh / polling while open
- Renaming or moving worktrees
