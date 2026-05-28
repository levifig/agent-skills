---
id: TASK-010
title: Add task board generation hook
spec: SPEC-002
status: done
priority: P1
created: '2026-01-24T02:45:00.000Z'
updated: '2026-01-24T02:45:00.000Z'
depends_on:
  - TASK-009
files:
  - src/config/hooks.yaml
verify: 'Edit a task file, confirm TASKS.md regenerates'
done: Hook triggers on task file changes
session: 20260124-151610-orchestration-spec-002.md
completed_at: '2026-01-24T02:45:00.000Z'
---

# TASK-010: Add task board generation hook

## Description

Add a post-tool hook that triggers the task board generation script whenever a task file is created, modified, or moved.

## Acceptance Criteria

- [ ] Hook defined in `src/config/hooks.yaml`
- [ ] Triggers on Write tool to `.agents/tasks/**/*.md`
- [ ] Calls `scripts/generate-task-board.sh`
- [ ] TASKS.md updates automatically on task changes

## Context

See SPEC-002 for hook configuration details.

## Work Log

<!-- Updated by session as work progresses -->
