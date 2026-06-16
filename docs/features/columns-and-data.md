# Columns and data

_Behavior spec. Index: [docs/README.md](../README.md) · Code: [docs/architecture.md](../architecture.md)._

## Status column

The status column is only for working-tree file changes:

- `✓` means clean
- Counts for dirty trees: `+2 ~3 ?1` (staged / modified / untracked); only non-zero parts shown
- `-` means the row is a local branch without a checked-out worktree

Lifecycle and Git state words do not appear in this column. `locked` and `prunable` are rendered as name-column lifecycle suffixes. Detached HEAD rows show `detached` in the name column. Upstream state is rendered in `remote`.

## Remote and main sync

`remote` compares `HEAD` to the upstream ref (`@{u}`):

- `✓` means upstream exists and `HEAD` is synced with it
- `↑2` means local has two commits not on upstream
- `↓1` means upstream has one commit not in local
- `↑2 ↓1` means local and upstream diverged
- `gone` means the configured upstream branch was deleted on the remote
- `-` means no upstream, no remote, or detached `HEAD`

`main±` compares `HEAD` to the detected local main branch. This is independent from the root repository. If the root repository is currently on a feature branch, it still gets a `main±` value. The column is blank only when the row is already on the detected main branch.

## GitHub data (PR column)

Loaded via the `gh` CLI, only if `gh` exists and is authed. The fetch is split in two:

- **Branch→PR mapping:** one `gh pr list` call for all branches (number, state, URL). It deliberately omits `statusCheckRollup`; that field forces GitHub's GraphQL API to resolve CI for every PR in the list and times out (HTTP 504) on large repositories, which would silently drop the whole PR column.
- **CI status:** fetched lazily per PR via `gh pr view <n> --json statusCheckRollup`, only for *open* PRs attached to a local row or branch (bounded by local branches, not the whole repo), in a small parallel worker pool. So PR association appears first and CI glyphs fill in shortly after.

- Shows: `#123` + state glyph (open/ready `○`, draft `◌`, approved `◆`, merged `⬡`, closed `✕`) + CI status (`✓` passing, `✗` failing, `●` running).
- PR number is an OSC 8 hyperlink to the PR page (clickable in supporting terminals).
- No configured remote → column hidden entirely.
- `gh` missing/unauthed → column remains reserved but empty, no error noise.

## Data loading model

Instant app frame, async enrichment:

1. **Synchronous (must be <50ms):** repository resolution plus one `git worktree list --porcelain`. The app frame renders immediately. Worktree rows may stay in a loading skeleton until local metadata is ready enough to sort consistently.
2. **Async, streamed in as each resolves:** local metadata (dirty status, branch-only rows from local refs, remote/main ahead-behind from already-fetched refs, commit + age), the PR mapping via `gh pr list` then per-PR CI via `gh pr view`, and size data. Pending cells and detail fields show a subtle `⋯`.
3. **Size data:** the table uses a fast Git-aware size from `git ls-files --cached --others --exclude-standard`. The selected-row detail panel may additionally load full filesystem size with a cancellable `du`-equivalent walk.
4. **No `git fetch` on startup.** Ahead/behind reflects the last fetch. The TUI reloads local state every 30 seconds while idle; `r` triggers `git fetch --prune`, then loads local metadata before swapping the table so the existing rows stay visible during refresh.

Each async result patches its cell in place; no full-table flicker.
Stale async results are ignored after reloads. PR data is cached for the current TUI session and refreshed less often than local Git state. Remote-configured repositories reserve the PR column from the first render. Reloads immediately reattach last-known PR data while a fresh `gh` lookup runs, so the PR column does not flicker away.
Manual refresh shows scoped feedback in the Worktrees title: an 80ms Braille spinner with `refreshing` while in flight, then `✓ refreshed` for about 3 seconds. Auto-refresh stays quiet.
