# Journal Render Template

This is the render template and entry-format reference for the project journal
stored in SQLite. The journal is the only session-related structure: entries are
project-scoped events, each tagged with an opaque harness id that correlates one
conversation's entries. Nobody opens, closes, or transitions anything. Agents do
not create or edit journal markdown as the source of truth — use
`loaf journal log "type(scope): desc"` and read with `loaf journal recent`,
`loaf journal context`, `loaf journal search`, and `loaf journal show`.

```markdown
# Project Journal: name

## Entries

[YYYY-MM-DD HH:MM] skill(implement): implementing TASK-042
[YYYY-MM-DD HH:MM] decision(scope): description of decision
[YYYY-MM-DD HH:MM] discover(scope): something learned
[YYYY-MM-DD HH:MM] block(scope): what is blocked
[YYYY-MM-DD HH:MM] unblock(scope): how it was resolved
[YYYY-MM-DD HH:MM] commit(abc1234): commit message
[YYYY-MM-DD HH:MM] wrap(scope): tried X, abandoned because Y, next is Z
```

## Entry Types

| Type | Use For | Written By |
|------|---------|------------|
| `skill(name)` | User-invocable workflow skill ran | Agent |
| `decision(scope)` | Key decisions with rationale | Agent |
| `discover(scope)` | Something learned | Agent |
| `block(scope)` | Blocker encountered | Agent |
| `unblock(scope)` | Blocker resolved | Agent |
| `spark(scope)` | Ideas to promote | Agent |
| `todo(scope)` | Action items | Agent |
| `finding(scope)` | Findings from analysis | Agent |
| `wrap(scope)` | Optional end-of-conversation synthesis | Agent |
| `commit(SHA)` | Code committed | Auto hook |

## Format Rules

1. **Timestamp format:** `YYYY-MM-DD HH:MM` (no seconds)
2. **Entry format:** `[<timestamp>] <type>(<scope>): <description>`
3. **Wrap is optional:** Write a `wrap(scope)` entry only when the conversation
   holds synthesis worth saving. Nothing is ever started, ended, or stopped;
   there is no `session` entry type.
4. **Blank lines:** Render burst boundaries only; SQLite journal rows are the
   durable ordering source.
5. **No manual markdown edits:** Rendered markdown is a projection. Persist new
   facts with `loaf journal log`.

## CLI Commands

```bash
loaf journal log      # Append typed entry: loaf journal log "decision(scope): desc"
loaf journal recent   # Show the timeline (newest first); --branch, --since-last-wrap
loaf journal context  # Emit the layered continuity digest (latest wrap + branch + tasks)
loaf journal search   # Full-text search across the project journal
loaf journal show     # Read one entry by id
loaf journal export   # Export the journal to markdown or JSONL
```
