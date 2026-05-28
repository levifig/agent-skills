---
id: TASK-005
title: Integrate patterns into start-session command
spec: SPEC-001
status: done
priority: P1
created: '2026-03-16T16:27:15.462Z'
updated: '2026-03-17T00:16:40.826Z'
depends_on:
  - TASK-001
  - TASK-002
files:
  - src/commands/start-session.md
verify: npm run build
done: 'Start session references new patterns, works without superpowers installed'
completed_at: '2026-03-17T00:16:40.826Z'
---

## Description

Update the start-session command to reference the new verification and finishing-work patterns. This makes Loaf self-sufficient - it no longer needs superpowers for workflow guidance.

## Acceptance Criteria

- [ ] `src/commands/start-session.md` references verification pattern
- [ ] `src/commands/start-session.md` references finishing-work pattern
- [ ] Command works correctly without superpowers plugin installed
- [ ] `npm run build` succeeds

## Context

See SPEC-001 for full context. This is P1 priority.

**Depends on:** TASK-001 (verification), TASK-002 (finishing-work) - needs those references to exist first.

Note: This is integration only, not the lean refactor (that's TASK-006).
