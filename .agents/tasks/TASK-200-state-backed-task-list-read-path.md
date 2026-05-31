---
id: TASK-200
title: Implement state-backed task list read path
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T17:42:47Z'
updated: '2026-05-28T17:48:18Z'
completed_at: '2026-05-28T17:48:18Z'
depends_on:
  - TASK-199
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-200-state-backed-task-list-read-path.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  Go-native `loaf task list` reads tasks from SQLite after migration, supports
  --json/--active/--status, and falls back to the TypeScript compatibility
  command while state is Markdown-only
---

# TASK-200: Implement state-backed task list read path

## Description

Add the first existing command-family SQLite read path: `loaf task list`.
When SQLite state is initialized, the Go front controller should serve task
list output from imported state. When state is still Markdown-only, the command
should keep delegating to the existing TypeScript compatibility implementation.

This task does not implement `task show`, task mutations, spec/session/report
list, or full output parity with every historical field in `TASKS.json`.

## Acceptance Criteria

- [x] `loaf task list --json` reads from SQLite after `loaf state migrate markdown --apply`.
- [x] JSON output includes imported task aliases, titles, statuses, priorities, spec aliases, source file paths, and dependency aliases.
- [x] Human output groups state-backed tasks by status and displays task id, priority, title, and spec alias.
- [x] `--active` hides done tasks.
- [x] `--status <status>` filters to one status and composes with `--active`.
- [x] Markdown-only state delegates to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of silently falling back.
- [x] Tests cover JSON output, human output, active/status filters, dependency aliases, and fallback delegation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
