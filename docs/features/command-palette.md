# Command palette

_Behavior spec. Index: [docs/README.md](../README.md) · Code: [docs/architecture.md](../architecture.md)._

`Ctrl+P` opens a fuzzy command palette over named actions, an alternative to memorizing keys. It contains:

- Row actions: `Go to selected row` (`Enter`), `Create worktree` (`n`), `Delete selected row` (`d`), `Open in editor` (`o`), `Open PR or branch page` (`p`), and `Copy path or branch name` (`y`).
- Palette-only actions: `Checkout root`, `Checkout PR`, `Copy PR URL`, and `Clean up merged`.
- Navigation and view actions: `Fetch and reload` (`r`), `Search branches` (`s`), jumps to root/active/top/bottom (`h`/`a`/`g`/`G`), `Open filter picker` (`Tab`), `Toggle help` (`?`), `Open config` (`Ctrl+O`), and `Quit` (`q`).
- Direct filters: all, modified, merged, prunable, locked, and detached.

Typing filters the list; `Enter` runs the highlighted command; `Esc` closes the palette. Each entry shows its direct keybinding when one exists.
