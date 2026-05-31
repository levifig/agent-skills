---
id: TASK-219
title: Record reasons for SQLite spark resolution
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T20:08:43Z'
updated: '2026-05-28T20:12:06Z'
completed_at: '2026-05-28T20:12:06Z'
depends_on:
  - TASK-218
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-219-state-backed-spark-resolve-reason.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf spark resolve <spark> --by <target>
  --reason <text>` stores the reason on the SQLite resolution relationship and
  status-change event while preserving Markdown compatibility fallback.
---

# TASK-219: Record reasons for SQLite spark resolution

## Description

SPEC-040's command surface includes `loaf spark resolve SPARK-... --by IDEA-...
--reason "..."`, but the SQLite-backed implementation currently accepts only
`--by`. Thread the reason through the Go CLI parser and SQLite mutation so the
resolution graph explains why a spark should no longer resurface in triage.

This task does not implement spark capture, spark promotion, generated exports,
or brainstorm commands.

## Acceptance Criteria

- [x] `loaf spark resolve <spark> --by <target> --reason <text>` parses in SQLite-backed mode.
- [x] The reason is stored on the `resolved_by` relationship row.
- [x] The reason is stored on the status-change event note when the spark transitions to `resolved`.
- [x] Re-resolving an already resolved spark can update the relationship reason without creating a duplicate status event.
- [x] Existing `loaf spark resolve <spark> --by <target>` behavior remains supported.
- [x] Markdown-only state delegates the full original argv, including `--reason`, to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of falling back.
- [x] Tests cover state mutation, CLI JSON path, fallback delegation, and invalid state.

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
XDG_STATE_HOME=<temp> bin/loaf spark resolve SPARK-smoke --by 20260528-target-idea --reason "triaged into target idea" --json
XDG_STATE_HOME=<temp> bin/loaf spark list --json
XDG_STATE_HOME=<temp> bin/loaf spark list --json --all
XDG_STATE_HOME=<temp> bin/loaf trace SPARK-smoke --json
```
