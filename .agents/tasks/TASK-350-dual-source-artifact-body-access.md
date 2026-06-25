---
id: TASK-350
title: Implement dual-source artifact body access
spec: SPEC-043
status: done
priority: P1
created: '2026-06-24T13:04:19Z'
updated: '2026-06-24T13:28:11Z'
completed_at: '2026-06-24T13:28:11Z'
depends_on:
  - TASK-349
files:
  - internal/state/
  - internal/state/markdown_import.go
  - internal/state/*_show.go
  - .agents/tasks/TASK-350-dual-source-artifact-body-access.md
verify: >-
  go test ./internal/state -run 'ArtifactBody|MarkdownMigration|Show|Trace' -count=1
done: >-
  Body-capable entities read SQLite artifact bodies first, fall back to imported
  Markdown sources, and keep FTS rows synchronized by Go-side writes in the same
  transaction.
---

# TASK-350: Implement dual-source artifact body access

## Description

Add SPEC-043 Track 1: shared body read/write helpers, Markdown import backfill
into `artifact_bodies`, dual-source read precedence, and same-transaction FTS
maintenance.

## Acceptance Criteria

- [x] Shared state helpers upsert/read/delete `artifact_bodies` rows by project, entity kind, entity id, and body kind.
- [x] Body reads prefer SQLite `artifact_bodies` and fall back to `body_source_id` Markdown files when no SQLite body exists.
- [x] Markdown migration imports current artifact bodies into `artifact_bodies` without mutating Markdown files.
- [x] FTS rows are upserted/deleted from the same Go transaction as body writes; no triggers maintain FTS.
- [x] Existing `show`/`trace` behavior for specs, tasks, ideas, brainstorms, sessions, reports, and sparks does not regress.
- [x] Multi-paragraph imported bodies round-trip byte-exact.

## Verification

```bash
go test ./internal/state -run 'ArtifactBody|MarkdownMigration|Show|Trace' -count=1
```
