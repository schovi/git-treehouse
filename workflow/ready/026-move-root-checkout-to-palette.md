# 026 — Move root checkout to the command palette

priority: 70
tags: tui, ux

## What & why

Make creating an isolated worktree the only direct branch-row action. Checkout-in-root remains available through the command palette for users who need it.

## Spec

Add a palette-only root-checkout command that reuses the existing checkout flow. Remove the `c` list keybinding and every direct-key reference from branch-row hints, inspector text, help, and behavior docs. Do not add a deprecation flash. Preserve clean-root switching, the dirty-root stash opt-in dialog, and existing error handling; invoking the palette action on a non-branch row keeps the current unavailable-action feedback.

Ownership: TUI palette dispatch, list key handling, detail/help rendering, and checkout dialog; `docs/features/keybindings.md`, `docs/features/command-palette.md`, `docs/features/create-and-checkout.md`, and `docs/features/main-view.md`. Likely tests: palette command matching/dispatch, removed `c` behavior, root checkout, and rendered hints. Excludes changing the checkout command family or stash safety policy.

## Acceptance criteria

- `Checkout root` is a palette-only command that works for a selected branch-only row.
- Pressing `c` in the list no longer starts a root checkout or advertises the action.
- The palette action preserves the existing clean-root switch and dirty-root stash confirmation behavior.
- Internal keybinding, palette, checkout-flow, and main-view docs match the new access path.
