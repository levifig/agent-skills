---
id: TASK-214
title: Create tasks in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T19:22:49Z'
updated: '2026-05-28T19:27:59Z'
completed_at: '2026-05-28T19:27:59Z'
depends_on:
  - TASK-213
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-214-state-backed-task-create.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf task create --title ...` creates a
  task row with a generated task alias, optional spec relationship, optional
  dependencies, a status-change creation event, and Markdown compatibility
  fallback.
---

# TASK-214: Create tasks in SQLite state

## Description

Add a state-backed task creation path. When SQLite state is initialized, `loaf
task create --title "..."` should create the task in SQLite instead of writing
`TASKS.json` and task Markdown as the source of truth.

This task covers the existing task-create options that are meaningful in
SQLite state: `--title`, optional `--spec`, optional `--priority`, optional
`--depends-on`, and `--json`.

Markdown-only projects continue delegating to the TypeScript compatibility
command.

This task does not generate Markdown compatibility files, sync external
trackers, create task body prose, or implement task archive/update fields beyond
status.

## Acceptance Criteria

- [x] `loaf task create --title <title>` creates a task row in SQLite when state is initialized.
- [x] Created tasks receive the next available `TASK-NNN` alias from SQLite task aliases.
- [x] Created tasks default to `todo` status and `P2` priority.
- [x] `--spec <spec>` resolves a spec alias and records both `tasks.spec_id` and an `implements` relationship.
- [x] `--depends-on <ids>` resolves dependency task aliases and records `blocked_by` relationships.
- [x] Invalid priority, missing title, missing spec, and missing dependencies fail clearly.
- [x] JSON and human output report the created task alias, title, status, priority, spec, dependencies, and event ID.
- [x] `loaf task list`, `loaf task show`, and `loaf trace` reflect the created task.
- [x] Markdown-only state delegates `task create` to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of silently falling back.
- [x] Tests cover create defaults, spec/dependencies, list/show/trace integration, validation, invalid state, and fallback delegation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
