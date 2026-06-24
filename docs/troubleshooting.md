# Troubleshooting

## Worktree shows as prunable

The directory was deleted without `git worktree remove`. Run `git worktree prune` or use the prune action in gwt.

## Locked worktree cannot be removed

Run `git worktree unlock <path>` first, or check the lock reason with `git worktree list --porcelain`.

## Shell does not change directory after selecting a worktree

Make sure the shell integration is installed, see README section "Shell integration".
