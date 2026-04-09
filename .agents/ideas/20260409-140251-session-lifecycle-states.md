---
captured: 2026-04-09T14:02:51Z
status: intake
tags: [sessions, lifecycle, housekeeping, archival]
related: [SPEC-029]
---

# Session Lifecycle States (stopped -> done -> processed -> archived)

## Idea

Current lifecycle: `active -> stopped -> archived` (jumps directly).

Proposed lifecycle with intermediate states:
- **stopped** — conversation ended, session may have loose ends
- **done** — confirmed complete, no more entries expected
- **processed** — wrap summary exists AND JSONL enrichment complete (SPEC-029)
- **archived** — moved to `archive/`, out of active view

Housekeeping manages transitions:
1. `stopped` sessions older than X -> housekeeping marks `done`
2. `done` sessions without wrap -> run wrap -> mark `processed`
3. `processed` sessions -> move to archive/ -> mark `archived`

The `done -> processed` gate prevents premature archival — don't archive until
wrap + JSONL sync are confirmed complete. This is particularly important once
SPEC-029 ships, as JSONL enrichment may not be immediate.

## Design Considerations

- Adds `done` and `processed` to `SessionFrontmatter.status` type
- `quickArchiveSession` (used by session start to close stale sessions) should
  transition to `done`, not directly to `archived`
- Housekeeping skill needs a new "process stale sessions" step
- The `loaf session archive` command becomes the manual override (skip to archived)
- SessionEnd hook sets `stopped`, not `done` — the distinction is that `done`
  means "reviewed by housekeeping and confirmed complete"
- Age threshold for `stopped -> done` should be configurable (default: 1 hour?)

## Scope

Medium. Touches session.ts (type + transitions), housekeeping skill, and
potentially the session template. Should be shaped as a spec.

## Source

User observation during review of session 20260408-212037 lifecycle gaps.
