---
id: TASK-013
title: Update resume command for task arguments
spec: SPEC-002
status: done
priority: P2
created: '2026-01-24T02:45:00.000Z'
updated: '2026-01-24T02:45:00.000Z'
depends_on:
  - TASK-012
files:
  - src/commands/resume.md
verify: '/loaf:resume TASK-XXX loads correct session'
done: 'Resume works with task ID, still works with session filename'
session: 20260124-151610-orchestration-spec-002.md
completed_at: '2026-01-24T02:45:00.000Z'
---

# TASK-013: Update resume command for task arguments

## Description

Extend the `/resume` command to accept task IDs in addition to session filenames. When given a task ID, it reads the task file, finds the `session:` field, and loads that session.

## Changes Required

1. Detect if argument is task ID (TASK-XXX) or session filename
2. If task ID: read task file, extract `session:` field
3. Load session using existing logic
4. Preserve backward compatibility with session filename arguments

## Acceptance Criteria

- [ ] `/loaf:resume TASK-002` finds and loads correct session
- [ ] `/loaf:resume 20260124-143000-session.md` still works
- [ ] Error message if task has no session field
- [ ] Error message if task file not found

## Context

See SPEC-002 for task-based resume design.

## Work Log

<!-- Updated by session as work progresses -->
