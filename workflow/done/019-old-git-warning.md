# 019 — Warn when Git < 2.41 disables branch metadata

done: 2026-08-04
tags: gitdata

## What & why

Git 2.41 introduced the `for-each-ref` `%(ahead-behind:...)` atom used for branch metadata. Older Git falls back silently: branch-only rows and main-sync/merged detection are unavailable, while each worktree gets extra subprocesses.

Warn users instead of adding a compatibility implementation.

## Spec

- Detect Git versions below 2.41 once and carry the compatibility result from `internal/gitdata` to the TUI.
- `git-treehouse doctor` reports a warning that Git < 2.41 cannot provide branch-only rows, main sync, or merged-branch detection.
- The TUI shows the same concise warning once per application run, including after automatic refreshes.
- Do not add per-branch `rev-list` compatibility queries or change `detectMainBranch` behavior for repositories without `origin/HEAD`.

Ownership: `internal/gitdata` load state and version handling; `internal/tui` metadata-result handling and existing flash/status renderer; `cmd/git-treehouse` doctor check. Test the Git-version boundary with a fake `Runner`, doctor output, and the TUI's one-time notification. Update `docs/features/edge-cases.md` and `docs/features/cli-commands.md`; verify whether `docs/architecture.md` needs a data-flow note, and skip it with a one-line reason if already aligned.

## Acceptance criteria

- On Git < 2.41, `doctor` warns which branch metadata is unavailable.
- On Git < 2.41, the TUI names that limitation once without repeating it on automatic refreshes.
- On Git 2.41 and newer, neither warning appears.
- The internal docs state that Git 2.41 is required for full branch metadata.
