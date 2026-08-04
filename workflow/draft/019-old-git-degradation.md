# 019 — Surface (or fix) silent degradation on git < 2.41

tags: gitdata

## What & why

On git older than 2.41 (Ubuntu 22.04 LTS ships 2.34) branch rows silently disappear, merged-to-main detection dies (so cleanup-merged finds nothing), and every refresh degrades to slow per-worktree subprocess loops — with no error or hint anywhere. Same failure triggers on any repo whose default branch is neither main nor master with no origin/HEAD symref.

## Spec (evidence)

From the production review (not adversarially verified — confirm before scoping):

- `loadRefMetadata` uses the `%(ahead-behind:...)` for-each-ref atom (`internal/gitdata/load.go:249-259`), added in git 2.41; on older git the command fails and `EnrichLocalMetadata` takes the fallback branch (`load.go:114-121`), which never sets `state.Branches` and runs `enrichWorktree` per row (5-8 subprocesses each, `load.go:434-479`).
- `detectMainBranch` returns literal "main" when detection fails (`load.go:491-496`), making the ahead-behind ref nonexistent → same fallback.
- No version check exists; `doctorGit` only prints the version (`cmd/git-treehouse/main.go:694-704`). No doc states a minimum git version; `docs/features/edge-cases.md` claims behavior "matches the user's git version".

Options (not mutually exclusive):

- A) Minimum: detect the failure once (or probe version) and surface it — `doctor` says "git X.Y < 2.41: branch rows and merged detection unavailable", and the TUI shows a one-time status hint instead of silently hiding the Branches section.
- B) Full: fall back to a for-each-ref format without the atom plus one `git rev-list --left-right --count` per branch, so branch rows still render on older git (slower but correct).

Boundary: `internal/gitdata/load.go` (loadRefMetadata, fallback path, detectMainBranch), `cmd/git-treehouse/main.go` (doctor). Tests: fake Runner returning the old-git error; doctor output. Routed docs: `docs/features/edge-cases.md`, `docs/features/cli-commands.md` (doctor).

## Acceptance criteria

- (A at minimum) On old git, `doctor` and the TUI both name the limitation; nothing silently disappears without explanation.
- If B: branch rows and merged detection work on git 2.34 via the fallback format (fake-Runner test).
- edge-cases.md states the minimum git version for full functionality.

## Open questions

- Scope: A only, or A+B? B is real work (second query path + tests); A is small. What git versions do actual users run?
- Should the mis-detected-main case (detectMainBranch literal fallback) get its own hint ("set main_branch in config"), since it hits new-git users too?
