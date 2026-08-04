# 006 — Auto-refresh must not revoke the 10s "u to restore" undo offer

done: 2026-08-04

tags: tui, safety

## What & why

The 30s background auto-refresh unconditionally clears feedback, including the pending branch-restore offer and its captured SHA. Delete a branch just before a tick and the undo silently vanishes ~1s later; pressing `u` does nothing and the SHA is only recoverable via reflog. The restore offer is the app's core safety affordance; an invisible background event destroying it undermines it exactly when it matters.

## Spec

Evidence (from UX review, not adversarially verified — confirm exact lines first):

- Auto-refresh ticks every 30s (`internal/tui/model.go:117`) and calls `startRefresh`, which unconditionally runs `model.clearFeedback()` (`model.go:1175`); `clearFeedback` nils `pendingRestore` and `pendingRestoreBatch` (`model.go:3580-3586`).
- `canApplyAutoRefresh` (`model.go:1197-1211`) checks dialogs etc. but not `hasPendingRestore`.
- `docs/features/delete-and-restore.md` says the offer is "superseded by the next delete or refresh" — written with manual refresh in mind; an automatic refresh is invisible to the user.

Fix (pick the minimal one): add a pending-restore check to `canApplyAutoRefresh` (`model.go:1197`) so the automatic tick is deferred while the offer is live (offer window is 10s, tick loss is negligible). Manual `r` clearing the offer stays as-is. Alternative if deferral is undesirable: make `startRefresh` preserve `pendingRestore`/`pendingRestoreBatch` when the refresh is automatic.

Boundary: `internal/tui/model.go` (canApplyAutoRefresh or startRefresh). Tests: `internal/tui/model_test.go` — auto-refresh tick during a live restore offer leaves the offer and its data intact; manual refresh still clears it. Routed doc: `docs/features/delete-and-restore.md` — clarify "refresh" means manual refresh.

## Acceptance criteria

- An auto-refresh tick landing inside the restore window leaves the offer visible and `u` functional.
- Manual `r` still clears the offer (documented behavior unchanged).
- delete-and-restore.md distinguishes manual vs automatic refresh for offer supersession.
