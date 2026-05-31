---
id: TASK-233
title: Detect schema mismatch and stale exports in state doctor
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T21:35:30Z'
updated: '2026-05-28T21:39:00Z'
completed_at: '2026-05-28T21:39:00Z'
depends_on:
  - TASK-232
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-233-state-doctor-diagnostics.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./...
done: >-
  `loaf state doctor` detects missing databases, markdown fallback mode, schema
  mismatch, and stale generated export records without mutating state unless
  `--fix` is explicitly requested.
---

# TASK-233: Detect schema mismatch and stale exports in state doctor

## Description

Complete the SPEC-040 doctor diagnostic surface for the SQLite runtime. Missing
database and markdown fallback diagnostics already exist; add read-only detection
for schema mismatch and stale compatibility export records.

Schema mismatch includes unknown future schema versions and checksum drift for
known Go-owned migrations. Stale exports are recorded `exports` rows whose
source entity has changed after the export was generated.

## Acceptance Criteria

- [x] `loaf state doctor` reports missing DB and markdown fallback mode without creating files.
- [x] `loaf state doctor` detects schema-version mismatch.
- [x] `loaf state doctor` detects known migration checksum drift.
- [x] `loaf state doctor` detects stale generated export records.
- [x] Stale export diagnostics are warnings and do not make an otherwise-ready database invalid.
- [x] `loaf state doctor --fix` still initializes a missing database.
- [x] Tests cover state-layer diagnostics and CLI output/JSON behavior.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
```
