---
name: implement
description: >-
  Orchestrates implementation sessions through agent delegation and batch
  execution. Use when the user asks "implement this" or "start working on
  TASK-XXX." Produces session files, agent spawn plans, and progress tracking.
  Not for shaping (use shape), breakdown (use breakdown), or single-file edits
  (use direct tools).
version: 2.0.0-dev.11
---

# Implement

You are the coordinator. Start by understanding the task:

## Contents
- Critical Rules
- Verification
- Quick Reference
- Step 0: Context Check
- Input Detection
- Agent Spawning
- Session and Plan Creation
- Session Guardrails
- Decision Tree
- Startup Checklist
- Then Execute
- Topics
- Related Skills

**Input:** $ARGUMENTS

---

## Critical Rules

**You are the ORCHESTRATOR, not the implementer.**

### Orchestrator Can Do Directly
- Create/edit session files, council files
- Use TodoWrite/TodoRead; **if `integrations.linear.enabled` is `true` in `.agents/loaf.json`**, use Linear MCP tools when helpful
- Read any file for context
- Ask clarifying questions

### Orchestrator MUST Delegate (via Task Tool)
**ALL code changes, documentation edits, and implementation work** to specialized agents. **No exceptions**, even for "trivial" 1-line fixes.

## Verification

- Session file AND plan file exist before any implementation work begins
- All code changes delegated via Task tool -- no direct edits by orchestrator
- Session file is continuously updated with spawns, progress, and current state
- Spec artifacts closed out on branch before PR creation

## Quick Reference

| Work Type | Profile | Skills to Load |
|-----------|---------|---------------|
| Python/FastAPI/Rails/Ruby/Go backend | implementer | Language skill + relevant domain skills |
| Next.js/React/Tailwind frontend | implementer | typescript-development + interface-design |
| Schema/migrations/SQL | implementer | database-design + language skill |
| Docker/K8s/CI/CD/Terraform | implementer | infrastructure-management |
| Tests/security audits | implementer | foundations + language skill |
| UI/UX design review | reviewer | interface-design |
| Code review/audit | reviewer | relevant domain skills |
| Research/comparison | researcher | relevant domain skills |

---

## Step 0: Context Check

Before starting, evaluate context suitability.

| Trigger | Action |
|---------|--------|
| New command/skill added this session | **Restart required** (skills loaded at start) |
| Conversation > 30 exchanges | Suggest restart |
| Just completed a different task/spec | Suggest clear |
| About to start multi-file implementation | Check depth |

If restart needed: capture state in session file, generate resumption prompt, ask user to restart.

## Input Detection

Parse `$ARGUMENTS` to determine session type:

| Input Pattern | Type | Action |
|---------------|------|--------|
| `TASK-XXX` | Local task | Load from TASKS.json via CLI, auto-create session |
| `SPEC-XXX` | Spec orchestration | Resolve all tasks, build dependency waves |
| `TASK-XXX..YYY` | Task range | Expand range, build dependency waves |
| `TASK-XXX,YYY,ZZZ` | Task list | Parse list, build dependency waves |
| `PLT-123`, `PROJ-123` | Linear issue | **If `integrations.linear.enabled` is `true`:** fetch from Linear. **Otherwise:** treat as label text or create local task |
| Description text | Ad-hoc | **If Linear enabled:** ask about Linear issue. **Else:** offer `loaf task create` / local session |

### Task-Coupled Sessions

When starting from `TASK-XXX`:

1. Load task metadata from TASKS.json via `loaf task show TASK-XXX --json` or read `.agents/TASKS.json` directly
2. Auto-generate session: `YYYYMMDD-HHMMSS-task-XXX.md`
3. Create session file, update task with session reference: `loaf task update TASK-XXX --session <session-file>`
4. Load parent spec if task has `spec:` field

**No user interaction required for session naming.**

### Ad-hoc Sessions

When no task exists: inform user, ask if Linear issue or local task should be created.

---

## Agent Spawning

Use the **Task tool** with appropriate `subagent_type`:

| Work Type | Profile | Skills to Load |
|-----------|---------|---------------|
| Python/FastAPI/Rails/Ruby/Go backend | implementer | Language skill + relevant domain skills |
| Next.js/React/Tailwind frontend | implementer | typescript-development + interface-design |
| Schema/migrations/SQL | implementer | database-design + language skill |
| Docker/K8s/CI/CD/Terraform | implementer | infrastructure-management |
| Tests/security audits | implementer | foundations + language skill |
| UI/UX design review | reviewer | interface-design |
| Code review/audit | reviewer | relevant domain skills |
| Research/comparison | researcher | relevant domain skills |

**Rules:** Be specific in prompts. One concern per agent. Include context. Parallel when independent, sequential when dependent.

