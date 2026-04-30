---
name: implement
description: >-
  Orchestrates implementation sessions through agent delegation and batch
  execution. Use for all implementation work — features, bug fixes, refactors,
  and code changes. Produces session files, agent spawn plans, and progress
  tracking. Not for shaping (use shape), breakdown (use breakdown), research, or
  review.
version: 2.0.0-dev.34
---

# Implement

You are the coordinator. Start by understanding the task:

## Contents
- Critical Rules
- Verification
- Quick Reference
- Step 0: Context Check
- Input Detection
- Linear-Native Routing
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

- Session file exists before any implementation work begins
- All code changes delegated via Task tool -- no direct edits by orchestrator
- Session file is continuously updated with spawns, progress, and current state
- Spec artifacts closed out on branch before PR creation
- **Linear-native mode:** `blockedBy` of the target sub-issue is fully `completed` before any session file is created; parent rollup is auto-closed only when all sub-issues are `completed`

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
| `SPEC-XXX` | Spec orchestration | If spec frontmatter has `linear_parent`, resolve to that Linear parent and follow Linear-Native Routing. Otherwise resolve local tasks and build dependency waves |
| `TASK-XXX..YYY` | Task range | Expand range, build dependency waves |
| `TASK-XXX,YYY,ZZZ` | Task list | Parse list, build dependency waves |
| `PLT-123`, `ENG-198`, `PROJ-123` | Linear issue | **If `integrations.linear.enabled` is `true`:** fetch via `get_issue`, then branch on parent vs sub-issue — see [Linear-Native Routing](#linear-native-routing). **Otherwise:** treat as label text or create local task |
| Description text | Ad-hoc | Auto-create local task from description, then fall through to task-coupled flow |

### Task-Coupled Sessions

When starting from `TASK-XXX`:

1. Load task metadata from TASKS.json via `loaf task show TASK-XXX --json` or read `.agents/TASKS.json` directly
2. Auto-generate session: `YYYYMMDD-HHMMSS-task-XXX.md`
3. Create session file, update task with session reference: `loaf task update TASK-XXX --session <session-file>`
4. Load parent spec if task has `spec:` field

**No user interaction required for session naming.**

### Ad-hoc Task Auto-Creation

When input is free-text description (not matching any known pattern):

1. **Parse the description:**
   - Single sentence → use entire text as task title
   - Multi-sentence → first sentence = title, remainder = acceptance criteria
   - Split on `. ` followed by uppercase letter only (conservative — avoids false positives from URLs, abbreviations)
2. **Create the task:** `loaf task create --title "<parsed title>"`
3. **Write criteria** (if multi-sentence): edit the task `.md` file body to add the remaining sentences as acceptance criteria
4. **Fall through** to the task-coupled flow above — the result is a `TASK-XXX` ID that enters the existing session/plan pipeline unchanged

**No user interaction required.** The description IS the task; invoking `/implement` already expressed intent.

### Non-Existent Task ID Error

If input matches `TASK-XXX` pattern but the task doesn't exist in TASKS.json:

1. Show error: `"TASK-XXX not found in TASKS.json"`
2. Ask the user: `"Did you mean to create a new task? You can re-run with the description as free text."`
3. **Do not silently create** — the user likely has a typo

---

## Linear-Native Routing

Applies when `integrations.linear.enabled` is `true` AND `$ARGUMENTS`
resolves to a Linear issue (direct Linear ID, or a `SPEC-XXX` whose
frontmatter has `linear_parent`).

Fetch the issue once via `get_issue` and branch on its shape:

### Parent rollup issue (has `spec` label)

The issue represents a spec. Do **not** implement it directly — spec-level
"work" is always done via sub-issues.

1. List sub-issues via `list_issues` with `parent: <parent-id>`.
2. Classify each by state:
   - `in_progress` — active work
   - `unstarted` + no open `blockedBy` — ready to start
   - `unstarted` + open `blockedBy` — blocked
   - `completed` — done, skip
3. Select the next work item:
   - If one or more sub-issues are `in_progress`, pick the **lowest-ID**
     in-progress sub-issue. Resume that.
   - Else, if one unblocked `unstarted` sub-issue exists, pick it.
   - Else, if multiple unblocked `unstarted` sub-issues exist, use
     `AskUserQuestion` to let the user choose: pick one, or delegate N in
     parallel via parallel agents. List each sub-issue's title + ID.
   - Else (all remaining sub-issues are blocked), refuse with a summary:
     "All remaining sub-issues under <parent-id> are blocked. Blockers:
     <list>."
4. Once a sub-issue is selected, recurse into the sub-issue flow below
   with that ID. The parent itself is never the implementation target.

### Sub-issue (has `parentId`, no `spec` label)

The issue is an actual task. Implement it directly — with a pre-flight gate.

1. **Pre-flight: verify `blockedBy` is clear.** For each issue in the
   sub-issue's `blockedBy` field, call `get_issue` and confirm its state is
   `completed`-type. If any blocker is not Done:
   - **Refuse to start.** Do not create a session. Do not move the issue.
   - Show the blockers: `"Cannot start <sub-issue-id>. Blocked by: <list
     with IDs, titles, and current states>."`
   - Suggest: `"Complete the blocker(s) first, or ask to override if the
     blockedBy link is stale."`
2. If blockers are clear:
   - Move the sub-issue to `in_progress` state via `update_issue`.
   - Resolve branch name from the sub-issue's `branchName` field (Linear
     auto-generates one) — see
     [session-management.md](references/session-management.md).
   - Create session file, continue with the standard Startup Checklist.

