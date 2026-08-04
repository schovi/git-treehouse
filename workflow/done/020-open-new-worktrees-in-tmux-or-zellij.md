# 020 — Open new worktrees in tmux or Zellij

done: 2026-08-04
tags: tui, ux

## What & why

Creating a worktree is the fan-out moment for parallel agents and builds. Inside tmux or Zellij, put the new worktree in a named window or tab and keep Treehouse open to create the next one. Outside a multiplexer, retain the current create-and-go flow.

## Spec

- Decide the post-create destination once for every successful creation route: new branch, existing local branch, and PR checkout. It runs only after `git worktree add`, file copies, and an approved `post_create` hook all succeed.
- `$TMUX` set and `$ZELLIJ` unset: start `tmux new-window -c <path> -n <branch>` without a shell. `$ZELLIJ` set and `$TMUX` unset: start `zellij action new-tab --cwd <path> --name <branch>` without a shell. Neither or both set: keep the current create-and-go behavior. Do not guess a graphical terminal or a nested multiplexer target.
- The new-branch and existing-branch modals state the detected result, such as `Enter create + open tmux window`. They retain their current create-and-go hint outside a supported, unambiguous multiplexer.
- After a multiplexer action starts, leave the TUI open, reload the table, and select the new worktree. If starting the action fails, report the error without exiting; the worktree remains created and selectable.
- Use the existing detached external-action pattern in `internal/tui/commands.go`, with argv arguments rather than a command template or shell interpolation. Keep the post-create success/error contract in `internal/tui/model.go` shared across all creation routes.
- Expected ownership: `internal/tui/dialog_create.go`, `internal/tui/dialog_checkout.go`, `internal/tui/model.go`, and `internal/tui/commands.go`. Add focused behavior tests beside the existing create and checkout dialog tests. Update `docs/features/create-and-checkout.md`.
- Exclude configurable commands, a separate open-selected-worktree action, terminal-emulator detection, and nested-multiplexer support.

## Acceptance criteria

- Creating from each supported route inside tmux opens a named tmux window at the new path, keeps Treehouse open, and selects the new worktree after reload.
- Creating from each supported route inside Zellij opens a named Zellij tab at the new path, keeps Treehouse open, and selects the new worktree after reload.
- In neither or both multiplexer environments, all creation routes retain create-and-go behavior.
- The create modals make the detected destination clear before Enter, and an action-start failure is reported without losing the created worktree.
- Focused tests cover detection, argv-safe command construction, all creation result paths, and the modal hint. `docs/features/create-and-checkout.md` matches the final behavior.
