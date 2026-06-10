# Create and checkout flows

_Behavior spec. Index: [docs/README.md](../README.md) · Code: [docs/architecture.md](../architecture.md)._

`n` opens a modal dialog seeded from the focused row:

```
┌ New worktree ──────────────────────┐
│ Branch name: fix-dedup-crash▏      │
│ Base: ● feature/grouping (local)   │
│       ○ origin/feature/grouping    │
│       ○ origin/main                │
│                                    │
│ Enter create · Tab switch · Esc ✕  │
└────────────────────────────────────┘
```

- **Branch name:** free text input. Validated live against `git check-ref-format` rules and existing branch names; invalid/duplicate shows inline error and blocks Enter.
- **Base picker** (Tab or ↑/↓ cycles), defaults to the first option:
  1. Focused row's branch at its **local tip** (includes unpushed work)
  2. `origin/<focused-branch>` (last fetched remote state)
  3. `origin/<main>` (last fetched; the everyday "fork off main" path)
  - Options that don't exist (no upstream, detached row) are omitted.
- **On Enter:**
  1. Compute target path from the effective path template (see [Configuration](./configuration.md)): repo `.worktree` `path_template` if set, otherwise global `config.toml`, otherwise the default `<repo-parent>/.worktrees/<repo-name>/<sanitized-branch>` (slashes → dashes).
  2. Path collision → inline error, dialog stays open.
  3. Run `git worktree add -b <name> <path> <base>`.
  4. Copy any repo-scoped `copy_untracked` files (see [Configuration](./configuration.md)) from the root repository into the new worktree.
  5. Run the approved `post_create` hook, if configured (see [Configuration](./configuration.md)).
  6. Success → **cd into the new worktree immediately** (write `--cd-file`, exit app).
  7. Failure → git's stderr or hook error shown in the dialog, dialog stays open. If the failure happened after `git worktree add`, the created worktree remains on disk.

## Existing branch worktree flow

`Enter` on a branch-only row opens a new-worktree confirmation modal:

```
┌ New worktree ────────────────────────────┐
│ Branch: feature/list-branches            │
│ Path: /repo/.worktrees/repo/feature-list │
│                                          │
│ Enter create + go · Esc cancel           │
└──────────────────────────────────────────┘
```

- The target path uses the same path template as the create flow.
- Path collision → inline error, dialog stays open.
- On Enter, run `git worktree add <path> <branch>`.
- Then copy repo-scoped `copy_untracked` files and run the approved `post_create` hook, if configured (see [Configuration](./configuration.md)).
- Success → cd into the new worktree immediately (write `--cd-file`, exit app).
- Failure → git's stderr or hook error shown in the dialog, dialog stays open. If the failure happened after `git worktree add`, the created worktree remains on disk.
- Copying arbitrary uncommitted changes from another worktree is intentionally not automatic. Only the named `copy_untracked` files are copied.

## Checkout branch in root flow

`c` on a branch-only row checks out that branch in the root worktree. The root worktree is the only checkout target for this action.

```
┌ Checkout root ─────────────────────────────────────────┐
│ Branch    feature/list-branches                       │
│ Root      /repo/main                                  │
│ Current   main                                        │
│                                                        │
│ Root has uncommitted changes.                         │
│ ~ modified 1                                          │
│                                                        │
│ Root changes                                          │
│ [ ] s stash current changes                           │
│ Checkout is blocked until root changes are stashed.   │
│ No checkout command will run.                         │
│                                                        │
│ s stash changes · Esc cancel                          │
└────────────────────────────────────────────────────────┘
```

- Clean root: run `git switch -- <branch>`, then write the root path to `--cd-file` and exit.
- Dirty root: open the confirmation modal above. Enter is blocked until `s` enables stashing.
- With stash enabled: Enter runs `git stash push -u -m "git-treehouse: before switching to <branch>"`, then `git switch -- <branch>`.
- Failure from either git command is shown in the dialog or status bar, and the app stays open.
- No force checkout, discard, or automatic copying happens.
