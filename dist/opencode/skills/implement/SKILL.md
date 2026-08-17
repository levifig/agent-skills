---
name: implement
description: >-
  Orchestrates implementation work through agent delegation and batch execution
  against Loaf issues. Use for all implementation work — features, bug fixes,
  refactors, and code changes. Picks the next issue from loaf issue frontier,
  delegates one agent per started worktree, and treats definition-of-done
  criteria as the completion contract. Logs to the project journal and produces
  agent spawn plans and progress tracking. Not for shaping or decomposition (use
  shape), research, or review.
subtask: false
version: 0.3.1
---

# Implement

You are the coordinator. Work units are issues.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Step 0: Context Check
- Input Detection
- Pick-up and Dispatch
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

- Log `loaf journal log "skill(implement): LOAF-42 — <what>"` as the first action. Substitute the real alias (or opaque id) and a short intent.
- **Pick-up-next is `loaf issue frontier`.** That view is open (`triage` / `backlog` / `todo`), unblocked, and unclaimed (not `active`, no started worktree). Derived at read time.
- **The delegation brief is the issue row** — `loaf issue show <ref>` / `loaf issue render <ref>`: body, definition-of-done criteria, children. There is no other packet.
- **One agent, one worktree.** `loaf issue start <ref>` creates the branch and worktree and moves status to `active`. Before dispatch, run `loaf issue list --started`. Never send two agents into the same worktree.
- **Definition of done is the completion contract.** `loaf issue verify <ref>` runs V-tier criteria from the repository root and writes nothing. H-tier is reviewed by a human or this orchestrator. Completion is the work landing plus `loaf issue status <ref> done`. Do not flip checkboxes. Provenance is the delivering commits and the PR whose body is `loaf issue render <ref>`.
- Shape prepares issues. If `loaf issue check <ref>` does not report the delivery issue shaped (or the decision issue ready), stop and send the work to shape. Do not mint a new issue from this skill.

### Orchestrator Can Do Directly
- Log journal entries, read journal context, create council files
- Use your harness's todo tracking surface; **if `integrations.linear.enabled` is `true` in `.agents/loaf.json`**, Linear MCP is an overlay only — Loaf issues remain the work unit and Linear never drives Loaf status
- Read any file for context
- Ask clarifying questions
- Run `loaf issue` read commands, `loaf issue start` / `stop`, `loaf issue status`, and open a PR whose body is `loaf issue render` output

### Orchestrator MUST Delegate (via agent spawn)
**ALL code changes, documentation edits, and implementation work** to specialized agents. **No exceptions**, even for "trivial" 1-line fixes. Spawn each agent into that issue's started worktree.

## Verification

- The invocation is logged to the project journal before implementation work begins — no session start step, no "active session" precondition
- All code changes delegated via your harness's agent-spawn mechanism -- no direct edits by orchestrator
- The journal is continuously updated with spawns, progress, and decisions as work happens
- Each in-flight issue has exactly one started worktree; `loaf issue list --started` was checked before every spawn
- V-tier criteria pass `loaf issue verify <ref>` (writes nothing); H-tier criteria were reviewed by a human or this orchestrator
- The PR body is `loaf issue render <ref>` with no manual editing; checkboxes stay unchecked until status is `done`
- Completion is landing plus `loaf issue status <ref> done` (usually via ship)

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

| Moment | Command |
|--------|---------|
| Pick next | `loaf issue frontier` |
| Brief | `loaf issue show <ref>` / `loaf issue render <ref>` |
| Claim workspace | `loaf issue start <ref>` |
| Occupied trees | `loaf issue list --started` |
| V-tier gate | `loaf issue verify <ref>` |
| Landed | `loaf issue status <ref> done` |

---

## Step 0: Context Check

Before starting, evaluate context suitability.

| Trigger | Action |
|---------|--------|
| New command/skill added this conversation | **Restart required** (skills loaded at start) |
| Conversation > 30 exchanges | Suggest restart |
| Just completed a different issue | Suggest clear |
| About to start multi-file implementation | Check depth |

If restart needed: log current state with `loaf journal log`, then ask the user to restart. A supported startup adapter may reconstruct continuity from the journal in the next conversation; when the exact current target mode is candidate or unsupported, explicitly run `loaf journal context` after restarting.

## Input Detection

Parse `$ARGUMENTS` to determine the work:

