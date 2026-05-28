---
id: TASK-007
title: Create goal-backward verification reference
spec: SPEC-001
status: done
priority: P3
created: '2026-03-16T16:27:15.463Z'
updated: '2026-03-17T00:16:40.930Z'
depends_on:
  - TASK-001
files:
  - src/skills/foundations/references/goal-verification.md
  - src/skills/foundations/SKILL.md
verify: npm run build
done: Goal verification reference exists
completed_at: '2026-03-17T00:16:40.930Z'
---

## Description

Create a goal-backward verification reference that provides guidance on verifying work against original goals - ensuring the solution actually solves the problem, not just passes tests.

## Acceptance Criteria

- [ ] `src/skills/foundations/references/goal-verification.md` exists
- [ ] Reference covers: goal recall, solution alignment check, edge case consideration
- [ ] `src/skills/foundations/SKILL.md` references the new goal-verification topic
- [ ] `npm run build` succeeds

## Context

See SPEC-001 for full context. This is P3 priority - deferrable per circuit breaker.

**Depends on:** TASK-001 (builds on basic verification patterns).
