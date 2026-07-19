---
name: orchestration
description: >-
  Coordinates multi-agent work: agent delegation, journal continuity, Linear
  integration, and council workflows. Use when delegating to agents or
  coordinating cross-cutting work across multiple agents. Not for single-task
  implementation (use direct tool delegation) or solo research (use research).
version: 2.0.0-alpha.11
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
- Three-Phase Workflow

Comprehensive patterns for orchestration: coordinating multi-agent work, keeping the project journal current, running councils, delegating to specialized agents, and integrating with Linear.

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
- Checkboxes only (`- [x]`), no emoji
- Outcome-focused, self-contained, no local file references
- Magic words in commit body, not subject

**If `integrations.linear.enabled` is `true` in `.agents/loaf.json`:** use Linear MCP workflows and [references/linear.md](references/linear.md) for issue updates and status.

**Otherwise:** coordinate with the project journal and `loaf task` / file-based tracking only; do not assume Linear MCP tools are available.

### Planning (Shape Up)
- Complexity-based sizing (small / medium / large)
- Shape before building (boundaries, not tasks)
- Priority ordering with go/no-go gates between tracks
- No backlogs -- bet or let go

## Verification

- Verify `loaf journal recent` / `loaf journal context` reflect the current work
- Validate council files with `validate-council.py` before concluding
- Confirm Linear issue updates are self-contained (no local paths, no emoji)

## Quick Reference

| Task | Action |
|------|--------|
| Multi-step work | Log the intent, spawn agents |
| Complex decision | Convene council (5-7 agents, odd) |
| Linear update | Checkboxes, no emoji, no local paths |
| Feature planning | Size by complexity, shape before building |
| Agent selection | Match domain expertise to task |
| Stuck on task | Check priority order, consider reshaping |
| Pre-compaction | On an exact target mode with supported compaction delivery, hooks may nudge a journal flush and emit the digest afterward; otherwise flush manually and run `loaf journal context` after compaction |
| Durable artifact handling | Delegate `.agents/`-scoped report/spec/handoff/knowledge tending to `librarian` |
| Low-priority work | Spawn background-runner with run_in_background |
| New feature workflow | Research -> Architecture -> Shape -> Breakdown -> Implement |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Shaping Specs | [../shape/SKILL.md](../shape/SKILL.md) | Creating specs, shaping work, defining test conditions |
| Breaking Work Into Tasks | [../breakdown/SKILL.md](../breakdown/SKILL.md) | Turning shaped specs into implementation tasks |
| Local Tasks | [references/local-tasks.md](references/local-tasks.md) | Managing tasks locally or with Linear backend |
| Agent Delegation | [references/delegation.md](references/delegation.md) | Choosing agents, spawning separate Codex thread or explicit multi-agent tool when available, decision trees |
| Parallel Agents | [references/parallel-agents.md](references/parallel-agents.md) | Dispatching independent work concurrently |
| separate Codex thread or explicit multi-agent tool when available Development | [references/separate Codex thread or explicit multi-agent tool when available-development.md](references/separate Codex thread or explicit multi-agent tool when available-development.md) | Delegating to specialized agents |
| Background Agents | [references/background-agents.md](references/background-agents.md) | Running non-interactive work in background |
| Council Workflow | [../council/SKILL.md](../council/SKILL.md) | Convening councils for complex decisions |
| Journal Continuity | [references/journal.md](references/journal.md) | Journal-first model, logging protocol, derived continuity, recovery |
| Context Management | [references/context-management.md](references/context-management.md) | Using /clear, /compact, managing context limits |
| Linear Integration | [references/linear.md](references/linear.md) | Updating Linear issues, magic words, status conventions |
| Script Surface | [references/script-surface.md](references/script-surface.md) | Deciding whether helper scripts should become CLI commands |

## Philosophy

**You are the orchestrator, not the implementer.**

The orchestrator:
1. Creates issues and logs the orchestration intent for tracking
2. Breaks down work into delegable tasks
3. Spawns specialized agents for implementation
4. Coordinates outcomes and updates external systems
5. Never implements code, tests, or documentation directly

Every release should be complete, polished, and delightful.

## Configuration

This skill uses paths from `.agents/loaf.json`:

```json
{
  "councils_directory": ".agents/councils",
  "linear": {
    "workspace": "your-workspace-slug",
    "project": { "id": "...", "name": "..." },
    "default_team": "Platform"
  }
}
```

## Artifact Locations

| Artifact | Location | Archive | Naming |
|----------|----------|---------|--------|
| Journal | Global SQLite (`loaf journal recent/search`) | N/A — continuous project-scoped log | Project-scoped, harness-id tagged |
| Councils | `.agents/councils/` | `.agents/councils/archive/` | `YYYYMMDD-HHMMSS-topic.md` |
| Handoffs | `.agents/handoffs/` | delete after deprecated | Created by `/handoff` |
| Reports | `.agents/reports/` | N/A | `YYYYMMDD-HHMMSS-subject.md` |
| Tasks | SQLite (`loaf task show/list`) | N/A | Per task manager conventions |

**Rule:** Agents write artifacts to disk, orchestrator reasons over artifacts, users retrieve from disk.

## Three-Phase Workflow

### BEFORE (Planning)
- Create/check external issue (Linear, GitHub)
- Log the orchestration intent with `loaf journal log`
- Break down into tasks, identify agents, get user approval

### DURING (Execution)
- Spawn specialized agents (never implement directly)
- Track progress with `loaf journal log` and external issue updates
- Convene councils for uncertain decisions

### AFTER (Completion)
- Code review + QA testing
- Update external issue to Done
- Ensure knowledge captured in permanent locations
- Write an optional `wrap` journal entry if the conversation holds synthesis worth saving