### Completion (after implementer + reviewer finish cleanly)

When the sub-issue's implementation passes review and tests:

1. Move the sub-issue to the team's `completed`-type state via
   `update_issue` (look up via `list_issue_statuses`, filter
   `type: "completed"`).
2. Query the parent's sub-issues again:
   - If **all** sub-issues are now `completed`-type, move the parent
     rollup to `completed` as well. Also mark the local spec as
     `complete` (see [Then Execute → AFTER](#then-execute)).
   - If **some** remain, list them as "next available" for the user,
     applying the same classification as step 2 of the parent flow above.
     Offer to continue with the next one in this session, or stop here.
3. **Do not** close the parent while any sub-issue is open — not even if
   only `blocked` ones remain. Blocked sub-issues are still in-flight
   work from the spec's perspective.

### Status flow summary

| Moment | Sub-issue state | Parent state |
|--------|----------------|--------------|
| Implementation starts | `in_progress` | unchanged |
| Implementation + review pass | `completed` | check: close only if all sibs completed |
| Blocker discovered mid-work | `in_progress` + blocker comment | unchanged |

### What Linear-native routing does NOT do

- Does not pull down the full spec text. The parent's description already
  links to `.agents/specs/SPEC-NNN-*.md`. Read the local file for shape,
  rabbit holes, and strategic tensions.
- Does not create or rewrite sub-issues. That's `/breakdown`'s job. If
  implementation reveals a missing task, surface it to the user; they
  decide whether to run `/breakdown` again or add an ad-hoc sub-issue.
- Does not sync in-progress state bidirectionally. Source of truth at any
  moment: Linear for issue state, local files for spec content, session
  file for current handoff.

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

## Session Creation

**MANDATORY: Create session file BEFORE any other work.**

1. Generate timestamps: `date -u +"%Y%m%d-%H%M%S"` and `date -u +"%Y-%m-%dT%H:%M:%SZ"`
2. Create session file following [session template](templates/session.md)
3. Verify session file exists with valid frontmatter
4. Suggest renaming Claude Code session with a meaningful name derived from context:
   - From spec: `Suggestion: /rename SPEC-027-session-stability`
   - From task: `Suggestion: /rename TASK-042-login-fix`
   - From ad-hoc: `Suggestion: /rename {short-slug-from-description}`

**DO NOT PROCEED WITHOUT A SESSION FILE.**

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

After creating session file:

1. [ ] Parse input (task, Linear ID, or description)
2. [ ] If TASK-XXX: load task via `loaf task show TASK-XXX`, update with `loaf task update TASK-XXX --session <session-file>`, load parent spec
3. [ ] If Linear ID (or `SPEC-XXX` with `linear_parent`): follow [Linear-Native Routing](#linear-native-routing). Parent → walk sub-issues and select next. Sub-issue → verify `blockedBy` is clear, then move to In Progress and continue
4. [ ] If description: auto-create task (see Ad-hoc Task Auto-Creation above)
5. [ ] Create dedicated branch (see [session-management.md](references/session-management.md))
6. [ ] Suggest team based on task context
7. [ ] Populate session Context section
8. [ ] Break down work using TodoWrite
9. [ ] Identify needed specialized agents
10. [ ] Update session Next Steps
11. [ ] **Get user approval** before spawning

---

## Then Execute

### BEFORE (Planning)
1. Create session file
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
   - **Local-tasks mode:** `loaf task update TASK-XXX --status done` (per task), then `loaf task archive --spec SPEC-XXX`
   - **Linear-native mode:** `update_issue` the sub-issue to `completed`-type state. Then query the parent's sub-issues; if all are `completed`, also close the parent. If some remain, list them for the user (see [Linear-Native Routing → Completion](#completion-after-implementer--reviewer-finish-cleanly))
   - Mark spec complete and archive: `loaf spec archive SPEC-XXX` (both modes)
   - Update session file (status: done, `archived_at`, `archived_by`)
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

## Suggests Next

After all tasks are complete, suggest `/release` to merge the work.

## Related Skills

- **orchestration/product-development** - Full workflow hierarchy
- **orchestration/specs** - Spec format and lifecycle
- **orchestration/local-tasks** - Task file format including `session:` field
- **orchestration/sessions** - Session lifecycle details
