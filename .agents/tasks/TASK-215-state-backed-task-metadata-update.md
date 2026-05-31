---
id: TASK-215
title: Update task metadata in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T19:29:26Z'
updated: '2026-05-28T19:37:54Z'
completed_at: '2026-05-28T19:37:54Z'
depends_on:
  - TASK-214
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-215-state-backed-task-metadata-update.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf task update <task>` updates priority,
  spec, dependencies, and session relationships in SQLite while preserving
  status-update behavior and Markdown compatibility fallback.
---

# TASK-215: Update task metadata in SQLite state

## Description

Extend the SQLite-backed task mutation path beyond status changes. When SQLite
state is initialized, `loaf task update <task>` should handle the existing
metadata options that previously remained delegated: `--priority`, `--spec`,
`--depends-on`, and `--session`.

Status updates from TASK-212 remain supported. Markdown-only projects continue
delegating the entire task update command to the TypeScript compatibility
command.

This task does not generate Markdown compatibility files, archive tasks,
refresh task indexes, or sync external trackers.

## Acceptance Criteria

- [x] `loaf task update <task> --priority <priority>` updates `tasks.priority`.
- [x] `loaf task update <task> --spec <spec>` resolves a spec alias, updates `tasks.spec_id`, and replaces the task's `implements` relationship.
- [x] `loaf task update <task> --spec none` clears `tasks.spec_id` and removes the task's `implements` relationship.
- [x] `loaf task update <task> --depends-on <ids>` resolves task aliases and replaces the task's `blocked_by` relationships.
- [x] `loaf task update <task> --depends-on none` clears task dependencies.
- [x] `loaf task update <task> --session <session>` resolves a session alias and replaces the task's session relationship.
- [x] `loaf task update <task> --session none` clears task session relationships.
- [x] Status updates from TASK-212 still mutate status and record status-change events.
- [x] `loaf task list`, `loaf task show`, and `loaf trace` reflect updated priority/spec/dependencies/session relationships.
- [x] Markdown-only state delegates `task update` to the TypeScript compatibility command before Go-side option validation.
- [x] Invalid priority, missing refs, wrong-kind refs, empty updates, and invalid SQLite state fail clearly.
- [x] Tests cover priority, spec set/clear, dependencies set/clear, session set/clear, composed updates, validation, invalid state, and fallback delegation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
go vet ./...
npm run typecheck
npm run build
```

Built-binary smoke:

```bash
XDG_STATE_HOME=<temp> bin/loaf state migrate markdown --apply --json
XDG_STATE_HOME=<temp> bin/loaf task update TASK-001 --priority P0 --spec SPEC-002 --depends-on TASK-002 --session 20260528-session --json
XDG_STATE_HOME=<temp> bin/loaf task show TASK-001 --json
XDG_STATE_HOME=<temp> bin/loaf trace TASK-001 --json
XDG_STATE_HOME=<temp> bin/loaf task update TASK-001 --spec none --depends-on none --session none
XDG_STATE_HOME=<temp> bin/loaf task show TASK-001 --json
```
