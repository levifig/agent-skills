---
id: TASK-199
title: Implement state-backed trace for imported relationships
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T17:36:48Z'
updated: '2026-05-28T17:41:07Z'
depends_on:
  - TASK-198
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-199-state-backed-trace-imported-relationships.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  Go-native `loaf trace <id>` resolves imported aliases from SQLite and shows
  entity metadata, source provenance, and inbound/outbound relationships for
  imported tasks/specs
---

# TASK-199: Implement state-backed trace for imported relationships

## Description

Add the first state-backed read model command for SPEC-040: `loaf trace <id>`.
This task should make imported task/spec relationship data inspectable from
SQLite using human-facing aliases such as `TASK-001` and `SPEC-001`.

This does not implement the full future spark -> idea -> spec/task resolution
graph, `link create`, or state-backed list/show commands.

## Acceptance Criteria

- [x] `loaf trace TASK-...` resolves imported task aliases from SQLite.
- [x] Trace output includes entity kind, alias, title/status where available, and source file path/hash.
- [x] Trace output includes outbound task-to-spec and task dependency relationships imported from `.agents/TASKS.json`.
- [x] `loaf trace SPEC-...` includes inbound task relationships.
- [x] `loaf trace <id> --json` returns the same information as structured JSON.
- [x] Missing SQLite state fails with an actionable initialization/migration message.
- [x] Tests cover task trace, spec inbound trace, JSON output, dependency alias display, and missing DB behavior.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
