---
id: TASK-225
title: List brainstorms from SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T20:43:36Z'
updated: '2026-05-28T20:50:37Z'
completed_at: '2026-05-28T20:50:37Z'
depends_on:
  - TASK-224
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-225-state-backed-brainstorm-list.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf brainstorm list` reads imported
  brainstorms from SQLite with JSON/human output, status filters, source
  provenance, and Markdown compatibility fallback.
---

# TASK-225: List brainstorms from SQLite state

## Description

Port `loaf brainstorm list` to SQLite-backed state. Once SQLite state is
initialized, the command should list imported brainstorm backlog items from the
`brainstorms` table with status-aware triage filtering and source provenance.
Markdown-only state continues delegating to the TypeScript compatibility
command.

This task does not implement brainstorm show, promote, archive, capture, or
generated Markdown exports.

## Acceptance Criteria

- [x] `loaf brainstorm list` parses in SQLite-backed mode.
- [x] `--json` output exposes a stable list model keyed by brainstorm alias.
- [x] Human output lists alias, title, source path, and status when requested.
- [x] Default output hides resolved and archived brainstorms.
- [x] `--all` includes hidden statuses.
- [x] `--status <status>` filters by exact status.
- [x] Markdown-only state delegates the full original argv to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of falling back.
- [x] Tests cover JSON/human output, default/all/status filtering, fallback delegation, and invalid state.

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
XDG_STATE_HOME=<temp> bin/loaf brainstorm list --json
XDG_STATE_HOME=<temp> bin/loaf brainstorm list --all
```
