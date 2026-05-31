---
id: TASK-213
title: Show SQLite-backed task details
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T19:15:12Z'
updated: '2026-05-28T19:20:24Z'
completed_at: '2026-05-28T19:20:24Z'
depends_on:
  - TASK-212
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-213-state-backed-task-show.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf task show <task>` reads the task row
  from SQLite with aliases, spec relationship, dependencies, provenance, and
  Markdown compatibility fallback.
---

# TASK-213: Show SQLite-backed task details

## Description

Add the state-backed read path for inspecting a single task. When SQLite state
is initialized, `loaf task show <task>` should resolve task aliases through the
state store and report the task's operational metadata without reading
`TASKS.json` or task Markdown as the authority.

Markdown-only projects continue delegating to the TypeScript compatibility
command.

This task does not implement task creation, archival, dependency mutation,
generated Markdown compatibility writes, or external tracker sync.

## Acceptance Criteria

- [x] `loaf task show <task>` resolves task aliases from SQLite state.
- [x] JSON output includes task alias, title, status, priority, timestamps, spec alias, dependency aliases, source provenance, and body/prose when imported.
- [x] Human output shows the same operational details in a compact readable format.
- [x] Showing a missing or non-task alias exits non-zero with a clear error.
- [x] Markdown-only state delegates `task show` to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of silently falling back.
- [x] Tests cover JSON output, human output, dependency/spec/source fields, missing alias, non-task alias, invalid state, and fallback delegation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
