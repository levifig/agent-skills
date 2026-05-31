---
id: TASK-220
title: Capture sparks in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T20:13:32Z'
updated: '2026-05-28T20:17:27Z'
completed_at: '2026-05-28T20:17:27Z'
depends_on:
  - TASK-219
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-220-state-backed-spark-capture.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf spark capture --scope <scope> --text
  <text>` creates an open spark in SQLite with an alias, optional scope,
  status-change event, list/trace visibility, and Markdown compatibility
  fallback.
---

# TASK-220: Capture sparks in SQLite state

## Description

Port `loaf spark capture --scope <scope> --text <text>` to SQLite-backed state.
Once SQLite state is initialized, capturing a spark should insert a structured
spark row and alias instead of relying on journal/Markdown scanning. Markdown-only
state continues delegating to the TypeScript compatibility command.

This task does not implement spark promotion, idea capture, brainstorm commands,
or generated Markdown exports.

## Acceptance Criteria

- [x] `loaf spark capture --scope <scope> --text <text>` parses in SQLite-backed mode.
- [x] Captured sparks insert into `sparks` with `status = open`, optional scope, text, and timestamps.
- [x] Captured sparks receive a stable `SPARK-*` alias derived from text with collision avoidance.
- [x] Captured sparks record a status-change event from null to `open`.
- [x] `loaf spark list` and `loaf trace` can read the captured spark.
- [x] `--json` output exposes the captured spark, scope, and event.
- [x] Human output reports the new spark alias.
- [x] Markdown-only state delegates the full original argv to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of falling back.
- [x] Tests cover capture success, alias collision, JSON/human output, fallback delegation, and invalid state.

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
XDG_STATE_HOME=<temp> bin/loaf spark capture --scope architecture --text "Repeat Spark" --json
XDG_STATE_HOME=<temp> bin/loaf spark capture --text "Repeat Spark" --json
XDG_STATE_HOME=<temp> bin/loaf spark list --json
XDG_STATE_HOME=<temp> bin/loaf trace SPARK-repeat-spark --json
```
