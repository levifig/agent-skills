---
name: orchestration
description: >-
  Use for PM-style agent coordination. Covers agent delegation patterns, council
  deliberation workflows, session file management, Linear issue integration, and
  product planning. Activate when coordinating multi-agent work, running
  councils for decisions, managing work sessions, or integrating with Linear.
---

# PM Orchestration

Comprehensive patterns for PM-style orchestration: coordinating multi-agent work, managing sessions, running councils, delegating to specialized agents, and integrating with Linear.

## Philosophy

**PM is the orchestrator, not the implementer.**

The PM agent:
1. Creates issues and session files for tracking
2. Breaks down work into delegable tasks
3. Spawns specialized agents for implementation
4. Coordinates outcomes and updates external systems
5. Never implements code, tests, or documentation directly

Every release should be complete, polished, and delightful - no MVPs or quick hacks.

## Quick Reference

| Task | Action |
|------|--------|
| Multi-step work | Create session file, spawn agents |
| Complex decision | Convene council (5-7 agents, odd) |
| Linear update | Checkboxes, no emoji, no local paths |
| Feature planning | Define appetite, shape before building |
| Agent selection | Match domain expertise to task |
| Stuck on task | Check circuit breaker, consider reshaping |
| Pre-compaction | Spawn context-archiver to preserve state |

## Topics

| Topic | Reference | Key Content |
|-------|-----------|-------------|
| Agent Delegation | [reference/delegation.md](reference/delegation.md) | Agent capabilities, spawn patterns, decision tree |
| Council Workflow | [reference/councils.md](reference/councils.md) | Composition, deliberation, synthesis, user approval |
| Session Management | [reference/sessions.md](reference/sessions.md) | Lifecycle, file format, handoffs, validation |
| Session Resume | [reference/session-resume.md](reference/session-resume.md) | CLI flags, checkpoints, context recovery |
| Context Management | [reference/context-management.md](reference/context-management.md) | /clear, /compact, 2-correction rule, subagents |
| Linear Integration | [reference/linear.md](reference/linear.md) | Update format, magic words, status conventions |
| Product Planning | [reference/planning.md](reference/planning.md) | Shape Up methodology, appetite, roadmaps, specs |

## Configuration

This skill uses paths from `.agents/config.json`:

```json
{
  "sessions": {
    "directory": ".agents/sessions",
    "councils_directory": ".agents/councils"
  },
  "linear": {
    "workspace": "your-workspace-slug",
    "project": { "id": "...", "name": "..." },
    "default_team": "Platform"
  }
}
```

## Available Scripts

| Script | Usage | Description |
|--------|-------|-------------|
| `new-session.sh` | `new-session.sh <description> [linear-issue]` | Generate session file |
| `new-council.sh` | `new-council.sh <topic> <session> <agents...>` | Generate council file |
| `validate-session.py` | `validate-session.py <file>` | Validate session format |
| `validate-council.py` | `validate-council.py <file>` | Validate council format |
| `validate-roadmap.py` | `validate-roadmap.py <file>` | Validate roadmap format |
| `get-config.py` | `get-config.py [key.path]` | Read config values |
| `suggest-team.py` | `suggest-team.py "task desc"` | Suggest Linear team |
| `check-linear-format.py` | `check-linear-format.py <file>` | Validate Linear text |
| `format-progress.sh` | `format-progress.sh "Done" -- "Todo"` | Format progress update |
| `extract-magic-words.sh` | `extract-magic-words.sh HEAD~10..HEAD` | Extract Linear refs |

## Three-Phase Workflow

### BEFORE (Planning)
- Create/check external issue (Linear, GitHub)
- Create session file for internal coordination
- Break down into tasks
- Identify which agents to spawn
- Get user approval before spawning

### DURING (Execution)
- Spawn specialized agents (never implement directly)
- Track progress in session file
- Post progress to external issue
- Convene councils for uncertain decisions

### AFTER (Completion)
- Run code review with backend/frontend devs (if significant changes)
- Run QA testing and security checks
- Update external issue to Done
- Ensure knowledge captured in permanent locations
- Archive session file (set status, `archived_at`, `archived_by`, move to `.agents/sessions/archive/`, update `.agents/` links)

## When to Use PM Orchestration

**Use for:**
- Feature implementation
- Non-trivial bug fixes
- Refactoring
- Infrastructure changes
- Multi-agent coordination

**Skip for:**
- Typo fixes
- Direct questions
- Quick clarifications
- Single-file trivial changes

## Critical Rules

### Sessions
- Filename: `YYYYMMDD-HHMMSS-description.md`
- Required fields: title, status, created, last_updated, current_task
- Archive fields: `archived_at`, `archived_by` (required when archived)
- Required sections: Context, Current State, Next Steps
- Archive when complete (set status, `archived_at`, `archived_by`, move to `.agents/sessions/archive/`, update `.agents/` links)

### Context Preservation (PreCompact)
- PreCompact hook identifies sessions modified in last 60 minutes
- Spawn `context-archiver` agent when prompted by hook
- Agent generates `## Resumption Prompt` section for seamless continuation
- After compaction, read session's Resumption Prompt to pick up where you left off

### Transcript Archival (Post-Compaction)
- Claude Code provides transcript path after `/compact` or `/clear`
- Copy transcript to `.agents/transcripts/` (create directory if needed)
- Add filename to session's `transcripts:` array in frontmatter
- Preserves full audit trail for context recovery

### Councils
- Always odd number: 5 or 7 agents
- Councils advise, users decide
- PM coordinates but doesn't vote
- Spawn all agents in parallel
- Document decision after user approval

### Linear
- Checkboxes only (`- [x]`), no emoji
- Outcome-focused, self-contained
- No local file references
- Use issue ID only (Linear auto-expands)
- Magic words in commit body, not subject

### Planning (Shape Up)
- Appetite over estimates (decide time, flex scope)
- Shape before building (boundaries, not tasks)
- Circuit breakers at 50% to reassess
- Now/Next/Later buckets, no version numbers
- Clear acceptance criteria (Given/When/Then)
- No backlogs - bet or let go
