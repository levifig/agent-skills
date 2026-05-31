---
id: TASK-197
title: Implement markdown migration dry-run preview
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T17:16:01Z'
updated: '2026-05-28T17:20:22Z'
depends_on:
  - TASK-196
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-197-markdown-migration-dry-run-preview.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  Go-native `loaf state migrate markdown --dry-run` previews non-destructive
  .agents import counts, inferred relationships, skipped files, and leaves
  SQLite state untouched
---

# TASK-197: Implement markdown migration dry-run preview

## Description

Add the first Track B import command surface: a non-destructive markdown
migration dry-run. The command should inspect the resolved project's `.agents/`
tree and report import counts without creating or mutating SQLite state.

This task does not implement `--apply`, row insertion, or full trace queries.

## Acceptance Criteria

- [x] `loaf state migrate markdown --dry-run` reports counts for specs, tasks, ideas, sparks, sessions, reports, relationships, and skipped files.
- [x] `loaf state migrate markdown --json` returns the same dry-run information as structured JSON.
- [x] Dry-run resolves `.agents/` from the project root and does not create a database file.
- [x] Task dependency relationships are inferred from task frontmatter / `TASKS.json` where available.
- [x] Spark counts are inferred from session journal entries using the compact `spark(scope): ...` convention.
- [x] Unknown regular files under `.agents/` are reported as skipped with repo-relative paths.
- [x] `--apply` is explicitly rejected until the apply importer is implemented.
- [x] Tests cover populated fixtures, JSON output, skipped files, no `.agents/` directory, and no state DB mutation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
