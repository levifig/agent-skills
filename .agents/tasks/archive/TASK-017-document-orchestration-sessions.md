---
id: TASK-017
title: Document orchestration session type
spec: SPEC-003
status: done
priority: P2
created: '2026-01-24T03:30:00.000Z'
updated: '2026-01-24T03:30:00.000Z'
depends_on:
  - TASK-016
files:
  - src/skills/orchestration/SKILL.md
  - src/skills/orchestration/references/sessions.md
verify: grep -l 'orchestration' src/skills/orchestration/references/sessions.md
done: 'Orchestration session type documented, SKILL.md topic table updated'
completed_at: '2026-01-24T03:30:00.000Z'
---

# TASK-017: Document orchestration session type

## Description

Update orchestration skill documentation to include the new orchestration session type and workflow.

## Changes Required

### sessions.md
- Add orchestration session type to lifecycle states
- Document orchestration-specific frontmatter fields:
  - `type: orchestration`
  - `spec:` - Parent spec
  - `tasks:` - List of all tasks
  - `orchestration.waves:` - Wave structure
  - `orchestration.current_wave:` - Progress tracking
- Add example orchestration session structure

### SKILL.md
- Add row to Topics table for orchestration workflow
- Update Quick Reference with orchestration command

## Acceptance Criteria

- [ ] Orchestration session type documented in sessions.md
- [ ] Wave structure documented
- [ ] Progress tracking fields documented
- [ ] Example session structure included
- [ ] SKILL.md topic table updated
- [ ] Quick reference updated

## Context

See SPEC-003 for session structure design.

## Work Log

<!-- Updated by session as work progresses -->
