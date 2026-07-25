---
name: implement
description: >-
  Orchestrates implementation work through agent delegation and batch execution.
  Use for all implementation work — features, bug fixes, refactors, and code
  changes. Logs to the project journal and produces agent spawn plans and
  progress tracking. Not for shaping (use shape), breakdown (use breakdown),
  research, or review.
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
- Journal First
- Guardrails
- Decision Tree
- Startup Checklist
- Then Execute
- Topics
- Related Skills

**Input:** $ARGUMENTS

---

## Critical Rules

**You are the ORCHESTRATOR, not the implementer.**

- Log `loaf journal log "skill(implement): <task/spec/context>"` as the first action.

### Orchestrator Can Do Directly
- Log journal entries, read journal context, create council files
- Use TodoWrite/TodoRead; **if `integrations.linear.enabled` is `true` in `.agents/loaf.json`**, use Linear MCP tools when helpful
- Read any file for context
- Ask clarifying questions

### Orchestrator MUST Delegate (via Task Tool)
**ALL code changes, documentation edits, and implementation work** to specialized agents. **No exceptions**, even for "trivial" 1-line fixes.

## Verification

- The invocation is logged to the project journal before implementation work begins — no session start step, no "active session" precondition
- All code changes delegated via Task tool -- no direct edits by orchestrator
- The journal is continuously updated with spawns, progress, and decisions as work happens
- Spec artifacts closed out on branch before PR creation
- **Linear-native mode:** `blockedBy` of the target sub-issue is fully `completed` before work begins; starting a sub-issue also promotes an unstarted parent rollup to active; parent rollup is auto-closed only when all sub-issues are `completed`

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
| New command/skill added this conversation | **Restart required** (skills loaded at start) |
| Conversation > 30 exchanges | Suggest restart |
| Just completed a different task/spec | Suggest clear |
| About to start multi-file implementation | Check depth |

If restart needed: log current state with `loaf journal log`, then ask the user to restart. A supported startup adapter may reconstruct continuity from the journal in the next conversation; when the exact current target mode is candidate or unsupported, explicitly run `loaf journal context` after restarting.

## Input Detection

Parse `$ARGUMENTS` to determine the work type:

| Input Pattern | Type | Action |
|---------------|------|--------|
| `TASK-XXX` | Local task | Load via `loaf task show`, log the task coupling |
| `SPEC-XXX` | Spec orchestration | If spec frontmatter has `linear_parent`, resolve to that Linear parent and follow Linear-Native Routing. Otherwise resolve local tasks and build dependency-ready rounds |
| `TASK-XXX..YYY` | Task range | Expand range, build dependency-ready rounds |
| `TASK-XXX,YYY,ZZZ` | Task list | Parse list, build dependency-ready rounds |
| `PLT-123`, `ENG-198`, `PROJ-123` | Linear issue | **If `integrations.linear.enabled` is `true`:** fetch via `get_issue`, then branch on parent vs sub-issue — see [Linear-Native Routing](#linear-native-routing). **Otherwise:** treat as label text or create local task |
| Description text | Ad-hoc | Auto-create local task from description, then fall through to task-coupled flow |

### Task-Coupled Work

When starting from `TASK-XXX`:

1. Load task metadata via `loaf task show TASK-XXX --json`; do not recreate `.agents/TASKS.json` after the SQLite cutover
2. Log the task coupling: `loaf journal log "decision(implement): implementing TASK-XXX"`
3. Load parent spec if task has `spec:` field

### Ad-hoc Task Auto-Creation

When input is free-text description (not matching any known pattern):

1. **Parse the description:**
   - Single sentence → use entire text as task title
   - Multi-sentence → first sentence = title, remainder = acceptance criteria
   - Split on `. ` followed by uppercase letter only (conservative — avoids false positives from URLs, abbreviations)
2. **Create the task:** `loaf task create --title "<parsed title>"`
3. **Write criteria** (if multi-sentence): edit the task `.md` file body to add the remaining sentences as acceptance criteria
4. **Fall through** to the task-coupled flow above — the result is a `TASK-XXX` ID that enters the existing planning pipeline unchanged

**No user interaction required.** The description IS the task; invoking `/implement` already expressed intent.

### Non-Existent Task ID Error

If input matches `TASK-XXX` pattern but `loaf task show` cannot resolve it:

1. Show error: `"TASK-XXX not found in local task state"`
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
   - **Refuse to start.** Do not begin work. Do not move the issue.
   - Show the blockers: `"Cannot start <sub-issue-id>. Blocked by: <list
     with IDs, titles, and current states>."`
   - Suggest: `"Complete the blocker(s) first, or ask to override if the
     blockedBy link is stale."`
2. If blockers are clear:
   - Start the sub-issue as one logical Linear operation. This moves
     the sub-issue to the team's `started`/In Progress state and, when the
     parent rollup is still `backlog` or `unstarted`, promotes the parent to
     the same `started`/In Progress state.
   - If the parent is already active, leave it unchanged. If the parent is
     `completed`, `canceled`, or archived, refuse to start unless the user
     explicitly asks to override the protected parent state.
   - If the child update succeeds but parent promotion fails, report a
     reconciliation error naming the parent issue before continuing.
   - Resolve branch name from the sub-issue's `branchName` field (Linear
     auto-generates one) — see
     [branch-and-completion.md](references/branch-and-completion.md).
   - Log the task coupling, then continue with the standard Startup Checklist.

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
| Implementation starts | `started` / In Progress | promoted to `started` / In Progress if still `backlog` or `unstarted` |
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
  moment: Linear for issue state, local files for spec content, the project
  journal for current handoff.

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

