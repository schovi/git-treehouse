# 005 — Create failure is silently swallowed after dialog close; no in-flight guard

priority: 50

tags: tui, ux

## What & why

Close the create dialog while `git worktree add` runs and a subsequent failure vanishes without a trace — no dialog, no flash. The user believes the worktree exists. The create dialog also lacks the in-flight guard its sibling delete dialog has, so Enter can start a second create while one runs.

## Spec

Evidence (from UX review, not adversarially verified — confirm exact lines first):

- `createMsg` handler `internal/tui/model.go:784-797`: on error with `createDialog == nil` it returns `model, nil` — no flash fallback. Contrast `checkoutMsg` at `model.go:822-823`, which falls back to `setFlash`.
- `updateCreate` (`model.go:1880-1919`) has no in-flight guard (unlike `updateDelete`'s `deleteInFlight` check at 2007): Esc closes the dialog while create runs (loading="creating…"), Enter can start a second create.

Fix:

1. In `createMsg`'s error branch, mirror checkoutMsg: fall back to `setFlash(errorText)` when the dialog is gone.
2. Add an in-flight guard at the top of `updateCreate`: ignore Enter while a create runs. Decide Esc semantics: either close-view-only (error still lands as flash — consistent with fix 1) or block like updateDelete. Recommended: allow Esc to close the view, since fix 1 guarantees the error still surfaces; note whatever is chosen in the feature doc.

Boundary: `internal/tui/model.go` (createMsg handler, updateCreate). Tests: `internal/tui/model_test.go` — error after dialog close produces a flash; double-Enter starts one create. Routed doc: `docs/features/create-and-checkout.md` — while touching it, also fix the stale "validated live" claim (validation runs on Enter only: `model.go:1894-1896`, `1921-1926`; every other keystroke clears the error) unless task 009 already landed it; skip with a one-line reason if already aligned.

## Acceptance criteria

- A create that fails after the dialog was closed shows the error as a status flash.
- Enter during an in-flight create does not start a second create.
- Chosen Esc-during-create semantics implemented and documented in create-and-checkout.md.
- Existing create dialog tests stay green.
