---
id: TASK-131
title: 'Add soul: field to loaf.json schema and integrate into loaf install'
spec: SPEC-033
status: done
priority: P0
created: '2026-04-28T22:39:12.502Z'
updated: '2026-04-28T23:25:36.318Z'
depends_on:
  - TASK-129
  - TASK-130
completed_at: '2026-04-28T23:25:36.318Z'
---

# TASK-131: Add soul: field to loaf.json schema and integrate into loaf install

## Description

Add `soul: string` to the `loaf.json` schema/types with default `none`. Update `loaf install` to:

1. **Fresh install** (no `.agents/SOUL.md`): write `none` SOUL.md content from the catalog to `.agents/SOUL.md`, set `soul: none` in `loaf.json`.
2. **Legacy upgrade** (existing `.agents/SOUL.md`, no `soul:` field in `loaf.json`): write `soul: fellowship` to `loaf.json` (zero-config preservation), leave SOUL.md untouched.
3. **Already-configured** (`.agents/SOUL.md` exists and `soul:` field present): no changes to either.

The migration is one-shot: never overwrites SOUL.md, never re-runs once `soul:` is set. Reuses the catalog reader from TASK-130.

## Acceptance Criteria

- [ ] `loaf.json` schema/types include `soul: string` with documented default `none`
- [ ] Fresh install writes `none` SOUL.md content + sets `soul: none` in `loaf.json`
- [ ] Legacy upgrade (existing SOUL.md, no `soul:` field) writes `soul: fellowship` and leaves SOUL.md untouched
- [ ] Already-configured install is a no-op for both files
- [ ] Tests cover all three paths

## Verification

```bash
npm run build:cli && \
npm run typecheck && \
npm run test -- install
```

## Context

See SPEC-033 test conditions T6, T7. Depends on TASK-129 (catalog files) and TASK-130 (souls library used to read catalog content + write to `.agents/SOUL.md`). Closes Track 1 (catalog mechanics).
