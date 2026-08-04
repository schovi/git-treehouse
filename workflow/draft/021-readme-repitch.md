# 021 — README: reposition the pitch, fix screenshot labels

tags: docs, positioning

## What & why

The README headline "Tame AI agent worktrees" has zero agent-specific capability behind it (adversarially verified: no agent-related code, no tmux/tab fan-out, Enter-to-jump exits the TUI). The pitch overpromises the parallel-agent scenario while the tool's real, verifiable strengths — live worktree state table, one-keypress shell jump, cleanup safe by construction — go under-sold. Also: README.md:31-32 labels two different screenshots "create new worktree" and misses a separator.

Constraint: README changes only at release time (AGENTS.md doc policy) — this task rides the next release, or ships with 020.

## Spec (starting point)

Recommended positioning (from the review, verified against code):

> Git Treehouse is a single-repo worktree lifecycle dashboard for GitHub-centric developers who keep many branches in flight at once — increasingly because coding agents generate them. One live table answering "what state is every worktree in" (dirty, sync vs upstream and main, PR and CI), a one-keypress jump that actually moves the shell, and cleanup that is safe by construction (no --force paths, hash-approved hooks, restorable deletes, PR-merge-aware batch cleanup).

- If task 020 (tmux/tab fan-out) ships first, the agent-parallelism framing becomes true and can stay, strengthened; if not, soften the headline to the dashboard framing.
- Keep the verified-accurate feature table (positioning review confirmed every checkable claim held).
- Fix screenshot labels at README.md:31-32 (relabel to "create from branch" and "help overlay", add missing separator).
- Also due at the same release: asciinema demo (docs/ideas.md:9, low effort, high adoption value) — decide whether to fold in here.

## Acceptance criteria (draft)

- README headline and intro claim only capabilities that exist in the released version.
- Screenshot labels match their images; list separators fixed.
- Ships alongside a release + CHANGELOG entry per the doc policy (never on main mid-cycle).

## Open questions

- Wait for 020 to ship (keep agent headline, now true) or reposition at the next release regardless? The review recommends: don't let the false headline ride another release.
- Fold the asciinema demo into this task or keep it separate?
