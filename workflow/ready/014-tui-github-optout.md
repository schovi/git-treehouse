# 014 — GitHub opt-out for the TUI (--no-github)

priority: 140

tags: github, tui

## What & why

`--no-github` exists only on the `list` subcommand; the TUI always attempts gh enrichment. Users with slow `gh` auth or who simply don't want PR data have no off switch (docs/ideas.md:32). The enrichment machinery is already cancellable and PR display already gates on state, so this is flag plumbing, not new machinery.

## Spec

Existing pieces (verified by the backlog audit):

- `--no-github` on list: `cmd/git-treehouse/main.go:268` area.
- GitHub lookup runs under the enrichment context, cancelled on quit/reload (`internal/tui/model.go:575, 1055, 1169`); PR display gated on a configured remote (`showPR`/`prLoading`, `model.go:587-588`).

Change (defaulted decisions — override by re-grooming if wrong):

1. Accept `--no-github` on the default (TUI) invocation, parsed alongside the global flags in main.go; thread a bool into `tui.New`.
2. When set: skip PR fetch commands entirely and hide the PR column + PR review frame, exactly like the gh-missing/silent path already behaves (architecture.md: "gh being missing/unauthenticated must stay silent").
3. Also add a global config key `github = false` in config.toml for a persistent opt-out (flag wins over config). Verify how Config plumbs to the model (`internal/config/config.go:15-21`) and follow the existing pattern (e.g. `show_branches`).
4. Update the `gth` wrapper docs/usage text if flags are listed there; `doctor` should still report gh status regardless.

Boundary: `cmd/git-treehouse/main.go` (flag parse + usage text), `internal/tui/model.go` (New signature or options, PR-fetch gating), `internal/config/config.go`. Tests: main_test.go flag parsing; model test that no PR commands are issued when disabled. Routed docs: `docs/features/invocation.md`, `docs/features/columns-and-data.md`, `docs/features/configuration.md`.

## Acceptance criteria

- `git-treehouse --no-github` (TUI) issues zero gh subprocess calls (assert via fake Runner) and shows no PR column/frame.
- Config `github = false` has the same effect; the CLI flag overrides config in both directions.
- `list --no-github` behavior unchanged.
- Routed feature docs updated in the same change.
