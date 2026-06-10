# Git Treehouse — behavior spec

A fast terminal UI for managing git worktrees and local branches: browse, switch, create, and delete worktrees from one keyboard-driven table.

- **Binary name:** `git-treehouse`
- **Smart shell wrapper:** `gth`
- **Stack:** Go + Bubble Tea (with Lip Gloss for styling, Bubbles for inputs/spinners)
- **Scope:** single repo per invocation (the repo containing the cwd), single user, local tool

This is the authoritative behavior spec, split by feature under [`features/`](./features). For code structure and implementation patterns, see [`architecture.md`](./architecture.md).

> **Docs policy:** these internal docs track `main` and are updated alongside the feature that changes them. The public user-facing [README](../README.md) tracks released versions and is updated only at release time. See [`../CLAUDE.md`](../CLAUDE.md) and [`harness.md`](./harness.md).

## Features

| Area | Spec |
|---|---|
| Invocation & subcommands | [features/invocation.md](./features/invocation.md) |
| Shell integration (cd mechanism) | [features/shell-integration.md](./features/shell-integration.md) |
| Main view (table, row icons, detail panel) | [features/main-view.md](./features/main-view.md) |
| Columns & data loading model | [features/columns-and-data.md](./features/columns-and-data.md) |
| Navigation, search, filtering | [features/navigation-and-filtering.md](./features/navigation-and-filtering.md) |
| Command palette | [features/command-palette.md](./features/command-palette.md) |
| Actions & keybindings | [features/keybindings.md](./features/keybindings.md) |
| Create & checkout flows | [features/create-and-checkout.md](./features/create-and-checkout.md) |
| Delete & restore flow | [features/delete-and-restore.md](./features/delete-and-restore.md) |
| CLI commands (`list`, `doctor`) | [features/cli-commands.md](./features/cli-commands.md) |
| Configuration (global, repo `.worktree`, hooks, `allow`) | [features/configuration.md](./features/configuration.md) |
| Edge cases & errors | [features/edge-cases.md](./features/edge-cases.md) |

## Out of scope (v1)

- Multi-select / bulk delete
- Multi-repo dashboard
- Renaming or moving worktrees
