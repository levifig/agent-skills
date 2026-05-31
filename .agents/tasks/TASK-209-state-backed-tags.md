---
id: TASK-209
title: Classify SQLite state rows with tags
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T18:41:26Z'
updated: '2026-05-28T18:48:40Z'
completed_at: '2026-05-28T18:48:40Z'
depends_on:
  - TASK-208
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-209-state-backed-tags.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf tag add/list/show/remove` classifies
  specs, tasks, ideas, sparks, brainstorms, sessions, reports, and journal
  entries through the `tags` and `entity_tags` many-to-many tables.
---

# TASK-209: Classify SQLite state rows with tags

## Description

Add state-backed tag commands over the existing `tags` and `entity_tags`
tables. Tags should classify imported or state-created rows by resolving aliases
or internal IDs, then writing many-to-many membership rows.

This task covers `loaf tag list`, `loaf tag show <tag>`, `loaf tag add <entity>
<tag>`, and `loaf tag remove <entity> <tag>` for the entity kinds required by
SPEC-040: specs, tasks, ideas, sparks, brainstorms, sessions, reports, and
journal entries.

Markdown-only projects continue delegating to the TypeScript compatibility
command.

This task does not implement bundles, generated exports, tag import from
Markdown frontmatter, or tag-based filtering on list commands.

## Acceptance Criteria

- [x] `loaf tag add <entity> <tag>` creates the tag if needed and writes an `entity_tags` membership row.
- [x] `loaf tag remove <entity> <tag>` removes only the requested membership row.
- [x] `loaf tag list` lists tags with membership counts.
- [x] `loaf tag show <tag>` lists tagged rows.
- [x] Tag membership supports specs, tasks, ideas, sparks, brainstorms, sessions, reports, and journal entries.
- [x] Tag writes are idempotent.
- [x] Markdown-only state delegates tag commands to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of silently falling back.
- [x] Tests cover many-to-many membership for all required entity kinds, idempotency, removal, invalid state, and fallback delegation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
