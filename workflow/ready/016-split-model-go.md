# 016 — Split internal/tui/model.go along its existing seams

priority: 160

tags: tui, code-health

## What & why

model.go is 5,840 lines, 310 functions, a 54-field Model, and a 427-line Update switch (adversarially verified) mixing 8 dialog subsystems, rendering primitives, help rendering, async command builders, and utilities. The seams already exist inside the file, so this is a mechanical same-package file split — no API change, no behavior change. Do it before the next feature lands more weight in the file. model_test.go (4,670 lines / 167 tests) splits along the same lines.

## Spec

Verified inventory (line refs to current file; re-locate after any prior task lands):

- Model struct `model.go:28-83` (54 fields, 8 dialog pointers at 55-68).
- Update switch 624-1051.
- Dialog trios (open/update/render), e.g. delete: openDelete 1942, updateDelete 2006, renderDeleteAtWidth 4256; same pattern for create/checkout/branch-worktree/cleanup/palette/filter/PR-checkout.
- Rendering primitives 3101-3273 (wrapOuter, sectionBox*, sectionTopLine/BottomLine, bottomBorderLine, renderBorderControls, padRight/padStyled/truncatePlain).
- Help rendering 3837-4020.
- Async command builders 5204-5660 (startEnrichment, enrichmentCommands, pullRequestFetchCommand, diskUsageCommand, reloadCmd).
- Utilities: fuzzyMatch 5787, clamp 5811.

Target layout (same package `tui`, git-friendly moves):

- `dialog_create.go`, `dialog_delete.go`, `dialog_cleanup.go`, `dialog_checkout.go` (checkout + branch-worktree + PR-checkout can share if cohesive), `dialog_palette.go` (palette + filter picker), each holding its dialog struct + open/update/render funcs.
- `render_primitives.go`, `help.go`, `commands.go` (async tea.Cmd builders).
- `model.go` keeps: Model struct, New, Init, Update dispatch, View, list update/render, selection/filter/search logic.
- Split `model_test.go` mirroring the new files.

Include while moving (cheap adjacent cleanups found by the same review — verify each, skip with one-line reason if wrong):

- Delete `FormatExitError` (`model.go:5835`, zero callers repo-wide).
- Delete package-scope `min`/`max` in tui (`model.go:5821-5834`) and onboarding (`internal/onboarding/onboarding.go:187-200`) — Go builtins since 1.21; keep `clamp`.
- Optional: collapse the four byte-identical spinner tick cmd/msg quadruplets (`model.go:3505-3527`, msg structs 408-412) into one `spinnerTickMsg{kind, id}` — do it only if the split makes it trivial; otherwise note as follow-up.

Constraints: pure moves + deletions; no renames of exported/unexported identifiers beyond the deletions listed; no behavior change. `docs/architecture.md` package map must be updated in the same change (it names model.go responsibilities).

## Acceptance criteria

- No file in internal/tui exceeds ~1,500 lines; model.go retains only model/dispatch/list concerns.
- `make build && make lint && make test && make security` all green; zero test assertions changed (moves only).
- FormatExitError and the shadowing min/max pairs are gone.
- docs/architecture.md notable-files table reflects the new layout.
