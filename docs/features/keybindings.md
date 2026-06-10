# Actions & keybindings

_Behavior spec. Index: [docs/README.md](../README.md) · Code: [docs/architecture.md](../architecture.md)._

| Key | Action |
|---|---|
| `↑`/`↓`, `k`/`j` | Move selection |
| `Enter` | cd to selected worktree and exit (writes `--cd-file`). On the active row: just quit. On a prunable row: disabled (hint shown in status bar). On a branch-only row: create a worktree for that existing branch, then cd there ([create-and-checkout.md](./create-and-checkout.md)). |
| `n` | Create a new worktree from the focused worktree row ([create-and-checkout.md](./create-and-checkout.md)). On branch-only rows, use `Enter` instead. |
| `c` | On a branch-only row, checkout that branch in the root worktree, then cd into root ([create-and-checkout.md](./create-and-checkout.md)). |
| `Delete` / `Backspace` / `d` | Delete flow ([delete-and-restore.md](./delete-and-restore.md)) |
| `o` | Open selected worktree in editor (config → `$EDITOR` fallback); TUI stays open |
| `p` | Open selected row's PR in browser (`gh pr view --web`); no PR → open repo page for the branch |
| `y` | Copy selected worktree's absolute path, or selected branch-only row's branch name, to clipboard; brief `copied` flash in status bar |
| `r` | `git fetch --prune` + stable refresh of all rows |
| `u` | Restore the just-deleted branch ([delete-and-restore.md](./delete-and-restore.md)) |
| `h` | Jump to the root repository worktree |
| `a` | Jump to the active worktree |
| `s` | Fuzzy branch search |
| `b` | Toggle branch-only rows and persist the setting |
| `Tab` | Open filter picker |
| `Ctrl+P` | Open command palette ([command-palette.md](./command-palette.md)), including the palette-only `Checkout PR` flow |
| `Ctrl+O` | Open the config file in the editor (also a palette command) |
| `Esc` | Contextual cancel or clear: close topmost dialog, clear current search, or clear active filter. Does not quit. |
| `q`, `Ctrl+C` | Quit immediately from list view (no cd) |
| `?` | Toggle a help overlay with the full key list |

No multi-select / bulk operations in v1; every action applies to the focused row.
