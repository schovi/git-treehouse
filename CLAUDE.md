# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Git Treehouse: a Bubble Tea TUI for managing Git worktrees. Module is `github.com/schovi/git-treehouse`, binary is `git-treehouse` (shell wrapper function: `gth`). The directory name `git-worktree-tui` is the old project name; ignore the stale `gwt` binary in the repo root.

`spec.md` is the authoritative behavior spec. `docs/ideas.md` is an aspirational backlog, not commitments.

## Commands

```sh
make build          # go build -o git-treehouse ./cmd/git-treehouse
make test           # go test -v -race ./...
make lint           # golangci-lint run
make security       # gosec (with exclusions) + govulncheck — CI runs this too
go test -run TestName ./internal/gitdata    # single test
```

Smoke checks after changes:

```sh
go run ./cmd/git-treehouse list --no-github
COLUMNS=80 go run ./cmd/git-treehouse list --no-github
go run ./cmd/git-treehouse init zsh
```

Releases: GoReleaser on `v*` tags builds `git-treehouse` + `gth` binaries and publishes a Homebrew cask to `schovi/homebrew-tap`.

## Architecture

### The cd-on-exit mechanism (central design decision)

A child process cannot change the parent shell's cwd. The whole I/O layout follows from this:

- The TUI renders to **stderr** (`tea.WithOutput(os.Stderr)`), keeping stdout clean so `cd "$(git-treehouse)"` works.
- With `--cd-file <path>`, the selected worktree path is written to that file. The generated `gth` shell function (see `internal/shellinit`) passes a `mktemp` file, then `cd`s into its contents after exit.
- The wrapper sets `GTH_SHELL_INTEGRATION=1`; its presence suppresses the first-run onboarding screen.

Preserve this stdout/stderr split in any new output paths.

### Data flow

```
cmd/git-treehouse/main.go     CLI dispatch: `init` → shellinit, `list` → listview, default → TUI
internal/tui/model.go         Bubble Tea model; composes everything below
internal/gitdata              Loads + parses git state (worktree list --porcelain, status, sync)
internal/github               PR/CI status by shelling out to `gh` CLI (not the API)
internal/listview             Pure table renderer, shared by the TUI and `list` subcommand
internal/pathutil             Branch sanitizing + worktree path templating
internal/config               TOML at ~/.config/git-treehouse/config.toml, live-reloaded by mtime
internal/onboarding           Separate Bubble Tea program for first-run shell setup
internal/shellinit            Generates the gth wrapper for zsh/bash/fish/sh/ksh/nushell/powershell
```

### Key patterns

- **`gitdata.Runner` is the single subprocess seam**: an interface (`runner.go`) wrapping `exec.CommandContext`. All git and `gh` calls go through it; tests inject fakes, so nothing in the data layer needs a real repo. New subprocess calls must use it.
- **Parsing is pure**: `gitdata/parse.go` is string-in/struct-out with no exec. Keep new parsers there and test them directly.
- **Async via typed messages**: TUI side effects are `tea.Cmd` closures returning typed msgs (`prLoadedMsg`, `reloadMsg`, `autoRefreshMsg`, ...). A monotonic `refreshID` discards stale reloads, and `selectionAnchor` re-anchors the cursor across reloads. Follow this for any new async work.
- **Enrichment is layered**: initial git load is synchronous; PR data and disk usage load afterwards as background commands so the table appears fast. `gh` being missing/unauthenticated must stay silent (columns hidden, no errors).
- TUI modes (filter, help, create dialog, delete dialog) are fields on the model; `Update` routes keys by active mode.
