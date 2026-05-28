---
id: TASK-003
title: Create auto-fix-rules reference
spec: SPEC-001
status: done
priority: P2
created: '2026-03-16T16:27:15.462Z'
updated: '2026-03-17T00:16:40.719Z'
files:
  - src/skills/orchestration/references/auto-fix-rules.md
  - src/skills/orchestration/SKILL.md
verify: npm run build
done: Auto-fix rules reference exists and is referenced by orchestration skill
completed_at: '2026-03-17T00:16:40.718Z'
---

## Description

Create an auto-fix-rules reference document that provides guidance on when and how to automatically fix issues during execution (linting, formatting, simple test failures).

## Acceptance Criteria

- [ ] `src/skills/orchestration/references/auto-fix-rules.md` exists
- [ ] Reference covers: what can be auto-fixed, what requires human decision, escalation criteria
- [ ] `src/skills/orchestration/SKILL.md` references the new auto-fix-rules topic
- [ ] `npm run build` succeeds

## Context

See SPEC-001 for full context. This is P2 priority.

Can run in parallel with other reference creation tasks.
