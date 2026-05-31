---
id: TASK-202
title: Implement state-backed session list read path
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T17:55:38Z'
updated: '2026-05-28T18:01:00Z'
completed_at: '2026-05-28T18:01:00Z'
depends_on:
  - TASK-201
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-202-state-backed-session-list-read-path.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  Go-native `loaf session list` reads sessions from SQLite after migration,
  supports JSON and --all archived output, and falls back to the TypeScript
  compatibility command while state is Markdown-only
---

# TASK-202: Implement state-backed session list read path

## Description

Add the next existing command-family SQLite read path: `loaf session list`.
When SQLite state is initialized, the Go front controller should serve session
list output from imported state. When state is still Markdown-only, the command
should keep delegating to the existing TypeScript compatibility implementation.

This task does not implement `session show`, `session log`, enrichment/report
commands, or full output parity with every historical timestamp field.

## Acceptance Criteria

- [x] `loaf session list --json` reads from SQLite after `loaf state migrate markdown --apply`.
- [x] JSON output includes imported session aliases, branches, statuses, harness session IDs, source file paths, and journal entry counts.
- [x] Human output lists active/non-archived sessions and displays branch plus source path.
- [x] `--all` includes archived sessions imported from `.agents/sessions/archive`.
- [x] Markdown-only state delegates to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of silently falling back.
- [x] Tests cover JSON output, human output, `--all`, archived import, invalid state, and fallback delegation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
