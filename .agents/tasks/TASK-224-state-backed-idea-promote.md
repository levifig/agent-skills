---
id: TASK-224
title: Promote ideas to specs in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T20:37:58Z'
updated: '2026-05-28T20:41:46Z'
completed_at: '2026-05-28T20:41:46Z'
depends_on:
  - TASK-223
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-224-state-backed-idea-promote.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf idea promote <idea> --to-spec <spec>`
  records a promoted_to relationship from idea to spec in SQLite, exposes it
  through trace/link/show reads, and preserves Markdown compatibility fallback.
---

# TASK-224: Promote ideas to specs in SQLite state

## Description

Port `loaf idea promote <idea> --to-spec <spec>` to SQLite-backed state. Once
SQLite state is initialized, promotion should record a structured
`promoted_to` relationship from the idea to the spec. Markdown-only state
continues delegating to the TypeScript compatibility command.

This task does not mark the idea resolved, create a new spec, mutate spec
status, or write Markdown files. Closure remains owned by `loaf idea resolve`
or `loaf idea archive`.

## Acceptance Criteria

- [x] `loaf idea promote <idea> --to-spec <spec>` parses in SQLite-backed mode.
- [x] The idea reference must resolve to an idea.
- [x] The target reference must resolve to a spec.
- [x] Promotion records or updates one `promoted_to` relationship from idea to spec.
- [x] Promotion does not mutate idea or spec status.
- [x] `loaf trace`, `loaf link list`, and `loaf idea show` expose the promotion relationship.
- [x] `--json` output exposes the idea, spec, and relationship id.
- [x] Human output reports the promoted idea, target spec, and relationship id.
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
XDG_STATE_HOME=<temp> bin/loaf idea promote 20260528-target-idea --to-spec SPEC-001 --json
XDG_STATE_HOME=<temp> bin/loaf idea show 20260528-target-idea --json
XDG_STATE_HOME=<temp> bin/loaf trace 20260528-target-idea --json
```
