---
id: TASK-014
title: Update documentation for new workflow
spec: SPEC-002
status: done
priority: P3
created: '2026-01-24T02:45:00.000Z'
updated: '2026-01-24T02:45:00.000Z'
depends_on:
  - TASK-012
  - TASK-013
files:
  - src/skills/orchestration/references/local-tasks.md
  - src/skills/orchestration/references/sessions.md
verify: 'Read updated docs, confirm accuracy'
done: local-tasks.md and sessions.md reflect new structure
session: 20260124-151610-orchestration-spec-002.md
completed_at: '2026-01-24T02:45:00.000Z'
---

# TASK-014: Update documentation for new workflow

## Description

Update orchestration skill references to document the new invisible sessions workflow and task directory structure.

## Changes Required

### local-tasks.md
- Update directory structure (root instead of active/)
- Document `session:` field in task frontmatter
- Update archive path references

### sessions.md
- Document invisible sessions for implementation work
- Clarify when explicit sessions are still used (research, architecture)
- Update session-task relationship

## Acceptance Criteria

- [ ] Task directory structure documented correctly
- [ ] Session invisibility explained
- [ ] Task `session:` field documented
- [ ] Examples updated to reflect new workflow

## Context

See SPEC-002 for workflow changes.

## Work Log

<!-- Updated by session as work progresses -->
