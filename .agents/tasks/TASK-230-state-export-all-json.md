---
id: TASK-230
title: Export complete SQLite state as internal JSON
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T21:18:31Z'
updated: '2026-05-28T21:23:35Z'
completed_at: '2026-05-28T21:23:35Z'
depends_on:
  - TASK-229
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-230-state-export-all-json.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf state export all --format json` writes
  a deterministic internal JSON snapshot to stdout with export metadata, schema
  version, table rows, and an explicit internal-audience marker without creating
  repository files.
---

# TASK-230: Export complete SQLite state as internal JSON

## Description

Start SPEC-040 Track D with an internal JSON export. The command should dump the
current SQLite state to stdout as structured JSON so humans and agents can
inspect or hand off the full state without copying the SQLite file.

This task intentionally implements only `loaf state export all --format json`.
Markdown exports, external-targeted leak validation, writing export files, and
recording generated artifact rows in the `exports` table remain later Track D
work.

## Acceptance Criteria

- [x] `loaf state export all --format json` parses as a native Go state command.
- [x] Output is valid JSON and includes `export_kind`, `format`, `audience`, `generated_at`, `project_id`, `database_path`, and `schema_version`.
- [x] Output marks the export as internal so internal IDs may appear without pretending to be external-safe.
- [x] Output includes deterministic table snapshots for core SQLite state tables.
- [x] Re-running the command against unchanged SQLite state does not mutate the database or create repository files.
- [x] Missing SQLite state is refused with a clear initialization/migration message.
- [x] Invalid SQLite state is refused with a `loaf state doctor` message.
- [x] Unsupported export kinds and formats return clear errors.
- [x] Tests cover state-layer export shape, deterministic repeated exports, CLI JSON output, missing/invalid state, and unsupported kind/format errors.

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
XDG_STATE_HOME=<temp> bin/loaf state export all --format json
```
