# Session Resume Patterns

Patterns for resuming work across Claude Code sessions using CLI flags and session files.

## CLI Resume Flags

Claude Code provides built-in session continuation:

| Flag | Purpose |
|------|---------|
| `--continue` | Continue the most recent conversation |
| `--resume <id>` | Resume a specific conversation by ID |
| `/resume` | In-session command to list and resume |

## Resume Methods

### Method 1: CLI Continue

```bash
# Continue most recent conversation
claude --continue

# Resume specific conversation
claude --resume abc123
```

**Best for:** Picking up exactly where you left off with full context.

### Method 2: Session File Resume

```bash
# Start new conversation, resume from session file
claude
> /resume-session 20250123-143000-feature-auth
```

**Best for:** Fresh context but task continuity from session file.

### Method 3: Hybrid Resume

```bash
# Continue conversation AND sync with session file
claude --continue
> Check session file for any updates: .agents/sessions/20250123-143000-feature-auth.md
```

**Best for:** Recovering from context pollution while preserving conversation history.

## When to Use Each Method

| Situation | Recommended Method |
|-----------|-------------------|
| Short break, same task | `--continue` |
| Long break, clean state needed | Session file resume |
| Context polluted but need history | Hybrid resume |
| Different machine | Session file resume |
| Handoff to another person | Session file resume |

## Session File as Resume Checkpoint

### Writing Resume-Ready State

Before ending a session, ensure session file contains:

```markdown
## Current State
**Last Action:** Completed API endpoint implementation
**Pending:** Tests need to be written

## Resumption Notes
- Branch: feature/auth-endpoints
- Key files modified: src/auth/routes.py, src/auth/models.py
- Tests to write: test_login, test_logout, test_refresh
- Blocker: None

## Next Steps
1. Spawn QA agent for test implementation
2. Run full test suite
3. Update API documentation
```

### Reading Resume State

When resuming:

1. Read session file for context
2. Check `## Current State` for where you left off
3. Check `## Resumption Notes` for key details
4. Check `## Next Steps` for immediate actions
5. Verify branch and file state match expectations

## Checkpoint Pattern

For long-running tasks, create explicit checkpoints:

```markdown
## Checkpoints

### Checkpoint 1: Schema Complete
**Timestamp:** 2025-01-23T14:30:00Z
**State:** Database schema implemented and migrated
**Can resume from:** This checkpoint if later work fails

### Checkpoint 2: API Complete
**Timestamp:** 2025-01-23T15:45:00Z
**State:** All endpoints implemented, passing basic tests
**Can resume from:** This checkpoint for test expansion
```

### Rewind to Checkpoint

If work goes wrong:

1. Identify safe checkpoint
2. `git checkout <commit>` to restore code state
3. Update session file to checkpoint state
4. `/clear` to reset context
5. Resume from checkpoint

## Naming Sessions Descriptively

Good session names aid resume:

```
# Good - descriptive, searchable
20250123-143000-auth-jwt-implementation.md
20250123-160000-api-rate-limiting.md

# Bad - generic, hard to find
20250123-143000-feature.md
20250123-160000-fix.md
```

### Rename Pattern

```bash
# If session name becomes unclear, rename for clarity
mv .agents/sessions/20250123-143000-feature.md \
   .agents/sessions/20250123-143000-user-auth-oauth.md
```

Update any references in Linear or other sessions.

## Context Recovery Strategies

### Strategy 1: Full Replay

For complex state, replay key decisions:

```markdown
## Decision Replay

When resuming, recall these decisions:
1. Chose JWT over sessions (see ADR-007)
2. Using Redis for token storage
3. 15-minute access token expiry
```

### Strategy 2: Minimal Context

For simple continuation:

```markdown
## Minimal Resume Context

Branch: feature/auth
Last commit: "feat: add login endpoint"
Next: Write tests for login endpoint
```

### Strategy 3: Reference Chain

For multi-session work:

```markdown
## Session Chain

Previous sessions:
- 20250122-100000-auth-design.md (planning)
- 20250122-143000-auth-schema.md (database)
- This session: API implementation
```

## Handling Stale Sessions

### Signs of Stale Session

- Code has changed since session was active
- Other sessions modified same files
- Branch was rebased or merged

### Stale Session Recovery

```
1. Read session file for intended state
2. Compare with actual code state (git diff)
3. Identify conflicts or drift
4. Update session file to reflect reality
5. Plan reconciliation if needed
```

## Multi-Session Coordination

When multiple sessions touch same codebase:

```markdown
## Related Sessions

| Session | Status | Overlaps |
|---------|--------|----------|
| auth-backend | completed | src/auth/ |
| auth-frontend | in_progress | src/components/auth/ |
| This session | in_progress | src/auth/models.py (shared) |

## Coordination Notes
- Wait for auth-backend completion before modifying models
- Sync with auth-frontend on API contract changes
```

## Best Practices

1. **Update session before stopping** - capture state while fresh
2. **Use descriptive names** - aid future resume
3. **Create checkpoints** - safe rollback points
4. **Note dependencies** - what must complete first
5. **Clear stale context** - don't resume into pollution
6. **Verify state on resume** - code may have changed
7. **Document decision context** - not just decisions
