# Workflow Contract — Git Treehouse

Read by the `workflow` plugin skills (`/workflow:groom`, `/workflow:work`, `/workflow:batch-work`) before acting. This file holds repo-specific facts only; the process lives in the plugin. Keep it short and current.

Rules that govern every agent session, not just carded work (autonomy limits, fix-vs-ask on a failed check, git and worktree safety), belong in the root `AGENTS.md`: that one loads once per session, this one is read per workflow command. Don't restate them here, and don't keep a separate process doc that restates either — a second copy drifts from the first.

## Project

Git Treehouse: a Go Bubble Tea TUI for managing Git worktrees. Module `github.com/schovi/git-treehouse`, binary `git-treehouse` (shell wrapper `gth`). Sources in `cmd/` and `internal/`; behavior spec in `docs/features/` (indexed by `docs/README.md`), code structure in `docs/architecture.md`.

## Validation

```bash
# targeted, during implementation
go test -run TestName ./internal/<pkg>

# smoke checks after TUI/output changes
go run ./cmd/git-treehouse list --no-github
COLUMNS=80 go run ./cmd/git-treehouse list --no-github
go run ./cmd/git-treehouse init zsh

# full gate, before every commit with non-trivial changes — separate steps
make build
make lint
make test
make security
```

Never gate a commit on a piped test run — run each check as its own step and chain the commit on its exit status. Build green is not test green.

## Verify mapping

None — no repo-local verify skills yet.

## Doc routing

Read the doc leaf before editing mapped paths — behavior and invariants live in docs, not code. Use code search to locate, not to learn behavior.

| If you'll edit | Read |
|---|---|
| `internal/**`, `cmd/**` | the matching `docs/features/*.md` leaf (find it via `docs/README.md`) |
| package layout, data flow, renderer seams | `docs/architecture.md` |

- Doc style rules: none
- Decision log: none

## Local notes

- Internal docs (`docs/features/*.md`, `docs/architecture.md`) are updated in the same change as the feature. Never touch `README.md` or other public docs during feature work — those change only at release time (see root `AGENTS.md`).
- Styling/layout changes need ANSI/border regression tests per `docs/harness.md`.
