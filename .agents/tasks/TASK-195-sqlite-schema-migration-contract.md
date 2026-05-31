---
id: TASK-195
title: Define initial SQLite schema migration contract
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T16:56:07Z'
updated: '2026-05-28T16:58:07Z'
depends_on:
  - TASK-194
files:
  - internal/state/
  - .agents/specs/SPEC-040-sqlite-backed-loaf-operational-state.md
  - .agents/tasks/TASK-195-sqlite-schema-migration-contract.md
verify: >-
  go test ./internal/state ./internal/cli && gofmt -w internal/state
done: >-
  Initial SQLite schema migration is represented in Go-owned state runtime code
  with tests covering the required table set, migration ordering, and no-secret
  storage guardrails
completed_at: '2026-05-28T16:58:07Z'
---

# TASK-195: Define initial SQLite schema migration contract

## Description

Add the first SQLite schema migration contract in the Go state runtime without
adding the SQLite driver dependency yet. This gives Track A a tested schema
surface that the next task can apply through the selected driver after explicit
dependency approval.

Keep this task dependency-free: no production SQLite imports and no new modules.

## Acceptance Criteria

- [x] Go code exposes ordered state schema migrations.
- [x] The first migration creates `schema_migrations` and the SPEC-040 initial table set.
- [x] Core operational tables include stable IDs plus created/updated timestamps.
- [x] Project-scoped tables constrain `project_id` with database-level foreign keys.
- [x] Relationship/provenance tables preserve source links and generated export metadata.
- [x] Backend mappings do not define token, password, key, secret, or credential columns.
- [x] Tests lock migration ordering, table coverage, and no-secret schema guardrails.
- [x] `go test ./internal/state ./internal/cli` passes.

## Verification

```bash
go test ./internal/state ./internal/cli
gofmt -w internal/state
```
