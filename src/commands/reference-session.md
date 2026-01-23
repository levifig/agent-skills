---
description: Import decisions from past sessions without full context duplication
---

# Reference Session

Import context from past sessions into current work. This command helps maintain continuity across sessions without duplicating full context.

**Input:** $ARGUMENTS

---

## Step 1: Parse Arguments

Parse `$ARGUMENTS` for:
- **Search term**: Session name fragment, Linear issue ID, or topic keyword
- **Content type** (optional): `decisions`, `context`, or `all` (default: `decisions`)

```
/reference-session auth              # Search for "auth" sessions, import decisions
/reference-session auth decisions    # Explicit: import decisions only
/reference-session auth context      # Import full context summary
/reference-session PLT-123           # Search by Linear issue
```

If no arguments provided, list recent decision memories:

```
/reference-session                   # List available session decisions
```

---

## Step 2: Search for Matching Sessions

### Option A: List Serena Memories

Use Serena MCP to list available decision memories:

```
mcp__serena__list_memories()
```

Filter for memories matching pattern: `session-*-decisions.md`

### Option B: Search Session Archives

If no matching memories found, search archived sessions:

```
.agents/sessions/archive/*<search-term>*.md
```

---

## Step 3: Display Matches

Present matching sessions/memories:

```markdown
## Matching Session Decisions

| Session | Archived | Linear | Decisions |
|---------|----------|--------|-----------|
| 20250115-140000-auth-jwt | 2025-01-15 | PLT-123 | 3 decisions |
| 20250110-090000-auth-oauth | 2025-01-10 | PLT-100 | 2 decisions |

Select a session number to import, or 'all' for multiple:
```

If single match, proceed directly to Step 4.

---

## Step 4: Read Decision Memory

For selected session(s), read the decision memory:

```
mcp__serena__read_memory(name="session-<slug>-decisions.md")
```

If memory doesn't exist but session file does, run extraction:

```bash
python3 src/skills/orchestration/scripts/extract-decisions.py \
  ".agents/sessions/archive/<session-file>.md"
```

---

## Step 5: Import into Current Session

If an active session exists (`.agents/sessions/` has `in_progress` files):

### Update Session Frontmatter

Add to the active session's frontmatter:

```yaml
session:
  # ... existing fields ...
  referenced_sessions:
    - session: "20250115-140000-auth-jwt.md"
      imported_at: "2025-01-23T14:30:00Z"
      content_type: decisions
      decisions_imported:
        - "JWT token rotation strategy"
        - "Refresh token storage approach"
```

### Add Reference Section

If importing decisions, add to session body:

```markdown
## Referenced Decisions

### From: 20250115-140000-auth-jwt.md

#### Decision: JWT Token Rotation Strategy
**Decision**: Rotate tokens every 15 minutes with sliding window
**Rationale**: Balance between security and user experience
**Context**: Authentication service design

#### Decision: Refresh Token Storage
**Decision**: Store in HttpOnly cookie, not localStorage
**Rationale**: Prevents XSS token theft
**Context**: Frontend security consideration

---
```

---

## Step 6: Report Import

Confirm what was imported:

```markdown
## Session Reference Complete

**Imported from**: 20250115-140000-auth-jwt.md
**Content type**: decisions
**Decisions imported**: 3

### Summary

1. **JWT Token Rotation Strategy** - Rotate every 15 minutes
2. **Refresh Token Storage** - HttpOnly cookies
3. **Token Validation Flow** - Validate signature before claims

These decisions are now tracked in the current session's frontmatter
under `referenced_sessions`.

**Current session updated**: .agents/sessions/20250123-100000-auth-v2.md
```

---

## No Active Session

If no active session exists, display the decisions without updating any file:

```markdown
## Decisions from: 20250115-140000-auth-jwt.md

[Display decision content]

**Note**: No active session to update. Start a session first with
`/start-session` to track this reference.
```

---

## Content Types

### `decisions` (default)

Imports only the `## Decisions` section formatted as memory:
- Decision name
- What was decided
- Rationale
- Council outcome (if applicable)

### `context`

Imports session context summary:
- Problem statement
- Technical approach
- Key files involved
- Final outcome

### `all`

Imports both decisions and context.

---

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Import full session files | Import only decisions/summaries |
| Import without tracking | Always update `referenced_sessions` |
| Import from unrelated sessions | Search by topic or Linear issue |
| Import during exploration | Import when decisions are relevant |

---

## Error Handling

### No matches found

```
No sessions found matching "auth-v3"

Try:
- Different search terms
- Check .agents/sessions/archive/ directly
- List all: /reference-session
```

### Memory not found

```
Decision memory not found for session 20250115-140000-auth-jwt.md

Would you like to extract decisions now? This will:
1. Read the archived session file
2. Extract the Decisions section
3. Create a Serena memory for future use

[y/n]
```
