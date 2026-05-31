---
id: TASK-218
title: Archive ideas in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T20:00:40Z'
updated: '2026-05-28T20:06:57Z'
completed_at: '2026-05-28T20:06:57Z'
depends_on:
  - TASK-217
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-218-state-backed-idea-archive.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf idea archive` archives ideas in
  SQLite, records status-change events, hides archived ideas from default
  triage lists, and preserves Markdown compatibility fallback.
---

# TASK-218: Archive ideas in SQLite state

## Description

Port `loaf idea archive` to SQLite-backed state. Once SQLite state is
initialized, archiving an idea should update structured state instead of moving
or editing Markdown. Markdown-only state continues delegating to the TypeScript
compatibility command.

This task does not implement idea capture, promotion, generated Markdown
exports, or physical movement of idea files under `.agents/ideas/archive/`.

## Acceptance Criteria

- [x] `loaf idea archive <idea...>` resolves idea aliases from SQLite state.
- [x] `loaf idea archive <idea...> --reason <text>` records the archive reason on the status-change event note.
- [x] Successful archives update `ideas.status`, `ideas.updated_at`, and record status-change events.
- [x] Already archived ideas are reported without creating duplicate events.
- [x] Missing refs and wrong-kind refs are reported as skipped entries.
- [x] `loaf idea list` hides archived ideas by default.
- [x] `loaf idea list --all`, `loaf idea list --status archived`, and `loaf trace` reflect archived status after archive.
- [x] `--json` output exposes archived and skipped result rows.
- [x] Markdown-only state delegates `idea archive` to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of falling back.
- [x] Tests cover archive success, skip conditions, JSON/human output, fallback delegation, and invalid state.

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
XDG_STATE_HOME=<temp> bin/loaf idea archive 20260528-open-idea 20260528-archived-idea SPEC-001 missing-idea --reason "covered by SPEC-001" --json
XDG_STATE_HOME=<temp> bin/loaf idea list --json
XDG_STATE_HOME=<temp> bin/loaf idea list --json --status archived
XDG_STATE_HOME=<temp> bin/loaf trace 20260528-open-idea --json
```
