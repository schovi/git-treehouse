# 020 — Open worktree in tmux/zellij window or terminal tab (configurable open command)

tags: tui, feature

## What & why

cd-on-exit serves one shell at a time; the tool's own pitch (and its real audience — parallel agent sessions, parallel builds) needs fan-out: open the selected worktree in a NEW tmux/zellij window or terminal tab without quitting the TUI. Rated the best big-ticket item on the backlog (docs/ideas.md:53) and the single capability that would make the README's "Tame AI agent worktrees" headline true (positioning review, adversarially verified: today no tmux/zellij/tab support exists anywhere; the only non-serial fan-out is `o` open-in-editor).

## Spec (starting point)

- Nothing exists: no open_command config key (`internal/config/config.go:15-21`), zero tmux/zellij references in code or docs.
- Precedent to mirror: `o` open-in-editor spawns detached without quitting (`internal/tui/model.go:1118-1123`, `openEditorCmd` 5681-5689 uses `exec.Command(...).Start()`).
- Sketch: a user-supplied command template in config (global config.toml, maybe overridable in `.worktree`), e.g. `open_command = "tmux new-window -c {path} -n {branch}"` with `{path}`/`{branch}` tokens (reuse `pathutil.ApplyTemplate` token style). New keybinding (e.g. `O` or `t`) + palette command "Open in terminal"; flash on success/failure; TUI stays open. Keeping it a template keeps per-multiplexer quirks out of the codebase.

Boundary when scoped: `internal/config/config.go`, `internal/tui/model.go` (keybinding, palette entry, spawn cmd), `internal/pathutil` (token expansion), docs/features/keybindings.md + configuration.md + command-palette.md. Follow-up (separate task): README repositioning (021) leans on this shipping.

## Acceptance criteria (draft)

- With `open_command` configured, a keypress on a worktree row spawns the command with tokens expanded, detached, TUI stays open, flash confirms.
- Without config, the key explains how to configure it (matches the app's disabled-actions-explain-themselves pattern).
- Docs updated in same change.

## Open questions

- Keybinding choice (`O`? `t`? something else) — `o` is taken by editor.
- Config shape: single `open_command` template vs named commands (open in tmux AND in editor AND ...)? Recommended: single template, YAGNI.
- Should it work on branch-only rows (create worktree first, then open)? Recommended: no for v1 — worktree rows only.
- Repo-scoped `.worktree` override wanted, or global-only? Recommended: global-only v1.
- Shell-quoting/exec model: run via `sh -c` like hooks, or argv-split? (Hooks precedent: `sh -c`, `internal/gitdata/hooks.go:5-8`.)
