# 013 — List feedback: hints under scrollbar, visible committed search, informative empty state

done: 2026-08-04

tags: tui, ux

## What & why

Three related failures of the list's self-explanation, all in the footer/status surface: (1) the scrollbar replaces the Tab-filter/search hints exactly when the list is long enough to need them; (2) a committed search is invisible — rows "go missing" with nothing on screen naming the query; (3) a filtered-empty list says only "No rows" with no cause. One cohesive outcome: the list always tells you why it shows what it shows and how to change it.

## Spec

Evidence (from UX review, not adversarially verified — confirm lines first):

1. `listFooterHintsForScrollbar` (`internal/tui/model.go:3312-3317`): when the scrollbar renders (total > visible, `model.go:2669-2671`), the right footer hints (`model.go:3298-3310`) are wholly replaced by "start/total". Fix: keep the highest-value hints and append the position, e.g. `Tab filter · s search · 12/80`; `joinPartsWithin` (`model.go:3327`) already degrades gracefully when width runs out.
2. Committed search invisible: after Enter (`model.go:1227-1229`) the query survives only as the row-count change in the title (`model.go:3392-3393`); footer reverts to "n new worktree" (`model.go:3291-3296`). Fix: show the active query in the footer or title (e.g. `search: fix ·` prefix), while a committed search filters rows.
3. Empty state: `model.go:2486-2488` renders bare "No rows" when filter/search matches nothing. Fix: name the cause from model state: `No rows match filter: locked (Esc to clear)`, including the search value when one is active.

Also update the undocumented scrollbar/footer behavior in `docs/features/main-view.md` to whatever this task ships (coordinates with task 009 item 11 — whichever lands second documents the final state).

Boundary: `internal/tui/model.go` footer/title/empty-state rendering (3291-3327, 3392, 2486-2488). Tests: `internal/tui/model_test.go` — footer content with scrollbar active, title/footer with committed search, empty-state message per filter and filter+search; narrow-width degradation per docs/harness.md (hint ladders). Routed doc: `docs/features/main-view.md`, `docs/features/navigation-and-filtering.md`.

## Acceptance criteria

- With an overflowing list, at least the filter and search hints remain visible alongside the scroll position.
- An active committed search is visibly named somewhere persistent (footer or title).
- An empty filtered list names the active filter (and search, when set) and how to clear it.
- Narrow widths degrade without overflow (existing hint-ladder tests extended, not weakened).
- main-view.md and navigation-and-filtering.md updated in the same change.
