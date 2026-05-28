---
id: SPEC-003
title: Orchestrated Multi-Task Execution
created: '2026-01-24T03:00:00.000Z'
status: complete
requirement: Enable unattended execution of specs or task groups with PM coordination
---

# SPEC-003: Orchestrated Multi-Task Execution

## Problem Statement

Current workflow requires manual intervention per task:
1. Run `/implement TASK-001`, wait, review
2. Run `/implement TASK-002`, wait, review
3. Repeat for each task

This is inefficient for well-defined specs where tasks have clear acceptance criteria and TDD-style verification. Users should be able to kick off a spec and let it run to completion.

## Proposed Solution

### New Command: `/orchestrate`

```
/loaf:orchestrate SPEC-002
/loaf:orchestrate TASK-009..012
/loaf:orchestrate TASK-009,011,015
```

PM agent coordinates execution:
1. Validates/creates task breakdown
2. Builds execution plan (respecting dependencies)
3. Creates single orchestration session
4. Executes tasks with specialized subagents
5. Runs until complete or blocked

### Execution Model

**Verification-driven, unattended:**
- Each task has `verify` command and acceptance criteria
- Run verification between tasks (per `foundations/references/verification.md`)
- Continue automatically on success
- Stop and report on failure/block

**Conservative parallelism:**
- Sequential by default
- Parallel only for truly independent tasks (no shared files, no dependencies)
- No worktrees or complex git branching
- Same working directory throughout
- Cap parallel agents at 2-3

### Task Breakdown Philosophy

Tasks follow separation of concerns (per `/tasks` command and `orchestration/local-tasks` reference):
- One task = one concern = one agent type
- Small enough for context, not smaller
- Tests stay with the code they test

## Scope

### In Scope

**P1: Core Orchestration**
- `/orchestrate` command
- Input parsing (spec, range, list)
- Execution plan generation
- Wave-based task grouping
- Single session for all tasks
- Task-to-session linking

**P2: Execution Engine**
- Sequential task execution
- Subagent spawning per task
- TDD verification between tasks
- Progress tracking in session
- Failure handling and reporting

**P3: Parallelism**
- Detect independent tasks
- Parallel execution within waves
- Resource-conscious spawning

### Out of Scope (Rabbit Holes)

- Git worktrees for true parallelism
- Complex dependency graphs (keep linear/simple)
- Cross-spec orchestration
- Distributed execution
- Pause/resume mid-orchestration (rely on session state)

### No-Gos

- Don't skip verification between tasks
- Don't continue after task failure (stop and report)
- Don't spawn unlimited parallel agents (cap at 2-3)

## Design Decisions

### Input Formats

| Format | Example | Meaning |
|--------|---------|---------|
| Spec ID | `SPEC-002` | All tasks for spec |
| Range | `TASK-009..012` | Tasks 009, 010, 011, 012 |
| List | `TASK-009,011,015` | Explicit tasks |

### Execution Plan

```
Input: SPEC-002 (7 tasks)

Dependency analysis:
  TASK-009: no deps
  TASK-010: depends on TASK-009
  TASK-011: no deps
  TASK-012: no deps
  TASK-013: depends on TASK-012
  TASK-014: depends on TASK-012, TASK-013
  TASK-015: no deps

Execution plan:
  Wave 1: TASK-009, TASK-011, TASK-015 (parallel candidates)
  Wave 2: TASK-010, TASK-012 (TASK-010 after 009; TASK-012 independent)
  Wave 3: TASK-013 (after TASK-012)
  Wave 4: TASK-014 (after TASK-012, TASK-013)

Conservative execution (default):
  Sequential within waves, parallel across independent waves
```

### Session Structure

```yaml
---
session:
  title: "Orchestration: SPEC-002 - Invisible Sessions and Task Board"
  type: orchestration
  status: in_progress
  created: "2026-01-24T03:00:00Z"
  last_updated: "2026-01-24T03:15:00Z"
  spec: SPEC-002
  tasks:
    - TASK-009
    - TASK-010
    - TASK-011
    - TASK-012
    - TASK-013
    - TASK-014
    - TASK-015

orchestration:
  mode: sequential  # or "parallel"
  current_wave: 2
  current_task: TASK-010
  waves:
    - wave: 1
      status: completed
      tasks:
        - id: TASK-009
          status: completed
          agent: backend-dev
          duration: "4m 32s"
        - id: TASK-011
          status: completed
          agent: backend-dev
          duration: "3m 15s"
        - id: TASK-015
          status: completed
          agent: backend-dev
          duration: "1m 08s"
    - wave: 2
      status: in_progress
      tasks:
        - id: TASK-010
          status: in_progress
          agent: backend-dev
        - id: TASK-012
          status: pending

  summary:
    total_tasks: 7
    completed: 3
    in_progress: 1
    pending: 3
    failed: 0
---
```

