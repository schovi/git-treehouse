# 018 — Carry enrichment results across auto-refresh instead of redoing everything

depends: 001

tags: gitdata, perf

## What & why

Every 30s auto-refresh redoes ALL enrichment from scratch: per non-main worktree ~6-7 git subprocesses (status, numstat, 4-5 log/merge-base for the context graph), plus full disk re-scans (`git ls-files` + one Lstat per tracked file per worktree, and a full WalkDir of the selected worktree) — for data that almost never changes. A 10-worktree monorepo burns ~60-70 subprocesses and re-walks hundreds of thousands of files every 30 seconds while idle: constant background CPU/disk/battery load.

## Spec (evidence)

From the production review (not adversarially verified — confirm before designing):

- `autoRefreshInterval` 30s (`internal/tui/model.go:117`); each reload runs `loadStableState` → full `EnrichLocalMetadata`: per-worktree status + numstat (`internal/gitdata/load.go:359, 401`) + graph calls (`load.go:211-242`), concurrency capped at 4.
- `LoadSkeleton` builds fresh rows, so `GitSizeLoaded`/`FullSizeLoaded` reset every reload → `diskUsageCommand` (`model.go:5517-5567`) re-runs the walks (`load.go:637-657, 709-735`) every 30s.
- Precedent to follow: `applyCachedPullRequests` already preserves PR data across reloads.

Sketch: copy sizes (and context graphs) from the old state into the new one when `Path`+`Head` match, mirroring the PR cache; skip graph re-fetch when both the branch tip and main tip are unchanged. Sizes could also age out on a longer TTL (sizes change without HEAD moving — e.g. builds) — decide staleness policy.

Interacts with task 001 (enrichment copies rows) — land 001 first to avoid churn on the same functions.

Boundary: `internal/gitdata/load.go` (skeleton/enrichment), `internal/tui/model.go` (reload adoption, diskUsageCommand, applyCached* pattern). Tests: fake-Runner tests counting subprocess calls across two reloads with unchanged tips.

## Acceptance criteria

- A second reload with unchanged branch tips issues no graph re-fetch and no disk re-walk for unchanged worktrees (asserted by counting fake-Runner calls / walk invocations).
- Changed rows (new HEAD) still re-enrich fully.
- Manual `r` (fetch + reload) behavior: decide whether it forces full re-enrichment.

## Open questions

- Staleness policy for sizes: keyed on Head only, TTL, or both?
- Should manual `r` bypass the cache entirely (user asked for fresh data) while the 30s tick uses it?
- Are context graphs also keyed on main-branch tip (they compare against main)?
