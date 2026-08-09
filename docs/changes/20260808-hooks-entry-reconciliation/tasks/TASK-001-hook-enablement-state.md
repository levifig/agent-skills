---
change: hooks-entry-reconciliation
id: TASK-001
title: Hook-enablement state
blocks:
  - TASK-002
  - TASK-003
---

# TASK-001 — Hook-enablement state

## Objective

A user-scoped enablement table plus per-target absorption markers in the global SQLite state, with accessors the reconciler and verb surface build on: absence of a record means enabled, a `disabled` record suppresses projection, absorption provenance is immutable, and the absorb-and-mark write is transactional.

## Scope boundaries

**In:** `internal/state` — new table(s), schema creation, accessors (get/set/list per target, absorption-marker read/write, transactional absorb-and-mark), tests.

**Out:** Any reconciliation logic or file I/O against hooks.json (TASK-002); the CLI verb surface (TASK-003); every existing table.

## Context pointers

- Contract: `shape.md` — Decisions 2, 4, 5, 10; Planning Contract → Absorption and migration marker, Placement.
- Identity discipline: state-dedupe `decision(identity)` journal entry of 2026-08-08 — mint-once opaque IDs, REAL UNIQUE constraint on the natural key, never derive keys from mutable inputs.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-001 — hook-enablement state table"
# Read internal/state schema setup and an existing table's accessor pattern before adding the new one.
```

## Steps

- [x] Add the enablement table: opaque mint-once ID, `target`, `event`, `hook_id`, enablement value, immutable `absorbed_at` (set once, never updated), `updated_at`, `UNIQUE(target, event, hook_id)`. No `project_id` — records are user-scoped and host-local.
- [x] Add the per-target absorption marker record (target, absorbed-from version, timestamp) — durable, independent of installed-manifest rows.
- [x] Add the per-target trusted-executable path record (current install path plus previously recorded paths) with its accessors — the authority Decision 6's resolved-path recognition reads. The installer-side write happens in TASK-002; this task delivers schema and accessors only.
- [x] Accessors: read effective enablement (absence → enabled), set enabled/disabled (upsert through the unique key, never re-mint on conflict, `absorbed_at` untouched by toggles), list by target, and a transactional absorb-and-mark write that commits the disabled records and the marker atomically.
- [x] Tombstone semantics: records for hook IDs no longer shipped are retained and inert; no deletion path exists.
- [x] Tests: default-enabled semantics, upsert idempotency, uniqueness enforcement, `absorbed_at` immutability across toggles, transactional absorb-and-mark (all-or-nothing under injected failure), isolation via temp DB per existing `internal/state` test conventions.

## Verification

- `go test ./internal/state/...` passes with the new coverage.
- A duplicate natural-key insert is impossible by construction (constraint test, not application-logic test).
- An injected failure inside absorb-and-mark leaves neither records nor marker behind.
