---
captured: 2026-04-09T14:02:50Z
status: intake
tags: [sessions, concurrency, jsonl, spec-029]
related: [SPEC-029]
---

# Concurrent Session Cursor Thrash (SPEC-029 Gap)

## Idea

SPEC-029's keyed cursor (`log_cursor: {session_id, offset}`) assumes one
conversation per branch at a time. If two Claude sessions are active on the same
branch, each sync rewrites the cursor for its own conversation — thrashing the
other's read position.

Options identified:
1. **Branch exclusivity** — detect and warn when a second conversation starts on
   an already-active session. Simple but restrictive.
2. **Per-conversation cursors** — store multiple cursor entries keyed by
   `session_id`. Each conversation tracks its own offset independently. More
   complex frontmatter, but no conflict.
3. **Session-per-conversation model** — each Claude conversation gets its own
   session file. Eliminates shared state but fragments the journal.

## Design Considerations

- Option 1 is simplest and matches current session model (one active per branch)
- Option 2 requires cursor cleanup when conversations end
- Option 3 conflicts with the "one continuous journal per branch" philosophy
- This is a load-bearing assumption across the framework — the session model
  implicitly assumes single-writer per branch
- May be addressed as part of SPEC-029 Track 2 (model filter) or as a constraint
  documented in the spec

## Scope

Design decision for SPEC-029. May require spec amendment before Track 1 ships.

## Source

spark(sessions) from session 20260408-212037 (archived).
