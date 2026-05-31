---
id: TASK-211
title: Link SQLite state rows with explicit relationships
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T19:00:37Z'
updated: '2026-05-28T19:05:54Z'
completed_at: '2026-05-28T19:05:54Z'
depends_on:
  - TASK-210
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-211-state-backed-links.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf link create/list/remove` writes,
  reads, and removes explicit relationship rows between resolved state
  entities, and `loaf trace` reflects the resulting relationship graph.
---

# TASK-211: Link SQLite state rows with explicit relationships

## Description

Add state-backed relationship commands over the existing `relationships`
table. Links are explicit user-facing edges between imported state rows. They
make lineage editable without hand-editing Markdown frontmatter and give
`loaf trace` a write path for relationships beyond importer inference and
specialized resolve commands.

This task covers `loaf link create <from> <to> --type <relationship>`,
`loaf link list <entity>`, and `loaf link remove <from> <to> --type
<relationship>`.

Markdown-only projects continue delegating to the TypeScript compatibility
command.

This task does not implement link import/export formats, complex graph
queries, relationship type registries, generated Markdown compatibility
views, or external-system sync.

## Acceptance Criteria

- [x] `loaf link create <from> <to> --type <relationship>` resolves aliases and writes a relationship row.
- [x] `loaf link list <entity>` displays inbound and outbound relationship rows for a resolved entity.
- [x] `loaf link remove <from> <to> --type <relationship>` removes only the matching explicit relationship.
- [x] Link writes are idempotent and update the relationship reason/timestamp rather than duplicating rows.
- [x] `loaf trace <entity>` reflects relationships created and removed through `loaf link`.
- [x] Markdown-only state delegates link commands to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of silently falling back.
- [x] Tests cover create/list/remove, idempotency, trace integration, invalid state, and fallback delegation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
