---
id: TASK-012
title: Update implement command for invisible sessions
spec: SPEC-002
status: done
priority: P2
created: '2026-01-24T02:45:00.000Z'
updated: '2026-01-24T02:45:00.000Z'
files:
  - src/commands/implement.md
verify: 'Run /loaf:implement TASK-XXX, confirm session created and task updated'
done: >-
  Session auto-created, task session: field populated, no user interaction
  needed
session: 20260124-151610-orchestration-spec-002.md
completed_at: '2026-01-24T02:45:00.000Z'
---

# TASK-012: Update implement command for invisible sessions

## Description

Modify the `/implement` command to automatically create and manage sessions without user visibility. Users work with tasks; sessions are implementation details.

## Changes Required

1. Remove explicit session naming from user-facing output
2. Auto-generate session filename from task ID
3. Update task file with `session:` field pointing to created session
4. Keep all session functionality (agent tracking, decisions, resumption)

## Acceptance Criteria

- [ ] Session created automatically when implementing a task
- [ ] Task frontmatter updated with `session:` field
- [ ] No user prompts about session naming
- [ ] Session still tracks all orchestration data
- [ ] Non-task sessions (research, architecture) still work as before

## Context

See SPEC-002 for invisible sessions design.

## Work Log

<!-- Updated by session as work progresses -->
