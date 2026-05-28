---
id: TASK-019
title: Enhance /resume for intelligent project resumption
spec: SPEC-002
status: done
priority: P1
created: '2026-01-24T03:20:00.000Z'
updated: '2026-01-24T03:20:00.000Z'
files:
  - src/commands/resume.md
verify: grep -l 'review.*sessions\|propose.*plan' src/commands/resume.md
done: /resume with no args reviews project state and proposes action plan
completed_at: '2026-01-24T03:20:00.000Z'
---

# TASK-019: Enhance /resume for intelligent project resumption

## Description

Transform `/resume` from "resume specific session" to "review project state and propose what to do next." Supports returning to a project after days/weeks away.

## Current Behavior

```
/resume <session-file>  → Load specific session, continue where left off
```

## New Behavior

```
/resume                 → Review all open work, propose action plan
/resume TASK-XXX        → Find task's session, resume that work
/resume <session-file>  → (Unchanged) Load specific session
```

### No-Argument Behavior

When `/resume` is called with no arguments:

1. **Scan project state:**
   - Open sessions in `.agents/sessions/` (status != archived)
   - Active tasks in `.agents/tasks/` (status != done)
   - Specs in progress in `.agents/specs/` (status = implementing)

2. **Check for resumption prompts:**
   - Look for `## Resumption Prompt` section in recent sessions
   - These contain actionable instructions from previous context

3. **Propose action plan:**
   ```
   ## Project State

   **Open Sessions:** 2
   - 20260124-025127-orchestrate-command.md (completed, not archived)
   - 20260124-015123-verification-reference.md (completed, not archived)

   **Active Specs:** 3
   - SPEC-001: Loaf Self-Sufficiency (7 tasks remaining)
   - SPEC-002: Invisible Sessions (7 tasks, ready to orchestrate)
   - SPEC-003: Orchestrated Execution (1 task remaining)

   **Resumption Prompt Found:**
   Session 20260124-025127 has a resumption prompt.

   ## Recommended Action

   Run `/loaf:orchestrate SPEC-002` to continue the invisible sessions work.

   Or choose:
   1. /loaf:orchestrate SPEC-002 (7 tasks)
   2. /loaf:implement TASK-017 (document orchestration)
   3. /loaf:implement TASK-018 (context awareness)
   ```

### Resumption Prompt Storage

Store resumption prompts in session files:

```markdown
## Resumption Prompt

> Resume Loaf development and run /loaf:orchestrate SPEC-002.
>
> ## Context
> - Branch: main
> - /loaf:orchestrate command is now available
>
> ## Action
> Run /loaf:orchestrate SPEC-002 to execute the tasks.
```

When context-awareness (TASK-018) recommends restart, it writes this section to the active session before the user leaves.

### Task-Based Resume

```
/resume TASK-016
  → Look up TASK-016 in TASKS.json
  → Find session: field
  → Load that session
  → Continue work
```

## Acceptance Criteria

- [ ] `/resume` with no args scans project state
- [ ] Lists open sessions, active specs, pending tasks
- [ ] Finds and displays resumption prompts from sessions
- [ ] Proposes prioritized action plan
- [ ] `/resume TASK-XXX` finds session from task file
- [ ] Backward compatible: `/resume <session-file>` still works

## Dependencies

- TASK-018 (writes resumption prompts to sessions)

## Context

Users return to projects after time away. They shouldn't need to remember where they left off - `/resume` should tell them.

## Work Log

<!-- Updated by session as work progresses -->
