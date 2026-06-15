---
id: TASK-234
title: Prove state init stays repository-external and secret-free
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T21:40:21Z'
updated: '2026-05-28T21:43:26Z'
completed_at: '2026-05-28T21:43:26Z'
depends_on:
  - TASK-233
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-234-state-init-safety-proof.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./...
done: >-
  Tests prove `loaf state init` creates a global project-partitioned SQLite database outside
  the repository, prints the database path, avoids repository `.agents` writes,
  and initializes a schema without secret-storage columns.
---

# TASK-234: Prove state init stays repository-external and secret-free

## Description

Close the SPEC-040 state-init safety condition with explicit tests. Existing
coverage proves initialization applies migrations; this task proves the public
command and schema satisfy the repository-boundary and no-secret guardrails.

This is verification hardening for the SQLite backend, not a behavior rewrite.

## Acceptance Criteria

- [x] `loaf state init` human output prints the SQLite database path.
- [x] The initialized database path is absolute and outside the working repository.
- [x] `loaf state init` does not create repository-local `.agents` state.
- [x] The generated SQLite schema has no secret-storage column names.
- [x] Tests cover the public CLI path and the schema guardrail.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
```
