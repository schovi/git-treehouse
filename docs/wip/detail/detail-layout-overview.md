# Detail Layout Sketches

Goal: use the empty right side of the Details panel for selected-row context, not more global help.

Recommended shape: responsive columns.

- Wide details content, 150+ columns: 3 columns.
- Standard details content, 104-149 columns: 2 columns.
- Compact details content, 72-103 columns: 1 column with grouped summary lines.
- Tiny details content, below 72 columns: keep only essential selected-row facts and the footer actions.

The layout should not use nested boxes inside the Details panel. Plain headings and aligned columns are enough.

## Information Groups

Identity, always visible:

- Branch
- HEAD
- Path, relative by default
- Status
- Dirty
- Remote
- Main
- Commit
- PR
- Delete

Work summary, shown when width allows:

- Branch relationship, local main to branch to remote
- Diff from main, commits/files/insertions/deletions
- Latest commit subjects
- Changed file preview

Context summary, state-driven:

- Dirty file preview when the row is dirty
- PR title, review state, and CI summary when PR data exists
- Delete safety explanation for prunable, locked, dirty, merged, or unmerged rows
- Recommended next action
- Disk usage detail when loaded and notable

## Priority

1. Identity and delete safety stay visible.
2. Dirty and locked/prunable states win the top context slot.
3. PR and CI summary win when the row has an open PR and no urgent local state.
4. Diff from main fills the context area for clean branches without PR data.
5. Disk usage stays secondary unless the size is large enough to matter.

## Column Rules

Wide, 3 columns:

- Identity: fixed 38-44 columns.
- Work: elastic, minimum 42 columns.
- Review and safety: elastic, minimum 38 columns.

Standard, 2 columns:

- Identity: fixed 38-44 columns.
- Context: elastic. Stack Work, PR/CI, Safety, Disk in priority order.

Compact, 1 column:

- Merge values into short grouped lines.
- Show at most one contextual preview list.
- Truncate lists to 1-2 items with `+N more`.

Tiny:

- Branch, HEAD/commit, state, PR/delete summary.
- Keep the selected-row footer actions if there is room.

