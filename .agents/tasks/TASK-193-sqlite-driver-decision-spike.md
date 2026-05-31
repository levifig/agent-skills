---
id: TASK-193
title: Decide Go SQLite driver and security posture
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T15:13:09Z'
updated: '2026-05-28T16:21:46Z'
depends_on:
  - TASK-190
files:
  - docs/decisions/
  - .agents/specs/SPEC-040-sqlite-backed-loaf-operational-state.md
  - go.mod
verify: >-
  go test ./... && go vet ./... && go list -m all
done: >-
  SQLite driver decision is documented with cgo, cross-compilation, dependency,
  binary-size, maintenance, and vulnerability criteria before Track A storage
  code depends on it
completed_at: '2026-05-28T16:21:46Z'
---

# TASK-193: Decide Go SQLite driver and security posture

## Description

Run the SQLite driver decision spike for the Go runtime. Compare viable Go
SQLite options against SPEC-040 and ADR-014 criteria, then record the selected
driver and rejected alternatives before Track A introduces real storage code.

This task may create a short ADR if the selected driver is architecturally
significant enough on its own; otherwise, document the decision in SPEC-040 or a
Track 0 report. Do not add the driver to production code until the decision is
made and the verification evidence is captured.

## Acceptance Criteria

- [x] Compare at least cgo-based `mattn/go-sqlite3`, cgo-free `modernc.org/sqlite`, and cgo-free `ncruces/go-sqlite3` or equivalent current candidates.
- [x] Evaluate cgo policy, cross-compilation, dependency count, binary size, maintenance health, vulnerability surface, and testability.
- [x] Run `go list -m all` for any candidate prototype and record dependency impact.
- [x] Decide whether the driver choice requires a new ADR or can live in SPEC-040 implementation notes.
- [x] Update SPEC-040 open questions or add the ADR/report so Track A has a clear driver decision.
- [x] No Linear tokens, auth secrets, or unrelated runtime state are introduced.

## Context

ADR-014 chooses Go for the stateful runtime but deliberately does not choose a
SQLite driver. SPEC-040 Track 0 must close that question before SQLite storage
helpers are implemented.

## Verification

```bash
go test ./...
go vet ./...
go list -m all
```
