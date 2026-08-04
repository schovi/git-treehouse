# 023 — List-wide PR fetch: open PRs beyond the newest 200 silently show as "none"

done: 2026-08-04
tags: github

## What & why

On repos with more than 40 local branches, the capped mixed-state PR query can omit an old open PR. Its row then shows no PR, and cleanup dialogs lose the active PR context. Keep the fallback fast while making active PRs visible.

## Spec

- In `internal/github/github.go`, split the list-wide mapping into `--state open` and `--state merged` queries, each with the existing limit of 200 and without `statusCheckRollup`.
- Merge the results by branch, preserving an open PR when the same branch also has merged history. Closed PRs remain excluded; merged history remains capped.
- Keep the >40-branch strategy and its 15-second timeout in `internal/tui/commands.go` unchanged.

Ownership: `internal/github/github.go` list-wide loader, its fake-Runner tests in `internal/github/github_test.go`, and `docs/features/columns-and-data.md`. The 40-branch threshold, per-branch path, CI loading, and PR checkout summaries are excluded.

## Acceptance criteria

- The list-wide PR mapping attaches an open PR older than 200 newer merged PRs to its branch (fake-Runner test).
- List-wide open and merged queries omit `statusCheckRollup`; the CI 504 guard remains.
- An open PR wins over merged history for the same branch.
- `columns-and-data.md` documents the split queries, capped merged history, and excluded closed history.