| Input Pattern | Type | Action |
|---------------|------|--------|
| `LOAF-42` or opaque id | Single issue | Load via `loaf issue show <ref>`; fall through to Pick-up and Dispatch |
| Parent ref with children | Tree | `loaf issue tree <ref>`; build rounds from children and `blocks` / `blocked_by` edges (see [batch-orchestration.md](references/batch-orchestration.md)) |
| Multiple refs | Batch | Same round construction across the named set |
| Empty / "next" | Frontier | `loaf issue frontier`; if one row, pick it; if several, ask (structured question tool if the harness has one); if none, stop |
| Description text | Ad-hoc | Match frontier by title. Do not mint. If nothing matches, stop and send to shape |
| Decision kind | Question | Not implementation. Surface the question; do not `loaf issue start` unless the user points at a delivery issue that records the decided answer |

### Missing ref

If input looks like an issue ref but `loaf issue show` cannot resolve it:

1. Show error: `"<ref> not found"`
2. Ask whether they meant a different alias, or to shape a new issue
3. **Do not silently create**

---

## Pick-up and Dispatch

1. **Confirm the issue is implementable.** `loaf issue check <ref>` must report a delivery issue shaped (or, if the user explicitly asked to resolve a decision issue, that it is ready). Unshaped work goes to shape.
2. **Honor the frontier.** An issue that is blocked does not appear on `loaf issue frontier`. `loaf issue link A blocks B` means A blocks B; B waits until A is `done`, `cancelled`, or `duplicate`. Do not start a blocked successor. Parent/child structure from `loaf issue tree` is not a sequencing edge — only `blocks` / `blocked_by` are. Use the tree to know who belongs in the batch; use the edges to order rounds.
3. **Parents with children are not the implementation target.** Dispatch leaf delivery children that are on the frontier. A parent executes through claimed child criteria, not by starting the parent worktree.
4. **Inspect occupied worktrees:**
   ```bash
   loaf issue list --started
   ```
   Columns: alias, title, `started_branch`, `started_worktree`, optional `(missing)`. If this ref is already started, resume in that worktree with one agent. If the path is occupied by another issue, refuse. A `(missing)` marker means the recorded path is gone — `loaf issue stop <ref>` (not from inside the tree) before starting again.
5. **Start the workspace** (skip if already started and the path exists):
   ```bash
   loaf issue start <ref>
   ```
   Creates branch `issue/<alias-or-id>` in lowercase (`issue/loaf-42`, id suffix when that name is claimed), a sibling worktree, and sets status to `active`. Base is the nearest started ancestor's branch, else the repository default branch. Start refuses archived rows and terminal statuses (`done`, `cancelled`, `duplicate`).
6. **Hand the agent the brief** from `loaf issue show <ref>` (body, criteria, children) and, when opening a PR, `loaf issue render <ref>`. Tell the agent to work only in `started_worktree`.
7. **Batch rounds.** When input is a parent or a set of refs, group unblocked delivery children into dependency-ready rounds from `blocked_by` edges and parent/child structure. Parallel only within a round, max 3, and only when each agent has its own worktree. See [batch-orchestration.md](references/batch-orchestration.md) for the round loop, `--dry-run` / `--parallel` / `--continue` / `--skip <ref>` / `--abort`, and blocked-state recovery.

---

## Agent Spawning

Spawn specialized agents with the appropriate profile:

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

**Rules:** Be specific in prompts. One concern per agent. Include the issue ref, `started_worktree`, body, and definition of done. Parallel when independent (separate worktrees), sequential when a `blocks` edge says so.

---

## Journal First

There is no session to start — journaling is continuous. Your first action is to log the invocation:

```bash
loaf journal log "skill(implement): LOAF-42 — <what>"
```

Entries are project-scoped and tagged with this conversation's harness id automatically. Continuity from prior conversations may arrive through a supported startup adapter; when the exact current target mode is candidate or unsupported, pull it explicitly with `loaf journal context`. Use `loaf journal recent` when you need a narrower timeline.

