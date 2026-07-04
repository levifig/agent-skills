---
id: ADR-019
title: "Journal-first: the project journal replaces the session entity"
status: Accepted
date: 2026-07-04
supersedes: null
superseded_by: null
---

# ADR-019: Journal-first — the project journal replaces the session entity

## Context

SPEC-048 gave sessions a start→log→end lifecycle with a six-state status model, and SPEC-032 added
a three-tier router to resolve which session a journal entry belonged to. In production the
lifecycle did not maintain itself: of 276 sessions, 19 were stuck `active` and 21 `paused`; 60
(22%) held zero journal entries; `session_state_snapshots` never held a single row; and ~24% of
4,541 journal entries were lifecycle noise (`session-start`/`session-end` bookkeeping rather than
work). The entity cost roughly 2,000 LOC of lifecycle code — rotation, consolidation, status
transitions, snapshot plumbing, and the router — all of it serving a structure nobody queried.

Concurrent conversations made it worse. Multiple conversations across branches, worktrees, and
harnesses fought over rotation and status semantics: each assumed it owned "the" active session,
so parallel work produced splits, duplicates, and misrouted entries by construction.

## Decision

Delete the session entity. The project journal is the only session-related structure:

1. **Journal entries are project-scoped, append-only rows** (`journal_entries`,
   `project_id NOT NULL`) tagged with an opaque `harness_session_id` that correlates one
   conversation's entries. There is no session table, no lifecycle, no status, and no rotation —
   nothing to open, close, or transition.
2. **Wrap is an optional checkpoint entry**, written only when a conversation holds synthesis
   worth saving. It is never a lifecycle transition, and nothing is ever "unwrapped."
3. **Continuity is derived at read time.** The SessionStart hook emits a layered digest
   (`loaf journal context --from-hook`) computed from the journal — latest project wrap, recent
   branch entries, open tasks — and never persisted.
4. **Migration 0010 is explicit-apply with a mandatory backup.** Applied to production
   2026-07-03: 4,664 → 3,542 entries (1,122 lifecycle-noise entries purged), 286 session rows
   dropped, schema v10.

Shipped by SPEC-056 (PR #87, squash commit `9189080c`).

## Consequences

### Positive

- Concurrent conversations across branches, worktrees, and harnesses are conflict-free by
  construction: entries interleave as project-scoped rows, so there is nothing to misroute and no
  status to fight over.
- No lifecycle to housekeep — no stuck-active sessions, no split consolidation, no rotation.
- The SPEC-032 routing tension (log by branch vs. enrich by `claude_session_id`) is dissolved
  rather than managed: branch is a query filter over rows, not an identity to resolve.

### Negative

- Continuity quality now depends entirely on journal discipline. The PreCompact flush still races
  compaction — the model must write entries before the window closes — and journal-discipline
  hooks mitigate but do not eliminate that race.
- A conversation that ends without a `wrap` leaves only raw entries; the derived digest must
  degrade gracefully without hand-written synthesis.

### Neutral

- SPEC-048's start→log→end session lifecycle and SPEC-032's three-tier session router are
  superseded.
- SPEC-049's unified status vocabulary no longer includes session states — there is no session to
  be in a state.
- Amends ADR-016: "sessions" leaves the canonical noun enumeration; journal entries remain the
  queryable record. Amends ADR-017: `.agents/sessions/` stands as a historical migration source
  only.

## Related

- [SPEC-056](../../.agents/specs/SPEC-056-journal-first-session-model.md) — Journal-first: the
  project journal replaces the session entity
- PR #87 / commit `9189080c` — implementation
- Migration 0010 (schema v10) — entity removal, applied to production 2026-07-03
