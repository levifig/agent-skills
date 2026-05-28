---
id: TASK-016
title: Create orchestrate command
spec: SPEC-003
status: done
priority: P1
created: '2026-01-24T03:30:00.000Z'
updated: '2026-01-24T02:56:10.000Z'
files:
  - src/commands/orchestrate.md
  - src/config/hooks.yaml
verify: npm run build && grep -l orchestrate plugins/loaf/commands/
done: 'Orchestrate command exists, builds successfully, registered in hooks'
session: 20260124-025127-orchestrate-command.md
completed_at: '2026-01-24T02:56:10.000Z'
---

# TASK-016: Create orchestrate command

## Description

Create the `/orchestrate` command that enables unattended execution of specs or task groups with PM coordination.

## Core Functionality

### Input Parsing
- `SPEC-XXX` → Find all tasks for spec
- `TASK-XXX..YYY` → Parse range (inclusive)
- `TASK-XXX,YYY,ZZZ` → Parse explicit list

### Execution Plan Generation
- Read task files, extract `depends_on` fields
- Build dependency graph
- Group into waves (independent tasks can be in same wave)
- Present plan to user (or run with `--dry-run`)

### Execution Engine
- Create single orchestration session
- Update all tasks with `session:` field
- Execute tasks sequentially (default) or with conservative parallelism
- Run verification between tasks (per `foundations/references/verification.md`)
- Stop on failure, report status

### Session Management
- Session type: `orchestration`
- Track waves, current task, progress summary
- Support `--continue` for resuming blocked orchestration

### Command Options
- `--parallel` - Enable parallel execution within waves
- `--dry-run` - Show plan without executing
- `--continue` - Resume blocked orchestration
- `--skip TASK-XXX` - Skip a task and continue
- `--abort` - Cancel orchestration

## Acceptance Criteria

- [ ] Parses spec input correctly
- [ ] Parses task range correctly (`..`)
- [ ] Parses task list correctly (`,`)
- [ ] Generates execution plan respecting dependencies
- [ ] Creates orchestration session with correct structure
- [ ] Updates task files with session reference
- [ ] Executes tasks sequentially
- [ ] Runs verification between tasks
- [ ] Stops on task failure with clear error
- [ ] Progress output shows wave/task status
- [ ] `--dry-run` works
- [ ] Command registered in hooks.yaml

## Context

See SPEC-003 for full design. References:
- `foundations/references/verification.md` - Verification patterns
- `orchestration/references/sessions.md` - Session structure
- `orchestration/references/local-tasks.md` - Task format

## Work Log

<!-- Updated by session as work progresses -->
