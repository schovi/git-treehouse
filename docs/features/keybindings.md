# Actions & keybindings

_Behavior spec. Index: [docs/README.md](../README.md) · Code: [docs/architecture.md](../architecture.md)._

| Key | Action |
|---|---|
| `↑`/`↓`, `k`/`j` | Move selection |
| `Enter` | cd to selected worktree and exit (writes `--cd-file`). On the active row: just quit. On a prunable row: disabled (hint shown in status bar). On a branch-only row: create a worktree for that existing branch, then cd there ([create-and-checkout.md](./create-and-checkout.md)). |
| `n` | Create a new worktree from the focused worktree row ([create-and-checkout.md](./create-and-checkout.md)). On branch-only rows, use `Enter` instead. |
| `Delete` / `Backspace` / `d` | Delete flow ([delete-and-restore.md](./delete-and-restore.md)) |
| `o` | Open selected worktree in editor (config → `$EDITOR` fallback); TUI stays open |
| `p` | Open selected row's PR in browser (`gh pr view --web`); no PR → open repo page for the branch |
| `y` | Copy selected worktree's absolute path, or selected branch-only row's branch name, to clipboard; brief `copied` flash in status bar |
| `r` | `git fetch --prune` + stable refresh of all rows |
| `u` | Restore the just-deleted branch ([delete-and-restore.md](./delete-and-restore.md)) |
| `h` | Jump to the root repository worktree |
| `a` | Jump to the active worktree |
| `g` / `G` | Jump to the first / last visible row |
| `s` | Fuzzy branch search |
| `b` | Toggle branch-only rows and persist the setting |
| `Tab` | Open filter picker |
| `Ctrl+P` | Open command palette ([command-palette.md](./command-palette.md)), including the palette-only `Checkout root` and `Checkout PR` flows |
| `Ctrl+O` | Open the config file in the editor (also a palette command) |
| `Esc` | Contextual cancel or clear: close the topmost dialog, leave search mode, then clear an active filter before a committed search. Does not quit. |
| `q` | Quit immediately from list view (no cd) |
| `Ctrl+C` | Quit immediately from any state (no cd) |
| `?` | Toggle a help overlay with the full key list |

No generic multi-select in v1. Most actions apply to the focused row; `Clean up merged` is a dedicated palette-only batch command over safe merged cleanup candidates.
