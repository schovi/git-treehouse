# 009 — Sync docs/features/* with implementation (12 verified disagreements)

priority: 90

tags: docs

model: sonnet

## What & why

A docs-vs-code audit found 12 disagreements. docs/features/* are treated as test contracts (docs/harness.md), so wrong docs are wrong contracts. All items below were verified against code by the audit; re-verify each briefly before editing (line numbers may drift). Docs-only change — internal docs track main, so no release gate. Do NOT touch README.md (release-time only).

## Spec

Fix each in the named doc (doc claim → code reality):

1. `docs/features/cli-commands.md` (~line 9): width thresholds "PR >= 128, size >= 144" don't exist. Visibility is computed by column fitting (`internal/listview/listview.go:107-117, 142-154`; `cmd/git-treehouse/main.go:230-231`); empirical cutoffs ~74 (PR) and ~92/101 (size without/with PR). Describe the mechanism, not fake constants — or derive and state the real computed cutoffs.
2. `docs/features/keybindings.md:23` + `docs/features/command-palette.md:5`: Ctrl+O is not a list-view binding today (only inside create dialog, `model.go:1892-1893`). Coordinate with task 004, which ADDS the binding — if 004 lands first, the doc becomes true; note which way it resolved.
3. `docs/features/command-palette.md:5`: `filter-branches` palette command does not exist (`model.go:489-516`); branches filter is Tab-picker-only.
4. `docs/features/command-palette.md:5`: no `checkout` palette command exists; palette omits from docs seven real commands: Fetch and reload (r), Search branches (s), Jump to root/active/top/bottom (h/a/g/G), Open filter picker (Tab), Quit (q). Rewrite the command list from `paletteCommands` (`model.go:489-516`).
5. `docs/features/main-view.md` (~line 56): detail panel shows the RELATIVE path via `model.relativePath` (`model.go:2733, 3050-3060`); absolute is only what `y` copies.
6. `docs/features/main-view.md` (~line 63): help legend group is titled "Row Icons", not "Worktree Markers" (`model.go:3935-3980`).
7. `docs/features/main-view.md` (~line 20): column-drop order is wrong. Actual `fitColumns` order (`listview.go:107-117, 86-91`): remove size → shrink commit → remove PR → remove age → shrink name → remove main± → remove remote → remove commit; only name+status survive to compact (<40 col). Doc omits main± and wrongly says remote outlives commit.
8. `docs/features/keybindings.md:24`: Esc clears active filter BEFORE a committed search (`model.go:1059-1076, 1227-1229`) — doc order reversed. Also add missing g/G rows. (Skip if task 004 already fixed; one-line reason.)
9. `docs/features/create-and-checkout.md` (~line 18): branch name is validated on Enter, not "live" (`model.go:1894-1896, 1921-1926`). (Coordinate with task 005 note; document whichever is true when this lands.)
10. `docs/features/shell-integration.md` (~line 11): path prints to stdout only when stdout is NOT a TTY (`main.go:969-978`); interactive bare runs print an install-gth hint to stderr instead (`main.go:1028-1038`). Document both branches.
11. `docs/features/main-view.md`: undocumented shipped behavior — scrollbar gutter when rows overflow, and the right footer hints replaced by a "start/total" position indicator while it shows (`model.go:2652-2695, 3312-3317`). Add to the Table / footer sections. (If task 013 changes this behavior, document the new behavior instead.)
12. `docs/wip/frames/frames-overview.md` + `docs/wip/detail/detail-layout-overview.md`: sketches describe shipped features (frames, detail layout) with a never-built design (toggles, 2x2 grid, paged overlay, 3-column details); shipped reality is a fixed two-column composition at >= 104 cols (`model.go:2591-2648`, `frames.go:30-44`). Add a loud header pointing to the shipped spec in docs/features/main-view.md, or delete the files.

Boundary: docs only (plus zero code). Verify each cited code location before editing its doc line.

## Acceptance criteria

- Each of the 12 items above is resolved: doc matches code, or a one-line reason records why it was skipped (e.g. superseded by tasks 004/005/013).
- No README.md or other public-doc changes.
- `docs/README.md` index still accurate (no files renamed/removed without index update).
