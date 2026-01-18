# Session Management

Sessions are coordination artifacts for active work. They are ephemeral - deleted when work completes, not archived.

## When to Use Sessions

- Multi-step work requiring agent coordination
- Handoffs between agents
- Tracking progress during implementation
- Context preservation across agent spawns

## Session File Format

### Location & Naming

```
.agents/sessions/YYYYMMDD-HHMMSS-<description>.md
```

**Generate timestamps:**
```bash
# Filename timestamp
date -u +"%Y%m%d-%H%M%S"

# ISO timestamp (for YAML)
date -u +"%Y-%m-%dT%H:%M:%SZ"
```

**Good**: `20251204-143000-weather-fallback.md`
**Bad**: `weather-fallback.md` (missing timestamp)
**Bad**: `PLT-123-weather-fallback.md` (Linear ID in filename)

### Required Frontmatter

```yaml
---
session:
  title: "Clear description of work"           # REQUIRED
  status: in_progress                          # REQUIRED: in_progress|paused|completed
  created: "2025-12-04T14:30:00Z"              # REQUIRED: ISO 8601
  last_updated: "2025-12-04T14:30:00Z"         # REQUIRED: ISO 8601
  linear_issue: "BACK-123"                     # Optional
  linear_url: "https://linear.app/..."         # Optional

orchestration:
  current_task: "What's actively being worked" # REQUIRED
  spawned_agents:
    - agent: backend-dev
      task: "Brief task description"
      status: completed                        # pending|in_progress|completed
      summary: "Outcome summary"
---
```

### Required Sections

Every session file MUST have:

1. **`## Context`** - Background for anyone picking up this work
2. **`## Current State`** - Where we are now (MUST be handoff-ready)
3. **`## Next Steps`** - Immediate actions for continuation

### Session Template

```markdown
# Session: [Title]

## Context
Background for anyone picking up this work.
What problem are we solving? Why now?

## Current State
Where we are right now. What just happened.
**This section should ALWAYS be handoff-ready.**

## Execution Progress

### Wave 1: [Name] (ACTIVE)
- [ ] BACK-123 - Brief description
- [x] BACK-124 - Brief description (completed)

### Wave 2: [Name] (blocked by Wave 1)
- [ ] BACK-125 - Brief description

## Technical Context

### Files to Update
- `path/to/file.py` - What needs to change

### Key Commands
```bash
pytest path/to/tests/ -v
mypy path/to/code/
```

## Acceptance Criteria
- [ ] Criterion 1
- [ ] Criterion 2

## Decisions

### Decision 1: [Title]
**Decision**: What was decided
**Rationale**: Why

## Blockers
- Current blocker (if any)

---

## Session Log

### YYYY-MM-DD HH:MM - [Agent/Action]
Brief description of what happened.
```

## Lifecycle States

| State | Description |
|-------|-------------|
| `in_progress` | Work actively happening |
| `paused` | Temporarily stopped |
| `completed` | Ready for cleanup |

## Updating During Work

After each significant action:

1. Update `Current State`
2. Add to progress checklist
3. Update `Files Modified` if applicable
4. Add entry to `Session Log` with timestamp
5. Update `Next Steps`

## Handoff Protocol

### Before Handing Off

1. Update session file with completed work
2. Update `Current State` with where you stopped
3. Update `Next Steps` with immediate actions
4. Include `Files Modified` for context

### Session as Handoff Medium

Each agent:
1. Reads current state from session
2. Performs assigned work
3. Updates session with outcomes
4. Sets up context for next agent

## Completing a Session

Sessions are **deleted, not archived** when complete.

### Knowledge Extraction Checklist

| Information Type | Where It Belongs |
|------------------|------------------|
| Work tracking | External issue (Linear, GitHub) |
| Implementation details | Git commits/PRs |
| Architectural decisions | ADRs (`docs/decisions/`) |
| API contracts | API documentation |
| Remaining work | External issue backlog |

### Deletion Checklist

- [ ] External issue updated with final status
- [ ] Decisions captured as ADRs (if architectural)
- [ ] Lessons learned added to relevant docs
- [ ] Remaining work created as issues
- [ ] No orphaned references to session file
- [ ] Delete the session file

## PM Start Protocol

When starting a new orchestration context:

1. Check for existing active sessions
2. If found, ask user which to resume or start fresh
3. If resuming, read session for context
4. If starting fresh, create new session file

## Hook Integration

### SessionStart Hook
- Lists active sessions
- Provides agent-specific context
- Suggests session review/resume

### SessionEnd Hook
- Displays completion checklist
- Reminds about session file updates
- Shows count of active sessions

### PreCompact Hook
- Identifies recently modified sessions
- Warns about sessions with recent activity
- Suggests state updates or memory snapshots

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Archive sessions indefinitely | Delete after capturing knowledge |
| Use sessions as permanent records | Use proper documentation locations |
| Reference stale sessions | Keep sessions current or delete |
| Store decisions only in sessions | Create ADRs for important decisions |
| Batch session updates | Update after each significant event |
| Keep status as `in_progress` when paused | Update status when state changes |
