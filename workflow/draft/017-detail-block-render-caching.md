# 017 — Stop re-rendering detail blocks for all filtered rows on every frame

tags: tui, perf

## What & why

Every `View()` — each keypress, each ~100ms spinner tick, the minute clock tick — fully renders the Details/Changes/PR-review/Git-context lipgloss frames for EVERY filtered row, twice, just to compute height budgets. With 80+ worktrees (the target audience) navigation lags and spinner animation stutters exactly where the tool should shine.

## Spec (evidence)

From the UX review (not adversarially verified — confirm before designing):

- `viewSnapshot` calls `reservedDetailBlockLines` over the full filtered row list on every View (`internal/tui/model.go:2568`).
- `reservedDetailBlockLines` (`model.go:5100-5106`) calls `detailBlocks` per row — each renders the full frame stack (`model.go:2600-2620`).
- `availableTableHeight` (`model.go:5037-5049`) repeats the same full pass in `visibleTableWindow` (`model.go:5015-5026`).
- Purpose: the detail region reserves the tallest row's height so the list doesn't jump while navigating (actively defended behavior — commit 9083482 "fix: keep list height stable while navigating"). Any fix must preserve that invariant.

Options:

- A) Cache `reservedDetailBlockLines` keyed on (row-set fingerprint, panel width, showPR/enrichment state), invalidate on state/width changes. Few lines, keeps render as source of truth.
- B) Compute block height arithmetically (line counting) instead of rendering. Faster but a second height model that can drift from the renderer — the exact class of duplication docs/harness.md warns about.

Recommendation: A.

Boundary: `internal/tui/model.go` (viewSnapshot, reservedDetailBlockLines, availableTableHeight). Tests: existing height-stability tests must stay green; add a counting-fake or instrumentation test asserting detailBlocks runs O(1) per frame, not O(N).

## Acceptance criteria

- Navigating a 100-row list performs a bounded (not per-row) number of detail-block renders per frame.
- List height stability across navigation preserved (regression tests from commit 9083482 stay green).

## Open questions

- Cache (A) vs arithmetic height (B)? A recommended; confirm.
- Is there a benchmark/threshold to lock in (e.g. a Go benchmark on View with 100 rows) so the regression is measurable?
