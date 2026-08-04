# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Git Treehouse: a Bubble Tea TUI for managing Git worktrees. Module is `github.com/schovi/git-treehouse`, binary is `git-treehouse` (shell wrapper function: `gth`). The directory name `git-worktree-tui` is the old project name; ignore the stale `gwt` binary in the repo root.

`docs/README.md` indexes the authoritative behavior spec, split by feature under `docs/features/`. `docs/architecture.md` documents code structure and implementation patterns. `docs/ideas.md` is an aspirational backlog, not commitments.

## Documentation policy

Internal docs track `main`; public docs track releases.

- When you add or change a feature, update its `docs/features/*.md` (and `docs/architecture.md` if code structure changed) **in the same change**. A feature change is not complete until its internal docs match.
- Do **not** touch `README.md` or any other public/user-facing doc during feature work. `main` may contain unreleased features, and the README must describe the latest *published* version, not unreleased `main`.
- Public docs (README, etc.) are updated **only at release time**, alongside the `CHANGELOG.md` entry, so they always describe the released version.

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

For every release, update `CHANGELOG.md` before tagging. The new changelog entry and the GitHub release body must describe the same final state. Commit the changelog update with the release changes before creating the version tag or publishing the GitHub release.

When finishing committed work or creating a version tag, merge the final commit to `main` before reporting done. Prefer a fast-forward merge. For releases, validate the branch first, fast-forward `main`, switch to `main`, create the tag there, then verify `main` contains the tagged commit. Do not report a version as ready while the tag exists only on a feature branch.

## Agent Harness

See `docs/harness.md` for the failure modes behind these rules.

- Treat the `docs/features/*` visual rules as test contracts. Width and string-presence checks are not enough for styled terminal output.
- When changing Lip Gloss styling, selected rows, borders, or overlays, add regression tests that inspect ANSI/SGR state or border geometry. Cover marker and no-marker rows, plus narrow and wide widths when layout changes.
- Force the Lip Gloss color profile inside tests that assert ANSI output, then restore the previous profile.
- Similar UI states must share one renderer. Do not hand-assemble sibling blocks, such as Worktree and Branch sections, through separate append paths.
- Dangerous Git actions must state the command family and data-loss effect in the dialog. Keep dependencies explicit: branch deletion is disabled unless worktree removal is enabled because the branch is checked out there.
- Defaults stay conservative: dirty worktree removal off, unmerged branch deletion off, merged branch deletion safe via `git branch -d`.

## Architecture

See `docs/architecture.md` for the full picture: the cd-on-exit mechanism, the package map and data flow, the `gitdata.Runner` subprocess seam, pure parsing, the async typed-message pattern (`refreshID`/`selectionAnchor`), layered enrichment, the shared `listview` renderer, and the testing approach. Keep that file in sync when code structure changes.

## Work tracking

Managed by the `workflow` plugin. Tasks are files in `workflow/<status>/`
(draft, ready, in-progress, blocked, done) — the folder IS the status;
moving a task is `git mv`. Board view: `./workflow/status`. Repo contract:
`workflow/AGENTS.md`. Commands: `/workflow:groom`, `/workflow:work`,
`/workflow:batch-work`, `/workflow:status`, `/workflow:framework-doctor`.