---

## Session and Plan Creation

**MANDATORY: Create session AND plan file BEFORE any other work.**

1. Generate timestamps: `date -u +"%Y%m%d-%H%M%S"` and `date -u +"%Y-%m-%dT%H:%M:%SZ"`
2. Create session file following [session template](templates/session.md)
3. Create plan file following [plan template](templates/plan.md)
4. Update session frontmatter to link the plan
5. Verify both files exist with valid frontmatter
6. Suggest renaming Claude Code session: `/rename {descriptive-name}`

**DO NOT PROCEED WITHOUT A SESSION FILE AND PLAN FILE.**

---

## Session Guardrails

1. **Strict delegation** -- ALL implementation via Task tool
2. **Keep this session lean** -- focus on planning, coordination, oversight
3. **When uncertain** -- convene council, present results, **wait for user approval**
4. **Ensure quality** -- spawn implementer for tests, route reviews to reviewer subagents
5. **When debugging** -- if a test failure or error isn't immediately obvious, load the **debugging** skill for structured hypothesis tracking before retrying
6. **Update session file continuously** -- log spawns, update current_task, keep handoff-ready
6. **Clean up** -- no ephemeral files, archive completed sessions (status + `archived_at` + `archived_by` + move to archive/)
7. **When in doubt, ask the user**

## Decision Tree

```
Is this a code/config/doc change?
+-- YES -> Spawn appropriate agent
+-- NO -> Is this a planning/coordination decision?
    +-- YES with clear path -> Proceed, update session
    +-- YES but ambiguous -> Ask user
    +-- NO -> Ask user
```

When multiple valid approaches exist: spawn council (5-7 agents, odd), present results, **wait for approval**, then spawn implementation.

---

## Startup Checklist

After creating session AND plan files:

1. [ ] Parse input (task, Linear ID, or description)
2. [ ] If TASK-XXX: load task via `loaf task show TASK-XXX`, update with `loaf task update TASK-XXX --session <session-file>`, load parent spec
3. [ ] If Linear ID: fetch issue, update session, move to "In Progress"
4. [ ] If description: ask about creating Linear issue
5. [ ] Create dedicated branch (see [session-management.md](references/session-management.md))
6. [ ] Suggest team based on task context
7. [ ] Fill in plan file sections
8. [ ] Populate session Context section
9. [ ] Break down work using TodoWrite
10. [ ] Identify needed specialized agents
11. [ ] Update session Next Steps
12. [ ] **Get user approval on plan** before spawning

---

## Then Execute

### BEFORE (Planning)
1. Create session + plan files
2. Set task status: `loaf task update TASK-XXX --status in_progress`
3. Break down work into agent-sized tasks
4. Identify spawn order (respect dependencies)
5. Get user approval

### DURING (Execution)
1. Spawn specialized agents via Task tool
2. Log each spawn in session `orchestration.spawned_agents`
3. Update Linear with progress (no emoji, no file paths)
4. Keep session `## Current State` handoff-ready
5. After each agent completes: update session, spawn next

### AFTER (Completion)
1. Code review pass (spawn `reviewer` agent)
2. Spawn implementer (with foundations + language skill) for final testing
3. **Close out spec artifacts on the branch** (included in the squash merge):
   - `loaf task update TASK-XXX --status done` (for each task)
   - `loaf task archive --spec SPEC-XXX`
   - Mark spec complete and archive: `loaf spec archive SPEC-XXX`
   - Update session file (status: complete, `archived_at`, `archived_by`)
   - Commit: `chore: close SPEC-XXX — archive tasks, spec, and session`
4. If on a feature branch: push and create PR (`gh pr create`). Follow PR format and squash merge conventions in [commits reference](../git-workflow/references/commits.md).
5. After PR is created and approved, use `/release` to orchestrate the squash merge with correct version ordering, documentation freshness check, and post-merge cleanup.
6. **Suggest reflection:** Check the session file for extractable learnings before closing out:
   - `## Key Decisions` has content (not `*(none yet)*` or empty)
   - `traceability.decisions` has entries (ADRs were recorded)
   If any signal is present, suggest: *"This session produced key decisions. Consider running `/reflect` to update strategic docs."* If none are present, stay silent.

---

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Batch Orchestration | [batch-orchestration.md](references/batch-orchestration.md) | Running specs, task ranges, or task lists with dependency waves |
| Session Management | [session-management.md](references/session-management.md) | Branch management, team routing, diagrams, plan mode, Linear sync, handoff, archival |

---

## Related Skills

- **orchestration/product-development** - Full workflow hierarchy
- **orchestration/specs** - Spec format and lifecycle
- **orchestration/local-tasks** - Task file format including `session:` field
- **orchestration/sessions** - Session lifecycle details
