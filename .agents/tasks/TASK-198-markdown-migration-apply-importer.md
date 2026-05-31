---
id: TASK-198
title: Implement markdown migration apply importer
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T17:27:44Z'
updated: '2026-05-28T17:34:54Z'
depends_on:
  - TASK-197
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-198-markdown-migration-apply-importer.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  Go-native `loaf state migrate markdown --apply` initializes SQLite when
  needed and imports current .agents specs, tasks, ideas, sparks, brainstorms,
  sessions, reports, relationships, and source links without deleting or
  rewriting Markdown
---

# TASK-198: Implement markdown migration apply importer

## Description

Add the first write-side markdown importer for SPEC-040. `--apply` should
initialize the project SQLite database when needed, import structurally knowable
`.agents/` artifacts, preserve source links, and remain non-destructive toward
the Markdown tree.

This task does not implement `--resume`, full archive import, state-backed
`list/show`, or `loaf trace`.

## Acceptance Criteria

- [x] `loaf state migrate markdown --apply` initializes the SQLite database when missing and applies current migrations.
- [x] Apply imports specs, tasks, ideas, brainstorms, sessions, reports, sparks, task-to-spec relationships, and task dependency relationships from a fixture.
- [x] Apply preserves source file paths and hashes in `sources`.
- [x] Apply is idempotent for the same `.agents/` fixture.
- [x] Apply does not delete, rewrite, or move Markdown source files.
- [x] `--json` returns structured apply output including database path and imported counts.
- [x] `--dry-run` remains non-mutating.
- [x] Tests cover apply import rows, source links, relationships, idempotency, and source-file preservation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
