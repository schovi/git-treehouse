# Navigation, search, and filtering

_Behavior spec. Index: [docs/README.md](../README.md) · Code: [docs/architecture.md](../architecture.md)._

- **Order:** root repository pinned first, remaining rows by last-commit date, newest first.
- **Search:** `s` opens a fuzzy search over branch names. Enter commits the query and keeps it applied; the Worktrees footer then shows `search: <query>` until the query is cleared from search mode.
- **Branches:** `b` toggles branch-only rows in the list and persists the preference to config.
- **Filter:** `Tab` opens a filter picker with all, modified, branches, merged, prunable, locked, and detached rows. Each filter shows its matching row count, and empty filters are disabled. In the picker, `↑`/`↓` moves selection, `Tab` jumps to the next enabled filter and wraps from the end to the first, `Enter` applies the selected filter, and `Esc` closes the picker. The branches filter shows branch-only rows even when the general branch-row toggle is off. The merged filter surfaces rows that are safe to clean up: worktrees with a clean working tree whose branch is merged to main or whose PR is merged/closed, plus merged branch-only rows. The root repository and detached worktrees never match. Search and filters compose. `Esc` clears the current search while searching; otherwise it clears the active filter. Bare `Esc` does not quit.
- **Empty results:** when a filter or search leaves no rows, the list names the active filter and search. It also states how to clear each: `Esc` clears a filter, and `s` then `Esc` clears a committed search.
