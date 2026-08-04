# 018 — Carry enrichment results across auto-refresh instead of redoing everything

done: 2026-08-04

depends: 001

tags: gitdata, perf

## What & why

Every 30s auto-refresh redoes ALL enrichment from scratch: per non-main worktree ~6-7 git subprocesses (status, numstat, 4-5 log/merge-base for the context graph), plus full disk re-scans (`git ls-files` + one Lstat per tracked file per worktree, and a full WalkDir of the selected worktree) — for data that almost never changes. A 10-worktree monorepo burns ~60-70 subprocesses and re-walks hundreds of thousands of files every 30 seconds while idle: constant background CPU/disk/battery load.

## Spec

The 30s refresh keeps status and changed-file counts fresh. It reuses only data whose acceptable staleness is explicit:

- For automatic refreshes, seed a worktree graph from the prior state only when its `Path`, its `HEAD`, and the main worktree `HEAD` still match. Loaded graphs skip their `git log` and `git merge-base` calls.
- For automatic refreshes, retain Git-aware size, full size, and disk breakdown only when `Path` and `HEAD` match. There is no TTL, so builds or generated files can stay stale until a manual refresh.
- Manual `r` fetches then bypasses every local enrichment cache. Delete and cleanup reloads remain uncached.

Production boundary: the refresh command and state adoption in `internal/tui`; skeleton/enrichment graph handling in `internal/gitdata/load.go`. Reuse the session-cache pattern in `model_list.go`; do not add a cache layer or persistent storage. Tests belong with the existing fake-Runner reload and disk-loading tests in `internal/tui` and `internal/gitdata`. Update `docs/features/columns-and-data.md` with automatic versus manual refresh freshness.

Task 001 is complete and supplies the required safe copy semantics for passing the prior state into an asynchronous reload.

## Acceptance criteria

- A second automatic refresh with unchanged worktree and main `HEAD`s makes no context-graph subprocess calls for matching worktrees.
- Automatic refresh still reloads working-tree status and changed-file counts.
- Matching automatic rows retain Git-aware size, full size, and disk breakdown without another `git ls-files` call or filesystem walk.
- A changed worktree or main `HEAD` gets fresh graph and size data on the next automatic refresh.
- Manual `r` fetches and reloads graph and size data without using the automatic-refresh cache.
- `docs/features/columns-and-data.md` describes the resulting freshness contract.
