# Auxiliary Frames Experiment

Inspiration: `docs/wip/detail`. That work fills the empty right side of the Details
panel with selected-row context. This experiment asks the next question:

If the Details panel answers "what should I know before I act on this worktree?",
what extra **frames** can sit next to and under it to answer the follow-up questions
in more depth, without crowding the identity column?

This is exploratory. None of it is committed. The point is to sketch shapes and
decide which ones earn their screen space.

## The four frames

1. **Git context tree** — the selected commit placed in its local graph: parent
   contributors, the integration base, and whether the base moved ahead. Answers
   "where does this branch sit in history, and is it stale?"
2. **PR review** — CI checks, unresolved review threads, and change requests for the
   branch's pull request. Answers "what blocks this PR from merging?"
3. **Changes** — a rich `git status`: added, deleted, modified, untracked files with
   per-file `+/-`. Answers "what local work exists, file by file?"
4. **Disk breakdown** — where the worktree's bytes go, as a small bar chart. Answers
   "is this worth cleaning up, and what is eating the space?"

The first three map directly to the user's four decisions from
`detail-information-value.md` (right target / risk / readiness / next action). The
fourth is the cleanup-cost decision, which the Details panel only summarizes.

## How they tile

The frames live around the Details panel, not inside it. Details keeps its stable
identity column. Frames are optional, individually toggled, and state-driven so an
irrelevant frame never steals space.

```
+-- worktree list --------------------+-- Details ------------------+
|                                      | Identity      Context       |
|  (table of worktrees)                | Branch  ...   PR #1 open    |
|                                      | HEAD    ...   Checks 4/5    |
|                                      | ...                         |
+--------------------------------------+-----------------------------+
                                       +-- Git context --+-- PR ------+
                                       | base o          | CI  4 ok   |
                                       |  \              | ✗ lint     |
                                       |   o HEAD        | 2 threads  |
                                       +-----------------+------------+
                                       +-- Changes ------+-- Disk ----+
                                       | M model.go +12  | deps 41M ▓▓ |
                                       | A login.txt +30 | .git 18M ▓  |
                                       +-----------------+------------+
```

"Next to" = right of Details on wide terminals. "Under" = stacked below Details when
width is tight but height is available.

## Responsive behavior

Reuse the Details panel's width bands, measured on the frame area (not the whole
terminal):

- **Wide, 150+ cols**: Details on the left; a 2x2 grid of frames on the right.
- **Standard, 104-149 cols**: Details on the left; one frame column on the right,
  stacked in priority order. Other frames move under Details if height allows.
- **Compact, 72-103 cols**: Details only. Frames become a paged overlay reachable by
  key (one frame at a time, full width).
- **Tiny, <72 cols**: no frames. Details stays minimal as today.

Frames never nest boxes inside Details. Each frame is its own bordered block with a
plain title, matching the existing `listview` renderer rather than a new style path
(see AGENTS.md: similar UI states share one renderer).

## Frame priority

When space allows only some frames, pick by the same state logic the Details context
slot uses, so the panel and the frames agree:

1. Dirty row -> **Changes** wins the first slot.
2. Row with an open PR and clean tree -> **PR review** wins.
3. Clean branch, behind main -> **Git context tree** wins.
4. Large worktree (size over a notable threshold) -> **Disk breakdown** wins.

Frames below the first slot fill in the remaining priority order.

## Data availability

Grounded in `detail-information-value.md` and the current `gitdata` model.

| Frame            | Has now                              | Needs new data |
|------------------|--------------------------------------|----------------|
| Git context tree | HEAD, main ahead/behind, upstream    | parent commit subjects, base-moved-ahead commits |
| PR review        | aggregate PR CI state                | per-check status, review threads, change requests |
| Changes          | staged/modified/untracked **counts** | per-file paths and `+/-` line stats |
| Disk breakdown   | total worktree size                  | per-directory size walk |

All four need at least one new data source. None of them block on a redesign of the
Details panel — they are additive. The cheapest to ship first is **Changes** (a single
`git status --porcelain` + `git diff --numstat` already in scope for the dirty-file
preview), then **Git context tree** (local-only `git log`/`rev-list`), then **Disk
breakdown** (an existing disk walk, just bucketed), with **PR review** last because it
needs the most GitHub API surface.

## What each sketch file contains

- `frame-git-context-tree.txt` — graph layouts, base-ahead state, detached state.
- `frame-pr-review.txt` — checks list, review threads, change-request states.
- `frame-changes.txt` — file list with stats, wide and compact, empty/clean state.
- `frame-disk-breakdown.txt` — bar chart, loading state, threshold behavior.
</content>
</invoke>
