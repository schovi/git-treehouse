# Detail Information Value

The detail panel should answer one question first:

What should I know before I act on this selected worktree?

This document treats each candidate field as an information item. The goal is to keep only items that help the user identify the target, understand risk, understand progress, or choose the next action.

## Item Breakdown

### Worktree Name

- Data: branch name, detached SHA label, and lifecycle suffixes such as `locked` or `prunable`.
- Says: which worktree is selected.
- Used for: confirming that `go`, `delete`, `editor`, or `PR` targets the intended branch.
- Value: high. This is the anchor for every other item.
- Display priority: always visible.

### Checkout Position

- Data: full or short `HEAD`, plus whether the row is on a branch or detached.
- Says: the exact commit currently checked out.
- Used for: comparing against table commit data, debugging detached rows, confirming branch state before destructive actions.
- Value: medium. Critical for detached rows, supportive for normal branches.
- Display priority: always visible, but compactable.

### Filesystem Location

- Data: relative path by default, absolute path available through action.
- Says: where the worktree lives on disk.
- Used for: recognizing generated worktrees, opening in an editor, copying a path, spotting stale paths.
- Value: medium. Useful, but long paths can dominate the panel.
- Display priority: visible, aggressively truncated.

### Local Cleanliness

- Data: clean or dirty summary, with staged, modified, and untracked counts.
- Says: whether local work could be lost.
- Used for: deciding whether delete is safe, whether to review changes, whether to switch context.
- Value: very high. Dirty state changes the meaning of almost every action.
- Display priority: always visible.

### Dirty File Preview

- Data: top staged, modified, and untracked file paths.
- Says: what local work exists, not just that work exists.
- Used for: deciding whether to open the editor, stash, commit, or avoid deletion.
- Value: very high when dirty, zero when clean.
- Display priority: context slot only when dirty.
- Current availability: needs new data. Current state has counts, not paths.

### Upstream Relationship

- Data: upstream branch name, ahead count, behind count, synced, gone, or no upstream.
- Says: relationship to the remote tracking branch.
- Used for: deciding whether to push, pull, create upstream, or treat a branch as stale after PR merge.
- Value: high. Remote state is a common next-action driver.
- Display priority: always visible in summary form.

### Main Relationship

- Data: ahead and behind counts vs local main branch.
- Says: how the selected work relates to the integration base.
- Used for: deciding whether to rebase, whether a branch has unique work, and how stale the branch is.
- Value: high. It explains branch freshness and cleanup risk.
- Display priority: always visible in summary form.

### Work Size Since Main

- Data: commit count, changed file count, insertions, deletions.
- Says: how large the branch is compared with main.
- Used for: judging review size, deciding whether to inspect before deleting, understanding why a branch matters.
- Value: high for clean feature branches, medium otherwise.
- Display priority: context slot when not dirty or blocked.
- Current availability: needs new data.

### Commit Stack Preview

- Data: latest commit subjects on the selected branch.
- Says: what the branch is about.
- Used for: recognizing work without opening the editor or PR.
- Value: medium. Useful, but can duplicate the table commit when only one commit exists.
- Display priority: show only when there is room or multiple commits.
- Current availability: needs new data beyond the current HEAD commit.

### Head Commit Summary

- Data: current commit short SHA, subject, and age.
- Says: the most recent saved work and how fresh it is.
- Used for: recognizing the worktree, spotting stale or recently active branches.
- Value: high. Already compact and familiar from the table.
- Display priority: always visible, compactable.

### Pull Request Presence

- Data: PR number, state, URL when available.
- Says: whether this branch already has review context.
- Used for: opening the PR, avoiding duplicate PR creation, understanding branch lifecycle.
- Value: high when present, low when absent.
- Display priority: always visible as a compact line if PR lookup is enabled.

### Pull Request Summary

- Data: PR title, draft state, author, requested reviewers.
- Says: what the external review object is about and whether it is ready.
- Used for: deciding whether to open the browser, wait, request review, or keep working locally.
- Value: high when PR exists.
- Display priority: context slot when PR exists and local state is clean.
- Current availability: needs new GitHub lookup data.

### Review State

- Data: approved, changes requested, review pending, reviewers requested.
- Says: human review status.
- Used for: deciding whether the next action is code work, reviewer follow-up, or merge readiness.
- Value: high when PR exists.
- Display priority: context slot when PR exists.
- Current availability: needs new GitHub lookup data.

