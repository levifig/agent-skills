---
description: Orchestrate unattended execution of specs or task groups
version: 1.13.2
---

# Orchestrate

You are the PM agent orchestrating multi-task execution.

**Input:** $ARGUMENTS

---

## Step 0: Context Check

**Before starting orchestration**, evaluate whether the current context is suitable. Multi-task orchestration consumes significant context - starting fresh is often better.

### When to Recommend Restart/Clear

| Trigger | Recommendation | Reason |
|---------|----------------|--------|
| New command/skill added this session | **Restart required** | Skills loaded at session start |
| Conversation > 20 exchanges | **Recommend restart** | Orchestration needs headroom |
| Just completed different work | **Recommend clear** | Fresh context for multi-task work |
| Prior context unrelated to this spec | **Recommend clear** | Avoid confusion between tasks |

### Context Check Process

1. **Assess conversation depth** - Orchestration works best with fresh context
2. **Check prior work** - Is the current context relevant to this spec/tasks?
3. **Evaluate scope** - How many tasks? More tasks = more context needed

### If Restart/Clear Recommended

1. **Explain why** (orchestration needs headroom, topic change, etc.)
2. **Generate resumption prompt** with the orchestrate command ready to run
3. **Ask user** to restart or `/clear`, then paste the prompt

### Resumption Prompt Format

Generate a copyable prompt for the user:

```markdown
Resume Loaf development and run /orchestrate [SPEC-XXX or task range]

## Context
- Branch: [current branch]
- Spec: [spec ID and title]
- Tasks: [count] tasks across [count] waves

## Action
Run /orchestrate [args] to execute the tasks.
```

**Write this to any active session file** under `## Resumption Prompt` before recommending restart.

---

## Overview

This command enables unattended execution of specs or task groups with PM coordination. Tasks execute sequentially by default, with verification between each task. Execution stops on failure and provides recovery options.

---

## Step 1: Parse Input

Detect the input type and options from `$ARGUMENTS`:

### Input Patterns

| Pattern | Example | Action |
|---------|---------|--------|
| `SPEC-XXX` | `SPEC-002` | Find all tasks with `spec: SPEC-XXX` |
| `TASK-XXX..YYY` | `TASK-009..012` | Parse range (009, 010, 011, 012) |
| `TASK-XXX,YYY,ZZZ` | `TASK-009,011,015` | Parse explicit list |

### Options

| Option | Description |
|--------|-------------|
| `--dry-run` | Show execution plan without running |
| `--parallel` | Enable parallel execution within waves (max 3 concurrent) |
| `--continue` | Resume blocked orchestration |
| `--skip TASK-XXX` | Skip a task and continue |
| `--abort` | Cancel orchestration |

---

## Step 2: Handle Special Options

### `--continue` - Resume Blocked Orchestration

1. Find the active orchestration session:
   ```bash
   grep -l "type: orchestration" .agents/sessions/*.md | \
     xargs grep -l "status: blocked" 2>/dev/null
   ```
2. Read session file, locate blocked task
3. Continue from `current_task`

### `--skip TASK-XXX` - Skip and Continue

1. Find active orchestration session
2. Mark task as `skipped` in session
3. Continue with next task in wave

### `--abort` - Cancel Orchestration

1. Find active orchestration session
2. Update session status to `aborted`
3. Report final state

---

## Step 3: Resolve Tasks

### For Spec Input (`SPEC-XXX`)

Find all tasks belonging to the spec:

```bash
grep -l "spec: SPEC-XXX" .agents/tasks/*.md .agents/tasks/active/*.md 2>/dev/null
```

If no tasks found:
```
No tasks found for SPEC-XXX.

Would you like me to break down the spec into tasks first?
[Y/n]
```

If yes, delegate to `/tasks SPEC-XXX` workflow, then return to orchestration.

### For Range Input (`TASK-XXX..YYY`)

Parse the range and generate task list:

```
TASK-009..012 → TASK-009, TASK-010, TASK-011, TASK-012
```

Validate each task file exists.

### For List Input (`TASK-XXX,YYY,ZZZ`)

Parse the comma-separated list:

```
TASK-009,011,015 → TASK-009, TASK-011, TASK-015
```

Validate each task file exists.

---

## Step 4: Build Dependency Graph

For each task file, extract the `depends_on` field and build execution order.

