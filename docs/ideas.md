# Ideas

Backlog notes for future `git-treehouse` releases. These are intentionally lightweight, not commitments.

## UI and Layout

- Improve narrow-mode layout beyond column collapse, especially when the outer frame and detail panel consume too much width.
- Add optional theme configuration for selected-row color and status colors.
- Add an asciinema demo to the README.

## Inspector

- Show the PR title when GitHub data is loaded.
- Add a clearer stale-worktree hint for clean, merged, upstream-gone branches. (Task 011.)
- Show absolute path in a help/detail overlay without changing the default relative path.

## Create Flow

- Offer branch prefix shortcuts such as `fix/`, `feat/`, and `chore/`.
- Show base commit short SHA for each base option.
- Consider prefilling new branch names on feature branches while keeping main empty.
- Validate target path collisions live. (Task 012.)

## Delete Flow

- Require typing the branch name for unmerged branch deletion.

## GitHub

- Make GitHub lookup optional in the TUI. (Task 014.)

## Performance

- Avoid walking heavyweight ignored directories where safe.

## CLI and Scripting

- Add `--sort age|branch|status|size`.

## Cleanup and Filtering

- Consider generic bulk selection later if there are enough useful actions to justify the extra UI state. Possible actions: copy selected paths or branch names, open selected worktrees in the configured editor, prune selected missing worktree metadata, delete selected safe branches with `git branch -d`, or lock/unlock selected worktrees. Bulk delete remains the risky case because rows can mix dirty worktrees, locked worktrees, active/root rows, detached worktrees, branch-only rows, unmerged branches, and hook failures.

## Navigation and Opening

- Open a worktree in a new tmux/zellij window or terminal tab instead of (or in addition to) cd-on-exit, via a configurable open command. Suits running several worktrees in parallel. (Task 020.)