### CI Health

- Data: aggregate CI state, optionally workflow names and failed checks.
- Says: automated readiness.
- Used for: deciding whether to fix checks, wait, merge, or ignore a branch for now.
- Value: very high when PR exists or checks are failing.
- Display priority: context slot when PR exists, or when failing.
- Current availability: aggregate PR CI exists, workflow detail needs new data.

### Delete Eligibility

- Data: allowed, blocked, force required, branch deletion optional, branch deletion disabled.
- Says: whether delete can run and what it would remove.
- Used for: deciding whether to press `d`, and understanding why a row is protected.
- Value: very high. This is the app's most safety-sensitive detail.
- Display priority: always visible.

### Delete Consequence

- Data: worktree removal effect, branch deletion effect, dirty discard risk, unmerged commit risk.
- Says: the actual data-loss behavior behind delete.
- Used for: making destructive choices explicit before opening or confirming the delete dialog.
- Value: high for dirty, unmerged, locked, prunable, or upstream-gone rows.
- Display priority: context slot for risky or cleanup states.

### Cleanup State

- Data: locked, prunable, upstream gone, merged to main.
- Says: whether the row is special in the worktree lifecycle.
- Used for: choosing prune, unlock, safe branch deletion, or stale branch cleanup.
- Value: high for affected rows, zero otherwise.
- Display priority: context slot when present.

### Disk Footprint

- Data: total worktree size.
- Says: storage cost.
- Used for: prioritizing cleanup when many worktrees exist.
- Value: medium. It becomes high only for unusually large worktrees.
- Display priority: secondary, unless size crosses a notable threshold.

### Disk Breakdown

- Data: largest directories or categories within the worktree.
- Says: why the worktree is large.
- Used for: deciding whether cleanup is worth it, and whether the cost is from dependencies, build output, or repository data.
- Value: medium to high for large worktrees.
- Display priority: context slot only for large worktrees.
- Current availability: needs new data.

### Recommended Next Action

- Data: derived action such as `review dirty files`, `push branch`, `sync with main`, `wait for checks`, `safe to prune`.
- Says: the most likely useful next step.
- Used for: reducing interpretation work across several raw fields.
- Value: high if conservative and explainable.
- Display priority: context slot, one line.
- Current availability: derived from existing and future data.

## Emerging Pattern

Most items fall into four user decisions:

- Am I looking at the right target?
- Is there local work at risk?
- Is this branch ready, stale, or blocked?
- What should I do next?

That pattern suggests the detail panel should not be a flat list of all possible facts. It should show a stable identity column plus one contextual decision area.

## Logical Groups

### Target Identity

Names:

- Worktree Name
- Checkout Position
- Filesystem Location
- Head Commit Summary

Value together: confirms the selected target and makes accidental actions less likely.

### Local Risk

Names:

- Local Cleanliness
- Dirty File Preview
- Delete Eligibility
- Delete Consequence

Value together: explains whether local work exists and whether an action could discard it.

### Branch Freshness

Names:

- Upstream Relationship
- Main Relationship
- Work Size Since Main
- Commit Stack Preview

Value together: explains whether the branch is synced, stale, ahead, behind, or meaningful enough to keep.

### Review Readiness

Names:

- Pull Request Presence
- Pull Request Summary
- Review State
- CI Health

Value together: explains whether the branch is already in the review pipeline and what blocks merge readiness.

### Cleanup Context

Names:

- Cleanup State
- Delete Eligibility
- Delete Consequence
- Disk Footprint
- Disk Breakdown

Value together: explains why a row might be worth removing and what removal would cost.

### Action Guidance

Names:

- Recommended Next Action
- Local Risk
- Branch Freshness
- Review Readiness
- Cleanup Context

Value together: converts raw facts into one conservative next step. This should be derived from the groups above, not invented independently.

## Display Implication

Default detail layout should probably be:

- Left: Target Identity, always stable.
- Right: one primary context group, chosen by state.
- Bottom line inside content: Recommended Next Action plus Delete Eligibility.

Primary context selection:

1. Dirty or delete-risky row: Local Risk.
2. Locked, prunable, upstream-gone, or very large row: Cleanup Context.
3. Row with PR: Review Readiness.
4. Clean branch without PR: Branch Freshness.

