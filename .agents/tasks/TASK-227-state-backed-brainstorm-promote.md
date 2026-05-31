---
id: TASK-227
title: Promote brainstorms to ideas in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T20:58:21Z'
updated: '2026-05-28T21:02:16Z'
completed_at: '2026-05-28T21:02:16Z'
depends_on:
  - TASK-226
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-227-state-backed-brainstorm-promote.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf brainstorm promote <brainstorm>
  --to-idea <idea>` records an idempotent promoted_to relationship from
  brainstorm to idea in SQLite, exposes it through trace/link/show reads, and
  preserves Markdown compatibility fallback.
---

# TASK-227: Promote brainstorms to ideas in SQLite state

## Description

Port `loaf brainstorm promote <brainstorm> --to-idea <idea>` to
SQLite-backed state. Once SQLite state is initialized, the command should record
an idempotent `promoted_to` relationship from a brainstorm to an idea so trace,
link, and brainstorm-show reads expose the promotion.

Markdown-only state continues delegating to the TypeScript compatibility
command.

This task does not implement brainstorm archive, capture, status mutation, or
generated Markdown exports.

## Acceptance Criteria

- [x] `loaf brainstorm promote <brainstorm> --to-idea <idea>` parses in SQLite-backed mode.
- [x] `--json` output exposes the brainstorm, idea, and relationship ID.
- [x] Human output reports the promoted brainstorm, target idea, and relationship ID.
- [x] The command records an idempotent `promoted_to` relationship from brainstorm to idea.
- [x] Wrong source or target kinds are rejected.
- [x] Trace, link, and brainstorm-show reads expose the relationship.
- [x] Markdown-only state delegates the full original argv to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of falling back.
- [x] Tests cover JSON/human output, relationship idempotence, wrong-kind rejection, read visibility, fallback delegation, and invalid state.

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
XDG_STATE_HOME=<temp> bin/loaf brainstorm promote <brainstorm> --to-idea <idea> --json
XDG_STATE_HOME=<temp> bin/loaf trace <brainstorm> --json
```
