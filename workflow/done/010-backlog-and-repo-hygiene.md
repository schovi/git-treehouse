# 010 — Backlog and repo hygiene: prune shipped ideas, stale binaries, AGENTS.md caveat

done: 2026-08-04

tags: docs

model: sonnet

## What & why

docs/ideas.md carries 5 already-shipped entries and 2 self-refuting ones, so the backlog misleads planning. The repo root carries year-old untracked build artifacts (5.8MB `gwt`, 7.2MB `git-treehouse`, `dist/`) that AGENTS.md has to warn every agent about.

## Spec

ideas.md — remove as shipped (verified in code by the backlog audit):

1. Copy PR URL action (palette command exists: `model.go:497, 1329, 5197`).
2. Check out a PR into a new worktree (full dialog: `model.go:1580-1685`).
3. Merged/done filter (filterMerged + Clean up merged: `model.go:542-550, 446, 508, 4160`).
4. `--repo <path>` flag (global flag: `main.go:82, 95-99, 137-141`).
5. Reflog recovery flash after force delete (shipped `u` restore is strictly better — it executes the recovery: `model.go:3588, 121`; `docs/features/delete-and-restore.md:22`).

Remove or mark as rejected (self-refuting / covered):

6. "Debounce resize renders if needed" — speculative, no evidence of a problem (`model.go:626` handles WindowSizeMsg directly; Bubble Tea coalesces frames).
7. "Make disk usage walks cancellable" — effectively done (all three walks ctx-aware: `load.go:637-735`; cancelled on quit/reload/delete/cleanup: `model.go:1055, 1169, 2319, 2154`); only the moot "resize" clause is unaddressed.

Keep (still open): narrow-mode layout, theme config, asciinema demo, PR title in inspector (only the title remains — review state and CI names shipped in the PR review frame), stale-worktree hint, absolute-path overlay, branch prefixes, base SHA, prefill-on-feature-branch, live path collision, typed-confirm for -D, TUI GitHub opt-out, --sort, bulk selection, tmux/zellij open. Where a board task now exists (011, 012, 014, 020), annotate the idea with the task id or drop it in favor of the task.

Repo artifacts:

8. `rm gwt git-treehouse && rm -rf dist/` (untracked, gitignored — confirm with `git status --ignored` before deleting; these are build outputs, `make build` regenerates `git-treehouse`).
9. Drop the "ignore the stale `gwt` binary in the repo root" sentence from AGENTS.md Project section.

## Acceptance criteria

- ideas.md contains no entry describing shipped or effectively-shipped behavior; removals/keeps match the enumeration above or carry a one-line reason.
- `gwt`, stale `git-treehouse` binary, and `dist/` no longer in the repo root; AGENTS.md caveat sentence removed.
- No public-doc (README) changes.
