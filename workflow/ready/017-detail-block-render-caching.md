# 017 — Stop re-rendering detail blocks for all filtered rows on every frame

priority: 10

tags: tui, perf

## What & why

Every `View()` — each keypress, each ~100ms spinner tick, the minute clock tick — fully renders the Details/Changes/PR-review/Git-context lipgloss frames for EVERY filtered row, twice, just to compute height budgets. With 80+ worktrees (the target audience) navigation lags and spinner animation stutters exactly where the tool should shine.

## Spec

Keep rendered detail blocks as the single height source. Cache only the maximum rendered height for the current visible rows and panel width; render the selected row's blocks fresh on every `View()`.

The cache must recompute whenever a detail-region input changes, including the visible rows, their loaded enrichment/PR-review state, GitHub visibility, or panel width. Reuse it from both `viewSnapshot` and the list-window path used to schedule disk-size work. Do not introduce an arithmetic second height model or cache rendered ANSI blocks.

Ownership: `internal/tui/model_view.go` (snapshot), `internal/tui/model_list.go` (height and list window), and `internal/tui/model.go` only if model-update invalidation is the smallest safe ownership. Likely tests: `internal/tui/model_view_test.go`, retaining the existing selection-height regression. Routed docs: `docs/features/main-view.md` only if the stable-height behavior changes (it should not).

## Acceptance criteria

- Repeated `View()` calls for an unchanged 100-row filtered list render detail blocks only for the selected row after the first height measurement; a focused regression test proves this without an exported seam.
- A visible-row, enrichment/PR-review-state, GitHub-visibility, or width change recomputes the reserved height before the next frame.
- List height stays stable across navigation, including narrow and wide layouts.
- The existing height-stability regression remains green.
