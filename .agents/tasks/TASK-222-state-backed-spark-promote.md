---
id: TASK-222
title: Promote sparks to ideas in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T20:25:33Z'
updated: '2026-05-28T20:29:53Z'
completed_at: '2026-05-28T20:29:53Z'
depends_on:
  - TASK-221
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-222-state-backed-spark-promote.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf spark promote <spark> --to-idea
  <idea>` records a promoted_to relationship from spark to idea in SQLite,
  exposes it through trace/link reads, and preserves Markdown compatibility
  fallback.
---

# TASK-222: Promote sparks to ideas in SQLite state

## Description

Port `loaf spark promote <spark> --to-idea <idea>` to SQLite-backed state. Once
SQLite state is initialized, promotion should record a structured `promoted_to`
relationship from the spark to the idea. Markdown-only state continues
delegating to the TypeScript compatibility command.

This task does not mark the spark resolved, create a new idea, mutate idea
status, or write Markdown files. Resolution remains owned by `loaf spark
resolve`.

## Acceptance Criteria

- [x] `loaf spark promote <spark> --to-idea <idea>` parses in SQLite-backed mode.
- [x] The spark reference must resolve to a spark.
- [x] The target reference must resolve to an idea.
- [x] Promotion records or updates one `promoted_to` relationship from spark to idea.
- [x] Promotion does not mutate spark or idea status.
- [x] `loaf trace` and `loaf link list` expose the promotion relationship.
- [x] `--json` output exposes the spark, idea, and relationship id.
- [x] Human output reports the promoted spark, target idea, and relationship id.
- [x] Markdown-only state delegates the full original argv to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of falling back.
- [x] Tests cover promotion success, kind validation, JSON/human output, fallback delegation, and invalid state.

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
XDG_STATE_HOME=<temp> bin/loaf spark promote SPARK-smoke --to-idea 20260528-target-idea --json
XDG_STATE_HOME=<temp> bin/loaf trace SPARK-smoke --json
XDG_STATE_HOME=<temp> bin/loaf link list SPARK-smoke --json
```

Note: the built-binary smoke should run from inside the fixture project root;
the Go front controller does not accept a global `--cwd` option.
