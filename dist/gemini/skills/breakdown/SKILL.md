---
name: breakdown
description: >-
  Decomposes specifications into atomic tasks with dependencies and priorities.
  Use when the user asks "break this down" or "create tasks for this spec."
  Produces task files with estimates, dependencies, and acceptance criteria. Not
  for shaping ideas (use shape) or implementation work (use implement).
version: 2.0.0-dev.22
---

# Breakdown

Decompose specifications into atomic, implementable tasks.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Task Breakdown Philosophy
- Task Backend Detection
- Process
- Priority Levels
- Guardrails
- Related Skills

**Input:** $ARGUMENTS

---

## Critical Rules

- **One concern per task** -- never mix unrelated layers (backend + frontend) in a single task
- **Every task includes its own verification** -- no separate "verify" tasks; each task must have an observable done condition
- **Own the decisions** -- decide granularity and priorities autonomously; only ask the user when two equally valid orderings have genuinely different trade-offs
- **Keep tests with the code they test** -- never split implementation and tests into separate tasks
- **Update spec status** -- mark the spec as `implementing` after tasks are created
- **Log outcome** -- log breakdown to session journal: `loaf session log "decision(breakdown): SPEC-NNN → N tasks created"`

---

## Verification

- Each created task has a clear title, priority, file hints, verification command, and observable done condition
- The dependency graph has no cycles and reflects actual implementation order
- Spec status has been updated to `implementing`

---

## Quick Reference

### Priority Levels

| Priority | Meaning |
|----------|---------|
| P0 | Urgent/blocking -- drop everything |
| P1 | High -- work next |
| P2 | Normal -- scheduled work (default) |
| P3 | Low -- when time permits |

### Right-Sizing Rules

| Rule | Guideline |
|------|-----------|
| **One agent type** | Completable by a single implementer (after skills narrowing) |
| **One concern** | Touches one layer, service, or component |
| **Context-appropriate** | Fits in model context with room for exploration |
| **Not over-fragmented** | Don't split what naturally belongs together |

---

## Task Breakdown Philosophy

**Primary principle: separation of concerns.**

### The Right Size Test

1. Can a single implementer complete this? If no, split by concern
2. Does it touch multiple unrelated concerns? If yes, split by concern
3. Will the agent need too much context? If yes, split into phases
4. Am I splitting just to have more tasks? If yes, merge back

### Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Split backend + tests into separate tasks | Keep tests with the code they test |
| Create a task per file | Group files by concern |
| Separate "implement" and "verify" tasks | Every task includes its own verification |

---

## Task Backend Detection

Check `.agents/loaf.yaml`:
```yaml
task_management:
  backend: linear  # or "local"
```
If no config exists, ask user.

---

## Process

### Step 1: Parse Input

`$ARGUMENTS` should reference a spec (e.g., "SPEC-001"). If unclear, list available specs.

### Step 2: Read the Spec

Extract: test conditions, scope, implementation notes, complexity size.

### Step 3: Identify Task Boundaries

Break down by concern (data layer, backend, frontend, infrastructure, etc.). One concern per task. Explicit dependencies for sequential tasks.

### Step 4: Decide Priorities and Granularity

Own the granularity and priority decisions. Apply the Right Size Test, assign priorities
based on dependencies, priority order, and go/no-go gates, and do a self-review pass. Do not
defer these decisions to the user — they trust agent judgment here.

If genuinely uncertain (e.g., two equally valid orderings with different trade-offs),
ask. Otherwise, decide and move on.

### Step 5: Draft Task List

Create tasks following [task template](templates/task.md). Each task needs: clear title, priority, file hints, verification command, observable done condition.

### Step 6: Present and Create

Show the dependency graph and task summary for awareness, then create the tasks.
Present it as "here's what I'm creating" not "which option do you prefer?" The user
can still adjust after creation, but the default is to proceed.

### Step 7: Create Tasks

**If `integrations.linear.enabled` is `true` in `.agents/loaf.json`:** create Linear issues with title, description, labels, priority (Linear MCP).

**Otherwise:** use `loaf task create --spec SPEC-XXX --title "Task title" --priority P1` for each task. The CLI creates the TASKS.json entry and .md skeleton file. Then edit the .md body content (description, acceptance criteria) directly.

### Step 8: Update Spec and Announce

Set spec status to `implementing`. Announce created tasks with next steps.

---

## Guardrails

1. **One concern per task** -- don't mix backend + frontend
2. **Clear verification** -- how to prove it works
3. **Observable done condition** -- not subjective
4. **File hints** -- help session know where to look
5. **Own the decisions** -- decide granularity and priorities, don't defer
6. **Update spec status** -- mark as implementing

---

## Suggests Next

After breakdown completes, suggest `/implement` to start working on the tasks.

## Related Skills

- **shape** -- Create specs that get broken down
- **implement** -- Start session for a task or coordinate multiple tasks
