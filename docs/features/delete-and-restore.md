# Delete and restore flow

_Behavior spec. Index: [docs/README.md](../README.md) · Code: [docs/architecture.md](../architecture.md)._

`Delete`/`Backspace`/`d` on a row opens a confirmation dialog. Never on the active or root repository row (status bar explains why). Branch-only rows delete only the local branch ref and never remove worktree files.

The delete flow states exactly what will happen:

- **Locked worktree:** opens a blocking modal explaining that the worktree is locked, including Git's lock reason when available. No deletion command runs.
- **Regular worktree:** opens a confirmation modal with metadata (`Path`, `Branch`, `PR`), a worktree block, and a branch block when the row has a local branch.
- **Worktree toggle:** `t` toggles worktree removal.
  - Clean worktree → checked by default and uses `git worktree remove`.
  - Dirty worktree → unchecked by default; checking it means uncommitted changes will be discarded with `git worktree remove --force`.
- **Cleanup hook:** when `.worktree` defines an approved `before_delete` hook, the dialog includes an `h` toggle to run it before `git worktree remove`. The hook is enabled by default for real worktree removal and does not run for prunable cleanup or branch-only deletion.
- **Branch toggle:** `b` (or `Space`) toggles local branch deletion. Branch deletion is disabled while worktree removal is unchecked, because Git will not delete a branch that is checked out in a worktree.
  - Branch merged into main → checked by default and uses safe `git branch -d`.
  - Branch unmerged → unchecked by default; checking it means force delete with `git branch -D`.
  - Upstream gone (PR merged) → hint `remote branch already deleted — likely safe`.
- **Branch-only row:** opens a branch-only confirmation with metadata (`Branch`, `HEAD`, `PR`) and the exact branch command. Merged branches use `git branch -d`; unmerged branches are explicit force deletes with `git branch -D`.
- `Enter` executes, `Esc` cancels. Delete and cleanup actions time out after 10 minutes; while one is running, `Esc` cancels it and leaves the error in the dialog. Success flashes in the Worktrees status; git errors stay in the dialog or status bar. The table reloads after writes, including a partial delete. If worktree removal succeeds but branch deletion fails, the refreshed list shows the remaining branch-only row and the dialog names that action.

After a branch ref is deleted, either from a branch-only row or a worktree plus branch delete, a green Worktrees status offer appears for about 10 seconds: `✓ deleted <name> (<short-sha>) · u to restore`, with `u` styled as a key. Pressing `u` recreates the ref with `git branch <name> <sha>`. A batch restore attempts every branch even when one already exists, then reports restored and failed counts. This restores only the branch ref, not worktree files or discarded uncommitted changes. The offer is superseded by the next delete or manual `r` refresh. Automatic refresh waits while the offer is live.

**Prunable rows** (directory missing) open a prune-only confirmation. The dialog offers `git worktree prune`-equivalent cleanup and does not show the branch deletion checkbox.

**Clean up merged** is a palette-only batch command. It scans all done rows, regardless of the current filter or search: worktrees or branch-only rows merged into main, plus worktrees whose PR is merged or closed. It opens a confirmation dialog with counts for worktrees to remove and branches to delete, representative affected rows, and the exact commands that will run.

The batch stays conservative:

- Worktree removal uses only `git worktree remove`, never `--force`.
- Branch deletion uses only `git branch -d`, never `-D`.
- Clean, unlocked, non-root, non-active worktrees are removed. Dirty, locked, active, root, detached, prunable, and not-yet-loaded rows are ignored.
- Worktree branches are deleted only when merged into main. PR merged/closed worktrees that are not merged into main are removed, but their local branch is kept.
- Merged branch-only rows are deleted with `git branch -d`; branch-only rows that are only PR merged/closed are skipped.
- Approved `before_delete` hooks run once per removed worktree before removal. Hook failure counts as that row's failure and skips its removal; the rest of the batch continues.

Full success closes the dialog and shows a Worktrees success message summarizing removed worktrees and deleted branches. Partial failure keeps a result dialog open with counts and failure reasons. When the batch deletes branch refs with known SHAs, the success message offers `u` to restore all deleted branch refs.
