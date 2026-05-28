---
id: TASK-009
title: Create task board generation script
spec: SPEC-002
status: done
priority: P1
created: '2026-01-24T02:45:00.000Z'
updated: '2026-01-24T02:45:00.000Z'
files:
  - scripts/generate-task-board.sh
verify: ./scripts/generate-task-board.sh && cat .agents/TASKS.md
done: 'TASKS.md generated with correct format, links work in preview'
session: 20260124-151610-orchestration-spec-002.md
completed_at: '2026-01-24T02:45:00.000Z'
---

# TASK-009: Create task board generation script

## Description

Create a shell script that generates `.agents/TASKS.md` from task files. The script will be called by a hook whenever task files change.

## Acceptance Criteria

- [ ] Script reads all task files from `.agents/tasks/` and `.agents/tasks/archive/`
- [ ] Groups tasks by status (In Progress, To Do by priority, Completed)
- [ ] Generates markdown links to task files and specs
- [ ] Completed section is reverse chronological (most recent first)
- [ ] Output matches format specified in SPEC-002

## Context

See SPEC-002 for full format specification.

## Work Log

<!-- Updated by session as work progresses -->