### Analysis Output

```
Dependency analysis:
  TASK-009: no deps
  TASK-010: depends on TASK-009
  TASK-011: no deps
  TASK-012: no deps
  TASK-013: depends on TASK-012
  TASK-014: depends on TASK-012, TASK-013
  TASK-015: no deps
```

### Wave Grouping

Group tasks into waves based on dependencies:

```
Wave grouping:
  Wave 1: TASK-009, TASK-011, TASK-012, TASK-015 (no dependencies)
  Wave 2: TASK-010, TASK-013 (depends on Wave 1 tasks)
  Wave 3: TASK-014 (depends on Wave 2 tasks)
```

### Parallel Safety Check (for `--parallel`)

Only mark tasks as parallel-safe if:
1. No `depends_on` relationship between them
2. No shared files in their `files:` lists
3. Wave contains more than one task

---

## Step 5: Present Execution Plan

Display the plan and request confirmation:

```markdown
## Orchestrating SPEC-002: Invisible Sessions and Task Board

### Execution Plan

| Wave | Tasks | Mode |
|------|-------|------|
| 1 | TASK-009, TASK-011, TASK-015 | sequential |
| 2 | TASK-010, TASK-012 | sequential |
| 3 | TASK-013 | sequential |
| 4 | TASK-014 | sequential |

**Total:** 7 tasks across 4 waves
**Mode:** sequential (use --parallel for parallel execution)

### Task Details

**Wave 1:**
- TASK-009: Task board generation script (no deps)
- TASK-011: Build-time command substitution (no deps)
- TASK-015: Migrate tasks to new structure (no deps)

**Wave 2:**
- TASK-010: Task board generation hook (depends on TASK-009)
- TASK-012: Implement invisible sessions (no deps)

**Wave 3:**
- TASK-013: Resume with task arguments (depends on TASK-012)

**Wave 4:**
- TASK-014: Update workflow documentation (depends on TASK-012, TASK-013)

---

Proceed with orchestration? [Y/n]
```

### For `--dry-run`

Show the plan but do not ask for confirmation. Stop after displaying the plan.

---

## Step 6: Create Orchestration Session

Generate timestamps and create the session file:

```bash
# Generate timestamps
date -u +"%Y%m%d-%H%M%S"      # For filename
date -u +"%Y-%m-%dT%H:%M:%SZ"  # For frontmatter
```

### Session Filename

Format: `YYYYMMDD-HHMMSS-{description}.md`

Use the spec or task title (kebab-case), not the ID. Session type goes in frontmatter, not filename.

Examples:
- `20260124-030000-invisible-sessions-task-board.md` (from spec title)
- `20260124-030000-task-board-and-hooks.md` (from task range description)
- `20260124-030000-auth-feature-tasks.md` (from task list description)

### Session Structure

```yaml
---
session:
  title: "Orchestration: SPEC-002 - Invisible Sessions and Task Board"
  type: orchestration
  status: in_progress
  created: "2026-01-24T03:00:00Z"
  last_updated: "2026-01-24T03:00:00Z"
  spec: SPEC-002  # or null if task range/list
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
  current_wave: 1
  current_task: null
  waves:
    - wave: 1
      status: pending
      tasks:
        - id: TASK-009
          status: pending
          agent: null
          duration: null
        - id: TASK-011
          status: pending
          agent: null
          duration: null
        - id: TASK-015
          status: pending
          agent: null
          duration: null
    - wave: 2
      status: pending
      tasks:
        - id: TASK-010
          status: pending
          agent: null
          duration: null
        - id: TASK-012
          status: pending
          agent: null
          duration: null
    # ... additional waves

  summary:
    total_tasks: 7
    completed: 0
    in_progress: 0
    pending: 7
    failed: 0
    skipped: 0
---

# Orchestration: SPEC-002 - Invisible Sessions and Task Board

## Progress

Execution has not started yet.

## Session Log

### YYYY-MM-DD HH:MM - PM
Created orchestration session for SPEC-002.
```

### Link Tasks to Session

Update each task file with the session reference:

```yaml
# Add to task frontmatter
session: 20260124-030000-orchestration-spec-002.md
```

---

## Step 7: Execute Tasks

For each wave, execute tasks in order (sequential by default, or parallel if `--parallel`).

