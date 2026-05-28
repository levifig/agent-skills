---
id: TASK-008
title: Create persistent debug sessions reference
spec: SPEC-001
status: done
priority: P3
created: '2026-03-16T16:27:15.463Z'
updated: '2026-03-17T00:16:40.982Z'
files:
  - src/skills/debugging/references/persistent-sessions.md
  - src/skills/debugging/SKILL.md
verify: npm run build
done: Persistent sessions reference exists
completed_at: '2026-03-17T00:16:40.982Z'
---

## Description

Create a persistent debug sessions reference that provides guidance on maintaining debug state across sessions - what to persist, where to store it, how to resume.

## Acceptance Criteria

- [ ] `src/skills/debugging/references/persistent-sessions.md` exists
- [ ] Reference covers: state to persist, storage location (.agents/debug/), session resume flow
- [ ] `src/skills/debugging/SKILL.md` references the new persistent-sessions topic
- [ ] `npm run build` succeeds

## Context

See SPEC-001 for full context. This is P3 priority - deferrable per circuit breaker.

Independent task - can be done in parallel with other P3 work.
