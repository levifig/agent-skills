---
captured: 2026-04-07T00:11:55Z
status: raw
tags: [sessions, concurrency, journal, architecture]
related: [SPEC-027]
---

# Rolling Journal Per Branch

## Idea

Replace file-per-session model with a single continuous journal file per branch.
Each entry gets a `[session:abc123]` prefix tying it to the originating Claude
conversation. No archive churn — history is naturally continuous and append-only.

Motivation: SPEC-027 fixes the immediate subagent session stomping via `agent_id`
detection, but the underlying model (one file per session, archive on close) is
fragile. A rolling journal is inherently concurrent-safe for appends and eliminates
the create/archive/resume lifecycle entirely.

## Design Considerations

- Append-only body is safe under POSIX `O_APPEND` (entries < 4KB)
- Frontmatter metadata (status, last_entry) requires read-modify-write — race
  condition with concurrent agents. Split metadata into a separate file or
  lock-protect it.
- Journal files would grow indefinitely per branch. Need a rotation or
  summarization strategy for long-lived branches.
- Changes session template, archival logic, `/wrap`, `/housekeeping`, and all
  skills that read/write session files.

## Scope

Major refactor of session lifecycle. Touches session.ts, all session-aware skills,
archive logic, and the spec/session template.

## Source

Discussion during SPEC-027 shaping session. User asked about concurrent agent
writes to journal files.
