# 011 — Stale-worktree "safe to remove" hint in the detail panel

done: 2026-08-04

tags: tui, ux

## What & why

The tool's core cleanup job lacks its cheapest affordance: a one-line hint on the selected row saying this worktree is finished (clean + merged/PR-closed + upstream gone) and safe to remove. Every input is already computed and already drives the merged filter and cleanup planner — only the surfacing line is missing. Best value-per-effort item on the ideas backlog (docs/ideas.md:14).

## Spec

Inputs all exist (verified by the backlog audit):

- `Status.Clean`, `BranchMergedToMain`, PR merged/closed already drive the merged filter (`internal/tui/model.go:542-550`, `prMergedOrClosed` 562-568).
- Upstream-gone is legended in help (`model.go:3957`); the delete dialog already says "Remote branch already deleted, likely safe." (~`model.go:4367`).

Change: in the Details panel renderer for the selected worktree/branch row, add one hint line when the row satisfies the merged-filter criteria (reuse the exact same predicate — do NOT fork a third copy of merged-ness logic; the merged filter at model.go:542-550 and cleanup planner at 2262-2289 already risk divergence, so factor/reuse one predicate). Suggested copy, aligned with existing dialog tone: `finished: clean, merged to main — safe to remove (d)` with a variant for PR merged/closed and for upstream gone.

Placement decision defaulted: Details box (left column), one line, muted/success styling; no new frame.

Boundary: `internal/tui/model.go` detail rendering (~2733 area) or `internal/tui/frames.go` Details composition; predicate reuse from model.go:542-568. Tests: `internal/tui/model_test.go` — hint shown for a clean+merged row, absent for dirty/unmerged/active rows; styling per docs/harness.md if the line is styled (force color profile). Routed doc: `docs/features/main-view.md` Detail panel section — same change.

## Acceptance criteria

- A clean worktree whose branch is merged to main (or PR merged/closed, or upstream gone + merged) shows a safe-to-remove hint in its detail panel; dirty, unmerged, root, and active rows never do.
- The hint predicate is shared with (not copied from) the merged-filter logic.
- main-view.md documents the hint in the same change.
