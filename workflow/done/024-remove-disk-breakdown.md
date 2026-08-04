# 024 — Remove unused disk-breakdown enrichment

done: 2026-08-04
tags: tui

## What & why

Remove the permanently disabled Disk frame and its bucketed disk-usage pipeline. The existing Git-aware and full-size display remains; it is the shipped size signal.

## Spec

Remove the Disk renderer and its layout hook, then remove the `DiskBreakdown` model, bucketed traversal, and async enrichment plumbing that exist only to feed it. Retain the normal full-size enrichment and its Detail-panel display. Update the Main view and architecture docs to stop describing the disabled feature.

Ownership: TUI frame/layout and size-enrichment paths; `gitdata` disk data types and loader; `docs/features/main-view.md` and `docs/architecture.md`. Likely tests: Disk-frame coverage and size-enrichment coverage. Excludes changes to the shipped size column or Detail-panel size fields.

## Acceptance criteria

- The app has no Disk frame or bucketed disk-breakdown enrichment path.
- Selected worktrees still load and display their existing Git-aware and full disk sizes.
- Internal behavior and architecture docs no longer describe a disabled Disk feature.
