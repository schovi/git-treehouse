# 002 — Delete/cleanup robustness: timeouts, in-flight cancel, partial-failure state

done: 2026-08-04

tags: tui, safety

## What & why

A hung `before_delete` hook freezes the TUI unrecoverably (adversarially verified): delete and cleanup run on `context.Background()` with no timeout, and while in flight the dialogs swallow every key including Esc and Ctrl+C. The only way out is killing the process. Two adjacent robustness gaps in the same flow: partial delete failure leaves stale rows on screen, and batch restore aborts on the first error.

## Spec

Verified mechanism (hang):

- `deleteAndLoadCmd` creates `ctx := context.Background()` with no timeout (`internal/tui/model.go:2338`, function 2336-2352); the action runs the `before_delete` hook with that ctx (`model.go:2395-2397`).
- `cleanupMergedAndLoadCmd` identical (`model.go:2173`, function 2171-2185; hooks at 2190-2194).
- `gitdata.RunHook` is `sh -c <hook>` via `runner.RunWithEnv` (`internal/gitdata/hooks.go:5-8`); `ExecRunner` uses `exec.CommandContext` (`runner.go:43`), so a cancelled/timed-out ctx WOULD kill it — only `Background` never fires.
- Key swallowing: `updateDelete` early-returns for every key while in flight (`model.go:2007-2009`), `updateCleanupMerged` likewise (`model.go:2131-2133`); dialogs stay non-nil during flight (`model.go:2329-2331`, `2164-2166`) and the KeyMsg dispatcher routes to them first (`model.go:1028-1033`). Ctrl+C/q live only in `updateList` (`model.go:1054-1056`).
- Contrast: create flow already caps at 10 minutes via `context.WithTimeout` (`model.go:1913`).

Fixes:

1. Wrap delete and cleanup action contexts in `context.WithTimeout` (10 min, matching create).
2. Store the `CancelFunc` on the model; let Esc (and Ctrl+C, see task 004) while in-flight cancel the context instead of being swallowed. Cancelled operation resolves to an error shown in the dialog, not a silent close.
3. Partial delete failure leaves UI stale (unverified, evidence cited): `deleteAndLoadCmd` returns `deleteMsg{err}` without reloaded state on any-step error (`model.go:2336-2341`); `deleteRow` removes the worktree before attempting branch deletion (`model.go:2394-2408`), so worktree-gone-but-listed + a second Enter re-runs the hook against a removed path. Fix: on partial failure still reload state and attach both err and state to `deleteMsg`, so the dialog error renders over fresh rows.
4. Batch restore aborts on first error (unverified, evidence cited): `startRestore` returns on first `CreateBranchAt` failure (`model.go:2359-2366`), silently skipping remaining branches. Fix: continue the loop, collect per-branch failures, report restored/failed counts (mirror `runCleanupMerged`'s failure accumulation).

Boundary: `internal/tui/model.go` (deleteAndLoadCmd, cleanupMergedAndLoadCmd, updateDelete, updateCleanupMerged, startRestore, deleteMsg/cleanupMergedMsg handlers). Tests: `internal/tui/model_test.go` (delete/cleanup dialog tests exist ~2515-2775). Routed docs: `docs/features/delete-and-restore.md` — update in same change. Contract: docs/harness.md "Dangerous Actions" — dialog wording must keep stating command family + data-loss effect.

Exclusions: the global Ctrl+C handler itself is task 004; here only ensure in-flight cancel hooks into whatever handler exists.

## Acceptance criteria

- A delete or cleanup whose hook never returns ends with a timeout error in the dialog, not a frozen TUI.
- Esc during an in-flight delete/cleanup cancels the operation and shows a cancelled/error state; keys are no longer silently discarded.
- After a partial delete failure (worktree removed, branch delete fails), the list shows the reloaded state and the dialog error refers to the remaining branch-only action.
- Batch `u` restore continues past an already-existing branch and reports restored/failed counts.
- `docs/features/delete-and-restore.md` reflects timeout/cancel behavior in the same change.
