---
id: TASK-185
title: Share lock staleness primitives across task and session locks
status: done
priority: P2
created: '2026-05-19T19:11:08.159Z'
updated: '2026-05-19T19:19:55.582Z'
completed_at: '2026-05-19T19:19:55.581Z'
---

# TASK-185: Share lock staleness primitives across task and session locks

## Description

The task index lock and session journal lock used duplicate PID/host staleness
logic. Extract the shared lock payload, stale detection, and diagnostics so both
callers keep one policy.

## Acceptance Criteria

- [x] Shared helper owns lock content creation, PID liveness, age fallback, and diagnostics.
- [x] `withTasksJsonLock` uses the shared helper.
- [x] Session lock acquisition uses and re-exports the shared staleness helper for tests.

## Verification

```bash
npm test -- cli/lib/tasks/lock.test.ts
npm test -- cli/commands/session.test.ts -t "session lock: isLockStale staleness contract"
```
