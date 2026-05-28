---
id: TASK-018
title: Add context-awareness to orchestrate and implement commands
spec: SPEC-003
status: done
priority: P1
created: '2026-01-24T03:10:00.000Z'
updated: '2026-01-24T03:10:00.000Z'
files:
  - src/commands/orchestrate.md
  - src/commands/implement.md
verify: grep -l 'context' src/commands/orchestrate.md src/commands/implement.md
done: >-
  Commands check context health before starting work and suggest
  PreCompact/resume if low
completed_at: '2026-01-24T03:10:00.000Z'
---

# TASK-018: Add context-awareness to orchestrate and implement commands

## Description

Before starting multi-step work, `/orchestrate` and `/implement` should evaluate context health and suggest appropriate action if context is low.

## Behavior

### Proactive Context Management

**Don't wait for context to fill.** Recommend restart/clear as soon as:
1. Current conversation no longer holds unique value (decisions captured in files)
2. Major phase of work completes (task done, spec done)
3. New skills/commands were added mid-session (restart needed to load them)
4. About to start large multi-task orchestration

### Context Check (at command start)

1. Evaluate conversation depth/complexity
2. Assess scope of upcoming work (number of tasks, estimated context consumption)
3. Check if new commands were added this session (require restart, not just /clear)
4. If restart/clear recommended:
   - Explain why (context value, new skills, upcoming work scope)
   - Generate resumption prompt
   - Update session file
   - Suggest: restart (if new skills) or /clear (if just context)

### Resumption Prompt Generation

Generate a self-contained prompt ready to paste, including:
- Current branch and git state
- Spec/task being worked on
- Immediate next action
- Key files to read
- Design decisions made in this session
- Any new commands/skills that require restart to load

Format as a code block the user can copy directly.

**Include the action in the prompt** - tell the agent what to do, not just context. User pastes, agent executes.

### When to Recommend

| Trigger | Recommendation | Reason |
|---------|----------------|--------|
| Task completed | Consider restart | Fresh context for next task |
| New command added | Restart required | Skill list determined at session start |
| Orchestration about to start | Restart if stale | Need full context for multi-task work |
| Conversation > 30 exchanges | Suggest restart | Context quality degrades |
| Major decisions captured in files | Suggest restart | Conversation no longer holds unique value |

### Integration with Session Files

Before recommending restart/clear:
1. Update session file with current state (handoff-ready)
2. Ensure all decisions captured in session or ADRs
3. **Write `## Resumption Prompt` section to session file** (for TASK-019 /resume to find)
4. Display resumption prompt to user (copyable)
5. Commit any uncommitted work (ask user)

## Acceptance Criteria

- [ ] Orchestrate command checks context before starting
- [ ] Implement command checks context before starting
- [ ] Warning displayed when context is likely insufficient
- [ ] Resumption prompt generated with all necessary context
- [ ] Session file updated with handoff-ready state
- [ ] Works with PreCompact hook

## Context

This emerged from a real session where orchestration was about to start but context was too low. Commands should be self-aware about this.

## Work Log

<!-- Updated by session as work progresses -->
