---
id: TASK-201
title: Implement state-backed spec list read path
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T17:50:33Z'
updated: '2026-05-28T17:54:26Z'
completed_at: '2026-05-28T17:54:26Z'
depends_on:
  - TASK-200
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-201-state-backed-spec-list-read-path.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  Go-native `loaf spec list` reads specs from SQLite after migration, supports
  JSON and human output with task counts, and falls back to the TypeScript
  compatibility command while state is Markdown-only
---

# TASK-201: Implement state-backed spec list read path

## Description

Add the next existing command-family SQLite read path: `loaf spec list`.
When SQLite state is initialized, the Go front controller should serve spec
list output from imported state. When state is still Markdown-only, the command
should keep delegating to the existing TypeScript compatibility implementation.

This task does not implement `spec show`, spec mutations/archive, session/report
read paths, or full output parity with every historical field in `TASKS.json`.

## Acceptance Criteria

- [x] `loaf spec list --json` reads from SQLite after `loaf state migrate markdown --apply`.
- [x] JSON output includes imported spec aliases, titles, statuses, source file paths, and task counts.
- [x] Human output groups state-backed specs by status and displays spec id, title, and task counts.
- [x] Markdown-only state delegates to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of silently falling back.
- [x] Tests cover JSON output, human output, task counts, invalid state, and fallback delegation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
