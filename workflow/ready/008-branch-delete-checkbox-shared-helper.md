# 008 — Branch-only delete dialog: render checkbox via the shared helper

priority: 80

tags: tui

model: sonnet

## What & why

The branch-only delete dialog hand-assembles its checkbox line as a raw string while the worktree delete dialog renders through `deleteCheckboxLine` (adversarially verified). This is the exact "Shared UI Blocks" failure mode docs/harness.md exists to prevent: a styling change to the helper updates one sibling dialog and silently leaves the other to drift. No visual bug today — the outputs are currently byte-identical — so this is a one-line guardrail fix.

## Spec

Verified:

- `internal/tui/model.go:4397`: `renderDeleteBranchAtWidth` builds `"[x] " + deleteBranchLabel(branch)` as a raw string — the only raw checkbox string in the file.
- Sibling worktree dialog renders via `renderDeleteToggleBlock` (called `model.go:4370`, helper 4502-4514) → `deleteCheckboxLine` (4516-4526), which returns the identical string for (checked=true, muted=false).
- Existing tests (`model_test.go:2515-2775`, e.g. asserting substrings like "[x] delete local branch" at 2568, 2596) are plain-substring checks that would pass through a style-only drift on both dialogs — nothing guards this.
- Contract: docs/harness.md "Shared UI Blocks" (lines 39-47) mandates sibling modal sections share one renderer, citing a prior real indentation regression.

Fix: replace `model.go:4397` with `deleteCheckboxLine(true, deleteBranchLabel(branch), false)`. Add one helper-level regression test that both dialogs' checkbox lines route through the helper (e.g. assert the branch dialog picks up a helper style change, or assert rendered equality between the two paths for the same inputs).

Boundary: `internal/tui/model.go:4397` + one test in `internal/tui/model_test.go`. No doc change (behavior identical).

## Acceptance criteria

- Branch-only delete dialog checkbox renders through `deleteCheckboxLine`; no raw `"[x] "` assembly remains in model.go.
- A regression test fails if the branch dialog's checkbox line stops matching the shared helper's output.
- Rendered output unchanged (existing dialog tests green).
