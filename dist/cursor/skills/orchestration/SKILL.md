---
name: orchestration
description: >-
  Coordinates multi-agent work: agent delegation, journal continuity, and
  council workflows. Use when delegating to agents or coordinating cross-cutting
  work across multiple agents. Not for single-task implementation (use direct
  tool delegation) or solo research (use research).
version: 0.3.1
---

# Orchestration

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Philosophy
- Configuration
- Artifact Locations
- Workflow by Lifecycle

Comprehensive patterns for orchestration: coordinating multi-agent work, keeping the project journal current, running councils, and delegating to specialized agents.

## Critical Rules

### Journal
- Log `loaf journal log "skill(orchestration): <intent>"` as the first action. There is no session to start — journaling is continuous.
- Use `loaf journal log` for entries: `decision(scope)`, `discover(scope)`, `block(scope)`, `spark(scope)`, `todo(scope)`
- **JOURNAL NUDGE**: When you see this hook trigger, log unrecorded decisions or findings before responding. Use `loaf journal log "entry(scope): description"`. Only log actions (decisions made, things discovered, conclusions reached) — not thoughts or read-only work.
- Write an optional `wrap(scope)` entry only when the conversation holds synthesis worth saving. Nothing is ever ended or archived; a conversation without a wrap leaves a valid journal.
- Continuity is derived and ephemeral. When the exact current target mode has a supported startup adapter, it may emit a layered digest at conversation start. When the capability is candidate or unsupported, explicitly run `loaf journal context` at conversation start. Pull more on demand with `loaf journal recent`, `loaf journal search`, or `loaf journal context`.

### Councils
- Always odd number: 5 or 7 agents
- Councils advise, users decide
- Orchestrator coordinates but doesn't vote
- Spawn all agents in parallel

### Linear

Use the `linear` skill for MCP selection, reads and mutations, update formatting, and the Loaf issue authority boundary. If Linear is unavailable or disabled for the project, coordinate with the project journal and `loaf issue` only.

### Planning (Shape Up)
- Complexity-based sizing (small / medium / large)
- Shape before building (boundaries, not tasks)
- Priority ordering with go/no-go gates between tracks
- No backlogs -- bet or let go

## Verification

- Verify `loaf journal recent` / `loaf journal context` reflect the current work
- Validate council files with `validate-council.py` before concluding
- When Linear participates, verify the Linear skill's read-before-write and outcome-reporting contract

## Quick Reference

| Task | Action |
|------|--------|
| Multi-step work | Log the intent, spawn agents |
| Complex decision | Convene council (5-7 agents, odd) |
| Linear work | Use the Linear skill; keep Loaf issue execution authoritative |
| Feature planning | Size by complexity, shape before building |
| Agent selection | Match domain expertise to task |
| Stuck on task | Check priority order, consider reshaping |
| Pre-compaction | On an exact target mode with supported compaction delivery, hooks may nudge a journal flush and emit the digest afterward; otherwise flush manually and run `loaf journal context` after compaction |
| Durable artifact handling | Delegate `.agents/`-scoped report/spec/handoff/knowledge tending to `librarian` |
| Low-priority work | Spawn background-runner (see Background Agents) |
| New feature workflow | Pitch -> Shape -> Implement -> Ship -> Release |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Shaping Issues | [../shape/SKILL.md](../shape/SKILL.md) | Preparing issues: body, definition of done, out of scope |
| Decomposition | [../shape/SKILL.md](../shape/SKILL.md) | Promoting a criterion that earns its own DoD (`loaf issue promote`) |
| Working Issues | [references/local-tasks.md](references/local-tasks.md) | Frontier, started worktrees, status, definition of done |
| Agent Delegation | [references/delegation.md](references/delegation.md) | Choosing agents, spawning subagents, decision trees |
| Parallel Agents | [references/parallel-agents.md](references/parallel-agents.md) | Dispatching independent work concurrently |
| Subagent Development | [references/subagent-development.md](references/subagent-development.md) | Delegating to specialized agents |
| Background Agents | [references/background-agents.md](references/background-agents.md) | Running non-interactive work in background |
| Council Workflow | [../council/SKILL.md](../council/SKILL.md) | Convening councils for complex decisions |
| Journal Continuity | [references/journal.md](references/journal.md) | Journal-first model, logging protocol, derived continuity, recovery |
| Context Management | [references/context-management.md](references/context-management.md) | Clearing/compacting context, managing context limits |
| Linear Workflows | `linear` skill | Selecting a configured Linear MCP, managing Linear work, or coordinating Loaf-backed issues |
| Script Surface | [references/script-surface.md](references/script-surface.md) | Deciding whether helper scripts should become CLI commands |

## Philosophy

**You are the orchestrator, not the implementer.**

The orchestrator:
1. Creates issues and logs the orchestration intent for tracking
2. Picks from `loaf issue frontier` and starts or joins the shippable root workspace
3. Spawns specialized agents for implementation
4. Coordinates outcomes and updates external systems
5. Never implements code, tests, or documentation directly

Every release should be complete, polished, and delightful.

## Configuration

This skill uses paths from `.agents/loaf.json`:

```json
{
  "councils_directory": ".agents/councils"
}
```

## Artifact Locations

| Artifact | Location | Archive | Naming |
|----------|----------|---------|--------|
| Journal | Global SQLite (`loaf journal recent/search`) | N/A — continuous project-scoped log | Project-scoped, harness-id tagged |
| Councils | `.agents/councils/` | `.agents/councils/archive/` | `YYYYMMDD-HHMMSS-topic.md` |
| Handoffs | `.agents/handoffs/` | delete after deprecated | Created by handoff |
| Reports | `.agents/reports/` | N/A | `YYYYMMDD-HHMMSS-subject.md` |
| Issues | SQLite (`loaf issue show/list`) | `cancelled` / `duplicate` via `loaf issue status` | Alias or opaque id |

**Rule:** Agents write artifacts to disk, orchestrator reasons over artifacts, users retrieve from disk.

## Workflow by Lifecycle

### BEFORE (Planning)
- Shape prepares issues; implement works the frontier. Decomposition is `loaf issue promote` inside shape.
- Log the orchestration intent with `loaf journal log`
- `loaf issue check <ref>` must report shaped (delivery) or ready (decision); identify agents; get user approval

### DURING (Execution)
- Spawn specialized agents (never implement directly)
- Track progress with `loaf journal log` and external issue updates
- Convene councils for uncertain decisions

### AFTER (Completion)
- Code review + QA testing
- Land via ship: `loaf issue status <ref> done`, then `loaf issue stop <ref>`
- Ensure knowledge captured in permanent locations
- Write an optional `wrap` journal entry if the conversation holds synthesis worth saving