### Execution Loop

```
For each wave:
  1. Update session: wave status = in_progress
  2. For each task in wave:
     a. Update session: task status = in_progress, set current_task
     b. Record start time
     c. Determine agent type (see Agent Selection)
     d. Spawn agent via Task tool
     e. Wait for completion
     f. Run verification (task's `verify` command)
     g. Check acceptance criteria
     h. Record duration
     i. If pass: Update session: task status = completed
     j. If fail: Stop execution, mark blocked (see Failure Handling)
  3. Update session: wave status = completed
  4. Move to next wave
```

### Agent Spawning

Use the **Task tool** with the appropriate `subagent_type`:

```
Task(
  subagent_type="backend-dev",
  description="Implement TASK-009: Task board generation script",
  prompt="You are implementing TASK-009 as part of orchestrated execution.

Task: [task title]
Spec: [spec reference]

## Acceptance Criteria
[from task file]

## Files
[from task file]

## Verification
Run this command when complete: [verify command]

## Instructions
1. Read the task file for full context
2. Implement the requirements
3. Verify locally before completing
4. Do NOT commit changes (orchestrator handles this)
"
)
```

### Progress Output

During execution, display progress:

```markdown
### Progress

[=========>          ] 3/7 tasks (43%)

Wave 1: COMPLETED
  [x] TASK-009 - Task board generation script (4m 32s)
  [x] TASK-011 - Build-time command substitution (3m 15s)
  [x] TASK-015 - Migrate tasks to new structure (1m 08s)

Wave 2: IN PROGRESS
  [ ] TASK-010 - Task board generation hook (running...)
  [ ] TASK-012 - Implement invisible sessions

Wave 3: PENDING
  [ ] TASK-013 - Resume with task arguments

Wave 4: PENDING
  [ ] TASK-014 - Update workflow documentation
```

---

## Step 8: Run Verification

After each task completes, run verification per `foundations/references/verification.md`:

### Verification Steps

1. **Run task's verify command:**
   ```bash
   # From task frontmatter
   verify: pytest tests/auth/test_oauth.py
   ```

2. **Check acceptance criteria:**
   - All criteria in task file should be checked `[x]`
   - If any unchecked, verification fails

3. **Run standard checks:**
   ```bash
   git status   # Check for unexpected changes
   npm run build  # Or project-appropriate build command
   npm test       # Or project-appropriate test command
   ```

### Verification Result

| Result | Action |
|--------|--------|
| Pass | Continue to next task |
| Fail | Stop execution, enter blocked state |

---

## Step 9: Handle Failures

When verification fails or a task errors:

### Update Session

```yaml
orchestration:
  status: blocked
  blocked_at: "2026-01-24T03:45:00Z"
  blocked_task: TASK-012
  blocked_reason: "Verification failed: npm run build returned exit code 1"
```

### Display Recovery Options

```markdown
## Orchestration Blocked

**Failed task:** TASK-012 - Implement invisible sessions
**Wave:** 2
**Reason:** Verification failed: npm run build returned exit code 1

### Error Output

```
[build output or error message]
```

### Recovery Options

1. **Fix and continue:** Resolve the issue manually, then run:
   ```
   /orchestrate --continue
   ```

2. **Skip this task:** Mark as skipped and proceed:
   ```
   /orchestrate --skip TASK-012
   ```

3. **Abort orchestration:** Cancel remaining work:
   ```
   /orchestrate --abort
   ```

### Session File
`.agents/sessions/20260124-030000-orchestration-spec-002.md`
```

---

## Step 10: Complete Orchestration

When all tasks complete successfully:

### Update Session

```yaml
session:
  status: completed
  last_updated: "2026-01-24T04:30:00Z"

orchestration:
  status: completed
  completed_at: "2026-01-24T04:30:00Z"
```

### Update Spec Status

If orchestrating a spec, check if all spec tasks are done:

```bash
# Count remaining incomplete tasks for spec
grep -l "spec: SPEC-002" .agents/tasks/active/*.md 2>/dev/null | wc -l
```

If 0 remaining: Update spec status to `complete`

### Display Summary

