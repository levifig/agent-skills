---
id: TASK-212
title: Update task status in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T19:07:33Z'
updated: '2026-05-28T19:12:13Z'
completed_at: '2026-05-28T19:12:13Z'
depends_on:
  - TASK-211
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-212-state-backed-task-status-update.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf task update <task> --status <status>`
  updates the task row, records a status-change event when appropriate, and
  leaves Markdown compatibility to fallback/export paths.
---

# TASK-212: Update task status in SQLite state

## Description

Add the first existing task mutation path over SQLite state: task status
updates. This narrows SPEC-040 Track C by moving a common lifecycle mutation
away from `TASKS.json`/Markdown frontmatter and into the state store.

This task covers `loaf task update <task> --status <status>` when SQLite state
is initialized.

Markdown-only projects continue delegating to the TypeScript compatibility
command. Non-status task updates continue delegating for now.

This task does not implement task create, archive, priority/spec/session
mutation, dependency replacement, generated Markdown compatibility writes, or
external tracker sync.

## Acceptance Criteria

- [x] `loaf task update <task> --status <status>` resolves task aliases from SQLite state.
- [x] Valid status updates mutate `tasks.status` and `updated_at`.
- [x] Status changes record one `events` row with previous and new status.
- [x] Repeating the same status update is idempotent and does not duplicate status-change events.
- [x] `loaf task list --json` and `loaf trace <task>` reflect the updated status.
- [x] Markdown-only state delegates task update commands to the TypeScript compatibility command.
- [x] Non-status task updates remain delegated until separately ported.
- [x] Invalid SQLite state reports the state error instead of silently falling back.
- [x] Tests cover update, idempotency, trace/list integration, invalid state, and fallback delegation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
