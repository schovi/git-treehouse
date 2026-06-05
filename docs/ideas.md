# Ideas

Backlog notes for future `gwt` releases. These are intentionally lightweight, not commitments.

## UI and Layout

- Add a narrow-mode layout designed specifically for small terminals.
- Add optional theme configuration for selected-row color and status colors.
- Show row count and filtered count in the title or status area.
- Keep PR and size columns hidden until data is available or terminal width is very wide.
- Add screenshots or an asciinema demo to the README.

### Boxed App Layout Sketch

Explore a full dashboard-style frame where each major region has its own box. This would make `gwt` feel less like rendered text and more like a terminal app surface.

```text
┌ gwt ─ activejob-temporal ─ 2 worktrees ───────────── n new · r refresh · ? help · q quit ┐
│ ┌ Worktrees ───────────────────────────────────────────────────────────────────────────┐ │
│ │   │ branch                  │ status │ head± │ main± │ commit              │ age │ PR │ │
│ │ ● │ main                    │ ✓      │ ↑1    │       │ 46264bf add AGENTS  │ 14h │    │ │
│ │ ○ │ codex/baseline-rspec-ci │ ✓      │       │ ↑1 ↓1 │ 886d3ef chore...    │ 29m │ #98│ │
│ └──────────────────────────────────────────────────────────────────────────────────────┘ │
│ ┌ Details ──────────────────────────────────────────┬ Current ─────────────────────────┐ │
│ │ Branch    main                                    │ ↵ go                              │ │
│ │ Path      .                                       │ o editor                          │ │
│ │ Status    clean                                   │ d delete                          │ │
│ │ Dirty     none                                    │ y abs path                        │ │
│ │ Sync      origin/main, ↑1                         │ p PR                              │ │
│ │ Commit    46264bf add AGENTS, 14h                 │                                   │ │
│ │ PR        none                                    │                                   │ │
│ │ Delete    blocked, active worktree                │                                   │ │
│ └───────────────────────────────────────────────────┴───────────────────────────────────┘ │
│ g/G top/bottom · m main · a active · Tab notable · / filter       + staged · ~ modified │
└──────────────────────────────────────────────────────────────────────────────────────────┘
```

Notes:

- Keep the table box dominant, because selecting a worktree is the primary task.
- Use one outer frame only if it does not waste too much width on smaller terminals.
- Let narrow terminals drop the outer frame first, then collapse the detail/action box.
- Keep selected-row highlight full width inside the table box.
- Avoid heavy borders inside rows; vertical column dividers are enough.

## Inspector

- Show PR title, review state, and CI workflow names when GitHub data is loaded.
- Show exact delete commands in destructive dialogs.
- Add a clearer stale-worktree hint for clean, merged, upstream-gone branches.
- Show absolute path in a help/detail overlay without changing the default relative path.

## Navigation

- Preserve selection by branch/path across all async enrichment updates, not only reloads.
- Add jump shortcuts for next dirty, next prunable, next upstream-gone row.
- Add a command palette for less common actions.
- Make `Esc` behavior visible when a filter or dialog is active.

## Create Flow

- Preview the target worktree path live while editing branch name.
- Offer branch prefix shortcuts such as `fix/`, `feat/`, and `chore/`.
- Show base commit short SHA for each base option.
- Consider defaulting new branch name to empty on main and prefilled on feature branches.
- Validate target path collisions live.

## Delete Flow

- Split delete confirmation into separate `Worktree` and `Branch` sections.
- Require typing the branch name for unmerged branch deletion.
- Add a dedicated prune action for prunable rows.
- Keep selection near the deleted row after reload.

## GitHub

- Cache PR data during a session so refresh is less jumpy.
- Add copy PR URL action.
- Add better fallback behavior when no PR exists for a branch.
- Make GitHub lookup visibly cancellable or optional.

## Performance

- Load disk usage for visible rows first.
- Make disk usage walks cancellable on quit, resize, and reload.
- Debounce resize renders if needed.
- Avoid walking heavyweight ignored directories where safe.

## CLI and Scripting

- Add `gwt list --json`.
- Add `gwt doctor` for checking `git`, `gh`, shell wrapper, config, editor, and clipboard.
- Add `--repo <path>` for explicit repo selection.
- Add `--sort age|branch|status|size`.

## Distribution

- Add GitHub Actions for `go test ./...`.
- Add tagged binary releases.
- Add Homebrew packaging.
- Add `CHANGELOG.md`.
