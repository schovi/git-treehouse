# Configuration

_Behavior spec. Index: [docs/README.md](../README.md) · Code: [docs/architecture.md](../architecture.md)._

Configuration has two layers. Everything works with zero config.

1. Global user config: `~/.config/git-treehouse/config.toml`.
2. Repo-scoped config: `.worktree` at the repository root.

Repo-scoped settings apply to that repository and all of its worktrees. A repo `.worktree` `path_template` overrides the global `path_template`; when it is absent, Git Treehouse falls back to global config, then the built-in default.

## Global `config.toml`

```toml
editor = "cursor"          # default: $EDITOR, else `code`

# default below; tokens: {repo}, {repo_name}, {repo_parent}, {branch} (sanitized)
path_template = "{repo_parent}/.worktrees/{repo_name}/{branch}"

main_branch = ""           # default: auto-detect (origin/HEAD, fallback main/master)
show_branches = false      # default: hide branch-only rows until `b` is pressed
github = true              # set false to disable TUI GitHub PR lookup

skip_shell_integration_welcome = false  # set true by onboarding once the gth wrapper is installed; suppresses the first-run welcome
```

`--no-github` overrides `github` for one invocation. Pass `--no-github=false` to temporarily enable GitHub when the config disables it.

Tokens: `{repo}` is the absolute root repository path, `{repo_name}` its basename, `{repo_parent}` its parent directory, `{branch}` the sanitized branch name (slashes, backslashes, and whitespace runs collapse to single dashes). The legacy default `{repo_parent}/{branch}` is silently upgraded to the current default on load.

## Repo `.worktree`

`.worktree` is a TOML file at the repository root:

```toml
# Overrides global path_template for this repo only.
path_template = "{repo_parent}/.worktrees/{repo_name}/{branch}"

# Named repo-relative files copied from the root repository into each new worktree.
copy_untracked = [".env", ".env.local"]

# Lifecycle hooks. See Hook approval and execution for approval and execution rules.
post_create = "npm install"
before_delete = "docker compose down"
```

Recognized keys:

| Key | Type | Behavior |
|---|---|---|
| `path_template` | string | Overrides global `path_template` for new worktrees in this repo. Uses the same tokens as global config. |
| `copy_untracked` | array of strings | Copies named repo-relative regular files from the root repository worktree into the new worktree after `git worktree add` succeeds. Missing files are skipped. Absolute paths, empty paths, paths that escape the repository, and directories are skipped with warnings. This is intentionally a named-file copy, not a copy of arbitrary dirty state. |
| `post_create` | string | Optional shell command run after a new worktree is created and `copy_untracked` files are copied. |
| `before_delete` | string | Optional shell command run before a real worktree removal when enabled in the delete dialog. |

## Hook approval and execution

Hooks are executable commands, so they require explicit local approval:

- Run `git-treehouse allow [--repo <path>]` to approve the current `post_create` and `before_delete` hook strings in `.worktree`.
- Approval is stored in the repo-local Git config as `treehouse.approvedHash`. The hash covers only hook fields, so changing `path_template` or `copy_untracked` does not invalidate approval.
- If hooks are absent, no approval is needed. If hooks are present but not approved, or if they changed since approval, they are skipped gracefully. Worktree creation and deletion continue without running the hook, and the UI reports that approval is needed where applicable.
- `git-treehouse doctor [--repo <path>]` reports recognized `.worktree` keys and whether hooks are approved, missing approval, or changed since approval.

In v1, hooks always run through POSIX `sh -c <command>` in the target worktree directory. They do not use the user's login shell, and Fish, Nushell, and PowerShell hook execution is out of scope.

Hook environment:

| Variable | Value |
|---|---|
| `GTH_EVENT` | `post_create` or `before_delete` |
| `GTH_WORKTREE_PATH` | New worktree path for `post_create`; selected worktree path for `before_delete` |
| `GTH_WORKTREE_BRANCH` | Branch name for the worktree |
| `GTH_REPO_ROOT` | Root repository path |
| `GTH_MAIN_BRANCH` | Detected or configured main branch |
