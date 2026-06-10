# Command palette

_Behavior spec. Index: [docs/README.md](../README.md) · Code: [docs/architecture.md](../architecture.md)._

`Ctrl+P` opens a fuzzy command palette over named actions, an alternative to memorizing keys. It includes the row actions (go, create, checkout, delete, open editor, open PR, copy, copy PR URL), `Checkout PR`, the view toggles, direct jumps to each filter (`filter-all`, `filter-modified`, `filter-branches`, `filter-merged`, `filter-prunable`, `filter-locked`, `filter-detached`), `toggle-help`, and `open-config` (`Ctrl+O`, opens the config file in the editor). Typing filters the list; `Enter` runs the highlighted command; `Esc` closes the palette. Each entry shows its direct keybinding when one exists; copy PR URL and checkout PR are palette-only.
