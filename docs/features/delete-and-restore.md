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
- `Enter` executes, `Esc` cancels. Success flashes in the Worktrees status; git errors stay in the dialog or status bar. The table reloads after successful writes.

After a branch ref is deleted, either from a branch-only row or a worktree plus branch delete, a green Worktrees status offer appears for about 10 seconds: `✓ deleted <name> (<short-sha>) · u to restore`, with `u` styled as a key. Pressing `u` recreates the ref with `git branch <name> <sha>`. This restores only the branch ref, not worktree files or discarded uncommitted changes. The offer is superseded by the next delete or refresh.

**Prunable rows** (directory missing) open a prune-only confirmation. The dialog offers `git worktree prune`-equivalent cleanup and does not show the branch deletion checkbox.