### Task Linking

All tasks in orchestration get same session reference:

```yaml
# In each task file
session: 20260124-030000-orchestration-spec-002.md
```

### Agent Selection

PM selects agent based on task content:

| Task Pattern | Agent |
|--------------|-------|
| Database/schema | `dba` |
| Backend code | `backend-dev` |
| Frontend code | `frontend-dev` |
| Tests | `qa` |
| Infrastructure | `devops` |
| Documentation | `backend-dev` or `frontend-dev` |

### Verification Between Tasks

Between each task, follow `foundations/references/verification.md`:

1. Task completes
2. Run task's `verify` command
3. Check acceptance criteria (all checked = pass)
4. If pass → update session, continue to next task
5. If fail → stop, report failure, update session with blocked status

### Failure Handling

```yaml
orchestration:
  status: blocked
  blocked_at: "2026-01-24T03:45:00Z"
  blocked_task: TASK-012
  blocked_reason: "Verification failed: npm run build returned exit code 1"

  recovery_options:
    - "Fix the issue and run: /loaf:orchestrate --continue"
    - "Skip this task: /loaf:orchestrate --skip TASK-012"
    - "Abort orchestration: /loaf:orchestrate --abort"
```

## Command Interface

### Start Orchestration

```
/loaf:orchestrate SPEC-002
/loaf:orchestrate TASK-009..012
/loaf:orchestrate TASK-009,011,015

Options:
  --parallel       Enable parallel execution (default: sequential)
  --dry-run        Show execution plan without running
  --continue       Resume blocked orchestration
  --skip TASK-XXX  Skip a task and continue
  --abort          Cancel orchestration
```

### Progress Output

```
## Orchestrating SPEC-002: Invisible Sessions and Task Board

### Execution Plan
Wave 1: TASK-009, TASK-011, TASK-015
Wave 2: TASK-010, TASK-012
Wave 3: TASK-013
Wave 4: TASK-014

### Progress

[=========>          ] 3/7 tasks (43%)

Wave 1: COMPLETED
  ✓ TASK-009 - Task board generation script (4m 32s)
  ✓ TASK-011 - Build-time command substitution (3m 15s)
  ✓ TASK-015 - Migrate tasks to new structure (1m 08s)

Wave 2: IN PROGRESS
  ◐ TASK-010 - Task board generation hook
  ○ TASK-012 - Implement invisible sessions

Wave 3: PENDING
  ○ TASK-013 - Resume with task arguments

Wave 4: PENDING
  ○ TASK-014 - Update workflow documentation
```

## Test Conditions

- [ ] `/loaf:orchestrate SPEC-XXX` parses spec and finds tasks
- [ ] `/loaf:orchestrate TASK-001..003` parses range correctly
- [ ] `/loaf:orchestrate TASK-001,002,003` parses list correctly
- [ ] Execution plan respects `depends_on` fields
- [ ] Single session created, all tasks link to it
- [ ] Tasks execute sequentially by default
- [ ] Verification runs between tasks
- [ ] Orchestration stops on task failure
- [ ] Session file reflects accurate progress
- [ ] `--dry-run` shows plan without executing
- [ ] `--continue` resumes blocked orchestration
- [ ] Spec status updates to `complete` when all tasks done

## Files to Create

| File | Purpose |
|------|---------|
| `src/commands/orchestrate.md` | Main orchestration command |

## Files to Modify

| File | Changes |
|------|---------|
| `src/skills/orchestration/SKILL.md` | Add orchestration reference |
| `src/skills/orchestration/references/sessions.md` | Document orchestration session type |
| `src/config/hooks.yaml` | Register command |

## Implementation Notes

### Task Breakdown Check

If spec has no tasks:
```
No tasks found for SPEC-002.

Would you like me to break down the spec into tasks first?
[Y/n]
```

If yes, run task breakdown (like `/tasks`), then continue to orchestration.

### Conservative Parallelism Rules

Only run in parallel if:
1. Tasks have no `depends_on` relationship
2. Tasks don't share files in their `files:` list
3. Total parallel agents ≤ 3

Otherwise, run sequentially.

### Session Naming

Format: `YYYYMMDD-HHMMSS-orchestration-{spec-or-task-range}.md`

Examples:
- `20260124-030000-orchestration-spec-002.md`
- `20260124-030000-orchestration-task-009-012.md`

## Circuit Breaker

At 50% appetite: If core orchestration (P1) not working, simplify to pure sequential execution without waves. Parallelism can be added later.
