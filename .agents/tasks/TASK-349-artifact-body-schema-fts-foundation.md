---
id: TASK-349
title: Add artifact body schema and FTS foundation
spec: SPEC-043
status: done
priority: P1
created: '2026-06-24T13:04:19Z'
updated: '2026-06-24T13:16:26Z'
completed_at: '2026-06-24T13:16:26Z'
depends_on: []
files:
  - internal/state/schema.go
  - internal/state/migrations/
  - internal/state/schema_test.go
  - internal/state/status.go
  - docs/schema/
  - .agents/tasks/TASK-349-artifact-body-schema-fts-foundation.md
verify: >-
  go test ./internal/state -run 'Schema|FTS|Status' -count=1 && CGO_ENABLED=0
  go test ./internal/state -run 'FTS|Schema' -count=1
done: >-
  Schema migration adds artifact_bodies, artifact_search FTS5, plan/handoff/council
  tables, schema docs, and state-doctor entity visibility without new Go module
  dependencies.
---

# TASK-349: Add artifact body schema and FTS foundation

## Description

Add SPEC-043 Track 0: the SQLite schema foundation for artifact bodies and
Tier-1 search. This is additive and non-breaking.

## Acceptance Criteria

- [x] A new schema migration adds `artifact_bodies` exactly as locked by the shared contract.
- [x] FTS5-backed `artifact_search` exists through SQL DDL, with no new Go import or module dependency.
- [x] `plan`, `handoff`, and `council` storage tables exist with project scope, stable IDs, timestamps, and useful indexes.
- [x] `status.go` entity-kind allowlists and local entity CTEs include the new entities/tables.
- [x] `docs/schema` SQL, DBML, and Mermaid docs mirror executable migrations.
- [x] A `CGO_ENABLED=0` test proves `CREATE VIRTUAL TABLE ... USING fts5` works with the embedded driver.

## Verification

```bash
go test ./internal/state -run 'Schema|FTS|Status' -count=1
CGO_ENABLED=0 go test ./internal/state -run 'FTS|Schema' -count=1
```