```markdown
## Orchestration Complete

**Spec:** SPEC-002 - Invisible Sessions and Task Board
**Duration:** 1h 30m
**Tasks:** 7/7 completed
**Status:** All verification passed

### Summary by Wave

| Wave | Tasks | Duration | Status |
|------|-------|----------|--------|
| 1 | 3 | 8m 55s | completed |
| 2 | 2 | 12m 30s | completed |
| 3 | 1 | 5m 20s | completed |
| 4 | 1 | 3m 15s | completed |

### Task Details

| Task | Title | Agent | Duration |
|------|-------|-------|----------|
| TASK-009 | Task board generation script | backend-dev | 4m 32s |
| TASK-010 | Task board generation hook | backend-dev | 6m 15s |
| TASK-011 | Build-time command substitution | backend-dev | 3m 15s |
| TASK-012 | Implement invisible sessions | backend-dev | 6m 15s |
| TASK-013 | Resume with task arguments | backend-dev | 5m 20s |
| TASK-014 | Update workflow documentation | backend-dev | 3m 15s |
| TASK-015 | Migrate tasks to new structure | backend-dev | 1m 08s |

---

Spec SPEC-002 marked as complete.

**Session file:** `.agents/sessions/20260124-030000-orchestration-spec-002.md`
```

---

## Agent Selection Logic

Determine the appropriate agent based on task content.

### By File Patterns

| Pattern | Agent |
|---------|-------|
| `*.py`, `backend/`, `api/`, `services/` | backend-dev |
| `*.rb`, `app/models/`, `app/controllers/` | backend-dev |
| `*.tsx`, `*.jsx`, `frontend/`, `components/` | frontend-dev |
| `migrations/`, `schema`, `*.sql` | dba |
| `*test*`, `*spec*`, `tests/`, `__tests__/` | qa |
| `Dockerfile`, `*.yaml` (k8s), `terraform/`, `.github/` | devops |
| `*.md` (in docs/) | backend-dev |

### By Task Title Keywords

| Keywords | Agent |
|----------|-------|
| database, migration, schema, query | dba |
| test, spec, coverage, e2e | qa |
| deploy, ci, docker, kubernetes, infrastructure | devops |
| component, ui, page, style, css | frontend-dev |
| api, endpoint, service, model | backend-dev |

### Default

If unclear from files or keywords: `backend-dev`

---

## Parallel Execution (--parallel)

When `--parallel` is specified:

### Safety Rules

1. **Max 3 concurrent agents** - Never spawn more than 3 at once
2. **No shared files** - Tasks with overlapping `files:` lists run sequentially
3. **Respect dependencies** - Only parallelize within waves, not across

### Parallel Execution Flow

```
Wave 1 (3 tasks, no conflicts):
  Spawn TASK-009, TASK-011, TASK-015 concurrently
  Wait for all to complete
  Run verification for each
  If any fail, stop and report

Wave 2 (2 tasks, no conflicts):
  Spawn TASK-010, TASK-012 concurrently
  Wait for all to complete
  ...
```

### Progress Display (Parallel)

```markdown
### Progress

[=========>          ] 3/7 tasks (43%)

Wave 1: COMPLETED
  [x] TASK-009 - Task board generation script (4m 32s)
  [x] TASK-011 - Build-time command substitution (3m 15s)
  [x] TASK-015 - Migrate tasks to new structure (1m 08s)

Wave 2: IN PROGRESS (parallel)
  [ ] TASK-010 - Task board generation hook (running...)
  [ ] TASK-012 - Implement invisible sessions (running...)
```

---

## Guardrails

### Required Behaviors

1. **Sequential by default** - Only use parallel execution with explicit `--parallel` flag
2. **Verification between tasks** - Run task's `verify` command and check acceptance criteria
3. **Stop on failure** - Do not continue past failed verification
4. **Session tracks everything** - All progress recorded in session file
5. **Update continuously** - Session file always reflects current state

### Forbidden Behaviors

1. **Do NOT skip verification** - Every task must be verified before continuing
2. **Do NOT continue after failure** - Stop and report, wait for user intervention
3. **Do NOT spawn more than 3 parallel agents** - Respect resource limits
4. **Do NOT commit changes** - Individual task agents should not commit; orchestrator may batch commits

---

## Related Skills

- **foundations/references/verification** - Verification patterns
- **orchestration/references/sessions** - Session lifecycle
- **orchestration/references/local-tasks** - Task format and management
- **orchestration/references/delegation** - Agent spawning patterns
