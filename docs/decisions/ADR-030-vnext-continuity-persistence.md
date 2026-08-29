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
  - ADR-031
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
5. **SQLite admission is exact.** `database/sql` is allowed only in package `vnext/continuity/sqlite`. A blank import of `github.com/ncruces/go-sqlite3/driver` is allowed only in file `vnext/continuity/sqlite/driver.go`. The Windows-only filesystem adapter may import `syscall` solely for `Win32FileAttributeData` and `FILE_ATTRIBUTE_REPARSE_POINT`. Path or prefix spoofing, aliased or dot imports, non-blank driver imports, extra `syscall` selectors, and every other ncruces or third-party import remain forbidden.
6. **Physical scratchpad prune is deferred to LOAF-97 sync safe-points.** Scratchpad facts are ephemeral in the catalog and retained on disk until that slice.
7. **A snapshot materializes one exact project corpus in one deferred read-only transaction.** The adapter strictly scans canonical rows in total order, commits the read transaction, releases the sole SQLite connection and store lock, and only then performs the cancellable in-memory fold. `AtMillis` evaluates scratchpad claim expiry only; the implementation never reads the current clock.
8. **Concurrent history converges by canonical causal closure and semantic class.** The least ordered mint is the record root. Successors of that root and its eligible descendants remain candidates even when they are sibling branches. The greatest candidate wins within a semantic class, while terminal classes such as idea disposition, decision supersession, finding retraction, scratchpad close, and claim release dominate later nonterminal candidates. Missing, cross-subject, future, or impossible-transition predecessors make the history corrupt rather than producing a partial projection.
9. **Derived context is one bounded projection, not another state model.** A valid focus must name an existing same-project subject, including a terminal or projection-hidden subject. Selection is exact and deterministic: focus, then scope, then branch, then project remainder, with subject deduplication before fixed per-layer caps. Scope applies only to journal entries, active sparks, decisions, and findings; branch applies only to the winning journal observation. Primary layers seed one-hop opaque references and direct verification evidence only after their caps, so omitted records cannot leak through attachments. One-hop records inherit their best selected target's precedence tier and retain snapshot order within a tier. A directly focused external reference forms a strict leading sub-tier before references that inherit focus precedence, even when none of its edges target a selected primary record, so the explicit focus cannot be capped out.

This is a vNext decision. ADR-014 chose Go and left the legacy SQLite driver to SPEC-040. ADR-029 is the shipped grow-only envelope for the current runtime. Neither authorizes vNext packages, schema identity, or the continuity API.

The kernel still does not open a database. The SQLite adapter and write chokepoint belong to the LOAF-96 continuity implementation; one-time archive migration remains a later slice.

## Consequences

### Positive

- Continuity can be queried, transactionally appended, and later synced without inventing a second storage engine.
- Read-time folds keep derived context honest: the digest cannot disagree with the fact log.
- Fixed layers and pre-cap availability counts make context omission explicit without persisting a cache or accepting caller-defined limits.
- Deferred read-only snapshots coexist with WAL writers and do not retain the database connection during CPU-heavy projection work.
- The source gate can admit SQLite without opening a general third-party or `database/sql` allowlist.

### Negative

- vNext takes a SQLite dependency at the exact adapter file, so the bootstrap tree is no longer uniformly stdlib-only.
- Deferring physical scratchpad prune means ephemeral facts occupy disk until LOAF-97.
- Filesystem checks are path-based. They do not defend against a hostile same-UID process racing path components between checks, or against a privileged process that can bypass filesystem policy.

### Neutral

- LOAF-96 establishes schema identity `vnext/1`; choosing an engine is not a schema bump. ADR-031 advances the same schema line to `vnext/2` when sync metadata and atomic remote receive arrive.
- Legacy ADR-014/029 remain in force for the shipped line until cutover.

### Filesystem Trust Boundary

The store targets one trusted operator account, not mutually hostile processes sharing an account.

- On POSIX, the state root must not be group- or world-writable, the private directory is mode `0700`, and database plus SQLite sidecar files are mode `0600`. Existing symlink components below the root-controlled top-level path component are rejected. A root-controlled top-level alias such as macOS `/var` to `/private/var` is trusted.
- On Windows, the state root must resolve beneath the local `LocalAppData` cache root. UNC paths and existing reparse-point components are rejected. The adapter trusts the operating system's inherited ACLs and does not independently prove the resulting DACL.
- Pre- and post-open identity checks reduce accidental path substitution but are not handle-based, no-follow traversal. Defending against same-UID time-of-check/time-of-use attacks would require a different filesystem authority model.
- Windows packages must cross-compile on every supported architecture. Native Windows runtime validation remains required because this macOS worktree cannot exercise reparse-point and ACL behavior; reparse regression tests may skip when the test account lacks symlink privilege.

## Alternatives Considered

### Define the schema and API but defer the adapter

Specify domain types and SQL without executing them against a concrete store. This loses. It cannot prove durability, concurrent sequence allocation, constraint enforcement, unknown persisted-kind refusal, or deterministic snapshot reads. Those are the seams most likely to falsify the model, and deferring them would push LOAF-96's central decision into sync and migration.

### Stdlib append-only file store

Append JSON lines or a custom log under the standard library, with no SQLite driver. This loses. Continuity needs project-scoped retrieval, transactional append, crash-safe readers across environments, and an ordered fold. A hand-rolled log reimplements indexes, locking, and compaction that SQLite already provides, and it still cannot satisfy later E2E sync as well as a fact log the operator already knows how to copy and inspect.

### Reuse the legacy state packages or ADR-029 as the vNext runtime

This loses the isolation contract. vNext may learn from that behavior and consume a versioned one-time export; it cannot import those packages or treat the shipped envelope as its API.

## Revisions

- 2026-08-29 — Initial record.
- 2026-08-29 — Pinned the Windows-only `syscall` admission and recorded filesystem authority, ACL, symlink-alias, race, and runtime-validation limits.
- 2026-08-29 — Pinned deferred read-only snapshot materialization, cancellable post-transaction folding, and branch-tolerant semantic-class convergence.
- 2026-08-29 — Pinned deterministic fixed-layer context selection, focus existence, and one-hop attachment boundaries.
- 2026-08-29 — Required one-hop records to inherit Focus, Scope, Branch, or project-remainder precedence before their own caps.
- 2026-08-29 — Reserved a leading external-reference sub-tier for the explicit reference focus so its own layer cannot cap it out.
- 2026-08-29 — Recorded ADR-031's later `vnext/2` sync migration without changing the LOAF-96 persistence decision.
