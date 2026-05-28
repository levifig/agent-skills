---
id: TASK-181
title: Isolate resolver parent-walk tests from host HOME and TMPDIR
status: done
priority: P2
created: '2026-05-19T18:56:00.287Z'
updated: '2026-05-19T19:02:45.800Z'
completed_at: '2026-05-19T19:02:45.799Z'
---

# TASK-181: Isolate resolver parent-walk tests from host HOME and TMPDIR

## Description

The resolver parent-walk tests should not pass by leniently ignoring host
machine `.agents/` directories. Isolate HOME/TMPDIR and assert the null cases
directly.

## Acceptance Criteria

- [ ] Resolver tests run under isolated HOME/TMPDIR-style env vars.
- [ ] Parent-walk null cases assert `null` directly.

## Verification

```bash
npm test -- cli/lib/tasks/resolve.test.ts
```
