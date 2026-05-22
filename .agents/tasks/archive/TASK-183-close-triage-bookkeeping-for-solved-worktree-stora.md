---
id: TASK-183
title: Close triage bookkeeping for solved worktree-storage follow-ups
status: done
priority: P3
created: '2026-05-19T18:56:01.237Z'
updated: '2026-05-19T19:02:46.677Z'
completed_at: '2026-05-19T19:02:46.676Z'
---

# TASK-183: Close triage bookkeeping for solved worktree-storage follow-ups

## Description

Close stale triage records for worktree-storage follow-ups already implemented
by `TASK-173..177` and this follow-up batch.

## Acceptance Criteria

- [ ] Solved session sparks have `resolve(spark)` entries.
- [ ] The symlink handling idea no longer appears as raw intake.

## Verification

```bash
rg -n "migrate-symlink-handling|resolve\\(spark\\)" .agents/ideas .agents/sessions
```
