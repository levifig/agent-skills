---
id: TASK-015
title: Migrate existing tasks to new structure
spec: SPEC-002
status: done
priority: P1
created: '2026-01-24T02:45:00.000Z'
updated: '2026-01-24T02:45:00.000Z'
files:
  - .agents/tasks/
verify: 'ls .agents/tasks/ shows task files, .agents/tasks/active/ removed'
done: 'Tasks in root, active/ directory gone, TASKS.md generated'
session: 20260124-151610-orchestration-spec-002.md
completed_at: '2026-01-24T02:45:00.000Z'
---

# TASK-015: Migrate existing tasks to new structure

## Description

Move existing tasks from `.agents/tasks/active/` to `.agents/tasks/` (root) and remove the empty `active/` directory.

## Steps

1. Move all files from `.agents/tasks/active/*.md` to `.agents/tasks/`
2. Remove empty `.agents/tasks/active/` directory
3. Run task board generation to create initial TASKS.md
4. Verify all task links still work

## Acceptance Criteria

- [ ] All tasks moved to `.agents/tasks/`
- [ ] `.agents/tasks/active/` directory removed
- [ ] No broken references
- [ ] TASKS.md generated (if TASK-009 complete)

## Context

See SPEC-002 for new directory structure.

## Work Log

<!-- Updated by session as work progresses -->
