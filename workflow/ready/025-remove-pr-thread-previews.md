# 025 — Remove inline PR-thread previews

priority: 60
tags: github, tui

## What & why

Keep the PR review frame focused on merge readiness. Inline review-thread content duplicates the browser and adds a second GitHub API surface.

## Spec

Remove the `reviewThreads` GraphQL request, thread parsing/data types, and inline-thread count and preview rendering. Preserve lazy `gh pr view` review loading, the state/review/check roll-ups, individual failing or running checks, and change-request previews. Update the Main view and architecture docs to match.

Ownership: `internal/github` PR-review loading and pure parsing; TUI PR-review renderer; `docs/features/main-view.md` and `docs/architecture.md`. Likely tests: PR-review parser, request behavior, and frame rendering. Excludes changing the GitHub opt-out, PR list loading, browser shortcut, or retained review/check summaries.

## Acceptance criteria

- A selected PR's review detail never requests GraphQL inline review threads.
- The PR review frame retains its state, review and check summaries, failing/running check links, and change-request previews, without inline comment counts or bodies.
- Internal behavior and architecture docs describe the reduced review detail accurately.
