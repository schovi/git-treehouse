# 004 — Advertised keys that do nothing: global Ctrl+C, list-view Ctrl+O

done: 2026-08-04

tags: tui, ux

## What & why

Two advertised keybindings silently no-op (both adversarially verified). Ctrl+C — the universal bail-out — is dead in every dialog and during search. Ctrl+O is listed in keybindings.md and rendered as the palette shortcut for "Open config", but only works inside the create dialog.

## Spec

Ctrl+C (verified):

- The only handler is `case "ctrl+c", "q"` in `updateList` (`internal/tui/model.go:1054`); repo-wide grep confirms no other occurrence and no `tea.KeyCtrlC` usage.
- KeyMsg dispatch at `model.go:1018-1046` routes to dialog updaters first whenever any of the 8 dialog pointers is non-nil or `model.searching` is true. None of the nine updaters handles ctrl+c: updatePalette (1251), updateFilterDialog (1398), updateSearch (1220), updatePullRequestCheckout (1505), updateBranchWorktree (1753), updateCheckout (1809), updateCreate (1880), updateDelete (2006), updateCleanupMerged (2130). Text-input dialogs swallow it via `textinput.Update` (bubbles v1.0.0, no ctrl+c binding); the rest hit `return model, nil`.
- Bubble Tea v1.3.10 delivers ^C as a plain KeyMsg in raw mode (no auto-quit); `main.go:956` uses no WithFilter/signal options.
- Fix: handle ctrl+c once, before the dialog dispatch at `model.go:1018`. Decide the semantic and document it: recommended = quit immediately (matches terminal convention); alternative = close topmost dialog first, quit on second press. Coordinate with task 002: during in-flight delete/cleanup, ctrl+c should cancel the operation (or quit), never be swallowed.

Ctrl+O (verified):

- `docs/features/keybindings.md:23` lists Ctrl+O in the list-view key table; `docs/features/command-palette.md:5` calls it the direct binding; palette entry `model.go:512` renders `shortcut: "ctrl+o"` (label rendered at 4603-4604).
- `updateList` (`model.go:1052-1163`) has no ctrl+o case; the sole handler is in updateCreate (`model.go:1892-1893`); only test covers that path (`model_test.go:2444-2453`). Palette execution via Enter works (`model.go:1362-1363`).
- Fix: add `case "ctrl+o": return model, openConfigCmd(model.config.Editor, model.config)` to updateList (mirror `model.go:1363`).

Boundary: `internal/tui/model.go` (Update dispatch, updateList). Tests: `internal/tui/model_test.go` — ctrl+c from each dialog kind quits (or documented two-step), ctrl+o from list view opens config. Routed docs: `docs/features/keybindings.md` (also fix the Esc order wording there while touching it — code clears filter before committed search, doc says the reverse, per model.go:1059-1076 vs 1227-1229; and add missing g/G entries) and `docs/features/command-palette.md` if wording changes.

## Acceptance criteria

- Ctrl+C exits (or follows the chosen documented two-step) from: list, search, palette, filter picker, create, checkout, branch-worktree, delete, cleanup, PR-checkout dialogs.
- Ctrl+O in list view opens the config in the editor; palette shortcut label matches reality.
- keybindings.md matches code for ctrl+c, ctrl+o, Esc clear order, and includes g/G.
- Existing dialog tests stay green.
