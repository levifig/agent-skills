---
id: TASK-207
title: Resolve imported ideas in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T18:29:36Z'
updated: '2026-05-28T18:34:37Z'
completed_at: '2026-05-28T18:34:37Z'
depends_on:
  - TASK-206
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-207-state-backed-idea-resolve.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf idea resolve <idea> --by <target>`
  marks the idea resolved, records the resolver relationship, and the default
  idea list no longer shows the resolved idea.
---

# TASK-207: Resolve imported ideas in SQLite state

## Description

Add the first state-backed triage closure command for ideas. When SQLite state
is initialized, `loaf idea resolve <idea> --by <target>` resolves an imported
idea alias or internal row ID, writes a `resolved_by` relationship to an
existing target, records a status-change event, and updates the idea status to
`resolved`.

`loaf idea list` should read from SQLite and omit resolved ideas by default so
the same raw idea does not resurface in triage. `--all` includes resolved ideas
for auditability. Markdown-only projects continue delegating to the TypeScript
compatibility command.

This task does not implement spark resolution, idea capture/promote/archive,
generated exports, tags, bundles, or Markdown compatibility writes.

## Acceptance Criteria

- [x] `loaf idea list` reads imported ideas from SQLite when state is initialized.
- [x] The default idea list excludes ideas with `resolved` status.
- [x] `loaf idea list --all` includes resolved ideas.
- [x] `loaf idea resolve <idea> --by <target>` updates the idea status to `resolved`.
- [x] Resolving an idea records a `resolved_by` relationship to the target entity.
- [x] Resolving an idea records a status-change event.
- [x] Markdown-only state delegates idea commands to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of silently falling back.
- [x] Tests cover state-backed list, resolve, trace relationship, event recording, invalid state, and fallback delegation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
