---
id: TASK-203
title: Implement state-backed report list read path
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T18:02:17Z'
updated: '2026-05-28T18:05:30Z'
completed_at: '2026-05-28T18:05:30Z'
depends_on:
  - TASK-202
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-203-state-backed-report-list-read-path.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  Go-native `loaf report list` reads reports from SQLite after migration,
  supports JSON/type/status filters, imports archived reports, and falls back
  to the TypeScript compatibility command while state is Markdown-only
---

# TASK-203: Implement state-backed report list read path

## Description

Add the next existing command-family SQLite read path: `loaf report list`.
When SQLite state is initialized, the Go front controller should serve report
list output from imported state. When state is still Markdown-only, the command
should keep delegating to the existing TypeScript compatibility implementation.

This task does not implement report creation, finalization, archive mutation,
or full output parity with every historical frontmatter field.

## Acceptance Criteria

- [x] `loaf report list --json` reads from SQLite after `loaf state migrate markdown --apply`.
- [x] JSON output includes imported report aliases, titles, kinds/types, statuses, and source file paths.
- [x] Human output groups state-backed reports by status and displays title, kind, and source path.
- [x] `--type <type>` filters by imported report kind.
- [x] `--status <status>` filters by status.
- [x] Archived reports are imported from `.agents/reports/archive`.
- [x] Markdown-only state delegates to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of silently falling back.
- [x] Tests cover JSON output, human output, type/status filters, archived import, invalid state, and fallback delegation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
