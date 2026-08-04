# 015 — `git-treehouse allow` requires interactive confirmation

done: 2026-08-04

tags: safety

## What & why

`allow` prints the hook commands and approves them in the same run with no y/N gate. A user told "run git-treehouse allow" by the TUI flash can approve a malicious `post_create` from an untrusted clone reflexively — the hook text scrolls past and the next worktree creation executes it. The approval mechanism itself (hash in repo-local config, fail-closed) is sound; only the consent step is missing.

## Spec

Evidence (from production review, not adversarially verified — confirm lines first):

- `allowRepoHooks` prints hooks and immediately writes the approved hash (`cmd/git-treehouse/main.go:638-666`).
- TUI guidance is the flash "run git-treehouse allow" (`internal/tui/model.go:2000-2002`).

Change:

1. After printing the hook commands, prompt `Approve these hooks? [y/N]` reading stdin; anything but y/Y exits non-zero without writing the hash.
2. Add `--yes` to skip the prompt for scripts. When stdin is not a TTY and `--yes` is absent, refuse with a message naming the flag (fail closed, never hang a pipeline).
3. Keep output format otherwise stable; update usage/help text.

Boundary: `cmd/git-treehouse/main.go` (runAllow/allowRepoHooks, usage text). Tests: `cmd/git-treehouse/main_test.go` — approve on y, refuse on n/empty, `--yes` bypass, non-TTY without `--yes` refuses. Routed doc: `docs/features/configuration.md` (allow section) and `docs/features/cli-commands.md` if allow is listed there.

## Acceptance criteria

- `allow` without `--yes` never writes `treehouse.approvedHash` unless the user answers y.
- `allow --yes` behaves as today (non-interactive approve).
- Non-TTY stdin without `--yes` exits non-zero with a clear message.
- Feature docs updated in the same change.
