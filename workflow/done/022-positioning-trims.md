# 022 — Scope trims: disk frame, PR-thread previews, `c` checkout-in-root

done: 2026-08-04
tags: tui, positioning

## What & why

Three features the positioning review flagged as scope drift away from the worktree-lifecycle core. The decisions are recorded below; each accepted change is tracked as its own implementation task.

## Spec (evidence per candidate)

1. **Disk frame — recommend delete.** Built, tested, permanently disabled via compile-time `diskFrameEnabled = false` (`internal/tui/frames.go:34-44`, renderer 619+, tests in frames_test.go). Not user-reachable; internal docs mark it as built but disabled. Pays the ANSI-regression-test maintenance tax on every frame-layout change for zero user value; the shipped per-row size column covers the need. Git history preserves it for reintroduction.
2. **PR review frame thread previews — recommend cap.** Dedicated GraphQL reviewThreads query fetching up to 100 inline threads (`internal/github/review.go:113-119`), rendered in the PR review frame. The roll-up (state/decision/failing checks) serves the jump/cleanup decision; thread CONTENT duplicates `gh pr view` and the browser (`p` is one keypress away) and grows a second GitHub API surface toward PR-client territory. Cap the frame at roll-up + failing checks; drop the threads query.
3. **`c` checkout-in-root — recommend demote to palette-only.** `c` on a branch-only row runs `git switch` in the shared root with a stash-arming flow for dirty roots (v0.7.0, `docs/features/create-and-checkout.md`); Enter on the same row creates an isolated worktree. Two adjacent keys, opposite philosophies; the stash path is the only flow that relocates uncommitted work. Demoting keeps the capability for those who want it without promoting it as a first-class key.

Related but decided differently: the Git context graph frame (adversarially verified as the largest, least differentiated render feature — ~390 lines + 3+ subprocesses per row, duplicating main±/remote columns) is recommended FREEZE, not cut: its header is the only remote-sync display in the detail area since v0.12.0 (`frames.go:356-359`). No task needed for a freeze — it's a "don't invest further" note; the reclaimed-space idea is covered by task 011 (stale hint).

## Acceptance criteria

- A decision (keep / cut / demote) is recorded per candidate; each accepted cut becomes its own small ready task with the evidence above.

## Notes

- 2026-08-04: delete the disabled Disk frame and its enrichment pipeline (`024`).
- 2026-08-04: remove inline PR-thread previews and the GraphQL query; keep the review roll-up, failing checks, and change-request previews (`025`).
- 2026-08-04: move checkout-in-root to the command palette and remove `c` immediately, with no deprecation flash (`026`).
