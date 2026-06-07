# Ideas

Backlog notes for future `git-treehouse` releases. These are intentionally lightweight, not commitments.

## UI and Layout

- Improve narrow-mode layout beyond column collapse, especially when the outer frame and detail panel consume too much width.
- Add optional theme configuration for selected-row color and status colors.
- Add an asciinema demo to the README.

## Inspector

- Show PR title, review state, and CI workflow names when GitHub data is loaded.
- Add a clearer stale-worktree hint for clean, merged, upstream-gone branches.
- Show absolute path in a help/detail overlay without changing the default relative path.

## Create Flow

- Offer branch prefix shortcuts such as `fix/`, `feat/`, and `chore/`.
- Show base commit short SHA for each base option.
- Consider prefilling new branch names on feature branches while keeping main empty.
- Validate target path collisions live.

## Delete Flow

- Require typing the branch name for unmerged branch deletion.

## GitHub

- Add copy PR URL action.
- Make GitHub lookup visibly cancellable or optional in the TUI.

## Performance

- Make disk usage walks cancellable on quit, resize, and reload.
- Debounce resize renders if needed.
- Avoid walking heavyweight ignored directories where safe.

## CLI and Scripting

- Add `--repo <path>` for explicit repo selection.
- Add `--sort age|branch|status|size`.
