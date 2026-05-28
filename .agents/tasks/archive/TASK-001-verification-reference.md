---
id: TASK-001
title: Create verification reference
spec: SPEC-001
status: done
priority: P1
created: '2026-03-16T16:27:15.461Z'
updated: '2026-03-16T16:27:15.461Z'
files:
  - src/skills/foundations/references/verification.md
  - src/skills/foundations/SKILL.md
verify: npm run build
done: Verification reference exists and is referenced by foundations skill
completed_at: '2026-03-16T16:27:15.461Z'
---

## Description

Create a verification reference document that provides guidance on verifying work before claiming completion. This is a core workflow pattern that Loaf needs to be self-sufficient.

## Acceptance Criteria

- [ ] `src/skills/foundations/references/verification.md` exists
- [ ] Reference covers: when to verify, how to verify, common verification commands
- [ ] `src/skills/foundations/SKILL.md` references the new verification topic
- [ ] `npm run build` succeeds

## Context

See SPEC-001 for full context. This is P1 priority - core to Loaf self-sufficiency.

Adapt from superpowers patterns but don't duplicate verbatim - fit Loaf's structure.
