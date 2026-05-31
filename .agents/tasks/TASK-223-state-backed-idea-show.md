---
id: TASK-223
title: Show SQLite-backed idea details
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T20:32:01Z'
updated: '2026-05-28T20:35:38Z'
completed_at: '2026-05-28T20:35:38Z'
depends_on:
  - TASK-222
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-223-state-backed-idea-show.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf idea show <idea>` reads idea
  metadata, source provenance, imported body, and immediate relationships from
  SQLite while preserving Markdown compatibility fallback.
---

# TASK-223: Show SQLite-backed idea details

## Description

Port `loaf idea show <idea>` to SQLite-backed state. Once SQLite state is
initialized, the command should read the canonical idea row, source provenance,
imported Markdown body, and immediate relationships from SQLite. Markdown-only
state continues delegating to the TypeScript compatibility command.

This task does not implement idea promotion, generated Markdown exports, or
physical `.agents/ideas/` file synchronization from SQLite rows.

## Acceptance Criteria

- [x] `loaf idea show <idea>` parses exactly one idea reference in SQLite-backed mode.
- [x] The reference must resolve to an idea.
- [x] JSON output includes query, id, alias, title, status, timestamps, sources, body, and relationships.
- [x] Human output reports idea alias, title, status, source provenance, relationships, and body.
- [x] Imported Markdown body is read from the recorded source and frontmatter is stripped.
- [x] Captured ideas without a source still show core metadata.
- [x] Markdown-only state delegates the full original argv to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of falling back.
- [x] Tests cover imported ideas, captured ideas, non-idea rejection, missing refs, JSON/human output, fallback delegation, and invalid state.

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
XDG_STATE_HOME=<temp> bin/loaf idea show 20260528-target-idea --json
XDG_STATE_HOME=<temp> bin/loaf idea show 20260528-target-idea
```
