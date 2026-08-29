---
id: ADR-030
title: "vNext continuity persists as SQLite-backed typed facts"
status: Accepted
date: 2026-08-29
supersedes: null
superseded_by: null
related:
  - ADR-014
  - ADR-019
  - ADR-028
  - ADR-029
---

# ADR-030: vNext continuity persists as SQLite-backed typed facts

## Context

vNext needs a private continuity store for one operator: project identity, journal entries, wraps, sparks, ideas, decisions, explorations, checkpoints, findings, handoffs, scratchpad coordination, opaque external references, verification evidence, and derived context. The kernel already owns schema identity `vnext/1` and does not open a database. Continuity cannot import the shipped runtime, reuse its packages, or treat ADR-014 or ADR-029 as vNext precedent.

The store has to survive compaction, harness restarts, and later private sync. Facts have to stay typed and closed. Projections must not become a second source of truth. Scratchpad coordination is ephemeral in meaning, but physical prune is a sync concern, not a continuity-schema concern.

## Decision

vNext continuity persistence is SQLite now.

1. **Typed append-only facts are canonical.** Every durable continuity change is an appended fact with a closed kind. There is no runtime registry, generic metadata map, `any`, or `json.RawMessage` payload bag.
2. **Projections are pure read-time folds for LOAF-96.** Current views, including derived conversation context, are functions of the fact log. This slice does not materialize projection tables that can drift from facts.
3. **Total order is HLC wall/logical, then environment ID, then fact ID.** Physical wall time may live inside a typed payload for display; it does not participate in fold order.
4. **The API is closed and typed.** Continuity exposes named domain types and a closed semantics catalog. It does not grow tracker, provider, credential, status, title, body, assignment, hierarchy, or dependency surfaces.
5. **SQLite admission is exact.** `database/sql` is allowed only in package `vnext/continuity/sqlite`. A blank import of `github.com/ncruces/go-sqlite3/driver` is allowed only in file `vnext/continuity/sqlite/driver.go`. Path or prefix spoofing, non-blank driver imports, and every other ncruces or third-party import remain forbidden.
6. **Physical scratchpad prune is deferred to LOAF-97 sync safe-points.** Scratchpad facts are ephemeral in the catalog and retained on disk until that slice.

This is a vNext decision. ADR-014 chose Go and left the legacy SQLite driver to SPEC-040. ADR-029 is the shipped grow-only envelope for the current runtime. Neither authorizes vNext packages, schema identity, or the continuity API.

The kernel still does not open a database. The SQLite adapter and write chokepoint belong to the LOAF-96 continuity implementation; one-time archive migration remains a later slice.

## Consequences

### Positive

- Continuity can be queried, transactionally appended, and later synced without inventing a second storage engine.
- Read-time folds keep derived context honest: the digest cannot disagree with the fact log.
- The source gate can admit SQLite without opening a general third-party or `database/sql` allowlist.

### Negative

- vNext takes a SQLite dependency at the exact adapter file, so the bootstrap tree is no longer uniformly stdlib-only.
- Deferring physical scratchpad prune means ephemeral facts occupy disk until LOAF-97.

### Neutral

- Schema identity remains `vnext/1`; choosing an engine is not a schema bump.
- Legacy ADR-014/029 remain in force for the shipped line until cutover.

## Alternatives Considered

### Define the schema and API but defer the adapter

Specify domain types and SQL without executing them against a concrete store. This loses. It cannot prove durability, concurrent sequence allocation, constraint enforcement, unknown persisted-kind refusal, or deterministic snapshot reads. Those are the seams most likely to falsify the model, and deferring them would push LOAF-96's central decision into sync and migration.

### Stdlib append-only file store

Append JSON lines or a custom log under the standard library, with no SQLite driver. This loses. Continuity needs project-scoped retrieval, transactional append, crash-safe readers across environments, and an ordered fold. A hand-rolled log reimplements indexes, locking, and compaction that SQLite already provides, and it still cannot satisfy later E2E sync as well as a fact log the operator already knows how to copy and inspect.

### Reuse the legacy state packages or ADR-029 as the vNext runtime

This loses the isolation contract. vNext may learn from that behavior and consume a versioned one-time export; it cannot import those packages or treat the shipped envelope as its API.

## Revisions

- 2026-08-29 — Initial record.
