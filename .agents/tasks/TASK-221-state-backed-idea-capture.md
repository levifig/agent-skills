---
id: TASK-221
title: Capture ideas in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T20:18:54Z'
updated: '2026-05-28T20:22:23Z'
completed_at: '2026-05-28T20:22:23Z'
depends_on:
  - TASK-220
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-221-state-backed-idea-capture.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf idea capture --title <title>` creates
  an open idea in SQLite with an alias, status-change event, list/trace
  visibility, and Markdown compatibility fallback.
---

# TASK-221: Capture ideas in SQLite state

## Description

Port `loaf idea capture --title <title>` to SQLite-backed state. Once SQLite
state is initialized, capturing an idea should insert a structured idea row and
alias instead of creating Markdown as the canonical state. Markdown-only state
continues delegating to the TypeScript compatibility command.

This task does not implement idea promotion, idea show, generated Markdown
exports, or physical `.agents/ideas/` file creation from SQLite rows.

## Acceptance Criteria

- [x] `loaf idea capture --title <title>` parses in SQLite-backed mode.
- [x] Captured ideas insert into `ideas` with `status = open`, title, and timestamps.
- [x] Captured ideas receive a stable `IDEA-YYYYMMDD-*` alias derived from title with collision avoidance.
- [x] Captured ideas record a status-change event from null to `open`.
- [x] `loaf idea list` and `loaf trace` can read the captured idea.
- [x] `--json` output exposes the captured idea and event.
- [x] Human output reports the new idea alias.
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
XDG_STATE_HOME=<temp> bin/loaf idea capture --title "Repeat Idea" --json
XDG_STATE_HOME=<temp> bin/loaf idea capture --title "Repeat Idea" --json
XDG_STATE_HOME=<temp> bin/loaf idea list --json
XDG_STATE_HOME=<temp> bin/loaf trace IDEA-YYYYMMDD-repeat-idea --json
```
