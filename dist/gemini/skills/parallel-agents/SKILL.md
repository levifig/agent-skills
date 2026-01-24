---
name: parallel-agents
description: >-
  Dispatch multiple agents to work in parallel on independent tasks. Use when
  facing multiple tasks that don't share state or have sequential dependencies.
  Maximizes throughput by running concurrent work streams.
version: 1.12.0
---

# Parallel Agent Dispatch

Run independent work concurrently for faster completion.

## Philosophy

**Independence is the prerequisite.** Parallel work only makes sense when tasks don't depend on each other. Shared state or sequential dependencies mean sequential execution.

**Divide by concern, not by size.** Split work along natural boundaries (frontend/backend, module A/module B), not arbitrary chunks. This minimizes merge conflicts and coordination overhead.

**Explicit boundaries, clean handoffs.** Each parallel stream should have clear inputs, outputs, and success criteria. Ambiguity creates integration nightmares.

**Fail fast, coordinate early.** If one stream discovers a blocker that affects others, surface it immediately. Don't let parallel streams diverge too far before checking alignment.

## Quick Reference

| Condition | Parallel? | Why |
|-----------|-----------|-----|
| Tasks touch different files | ✅ Yes | No merge conflicts |
| Tasks modify same module | ⚠️ Maybe | Need clear boundaries |
| Task B needs Task A's output | ❌ No | Sequential dependency |
| Tasks share state/database | ⚠️ Maybe | Risk of race conditions |
| Tasks are truly independent | ✅ Yes | Maximum parallelism |

## When to Parallelize

### Good Candidates

- **Different concerns:** Backend API + Frontend UI for same feature
- **Different modules:** User service + Payment service
- **Different layers:** Database migration + Application code
- **Research tasks:** Investigate option A + Investigate option B

### Poor Candidates

- **Shared files:** Both tasks modify `config.yaml`
- **Sequential logic:** Task B calls API built by Task A
- **Shared state:** Both tasks write to same database table
- **Unclear boundaries:** "Implement feature" with no decomposition

## Dispatch Pattern

### 1. Identify Independence

Before dispatching, verify:

```
□ No shared files between tasks
□ No sequential dependencies (A before B)
□ Clear success criteria for each task
□ Defined integration point (how streams merge)
```

### 2. Define Each Stream

For each parallel task:

```markdown
## Task: [Name]

**Scope:** [What this stream handles]
**Files:** [Files this stream will touch]
**Success:** [How to know it's done]
**Output:** [What it produces for integration]
```

### 3. Dispatch

Use the Task tool with multiple parallel invocations:

```
[Single message with multiple Task tool calls]
- Task 1: Backend API implementation
- Task 2: Frontend component implementation
- Task 3: Database migration
```

### 4. Coordinate Results

When streams complete:
- Verify each stream's success criteria
- Check for unexpected conflicts
- Integrate outputs
- Run integration tests

## Critical Rules

### Always

- Verify independence before dispatching
- Define clear boundaries for each stream
- Specify success criteria upfront
- Plan the integration point
- Check for conflicts after completion

### Never

- Parallelize tasks with shared mutable state
- Assume streams won't conflict
- Skip the integration verification
- Dispatch without clear success criteria
- Ignore early warnings from parallel streams

## Integration with Loaf Workflow

| Command | Parallel Opportunity |
|---------|---------------------|
| `/breakdown` | Identify parallelizable tasks during decomposition |
| `/implement` | Single task, usually sequential |
| `/orchestrate` | Automatically parallelizes independent tasks |

## Conflict Resolution

When parallel streams produce conflicts:

1. **File conflicts:** Review both changes, merge manually
2. **Logic conflicts:** Determine correct behavior, update one stream
3. **Interface conflicts:** Agree on contract, update implementations

## Example: Parallel Feature Implementation

```markdown
## Feature: User Profile Page

### Stream A: Backend (backend-dev agent)
- Files: `api/users.py`, `tests/test_users.py`
- Success: GET /users/{id}/profile returns user data
- Output: API contract documented

### Stream B: Frontend (frontend-dev agent)
- Files: `components/Profile.tsx`, `tests/Profile.test.tsx`
- Success: Profile component renders mock data
- Output: Component accepts ProfileData prop

### Integration Point
- Connect frontend to real API
- E2E test: profile page loads real user data
```

## Related Skills

- `orchestration` - Manages multi-task execution
- `foundations` - Task decomposition principles
- Agent definitions - Specialized agents for parallel dispatch
