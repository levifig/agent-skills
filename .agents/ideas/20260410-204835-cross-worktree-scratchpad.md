---
captured: 2026-04-10T20:48:35Z
status: intake
tags: [sessions, worktrees, cross-branch, cli, triage]
related: [concurrent-session-cursor-thrash]
---

# Cross-Worktree Scratchpad

## Idea

A shared, branch-agnostic inbox for capturing sparks, ideas, and notes from any
session, branch, or worktree — without leaving the current context or polluting
the current branch's tracked files.

## Problem

Today, capturing an idea while deep in a feature branch means either:
- Writing to `.agents/ideas/` on the feature branch (artifact lives on wrong branch until merge)
- Switching branches to write it on main (disruptive, risky with uncommitted work)
- Hoping you remember it later (you won't)

Worktrees make this worse: each worktree is a separate filesystem, so untracked
files in one don't exist in another.

## Design

**Storage:** `$(git rev-parse --git-common-dir)/scratch/` — a directory inside
the shared `.git/` that all worktrees can access. Not tracked, not gitignored,
just invisible to git.

**Access:** CLI-mediated only. No symlinks, no filesystem visibility from agents.
```bash
loaf scratch "spark: consider leaving CC plugin system for unified CLI install"
loaf scratch "idea: hook-created symlinks for cross-worktree state"
loaf scratch list
```

**Format:** One file per item, timestamped filename, minimal structure (type tag
+ content). Concurrent writes from different sessions/worktrees are safe because
separate files.

**Lifecycle — presence is liveness:**
- If it's in the scratchpad, it hasn't been processed
- Processing = promote to tracked artifact + delete the scratch file
- No status fields, no staleness detection, no metadata to maintain
- A 3-week-old item isn't stale — it's unprocessed. That's a signal, not a problem

**Processing via triage:**
- `/triage` always checks the scratchpad as part of its intake scan
- Each item is presented for confirmation before processing
- "Process" means: promote to `.agents/ideas/`, session journal spark, task, etc.
- "Skip" means: leave it in the scratchpad for next time
- Triage should request to be on the default branch (main) before promoting, since
  tracked artifacts belong on main, not feature branches

## Design Considerations

- CLI resolves `git rev-parse --git-common-dir` at runtime — works in any worktree
- No symlinks needed — avoids the `.git`-is-a-file-in-worktrees problem
- Agents don't need direct filesystem access — they use `loaf scratch` commands
- Concurrent writes are safe (one file per item, no shared state)
- Scratchpad survives worktree cleanup (lives in shared `.git/`)
- `.git/scratch/` is never pushed, never cloned — purely local working state

## Scope

Medium. New CLI subcommand (`loaf scratch`), triage skill integration, possibly
a SessionStart hook to surface unprocessed count.

## Source

Conversation exploring cross-session/cross-worktree state sharing (2026-04-10).
