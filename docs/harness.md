# Agent Harness Notes

These notes capture recurring agent failure modes from Git Treehouse sessions.
They are not product backlog. Turn each repeated correction into a test, spec
rule, or instruction update.

## Release Order

Failure: `v0.4.0` was first tagged on a feature branch. The user had to ask for
the same work to be merged to `main`.

Guardrail: validate the feature branch, fast-forward `main`, switch to `main`,
create the version tag there, then verify:

```sh
git show --no-patch --decorate --oneline HEAD
git tag --points-at HEAD
git branch --contains <tag>
git status --short --branch
```

Do not call work finished or versioned until `main` contains the final commit
and the tagged commit.

## Terminal Styling

Failure: the selected-row highlight visually broke because nested ANSI resets
from styled cells cleared an outer row background. Existing tests still passed
because they only checked row width and text presence.

Guardrail: visual contracts in `spec.md` need visual tests. For styled terminal
output, assert the actual ANSI/SGR state or exact border geometry. If a test
expects color output, set the Lip Gloss color profile in the test and restore
the previous profile afterward.

Selected-row tests must cover both marker rows, such as the root worktree, and
ordinary branch rows. Width checks alone are insufficient.

## Shared UI Blocks

Failure: delete modal Worktree and Branch blocks drifted in indentation because
they were assembled through separate manual append paths.

Guardrail: sibling UI states should share one renderer and one test surface.
For modal sections, render header, checkbox, details, commands, disabled text,
and indentation through the same helper. Add one helper-level regression test
plus representative state tests.

## Dangerous Actions

Failure: branch deletion copy was confusing until safe delete and force delete
semantics were made explicit.

Guardrail: destructive dialogs must show what command family will run and what
data can be lost. Keep action dependencies visible:

- Dirty worktree removal defaults off and requires explicit confirmation.
- Merged branch deletion uses safe `git branch -d`.
- Unmerged branch deletion defaults off and uses `git branch -D` only when the
  user explicitly opts in.
- Branch deletion is disabled unless worktree removal is enabled, because Git
  will not delete a branch that is checked out in a worktree.

## Session Review

Before committing or releasing substantial work, scan recent user corrections
in the active session. If a correction repeats or reveals a blind spot, harden
one of these before reporting done:

- A focused regression test.
- `spec.md` when behavior changed or was underspecified.
- `CLAUDE.md` when the agent workflow failed.
- This file when the lesson explains why the harness rule exists.
