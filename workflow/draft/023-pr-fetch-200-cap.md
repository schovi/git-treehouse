# 023 — List-wide PR fetch: open PRs beyond the newest 200 silently show as "none"

tags: github

## What & why

On repos with >40 local branches, PR data comes from one `gh pr list --limit 200 --state all` (newest first). Open+closed combined, so a months-old open PR on a long-lived branch falls outside the newest 200: its row shows no PR, and delete/cleanup dialogs display misleading PR context on exactly the rows where it matters.

## Spec (evidence)

From the production review (not adversarially verified — confirm before choosing):

- `internal/github/github.go:88`: `gh pr list --limit 200 --state all`.
- The accurate per-branch path (`gh pr list --head`, 8 bounded workers, 15s timeouts) is used only at <=40 local branches (`internal/tui/model.go:124` prPerBranchThreshold, `5282`).
- Context: the list-wide query deliberately omits statusCheckRollup because it 504s on large repos (documented in columns-and-data.md) — any fix must keep that property.

Options:

- A) Split the list-wide query: `--state open` (usually well under 200) + a second capped `--state merged` query, merged client-side. Open PRs become complete; merged history stays capped (acceptable — merged/closed matters mostly for recent cleanup).
- B) Raise `prPerBranchThreshold` — the per-branch path fans out with bounded workers anyway; measure where it actually breaks.
- C) Both.

Boundary: `internal/github/github.go` (LoadPullRequests), `internal/tui/model.go` (threshold/strategy), tests in `internal/github/github_test.go`. Routed doc: `docs/features/columns-and-data.md` (GitHub data section documents the strategy and the 40 threshold — update).

## Acceptance criteria (draft)

- An open PR older than the newest 200 PRs is attached to its branch row on a >40-branch repo (fake-Runner test).
- No statusCheckRollup on list-wide queries (the 504 guard stays).
- columns-and-data.md describes the final strategy.

## Open questions

- A, B, or C? A recommended (deterministic completeness for open PRs, no new API risk).
- Is the closed/merged cap acceptable for the merged filter and cleanup context, or do those need completeness too?
