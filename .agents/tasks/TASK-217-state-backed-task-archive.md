---
id: TASK-217
title: Archive tasks in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T19:48:48Z'
updated: '2026-05-28T19:57:10Z'
completed_at: '2026-05-28T19:57:10Z'
depends_on:
  - TASK-216
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-217-state-backed-task-archive.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf task archive` archives done tasks in
  SQLite by ID or spec, records status-change events, reports skipped tasks
  clearly, and preserves Markdown compatibility fallback.
---

# TASK-217: Archive tasks in SQLite state

## Description

Port the existing `loaf task archive` mutation to SQLite-backed state. When
SQLite state is initialized, archiving a task should no longer move Markdown
files or mutate `TASKS.json`; it should transition done tasks to an archived
state, record status-change events, and let list/show/trace read the new state.

Markdown-only state continues delegating the command to the TypeScript
compatibility implementation. This task does not implement generated Markdown
compatibility exports or physical movement of task files under
`.agents/tasks/archive/`.

## Acceptance Criteria

- [x] `loaf task archive <task...>` resolves task aliases from SQLite state.
- [x] `loaf task archive --spec <spec>` resolves a spec alias and archives done tasks associated with that spec.
- [x] Tasks must be `done` before they can transition to `archived`.
- [x] Already archived tasks are reported as skipped without rewriting state.
- [x] Missing refs and wrong-kind refs are reported as skipped entries when direct task IDs are provided.
- [x] Successful archives update `tasks.status`, `tasks.updated_at`, and record status-change events.
- [x] `loaf task list --active`, `loaf task list --status archived`, `loaf task show`, and `loaf trace` reflect archived status after archive.
- [x] `--json` output exposes archived and skipped result rows.
- [x] Markdown-only state delegates `task archive` to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of falling back.
- [x] Tests cover task IDs, `--spec`, skip conditions, JSON/human output, fallback delegation, and invalid state.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run typecheck
npm run build
```

Built-binary smoke:

```bash
XDG_STATE_HOME=<temp> bin/loaf state migrate markdown --apply --json
XDG_STATE_HOME=<temp> bin/loaf task archive TASK-001 TASK-002 SPEC-001 TASK-999 --json
XDG_STATE_HOME=<temp> bin/loaf task list --json --status archived
XDG_STATE_HOME=<temp> bin/loaf task list --json --active
XDG_STATE_HOME=<temp> bin/loaf task show TASK-001 --json
XDG_STATE_HOME=<temp> bin/loaf trace TASK-001 --json
XDG_STATE_HOME=<temp> bin/loaf task archive --spec SPEC-001 --json
XDG_STATE_HOME=<temp> bin/loaf task archive --spec SPEC-001
```
