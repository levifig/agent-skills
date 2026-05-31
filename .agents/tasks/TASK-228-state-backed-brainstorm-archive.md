---
id: TASK-228
title: Archive brainstorms in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T21:03:47Z'
updated: '2026-05-28T21:08:03Z'
completed_at: '2026-05-28T21:08:03Z'
depends_on:
  - TASK-227
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-228-state-backed-brainstorm-archive.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf brainstorm archive <brainstorm>
  [--reason <reason>]` archives brainstorms in SQLite, records status-change
  event notes, hides archived brainstorms from default triage lists, and
  preserves Markdown compatibility fallback.
---

# TASK-228: Archive brainstorms in SQLite state

## Description

Port `loaf brainstorm archive <brainstorm> [--reason <reason>]` to
SQLite-backed state. Once SQLite state is initialized, the command should mark
brainstorms archived, record status-change events with the optional reason, and
keep archived brainstorms out of the default triage list.

Markdown-only state continues delegating to the TypeScript compatibility
command.

This task does not implement brainstorm capture, generated Markdown exports, or
automatic Markdown rewrite.

## Acceptance Criteria

- [x] `loaf brainstorm archive <brainstorm> [--reason <reason>]` parses in SQLite-backed mode.
- [x] `--json` output exposes archived and skipped brainstorm outcomes.
- [x] Human output reports archived/skipped brainstorms and counts.
- [x] Archiving updates brainstorm status to `archived`.
- [x] Archiving records a `status_changed` event with the optional reason note.
- [x] Already archived, wrong-kind, and missing refs are skipped without aborting other refs.
- [x] Default `loaf brainstorm list` hides newly archived brainstorms.
- [x] `loaf brainstorm list --status archived` includes archived brainstorms.
- [x] Trace and brainstorm-show reads reflect archived status.
- [x] Markdown-only state delegates the full original argv to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of falling back.
- [x] Tests cover JSON/human output, archive/skipped outcomes, event notes, list filtering, trace/show visibility, fallback delegation, and invalid state.

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
XDG_STATE_HOME=<temp> bin/loaf brainstorm archive <brainstorm> --reason <reason> --json
XDG_STATE_HOME=<temp> bin/loaf brainstorm list --status archived --json
```
