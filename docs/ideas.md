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
- After a force delete (`git branch -D`), flash the reflog recovery command (`git branch <name> <sha>`) in the status bar so the action stays reversible.

## GitHub

- Add copy PR URL action.
- Make GitHub lookup visibly cancellable or optional in the TUI.
- Check out a PR into a new worktree (`gh pr checkout <n>`), including PRs whose branch is not yet local, then cd into it. Leverages the existing `gh` integration.

## Performance

- Make disk usage walks cancellable on quit, resize, and reload.
- Debounce resize renders if needed.
- Avoid walking heavyweight ignored directories where safe.

## CLI and Scripting

- Add `--repo <path>` for explicit repo selection.
- Add `--sort age|branch|status|size`.

## Repo-scoped configuration

Some settings are inherently per-repo (worktree layout, setup steps) and do not belong in the single global config. Lean toward the repo's `.git/config` under a custom `[treehouse]` section, read through the existing git Runner seam (`git config --get-all treehouse.<key>`):

- `.git/config` is shared across all worktrees of a repo and is machine-local, which matches "settings for this clone." No new dotfile, no extra parser.
- Precedence: repo `.git/config` overrides global `~/.config/git-treehouse/config.toml`.
- Not universal: git core has no worktree-add hook, so the section is our own convention, not a cross-tool standard.
- Limitation: `.git/config` is not committed, so it cannot carry team-shared defaults. A committed file (for example `.git-treehouse.toml` at the repo root) could be a later layer for shareable settings; start with `.git/config`.

Features this unlocks:

- Per-repo `path_template` override.
- Post-create hook: run a command after `git worktree add` (for example `npm install`, `direnv allow`).
- Copy a named list of untracked/gitignored files into a new worktree (for example `.env`, `.env.local`). The safe, scoped slice of "copy uncommitted changes": named files only, never arbitrary dirty state.

## Cleanup and Filtering

- Add a "merged" / "done" filter that surfaces live worktrees safe to remove (clean working tree, merged to main, or PR merged/closed). The current `Tab` `prunable` filter only catches worktrees whose directory is already gone, not finished-but-still-present ones.
- Consider a bulk "clean up merged" action over that filtered set, staying within conservative defaults (clean + merged only).

## Navigation and Opening

- Open a worktree in a new tmux/zellij window or terminal tab instead of (or in addition to) cd-on-exit, via a configurable open command. Suits running several worktrees in parallel.
