# 003 — Refresh fetch: credential-prompt guards and timeout

done: 2026-08-04

tags: gitdata, tui

## What & why

`r` (fetch + reload) can hang forever on a credential prompt and permanently disable all refreshing for the session. git/ssh prompts read /dev/tty, which the TUI owns in raw mode, so the user can never answer; `refreshInFlight` never clears, and both manual and auto refresh refuse to run for the rest of the session while the spinner runs forever.

## Spec

Evidence (from production review, not adversarially verified — confirm exact lines first):

- `reloadCmd` uses `context.Background()` with no timeout and runs `git fetch --prune` (`internal/tui/model.go:5635-5650`, `internal/gitdata/load.go:632-635`).
- The runner sets no `GIT_TERMINAL_PROMPT=0` / `GIT_ASKPASS` / ssh BatchMode (`internal/gitdata/runner.go:42-53`).
- `refreshInFlight` is only cleared by a matching `reloadMsg` (`model.go:760-761`) or by starting a delete/cleanup; `startRefresh` (`model.go:1166`) and `canAutoRefresh` (`model.go:1192-1195`) both refuse while it is true.

Fix:

1. For background/non-interactive git operations, set `GIT_TERMINAL_PROMPT=0` and `GIT_SSH_COMMAND=ssh -oBatchMode=yes` (respect an existing user `GIT_SSH_COMMAND` — decide: only set when unset). Decide whether to scope this to fetch or apply to all Runner calls; fetch is the only network+credential path today.
2. Give `reloadCmd` a timeout (~2 min) so `refreshInFlight` always resolves; on failure show an error flash naming the fetch failure instead of a stuck spinner.

Boundary: `internal/gitdata/runner.go` (env injection point), `internal/gitdata/load.go` (FetchPrune), `internal/tui/model.go` (reloadCmd). Tests: fake-Runner tests in `internal/gitdata` asserting env vars present on fetch; a model test that a reload error clears `refreshInFlight` and flashes. Routed doc: `docs/features/edge-cases.md` or `columns-and-data.md` (refresh semantics) — update whichever documents `r`.

## Acceptance criteria

- A fetch against an auth-prompting remote fails fast with a visible error flash instead of hanging; subsequent `r` and auto-refresh work again in the same session.
- Background git fetch runs with `GIT_TERMINAL_PROMPT=0` and ssh BatchMode (asserted via fake Runner).
- `refreshInFlight` provably always resolves: no code path leaves it true after reloadCmd returns/errors/times out.
- Routed feature doc updated in the same change.
