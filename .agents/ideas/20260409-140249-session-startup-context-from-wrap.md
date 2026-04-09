---
captured: 2026-04-09T14:02:49Z
status: intake
tags: [sessions, continuity, startup, wrap]
related: []
---

# Session Startup Context from Previous Wrap-Up

## Idea

On SessionStart, when creating a new session (not resuming), detect if the most
recently archived/stopped session for this branch has a `## Session Wrap-Up`
section. If it does, surface it as startup context — a few-line summary of what
the previous session shipped, what decisions were made, and what's next.

This gives the new session a running start from where the last one left off,
without the model having to read the full archived session file.

## Design Considerations

- Only surface for NEW sessions on the same branch, not cross-branch or first-ever
- Extract the **What's Next** and **Decisions** subsections (most actionable)
- Keep the surfaced context brief — 5-10 lines max, not the full wrap-up
- Falls back gracefully when no previous session exists or has no wrap-up
- Implementation: extend `loaf session start` to scan archive for most recent
  session matching the branch, extract wrap-up section

## Scope

Small. Single-file change to `cli/commands/session.ts` session start handler.

## Source

spark(session) from session 20260408-212037 (archived).
