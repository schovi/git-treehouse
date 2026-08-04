# 001 — Fix startup enrichment data race on shared Worktree slice

priority: 10

tags: gitdata, tui

## What & why

`EnrichLocalMetadata` mutates the same `Worktree` backing array the Bubble Tea event loop renders, on every launch. It is a genuine Go memory-model violation (adversarially verified): torn struct copies are possible, and a cancelled enrichment goroutine keeps writing the live array after a refresh. User-visible impact is mostly masked by the loading gate, but `go test -race`-style instrumentation flags it and a rare crash from a torn slice header is possible.

## Spec

Verified mechanism:

- `cmd/git-treehouse/main.go:952-956` passes the `LoadSkeleton` state into `tui.New`; stored un-copied (`model.go:570-593`).
- `enrichmentCommands` captures `state := model.state` at `internal/tui/model.go:5228` — slice header copy, shared backing array — and runs `gitdata.EnrichLocalMetadata` in a tea.Cmd goroutine (`model.go:5230-5232`).
- Concurrent writes: `&state.Rows[index]` in-place writes (`internal/gitdata/load.go:116-117, 124-139`), `sortWorktrees(state.Rows)` swaps whole structs (`load.go:119, 145`), `enrichStatusCounts` writes `rows[result.index]` (`load.go:373-382`), `enrichContextGraphs` goroutines write `row.Graph` (`load.go:176-185`).
- Concurrent reads every frame: `View` → `totalRowCount` (`model.go:2462, 2551-2553`) → `State.TableRows` → `RowsFromWorktrees` ranging `state.Rows` (`internal/gitdata/types.go:55-64`); `localMetadataReady` ranges rows (`model.go:2542-2548`).
- Exposure: the Init path only. Refresh/delete/cleanup build fresh state inside their own goroutine via `loadStableState` (`model.go:5643-5657`, `load.go:31-49`), so no race there. Second exposure: a refresh cancels the enrichment ctx (`model.go:5214-5222`) but cannot stop in-flight writes; the id guard at `model.go:631` discards the message, not the mutations.

Fix: make `EnrichLocalMetadata` operate on a copy — `rows := append([]gitdata.Worktree(nil), state.Rows...)` (copy before `sortWorktrees` too) — and return the copy in State. The UI already adopts it atomically via `localMetadataLoadedMsg` (`model.go:630-637`), so no new synchronization is needed.

Boundary: `internal/gitdata/load.go` (EnrichLocalMetadata and helpers), possibly `internal/tui/model.go:5227-5233`. Tests: `internal/gitdata/load_test.go`. Routed doc: `docs/architecture.md` (async typed-message pattern) — verify whether it needs a line about copy semantics; skip with a one-line reason if already aligned.

## Acceptance criteria

- A test that runs EnrichLocalMetadata concurrently with reads of the input state passes under `go test -race ./internal/gitdata`.
- The full suite passes under `-race` (`make test` already uses `-race`).
- Enriched/sorted rows still land atomically via `localMetadataLoadedMsg`; selection anchoring behavior unchanged (existing model tests stay green).
- No in-place mutation of the caller's `State.Rows` remains in EnrichLocalMetadata or its spawned goroutines.
