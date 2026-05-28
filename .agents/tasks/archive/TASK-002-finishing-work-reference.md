---
id: TASK-002
title: Create finishing-work reference
spec: SPEC-001
status: done
priority: P1
created: '2026-03-16T16:27:15.461Z'
updated: '2026-03-17T00:16:40.658Z'
files:
  - src/skills/orchestration/references/finishing-work.md
  - src/skills/orchestration/SKILL.md
verify: npm run build
done: Finishing work reference exists and is referenced by orchestration skill
completed_at: '2026-03-17T00:16:40.658Z'
---

## Description

Create a finishing-work reference document that provides guidance on properly completing work sessions - cleanup, verification, handoff.

## Acceptance Criteria

- [ ] `src/skills/orchestration/references/finishing-work.md` exists
- [ ] Reference covers: session cleanup, verification checklist, handoff documentation
- [ ] `src/skills/orchestration/SKILL.md` references the new finishing-work topic
- [ ] `npm run build` succeeds

## Context

See SPEC-001 for full context. This is P1 priority - core to Loaf self-sufficiency.

Can run in parallel with TASK-001.
