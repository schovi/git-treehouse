# 007 — Bare-repo setups misidentify the main worktree

priority: 70

tags: gitdata

## What & why

In the standard bare-repo + worktrees layout, the "main" worktree crown (sorted first, deletion-blocked, MainWorktree for main± comparisons) lands on the first worktree that is NOT the one you invoked from — so it flips depending on your cwd. Run gth inside worktree A and B gets protected; run it inside B and A does.

## Spec

Evidence (from production review, not adversarially verified — confirm the logic before fixing):

- `internal/gitdata/load.go:86-94`: `if index == 0 || repoRoot == activeRoot && !samePath(row.Path, activeRoot)`. In a bare repo, `git worktree list` puts the bare entry at index 0, so `index == 0` never matches a real worktree; the fallback clause explicitly excludes the active worktree via `!samePath(row.Path, activeRoot)`.
- The chosen repoRoot becomes `MainWorktree` (`load.go:104`) and `IsMain` (`load.go:45`), which sorts first (`load.go:772-781`) and blocks deletion (`internal/tui/model.go:1971-1973`).
- Existing coverage: `load_test.go:116-133` only tests bare invocation from the bare dir with one worktree.

Fix: in the loop, the first non-bare row wins unconditionally — drop the `!samePath(...)` exclusion (verify the non-bare case at `index == 0` keeps its behavior: in a normal repo the main worktree is index 0 and still wins).

Boundary: `internal/gitdata/load.go` (main-worktree selection). Tests: `internal/gitdata/load_test.go` — bare repo + two worktrees, invoked from each worktree, asserting the same stable main designation both times; keep the existing bare single-worktree test green. Routed doc: `docs/features/edge-cases.md` (bare repo behavior) — verify whether it documents main selection; update if it does.

## Acceptance criteria

- Bare repo with worktrees A and B: the main/root designation is identical whether gth is invoked from A or from B.
- Normal (non-bare) repos keep their current main selection (existing tests green).
- New regression test covers bare + two worktrees from both invocation points.
