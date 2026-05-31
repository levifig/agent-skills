---
id: TASK-226
title: Show SQLite-backed brainstorm details
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T20:52:53Z'
updated: '2026-05-28T20:56:53Z'
completed_at: '2026-05-28T20:56:53Z'
depends_on:
  - TASK-225
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-226-state-backed-brainstorm-show.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf brainstorm show <brainstorm>` reads
  imported brainstorm metadata, source provenance, body content, and immediate
  relationships from SQLite with JSON/human output and Markdown compatibility
  fallback.
---

# TASK-226: Show SQLite-backed brainstorm details

## Description

Port `loaf brainstorm show <brainstorm>` to SQLite-backed state. Once SQLite
state is initialized, the command should show imported brainstorm metadata,
source provenance, frontmatter-stripped body content, and immediate
relationships.

Markdown-only state continues delegating to the TypeScript compatibility
command.

This task does not implement brainstorm promotion, archive, capture, or
generated Markdown exports.

## Acceptance Criteria

- [x] `loaf brainstorm show <brainstorm>` parses in SQLite-backed mode.
- [x] `--json` output exposes a stable detail model for one brainstorm.
- [x] Human output lists alias, title, status, source path/hash, relationships, timestamps, and imported body.
- [x] Imported body output strips Markdown frontmatter.
- [x] Immediate relationships for the brainstorm are included.
- [x] Markdown-only state delegates the full original argv to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of falling back.
- [x] Tests cover JSON/human output, source/body handling, relationships, fallback delegation, and invalid state.

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
XDG_STATE_HOME=<temp> bin/loaf brainstorm show <brainstorm> --json
XDG_STATE_HOME=<temp> bin/loaf brainstorm show <brainstorm>
```