## Journal First

There is no session to start — journaling is continuous. Your first action is to log the invocation:

```bash
loaf journal log "skill(implement): <task/spec/context>"
```

Entries are project-scoped and tagged with this conversation's harness id automatically. Continuity from prior conversations may arrive through a supported startup adapter; when the exact current target mode is candidate or unsupported, pull it explicitly with `loaf journal context`. Use `loaf journal recent` when you need a narrower timeline.

Suggest renaming the harness conversation with a meaningful name derived from context:
- From spec: `Suggestion: /rename SPEC-027-session-stability`
- From task: `Suggestion: /rename TASK-042-login-fix`
- From ad-hoc: `Suggestion: /rename {short-slug-from-description}`

---

## Guardrails

1. **Strict delegation** -- ALL implementation via Task tool
2. **Keep this conversation lean** -- focus on planning, coordination, oversight
3. **When uncertain** -- convene council, present results, **wait for user approval**
4. **Ensure quality** -- spawn implementer for tests, route reviews to reviewer subagents
5. **When debugging** -- if a test failure or error isn't immediately obvious, load the **debugging** skill for structured hypothesis tracking before retrying
6. **Journal continuously** -- log spawns, progress, blockers, and decisions with `loaf journal log` as they happen
7. **Clean up** -- no ephemeral files; write an optional `wrap` entry only when there's synthesis worth saving
8. **When in doubt, ask the user**

## Decision Tree

```
Is this a code/config/doc change?
+-- YES -> Spawn appropriate agent
+-- NO -> Is this a planning/coordination decision?
    +-- YES with clear path -> Proceed, log the decision
    +-- YES but ambiguous -> Ask user
    +-- NO -> Ask user
```

When multiple valid approaches exist: spawn council (5-7 agents, odd), present results, **wait for approval**, then spawn implementation.

---

## Startup Checklist

1. [ ] Log the invocation: `loaf journal log "skill(implement): <context>"`
2. [ ] Parse input (task, Linear ID, or description)
3. [ ] If TASK-XXX: load task via `loaf task show TASK-XXX`, log task coupling, load parent spec
4. [ ] If Linear ID (or `SPEC-XXX` with `linear_parent`): follow [Linear-Native Routing](#linear-native-routing). Parent → walk sub-issues and select next. Sub-issue → verify `blockedBy` is clear, then start it as one logical Linear operation so the parent is promoted when needed
5. [ ] If description: auto-create task (see Ad-hoc Task Auto-Creation above)
6. [ ] Create dedicated branch (see [branch-and-completion.md](references/branch-and-completion.md))
7. [ ] Suggest team based on task context
8. [ ] Log initial context and references with `loaf journal log`
9. [ ] Break down work using TodoWrite
10. [ ] Identify needed specialized agents
11. [ ] Log next steps before spawning
12. [ ] **Get user approval** before spawning

---

## Then Execute

### BEFORE (Planning)
1. Log the invocation with `loaf journal log`
2. Set task status: `loaf task update TASK-XXX --status in_progress`
3. Break down work into agent-sized tasks
4. Identify spawn order (respect dependencies)
5. Get user approval

### DURING (Execution)
1. Spawn specialized agents via Task tool
2. Log each spawn with `loaf journal log "todo(agent): spawned <agent> for <task>"`
3. Update Linear with progress (no emoji, no file paths)
4. Keep journal entries handoff-ready
5. After each agent completes: log outcome, spawn next

### AFTER (Completion)
1. Code review pass (spawn `reviewer` agent)
2. Spawn implementer (with foundations + language skill) for final testing
3. **Close out spec artifacts on the branch** (included in the squash merge):
   - **Local-tasks mode:** `loaf task update TASK-XXX --status done` (per task), then `loaf task archive --spec SPEC-XXX`
   - **Linear-native mode:** `update_issue` the sub-issue to `completed`-type state. Then query the parent's sub-issues; if all are `completed`, also close the parent. If some remain, list them for the user (see [Linear-Native Routing → Completion](#completion-after-implementer--reviewer-finish-cleanly))
   - Mark spec complete and archive: `loaf spec archive SPEC-XXX` (both modes)
   - Write a `wrap(scope)` journal entry if the work produced synthesis worth saving (next steps, abandoned paths); otherwise skip it
   - Commit: `chore: close SPEC-XXX — archive tasks and spec`
4. If on a feature branch: push and create PR (`gh pr create`). Follow PR format and squash merge conventions in [commits reference](../git-workflow/references/commits.md).
5. After PR is created and approved, use `/ship` to review, verify, and land the PR. Use `/release` later when a coherent batch of landed work is ready to publish.
6. **Suggest reflection:** Check the journal for extractable learnings before closing out:
   - `decision(...)` entries are present
   - ADRs, report verdicts, or spec changelog entries were recorded
   If any signal is present, suggest: *"This produced key decisions. Consider running `/reflect` to update strategic docs."* If none are present, stay silent.

---

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Batch Orchestration | [batch-orchestration.md](references/batch-orchestration.md) | Running specs, task ranges, or task lists with dependency-ready rounds |
| Branch and Completion | [branch-and-completion.md](references/branch-and-completion.md) | Branch management, team routing, diagrams, Linear sync, journaling, task completion |

---

## Suggests Next

After all tasks are complete, suggest `/ship` to land the PR. Suggest `/release` only when the landed work forms a coherent release batch.

## Related Skills

- **shape** - Spec format and lifecycle
- **breakdown** - Turning specs into tasks
- **orchestration/local-tasks** - Task file format and lifecycle
- **orchestration/journal** - Project journal continuity model
