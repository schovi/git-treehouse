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

- **Branch name:** free text input. Validated on Enter against `git check-ref-format` rules and existing branch names; invalid/duplicate shows inline error and blocks Enter. Editing clears the previous error.
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
  7. Failure → git's stderr or hook error shown in the dialog, dialog stays open. If Esc closed the dialog while the command ran, the error appears in the status flash. If the failure happened after `git worktree add`, the created worktree remains on disk.
  8. While the command runs, Enter is ignored to prevent a duplicate create. Esc closes only the dialog.

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

## Pull request worktree flow

`Checkout PR` is available from the command palette only. It opens a centered picker and immediately loads up to 200 pull requests through `gh pr list --state all`, sorted by most recently updated. The picker includes open, draft, merged, and closed pull requests.

```
┌ Checkout PR ─────────────────────────────────────────────────────┐
│ > auth cleanup                                                   │
│                                                                  │
│   #128  ○  Fix auth cleanup after token refresh  feature/auth   │
│ › #127  ⎇  Add PR checkout into worktree         alice/pr-flow  │
│   #126  ✕  Remove stale worktree copy path       path-fix       │
│   #125  ◌  Draft: improve status loading         loading        │
│                                                                  │
│ Enter checkout · o open · ↑/↓ move · Esc cancel                 │
└──────────────────────────────────────────────────────────────────┘
```

- Typing filters by PR number, title, URL, owner, head branch, and local branch name.
- `↑`/`↓` or `k`/`j` moves the highlighted PR. The highlighted row uses the same full-width selection background as the filter picker.
- `o` opens the highlighted PR in the browser via `gh pr view <number> --web`. If no loaded row matches and the input contains a PR URL or number, `o` opens that query directly. Open failures are shown inline.
- `Esc` closes the picker. No global keybinding opens this flow.
- While `gh` is loading, the modal stays open and shows `loading pull requests`. If `gh` fails, the error is shown inline in the modal.
- Pressing `Enter` on a highlighted PR:
  1. Computes the local branch name from the PR head. Same-repo PRs use `headRefName`; fork PRs use `<owner>/<headRefName>`.
  2. Existing non-prunable worktree for that branch → cd into it immediately.
  3. Existing local branch without a worktree → compute the target path from the normal path template, run `git worktree add <path> <branch>`, then run normal post-create steps.
  4. New PR branch → run `git fetch origin pull/<number>/head`, then `git worktree add -b <branch> <path> FETCH_HEAD`, then run normal post-create steps.
  5. Success → cd into the worktree immediately (write `--cd-file`, exit app).
  6. Failure → show the error inline in the picker, and keep the app open.
- If the input is a PR URL or number that is not in the loaded list, `Enter` tries `gh pr view <input>` directly. Invalid input shows `No matching PR`.
- Existing local branch reuse does not force-update the branch from the PR head.
