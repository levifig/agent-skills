---
id: TASK-210
title: Collect SQLite state rows into bundles
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T18:50:00Z'
updated: '2026-05-28T18:58:27Z'
completed_at: '2026-05-28T18:58:27Z'
depends_on:
  - TASK-209
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-210-state-backed-bundles.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf bundle create/show/add/remove`
  collects rows by tag query and explicit membership, then displays the full
  related set.
---

# TASK-210: Collect SQLite state rows into bundles

## Description

Add state-backed bundle commands over the existing `bundles` and
`bundle_members` tables. Bundles collect rows by a tag query and explicit
membership so a user can ask for everything related to a theme without relying
on one parent artifact or filename prefix.

This task covers `loaf bundle create <slug> --tag <tag>`, `loaf bundle show
<slug>`, `loaf bundle add <slug> <entity>`, and `loaf bundle remove <slug>
<entity>`.

Markdown-only projects continue delegating to the TypeScript compatibility
command.

This task does not implement complex boolean tag expressions, generated
exports, bundle update beyond explicit add/remove, or bundle import from
Markdown.

## Acceptance Criteria

- [x] `loaf bundle create <slug> --tag <tag>` creates a bundle with a tag query.
- [x] `loaf bundle show <slug>` includes rows matched by the bundle tag query.
- [x] `loaf bundle add <slug> <entity>` adds explicit membership.
- [x] `loaf bundle remove <slug> <entity>` removes only explicit membership.
- [x] `loaf bundle show <slug>` displays the union of tag-query and explicit members without duplicates.
- [x] Bundle writes are idempotent.
- [x] Markdown-only state delegates bundle commands to the TypeScript compatibility command.
- [x] Invalid SQLite state reports the state error instead of silently falling back.
- [x] Tests cover tag-query membership, explicit membership, duplicate union behavior, removal, invalid state, and fallback delegation.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
