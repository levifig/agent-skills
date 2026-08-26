# ADR-029: Fact envelope as the grow-only sync contract

Decision Date: 2026-08-26
Status: Accepted

## Context

The minimal core mixed append-only journal rows with mutable tables and side
event logs. Sync, convergence, and trustworthy history require one write
discipline: every durable change is an appended fact; current truth is a
projection replayed from facts; nothing is edited in place on the synced core.

LOAF-71 proves the model on the journal — the organ that already behaved
append-only — before the wider mutable-core migration (LOAF-72).

## Decision

1. **The fact envelope is the fleet-wide sync contract (v1).** Fields:
   `{ id, project_id, kind, payload, env_id, seq, hlc, envelope_v }`.
   There is no supersession field in v1; the synced set is grow-only and
   projections fold latest-event-wins. Physical wall time, when retained for
   display or source provenance, lives inside the payload and never
   participates in fold order.
2. **One write chokepoint.** All minimal-core writers append through
   `AppendFact`; no other path inserts into `facts`.
3. **Closed fact kinds.** Kinds are code-registered. Unknown kinds are an
   attach-time availability event (hard refusal naming the upgrade). Journal
   free-text `entry_type` values live inside the journal fact payload.
4. **Hybrid logical clock ordering.** The envelope clock is wall-seeded and
   advanced past the highest clock seen on every receive. Total order is
   `(hlc, env_id, id)`. HLC is stored as zero-padded
   `wall_ms:logical`.
5. **Mint-once opaque ids.** New facts receive UUIDv7-style ids at append time;
   ids are never derived from content. Wrapped journal history preserves
   existing journal entry ids as fact ids.
6. **Pre-auth env identity.** Until attach lands (LOAF-67), new facts carry
   the local host identity as `env_id`; wrapped corpus backfills
   `legacy-host`.
7. **Journal on facts.** `loaf journal log` appends journal-kind facts;
   recent/context/search read the `journal_entries` projection rebuilt from
   facts. FTS (`journal_search`) is a local derived index, never synced.
8. **Parity is a hard invariant.** `loaf state doctor` compares the journal
   projection to the fold of journal facts over the full corpus, including
   wrapped history, and invalidates SQLite-ready mode on divergence.
   `loaf state repair journal-facts` rebuilds the projection from facts
   after a verified backup.

## Permanence classes (kind metadata)

| Kind | Class | Notes |
|------|-------|-------|
| `journal` | notebook | Durable retrieval; display timestamps in payload |

## Consequences

- Journal writes route through `AppendFact`; reads stay on the projection
  tables until broader entity migration (LOAF-72).
- Cleartext payloads remain local-first; sealing and E2E are LOAF-66 scope.
- Transport and attach ceremony remain out of scope (LOAF-75/76, LOAF-67).

## Evidence

- LOAF-71 implementation and `go test ./internal/state/...`
- Parent shaping: LOAF-63 grow-only fact model
