---
id: TASK-196
title: Add SQLite driver and state init/status/doctor storage
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T17:01:40Z'
updated: '2026-05-28T17:08:01Z'
depends_on:
  - TASK-195
files:
  - internal/state/
  - internal/cli/
  - go.mod
  - go.sum
  - .agents/tasks/TASK-196-state-status-doctor-diagnostics.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && go vet ./... &&
  go run golang.org/x/vuln/cmd/govulncheck@latest ./...
done: >-
  The approved ncruces SQLite driver is pinned, `loaf state init` applies
  Go-owned migrations, and `state status/doctor` report database readiness and
  schema version
completed_at: '2026-05-28T17:08:01Z'
---

# TASK-196: Add SQLite driver and state init/status/doctor storage

## Description

Add the approved `github.com/ncruces/go-sqlite3/driver` dependency and wire the
first real storage lifecycle commands. `loaf state init` should create the
project-scoped SQLite database outside the repository and apply Go-owned schema
migrations. `loaf state status` and `loaf state doctor` should report the
resolved project root, intended database path, DB presence, schema version, and
Markdown fallback state.

## Acceptance Criteria

- [x] `github.com/ncruces/go-sqlite3` is pinned in `go.mod`/`go.sum`.
- [x] `loaf state init` creates the project-scoped SQLite database outside the repository.
- [x] `state init` applies the ordered Go-owned migrations and records checksums in `schema_migrations`.
- [x] `state init` is idempotent and detects migration checksum drift.
- [x] `loaf state status` prints project root, database path, database presence, mode, and schema version.
- [x] `loaf state status --json` returns the same information as structured JSON.
- [x] Missing DB state reports `markdown-only` mode without creating files.
- [x] Existing migrated DB reports `sqlite-ready` mode after opening SQLite.
- [x] `loaf state doctor` detects missing DB and Markdown-only fallback mode.
- [x] `loaf state doctor --fix` initializes a missing DB by applying the same storage path as `state init`.
- [x] Tests cover init, status JSON, doctor output, missing DB, migrated DB, and checksum drift.
- [x] `go test ./...`, `go vet ./...`, and `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` pass.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
gofmt -w internal/state internal/cli
```
