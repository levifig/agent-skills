---
id: TASK-229
title: Back up SQLite state database
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T21:11:27Z'
updated: '2026-05-28T21:16:45Z'
completed_at: '2026-05-28T21:16:45Z'
depends_on:
  - TASK-228
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-229-state-backup-sqlite-database.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf state backup` writes a timestamped
  SQLite database copy outside the repository, reports the backup path and byte
  count in human and JSON output, and refuses missing or invalid state without
  creating misleading backups.
---

# TASK-229: Back up SQLite state database

## Description

Add a Go-native `loaf state backup` lifecycle command for the SQLite state
database. Backups should stay under Loaf's XDG state project directory, not the
repository, and should be timestamped to avoid clobbering prior backups.

This task implements a database copy backup only. Generated Markdown exports
and JSON state exports remain separate Track D work.

## Acceptance Criteria

- [x] `loaf state backup` parses as a native Go state command.
- [x] `--json` output reports source database path, backup path, byte count, and creation timestamp.
- [x] Human output reports the same essential backup location and size information.
- [x] Backups are written outside the project repository under the project's state directory.
- [x] Backup filenames are timestamped and do not overwrite previous backups.
- [x] Missing SQLite state is refused with a clear initialization/migration message.
- [x] Invalid SQLite state is refused with a `loaf state doctor` message.
- [x] Tests cover state-layer backup creation, CLI JSON/human output, missing-state refusal, and invalid-state refusal.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run typecheck
npm run build
```

Built-binary smoke:

```bash
XDG_STATE_HOME=<temp> bin/loaf state init --json
XDG_STATE_HOME=<temp> bin/loaf state backup --json
```
