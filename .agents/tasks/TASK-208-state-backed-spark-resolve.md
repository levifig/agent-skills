---
id: TASK-208
title: Resolve imported sparks in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T18:36:07Z'
updated: '2026-05-28T18:39:51Z'
completed_at: '2026-05-28T18:39:51Z'
depends_on:
  - TASK-207
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-208-state-backed-spark-resolve.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf spark resolve <spark> --by <target>`
  marks the spark resolved, records the resolver relationship, and the default
  spark list no longer shows the resolved spark.
---

# TASK-208: Resolve imported sparks in SQLite state

## Description

Add the state-backed triage closure command for sparks. When SQLite state is
initialized, `loaf spark resolve <spark> --by <target>` resolves an imported
spark alias or internal row ID, writes a `resolved_by` relationship to an
existing target, records a status-change event, and updates the spark status to
`resolved`.

`loaf spark list` should read from SQLite and omit resolved sparks by default
so the same spark does not resurface in triage. `--all` includes resolved sparks
for auditability. Markdown-only projects continue delegating to the TypeScript
compatibility command.

This task does not implement spark capture/promote, idea resolution, generated
exports, tags, bundles, or Markdown compatibility writes.

## Acceptance Criteria

- [x] `loaf spark list` reads imported sparks from SQLite when state is initialized.
- [x] The default spark list excludes sparks with `resolved` status.
- [x] `loaf spark list --all` includes resolved sparks.
- [x] `loaf spark resolve <spark> --by <target>` updates the spark status to `resolved`.
- [x] Resolving a spark records a `resolved_by` relationship to the target entity.
- [x] Resolving a spark records a status-change event.
- [x] Markdown-only state delegates spark commands to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of silently falling back.
- [x] Tests cover state-backed list, resolve, trace relationship, event recording, invalid state, and fallback delegation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
