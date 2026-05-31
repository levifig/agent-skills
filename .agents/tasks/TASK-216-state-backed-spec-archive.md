---
id: TASK-216
title: Archive specs in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T19:41:29Z'
updated: '2026-05-28T19:46:20Z'
completed_at: '2026-05-28T19:46:20Z'
depends_on:
  - TASK-215
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-216-state-backed-spec-archive.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf spec archive <spec>` archives
  complete specs in SQLite, records a status-change event, reports skipped specs
  clearly, and preserves Markdown compatibility fallback.
---

# TASK-216: Archive specs in SQLite state

## Description

Port the existing `loaf spec archive <spec...>` mutation to SQLite-backed state.
When SQLite state is initialized, archiving a spec should no longer mutate
Markdown or `TASKS.json`; it should update the spec row, record a status-change
event when the spec transitions to `archived`, and leave Markdown compatibility
to fallback/export paths.

The TypeScript compatibility command remains authoritative for Markdown-only
state. This task does not implement spec creation/update/show, generated
Markdown exports, or physical movement of spec files under `.agents/specs/`.

## Acceptance Criteria

- [x] `loaf spec archive <spec>` resolves spec aliases from SQLite state.
- [x] Specs must be `complete` before they can transition to `archived`.
- [x] Already archived specs are reported as skipped without rewriting state.
- [x] Missing refs and wrong-kind refs are reported as skipped entries, not as a whole-command crash when other refs are archiveable.
- [x] Successful archives update `specs.status`, `specs.updated_at`, and record a status-change event.
- [x] `loaf spec list` and `loaf trace` reflect archived status after archive.
- [x] `--json` output exposes archived and skipped result rows.
- [x] Markdown-only state delegates `spec archive` to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of falling back.
- [x] Tests cover success, skip conditions, JSON/human output, fallback delegation, and invalid state.

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
XDG_STATE_HOME=<temp> bin/loaf spec archive SPEC-001 SPEC-002 TASK-001 SPEC-999 --json
XDG_STATE_HOME=<temp> bin/loaf spec list --json
XDG_STATE_HOME=<temp> bin/loaf trace SPEC-001 --json
XDG_STATE_HOME=<temp> bin/loaf spec archive SPEC-001
```
