---
id: TASK-180
title: Simplify worktree migration no-local-agents control flow
status: done
priority: P3
created: '2026-05-19T18:55:59.829Z'
updated: '2026-05-19T19:02:45.361Z'
completed_at: '2026-05-19T19:02:45.360Z'
---

# TASK-180: Simplify worktree migration no-local-agents control flow

## Description

Remove redundant `runMigration()` control flow around the no-local-agents path
without changing migration behavior.

## Acceptance Criteria

- [ ] Already-migrated state is still detected before the no-local-agents path.
- [ ] Empty or missing local `.agents/` still returns `no-local-agents`.

## Verification

```bash
npm test -- cli/lib/migrate/worktree-storage.test.ts
```
