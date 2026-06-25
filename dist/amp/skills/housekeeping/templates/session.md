# Session Render Template

This is the render template and journal-format reference for sessions stored in
SQLite. Agents do not create or edit session markdown as the source of truth.
Use `loaf session start`, `loaf session log "type(scope): desc"`, and
`loaf session end --wrap`; use `loaf session show <session-ref>` to inspect the
rendered view.

```markdown
# Session: Brief Description

- Branch: feat/feature-name
- Status: active
- Created: YYYY-MM-DDTHH:MM:SSZ
- Updated: YYYY-MM-DDTHH:MM:SSZ

## Journal

[YYYY-MM-DD HH:MM] session(start): === SESSION STARTED ===
[YYYY-MM-DD HH:MM] decision(scope): description of decision
[YYYY-MM-DD HH:MM] discover(scope): something learned
[YYYY-MM-DD HH:MM] block(scope): what is blocked
[YYYY-MM-DD HH:MM] hypothesis(scope): theory being tested
[YYYY-MM-DD HH:MM] unblock(scope): how it was resolved
[YYYY-MM-DD HH:MM] commit(abc1234): commit message
[YYYY-MM-DD HH:MM] session(end): concise wrap summary
[YYYY-MM-DD HH:MM] session(stop): === SESSION STOPPED ===
```

## Entry Types

| Type | Use For | Written By |
|------|---------|------------|
| `session(start)` | Session started | CLI |
| `session(resume)` | Session resumed | CLI |
| `session(end)` | Session ended or wrapped | CLI |
| `session(stop)` | Session stopped | CLI |
| `commit(SHA)` | Code committed | Auto hook |
| `skill(name)` | User-invocable workflow skill ran | Agent |
| `decision(scope)` | Key decisions with rationale | Agent |
| `discover(scope)` | Something learned | Agent |
| `block(scope)` | Blocker encountered | Agent |
| `unblock(scope)` | Blocker resolved | Agent |
| `spark(scope)` | Ideas to promote | Agent |
| `todo(scope)` | Action items | Agent |
| `finding(scope)` | Findings from analysis | Agent |

## Format Rules

1. **Timestamp format:** `YYYY-MM-DD HH:MM` (no seconds)
2. **Entry format:** `[<timestamp>] <type>(<scope>): <description>`
3. **Lifecycle entries:** Use `session(start)`, `session(resume)`,
   `session(end)`, and `session(stop)` for session lifecycle state.
4. **Blank lines:** Render burst boundaries only; SQLite journal rows are the
   durable ordering source.
5. **No manual markdown edits:** Rendered markdown is a projection. Persist new
   facts with `loaf session log`.

## CLI Commands

```bash
loaf session start      # Find/create session, append resume entry, output context
loaf session log        # Append typed entry: loaf session log "decision(scope): desc"
loaf session show       # Read the SQLite-backed rendered view
loaf session end --wrap # Persist wrapped end state for the active session
loaf session archive    # Archive when branch merges
```
