---
id: SPEC-004
title: Frictionless Task Handoff
created: '2026-01-24T16:00:00.000Z'
status: complete
requirement: >-
  Eliminate copy/paste friction when switching sessions by making task files
  self-contained handoff artifacts
---

# SPEC-004: Frictionless Task Handoff

## Problem Statement

Current workflow has unnecessary friction when switching sessions:

1. **Copy/paste resumption prompts** - End of session generates a prompt, user must copy it, start new session, paste it
2. **Context scattered across artifacts** - Task file has description, session file has decisions, resumption prompt has immediate action
3. **No link between task and creating session** - Tasks don't know where they came from
4. **Manual handoff generation** - Agent must remember to generate resumption prompt

Users think in tasks, not sessions. The task file should BE the handoff artifact.

## Proposed Solution

### 1. Task Files as Self-Contained Context

Task files already have most context. Add:

```yaml
# Task frontmatter additions
created_in_session: 20260124-151610-invisible-sessions.md  # Where task was created
context_sessions:  # Sessions that contributed context (optional)
  - 20260124-143000-research-session.md
handoff: |
  Ready to implement. Start by reading the acceptance criteria.
  Key decision: Use frontmatter version field, not runtime lookup.
```

The `handoff:` field is a brief, actionable instruction - what to do first.

### 2. Session Handoff Section

Sessions get a `## Handoff` section (auto-generated or manual):

```markdown
## Handoff

**For TASK-020:**
Ready to implement. The version should come from command frontmatter.
Start with `src/commands/version.md`.

**For TASK-021:**
Blocked on TASK-020. Wait until /version exists.
```

When a task is created mid-session, the session's handoff section captures context that won't fit in the task file itself.

### 3. `/implement` Consumes Handoff Automatically

When running `/implement TASK-XXX`:

1. Read task file
2. If `created_in_session:` exists, check that session's `## Handoff` section
3. Load any additional context from `context_sessions:`
4. Present unified context to agent - no copy/paste needed

### 4. Session End Creates Handoff

When a session ends (or before `/clear`):

1. Check for tasks created this session
2. Auto-generate `## Handoff` section with context for each
3. Update task files with `created_in_session:` link
4. User can restart cleanly - context lives in files, not clipboard

## Design Decisions

### Task Handoff Field

**Keep it brief** - The `handoff:` field is 1-3 lines, not a full resumption prompt.

```yaml
# Good - actionable, brief
handoff: |
  Start with acceptance criteria. Key file: src/commands/version.md

# Bad - too verbose, duplicates description
handoff: |
  This task implements a /version command that displays the loaded
  Loaf version. The version is stored in frontmatter...
```

### Session Linking

**One-way link from task to session** - Tasks point to sessions, not vice versa.

```yaml
# Task points to creating session
created_in_session: 20260124-151610-session.md

# Session does NOT list tasks it created (would get stale)
```

### Context Sessions

**Optional, for research/architecture sessions** - When a task draws from multiple sessions:

```yaml
context_sessions:
  - 20260124-100000-architecture-research.md  # Design decisions
  - 20260124-120000-spike-prototype.md        # Prototype learnings
```

### Handoff Section Placement

**At end of session file, before Session Log:**

```markdown
## Handoff

[Task-specific handoff notes]

---

## Session Log
```

## Workflow Changes

### Before (Current)

```
1. Work in session
2. Create task
3. Agent generates resumption prompt
4. User copies prompt
5. User runs /clear or restarts
6. User pastes prompt
7. New session begins
```

### After (Proposed)

```
1. Work in session
2. Create task (auto-links to session)
3. Agent updates session ## Handoff
4. User runs /clear or restarts
5. User runs /implement TASK-XXX
6. Command reads task + session handoff
7. New session begins with full context
```

**Elimination:** Steps 3-4-6 (copy/paste) become automatic.

## Scope

### In Scope

**P1: Core Changes**
- Add `created_in_session:` and `handoff:` to task schema
- Add `## Handoff` section to session template
- Update `/implement` to read handoff from linked session

**P2: Automation**
- Auto-populate `created_in_session:` when task created
- Auto-generate `## Handoff` section on session end
- Update `/tasks` command to set session link

**P3: Polish**
- `context_sessions:` support for multi-session context
- Handoff validation (warn if empty)

### Out of Scope

- Automatic session archival (separate concern)
- Linear integration for handoff (keep local-first)
- Resumption across days/weeks (that's `/resume` with project state)

### No-Gos

- Don't duplicate task description in handoff (keep DRY)
- Don't make handoff required (some tasks are self-explanatory)
- Don't auto-generate verbose handoffs (brief > comprehensive)

## Test Conditions

- [ ] Task created mid-session gets `created_in_session:` populated
- [ ] Session `## Handoff` section generated before `/clear`
- [ ] `/implement TASK-XXX` loads context from linked session
- [ ] No copy/paste needed between sessions
- [ ] Backward compatible with existing tasks (missing fields = skip)

## Files to Modify

| File | Changes |
|------|---------|
| `src/skills/orchestration/references/local-tasks.md` | Document new fields |
| `src/skills/orchestration/references/sessions.md` | Document Handoff section |
| `src/commands/implement.md` | Read handoff from linked session |
| `src/commands/tasks.md` | Set `created_in_session:` when creating tasks |

## Migration

Existing tasks work as-is (missing fields are optional). New tasks get fields automatically.

## Circuit Breaker

At 50% appetite: If P1 not working smoothly, defer P2 automation and ship manual workflow.
