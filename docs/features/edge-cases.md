# Edge cases & errors

_Behavior spec. Index: [docs/README.md](../README.md) · Code: [docs/architecture.md](../architecture.md)._

- **Main branch detection:** `origin/HEAD` symref; fallback to local `main`, then `master`. Override via config.
- **Detached HEAD rows:** name column shows `⊡ <sha> detached`; `remote` shows `-`; `main±` is computed against the commit; create-base option 2 omitted.
- **No remotes at all:** `remote` shows `-`, PR column hidden, create dialog offers only local bases.
- **Worktree path with uncommitted submodule/locked state:** surface git's own error verbatim in the status bar, never swallow it.
- **Terminal too narrow (<60 cols):** drop columns per the [main view](./main-view.md) column priority; below ~40 cols show name + status only.
- All git interaction shells out to `git` (no libgit2): behavior matches the user's git version and config, and porcelain formats keep parsing stable.
