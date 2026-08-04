# 012 — Validate target path collisions live in the create dialog

priority: 120

tags: tui, ux

## What & why

The create dialog already shows a live path preview per keystroke but only checks for an existing target path on Enter, so the user finds out about a collision after committing. One `os.Stat` in the existing preview code path turns the failure into inline feedback (docs/ideas.md:22; second-best value-per-effort on the backlog).

## Spec

Existing pieces (verified by the backlog audit):

- Submit-time check "target path already exists" at `internal/tui/model.go:1902` (also 1761, 1663 for the other create entry points).
- Live path preview recomputed per keystroke: `createPathPreview` (`model.go:4049`).

Change: in the same code path that computes the preview, `os.Stat` the derived path and render an inline error style + message when it exists (reuse the same wording as the submit check). Keep the Enter-time check as the authoritative gate (races, and the other entry points at 1761/1663 don't have a preview — verify whether the branch-worktree and PR-checkout dialogs show a path preview; if they do, apply there too, otherwise skip with a one-line reason).

Note: docs/features/create-and-checkout.md currently claims live validation that doesn't exist (branch-name validation runs on Enter, model.go:1894-1896); this task makes the path-collision part of that claim true. Coordinate wording with tasks 005/009.

Boundary: `internal/tui/model.go` (createPathPreview and its render site, ~4049; possibly updateCreate). Tests: `internal/tui/model_test.go` — typing a branch whose derived path exists shows the inline error before Enter; non-colliding path shows none. Routed doc: `docs/features/create-and-checkout.md`.

## Acceptance criteria

- Typing a branch name whose derived worktree path already exists shows an inline collision error in the create dialog before Enter.
- Enter remains blocked on collision with the same message (existing behavior).
- create-and-checkout.md accurately describes what is live vs Enter-time after this change.