Suggest renaming the harness conversation with a meaningful name derived from the issue (use your harness's rename surface if it has one):
- From issue: `LOAF-42-login-fix`
- From ad-hoc match: `{alias}-{short-slug}`

---

## Guardrails

1. **Strict delegation** -- ALL implementation via your harness's agent-spawn mechanism
2. **Keep this conversation lean** -- focus on planning, coordination, oversight
3. **When uncertain** -- convene council, present results, **wait for user approval**
4. **Ensure quality** -- spawn implementer for tests, route reviews to reviewer subagents
5. **When debugging** -- if a test failure or error isn't immediately obvious, load the **debugging** skill for structured hypothesis tracking before retrying
6. **Journal continuously** -- log spawns, progress, blockers, and decisions with `loaf journal log` as they happen
7. **Clean up** -- no ephemeral files; write an optional `wrap` entry only when there's synthesis worth saving
8. **When in doubt, ask the user**
9. **Never `loaf issue stop` from inside the started worktree** -- stop does not change status; `--force` removes a dirty tree
10. **Do not tick definition-of-done boxes** -- `loaf issue verify` writes nothing; render checks a box only when status is already `done`

## Decision Tree

```
Is this a code/config/doc change?
+-- YES -> Spawn appropriate agent into the issue worktree
+-- NO -> Is this a planning/coordination decision?
    +-- YES with clear path -> Proceed, log the decision
    +-- YES but ambiguous -> Ask user
    +-- NO -> Ask user
```

When multiple valid approaches exist: spawn council (5-7 agents, odd), present results, **wait for approval**, then spawn implementation.

---

## Startup Checklist

1. [ ] Log the invocation: `loaf journal log "skill(implement): LOAF-42 — <what>"`
2. [ ] Parse input (issue ref, parent, set, frontier, or description)
3. [ ] Load `loaf issue show <ref>`; if children, `loaf issue tree <ref>`
4. [ ] `loaf issue check <ref>` — shaped/ready, or stop and send to shape
5. [ ] Confirm the ref is on `loaf issue frontier` (or already started for resume)
6. [ ] `loaf issue list --started` — one agent per worktree
7. [ ] `loaf issue start <ref>` unless already started
8. [ ] Suggest conversation rename (`LOAF-42-login-fix`)
9. [ ] Identify specialized agents; log next steps
10. [ ] **Get user approval** before spawning

---

## Then Execute

### BEFORE (Planning)
1. Log the invocation with `loaf journal log`
2. `loaf issue start <ref>` (status becomes `active` through start)
3. Slice work into agent-sized units that still belong to this one issue
4. Identify spawn order (respect `blocked_by` edges and parent/child rounds)
5. Get user approval

### DURING (Execution)
1. Spawn specialized agents into `started_worktree` via your harness's agent-spawn mechanism
2. Log each spawn with `loaf journal log "todo(agent): spawned <agent> for <ref>"`
3. Keep journal entries handoff-ready
4. After each agent completes: log outcome, spawn next
5. If Linear overlay is enabled, you may comment there — Loaf status stays on `loaf issue`

### AFTER (Completion)
1. Code review pass (spawn `reviewer` agent)
2. Spawn implementer (with foundations + language skill) for final testing
3. Run `loaf issue verify <ref>` (V-tier, writes nothing). Review every H-tier row yourself or with the user — a skip from verify is not a pass
4. Open or update the PR with body `loaf issue render <ref>` — no manual editing. Follow PR format and squash merge conventions in [commits reference](../git-workflow/references/commits.md)
5. After the PR is created, use ship to review, verify, land, mark `loaf issue status <ref> done`, and `loaf issue stop <ref>`. Use release later when a coherent batch of landed work is ready to publish
6. Write a `wrap(scope)` journal entry if the work produced synthesis worth saving; otherwise skip it
7. **Suggest reflection:** Check the journal for extractable learnings before closing out:
   - `decision(...)` entries are present
   - ADRs or report verdicts were recorded
   If any signal is present, suggest: *"This produced key decisions. Consider running reflect to update strategic docs."* If none are present, stay silent.

---

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Batch Orchestration | [batch-orchestration.md](references/batch-orchestration.md) | Running a parent or a set of issue refs with dependency-ready rounds |
| Branch and Completion | [branch-and-completion.md](references/branch-and-completion.md) | Team routing, diagrams, exploration, journaling alongside `loaf issue start` / `stop` |
| Working issues locally | [../orchestration/references/local-tasks.md](../orchestration/references/local-tasks.md) | Frontier, started worktrees, status vocabulary, definition of done |

---

## Suggests Next

After the PR exists, suggest ship to land it. Suggest release only when the landed work forms a coherent release batch.

## Related Skills

- **shape** — Issue preparation and decomposition
- **orchestration/journal** — Project journal continuity model
- **orchestration/local-tasks** — Frontier, started worktrees, status, definition of done
