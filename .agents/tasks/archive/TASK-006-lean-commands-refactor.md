---
id: TASK-006
title: Lean commands refactor
spec: SPEC-001
status: done
priority: P3
created: '2026-03-16T16:27:15.463Z'
updated: '2026-03-17T00:16:40.877Z'
depends_on:
  - TASK-001
  - TASK-002
  - TASK-003
  - TASK-004
  - TASK-005
files:
  - src/commands/start-session.md
  - src/commands/council-session.md
verify: |
  npm run build && \
  wc -l src/commands/start-session.md && \
  wc -l src/commands/council-session.md
done: >-
  start-session.md ≤200 lines, council-session.md ≤200 lines, no functionality
  lost
completed_at: '2026-03-17T00:16:40.877Z'
---

## Description

Refactor verbose commands to be lean invokers of skills. Commands should delegate to reference documents rather than contain all logic inline.

**Current state:**
- `start-session.md`: 731 lines
- `council-session.md`: 569 lines

**Target:**
- Both ≤200 lines

## Acceptance Criteria

- [ ] `start-session.md` reduced to ≤200 lines
- [ ] `council-session.md` reduced to ≤200 lines
- [ ] All functionality preserved (delegated to skills/references)
- [ ] `npm run build` succeeds
- [ ] Commands work correctly after refactor

## Context

See SPEC-001 for full context. This is P3 priority - deferrable per circuit breaker.

**Depends on:** All reference tasks (TASK-001 through TASK-005) - need references to delegate to.
