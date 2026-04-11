# Cross-Session References

Patterns for importing decisions and context from past sessions into current work without duplicating full context.

## Contents

- When to Reference Past Sessions
- What to Import
- Session Frontmatter for Tracking
- Decision Memory Format
- Workflow
- Serena MCP Integration (Optional)
- Anti-Patterns
- When Decisions Conflict
- Best Practices
- Example Session Log Entry

## When to Reference Past Sessions

### Good Reasons to Reference

- **Continuing related work**: Building on previous feature decisions
- **Consistency**: Ensuring alignment with prior architectural choices
- **Avoiding re-deliberation**: Council decisions already made on similar topics
- **Context recovery**: Picking up work after long gaps

### Skip Referencing When

- Starting genuinely new work unrelated to past sessions
- Past decisions are documented in ADRs (use ADRs instead)
- Context would create more noise than value
- Decisions were superseded or invalidated

## What to Import

### Decisions Only (Default)

Import the distilled outcomes:

```markdown
## Referenced Decisions

### From: 20250115-140000-auth-jwt.md

#### Decision: Token Rotation Strategy
**Decision**: Rotate every 15 minutes
**Rationale**: Security/UX balance
```

**Why decisions only:**
- Minimal context footprint
- Focus on outcomes, not process
- Easy to scan and validate relevance

### Context Summary (When Needed)

Import broader context when:
- Problem space is complex and unfamiliar
- Technical approach was non-obvious
- Multiple related decisions form a coherent strategy

```markdown
## Referenced Context

### From: 20250115-140000-auth-jwt.md

**Problem**: Session management for distributed services
**Approach**: JWT with refresh token rotation
**Key Files**: `src/auth/`, `src/middleware/auth.py`
**Outcome**: Implemented, deployed to production 2025-01-20
```

## Session Frontmatter for Tracking

Track all cross-session references in frontmatter:

```yaml
---
session:
  title: "Auth V2 Implementation"
  status: in_progress
  created: "2025-01-23T10:00:00Z"
  last_updated: "2025-01-23T14:30:00Z"
  linear_issue: "PLT-200"
  referenced_sessions:
    - session: "20250115-140000-auth-jwt.md"
      imported_at: "2025-01-23T14:30:00Z"
      content_type: decisions
      decisions_imported:
        - "JWT token rotation strategy"
        - "Refresh token storage approach"
    - session: "20250110-090000-auth-oauth.md"
      imported_at: "2025-01-23T14:35:00Z"
      content_type: context
      summary: "OAuth provider integration patterns"

orchestration:
  current_task: "Implement token refresh endpoint"
---
```

## Decision Memory Format

When sessions are archived, decisions are extracted to Serena memory:

```markdown
# Memory: session-auth-jwt-decisions.md

## Session Context
- **Session**: 20250115-140000-auth-jwt.md
- **Archived**: 2025-01-15T18:00:00Z
- **Linear Issue**: PLT-123

## Key Decisions

### Decision 1: JWT Token Rotation Strategy
**Decision**: Rotate tokens every 15 minutes with sliding window refresh
**Rationale**: Balances security (limited token lifetime) with UX (seamless refresh)
**Council**: None - implementer recommendation accepted

### Decision 2: Refresh Token Storage
**Decision**: Store refresh tokens in HttpOnly cookies, not localStorage
**Rationale**: Prevents XSS-based token theft; aligns with OWASP recommendations
**Council**: Security council (5 agents) - unanimous

### Decision 3: Token Validation Order
**Decision**: Validate signature before parsing claims
**Rationale**: Fail fast on tampered tokens; reduce processing for invalid requests
**Council**: None - standard security practice
```

## Workflow

### At Wrap Time (CLI)

1. `loaf session end --wrap` persists decisions to spec changelog
2. Session stays in `sessions/` with `status: done` — no immediate archive
3. `loaf session housekeeping` archives complete sessions after 7-day age threshold

### At Reference Time (spec changelog)

1. Read linked spec's `## Changelog` section for archived decisions
2. Decisions are extracted with session branch + timestamp context
3. Track in `referenced_sessions` frontmatter if importing into current session

## Serena MCP Integration (Optional)

If Serena MCP is configured, it can store decision memories across sessions.

### List Available Decision Memories

```python
mcp__serena__list_memories()
# Filter results for: session-*-decisions.md
```

### Read Decision Memory

```python
mcp__serena__read_memory(name="session-auth-jwt-decisions.md")
```

### Write Decision Memory (at archive time)

```python
mcp__serena__write_memory(
    name="session-auth-jwt-decisions.md",
    content=extracted_decisions_markdown
)
```

If Serena is not configured, decision memories can be stored as files in `.agents/decisions/` instead.

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Import entire session files | Import only decisions/summaries |
| Import without tracking | Update `referenced_sessions` in frontmatter |
| Import unrelated sessions | Search by topic, Linear issue, or keyword |
| Duplicate ADR content | Reference the ADR directly |
| Import stale decisions | Verify decisions are still relevant |
| Over-reference | Import only what's needed for current work |
| Reference without reading | Always review imported content for relevance |

## When Decisions Conflict

If imported decisions conflict with current approach:

1. **Note the conflict** in session file
2. **Assess if prior decision still applies**
   - Different context? Proceed with new approach
   - Same context? Consider why original decision was made
3. **Document the change** if deviating
4. **Consider council** if uncertainty remains
5. **Update ADRs** if architectural decision changes

## Best Practices

1. **Reference early** - Import relevant decisions before planning
2. **Validate relevance** - Review each decision's applicability
3. **Track everything** - All imports go in frontmatter
4. **Summarize in session log** - Note what was imported and why
5. **Don't over-import** - Less is more for context management
6. **Update if superseding** - Mark old decisions as superseded when creating new ones

## Example Session Log Entry

```markdown
### 2025-01-23 14:30 - orchestrator
Referenced decisions from auth-jwt session (PLT-123):
- Token rotation strategy (15-min window)
- Refresh token storage (HttpOnly cookies)
Relevant to current auth-v2 work; maintaining consistency.
```
