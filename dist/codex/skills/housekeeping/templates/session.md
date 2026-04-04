# Session File Template

**Location:** `.agents/sessions/YYYYMMDD-HHMMSS-<description>.md`

```yaml
---
spec: SPEC-XXX
branch: feat/feature-name
status: active
created: "YYYY-MM-DDTHH:MM:SSZ"
last_entry: "YYYY-MM-DDTHH:MM:SSZ"
---

# Session: Brief Description

## Journal

- YYYY-MM-DD HH:MM resume(branch-name): from commit abc1234, context summary

- YYYY-MM-DD HH:MM decide(scope): description of decision
- YYYY-MM-DD HH:MM discover(scope): something learned

- YYYY-MM-DD HH:MM block(scope): what is blocked
- YYYY-MM-DD HH:MM hypothesis: theory being tested

- YYYY-MM-DD HH:MM unblock(scope): how it was resolved
- YYYY-MM-DD HH:MM commit(abc1234): "commit message"

--- PAUSE YYYY-MM-DD HH:MM ---

- YYYY-MM-DD HH:MM resume(branch-name): duration paused, last action summary
```

## Entry Types

| Type | Use For | Written By |
|------|---------|------------|
| `resume(scope)` | Session started/resumed | Auto (hook) |
| `pause` | Session ended | Auto (hook) |
| `commit(SHA)` | Code committed | Auto (hook) |
| `decide(scope)` | Key decisions with rationale | Agent |
| `discover(scope)` | Something learned | Agent |
| `block(scope)` | Blocker encountered | Agent |
| `unblock(scope)` | Blocker resolved | Agent |
| `spark(scope)` | Ideas to promote | Agent |
| `todo(scope)` | Action items | Agent |
| `conclude(scope)` | Conclusions reached | Agent |

## Format Rules

1. **Timestamp format:** `YYYY-MM-DD HH:MM` (no seconds)
2. **Entry format:** `- <timestamp> <type>(<scope>): <description>`
3. **Blank lines:** Insert when gap ≥ 5 minutes OR state transition (block/unblock)
4. **PAUSE header:** `--- PAUSE YYYY-MM-DD HH:MM ---` (auto-generated, never manual)
5. **Resume after pause:** Always starts new section after `--- PAUSE ---`

## Filename Conventions

- **Task-coupled:** `YYYYMMDD-HHMMSS-task-XXX.md` (auto-generated)
- **Ad-hoc:** `YYYYMMDD-HHMMSS-<description>.md` (kebab-case)
- **No IDs in slugs:** Use descriptive names, not `SPEC-002` or `PLT-123`

## CLI Commands

```bash
loaf session start    # Find/create session, append resume entry, output context
loaf session end      # Append pause entry with summary
loaf session log      # Append typed entry: loaf session log "decide(scope): desc"
loaf session archive  # Move to archive when branch merges
```
