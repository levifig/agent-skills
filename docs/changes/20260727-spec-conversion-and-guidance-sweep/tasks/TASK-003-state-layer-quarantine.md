---
change: spec-conversion-and-guidance-sweep
id: TASK-003
title: State-layer quarantine
blocked-by:
  - TASK-002
---

# TASK-003 — State-layer quarantine

## Objective

The migration is the legacy tables' only reader: spec/task writers and readers are removed from `internal/state/`, the entity registry and lifecycle vocabulary no longer name them, and derived surfaces (export, housekeeping, render sweep, journal-context digest) stop projecting them.

## Scope boundaries

**In:** The 11 dedicated `spec_*.go`/`task_*.go` files; `entity_registry.go` and `lifecycle_status.go` entries; `export.go`, `housekeeping.go`, `status.go`, `trace.go`, `link.go`, alias machinery legacy paths; `durable_render*` spec paths; the `transitional_tasks` journal-context layer and its cursor; `docs/schema/` README + DBML/MMD diagrams annotated as quarantined; associated state tests.

**Out:** Schema drops or `0001_initial.sql` edits beyond what TASK-001's migration required (Rabbit Holes: quarantine is not cleanup). Historical `SPEC-*`/`TASK-*` text in journal entries stays as text.

## Context pointers

- Contract: `shape.md` — Decisions 2 and 11, Rabbit Holes ("Entity-registry ripples")

## Acquisition

```bash
loaf journal log "skill(implement): TASK-003 — state-layer quarantine"
```

## Steps

- [ ] Remove legacy writers/readers; migration reader survives as the sole access path
- [ ] Deregister `spec`/`task` from entity registry and lifecycle vocabulary; text mentions in history unaffected
- [ ] Converge export, housekeeping, render sweep, trace/link/alias paths
- [ ] Remove the `transitional_tasks` digest layer and cursor
- [ ] Annotate schema docs: tables quarantined, zero-row cleanup later

## Verification

- `go test ./internal/state ./internal/cli` green
- No non-migration code path issues `INSERT/UPDATE/DELETE` against `specs`/`tasks`
