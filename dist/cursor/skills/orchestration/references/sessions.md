# Session Management

Sessions are SQLite-backed coordination records for active work. Markdown is a
rendered view for reading or compatibility export; agents persist new facts with
`loaf session` commands.

## Contents

- Core Model
- When to Use Sessions
- Lifecycle
- Journal Protocol
- Continuity
- Completion
- Hook Integration
- Anti-Patterns

## Core Model

Use the `wrap` skill as the canonical session model:

1. `loaf session start` finds or creates the active session for the current branch
   and prints resumption context.
2. `loaf session log "type(scope): description"` writes durable journal rows.
3. `loaf session show <session-ref> --json` reads the SQLite-backed session view.
4. `loaf session end --wrap` persists the wrapped end state.
5. `loaf session archive` archives closed sessions when the branch/work is done.

The render format is documented in `templates/session.md`; it is not an
instruction to author markdown directly.

## When to Use Sessions

- Multi-step work requiring coordination
- Handoffs between agents
- Implementation or release work that must survive compaction
- Long research, architecture, council, or review efforts

For quick unrelated questions, start a fresh conversation so the active session
stays focused.

## Lifecycle

The interim runtime lifecycle before SPEC-049 is:

| State | Source | Meaning |
|-------|--------|---------|
| `active` | `loaf session start` | Current work may receive journal entries |
| `stopped` | `loaf session end` | Work paused or closed without wrap |
| `done` | `loaf session end --wrap` | Wrapped work is complete |
| `archived` | `loaf session archive` | Closed session preserved outside active list |

Do not introduce additional session vocabulary in guidance. SPEC-049 owns the
durable status vocabulary.

## Journal Protocol

Log compact, factual entries:

```bash
loaf session log "decision(scope): chose X because Y"
loaf session log "discover(scope): learned Z from file/path"
loaf session log "block(scope): waiting on external approval"
loaf session log "unblock(scope): approval received"
loaf session log "spark(scope): possible follow-up idea"
loaf session log "todo(scope): concrete follow-up action"
```

Log durable facts, not thoughts. The journal should let another agent resume
without reading the whole conversation.

## Continuity

Before compaction, branch switches, or agent handoff:

1. Flush unrecorded decisions, discoveries, blockers, and next actions with
   `loaf session log`.
2. Use `loaf session show <session-ref> --json` to confirm the journal has the
   required resumption context.
3. Pass task IDs, spec IDs, report IDs, and commit refs rather than duplicating
   large prose into the journal.

Background and delegated agents should receive the relevant task/spec/report
references plus the active session alias when the caller has one.

## Completion

When work is complete:

1. Ensure decisions and discoveries are captured in durable homes such as ADRs,
   specs, reports, or docs.
2. Run `loaf session end --wrap` so the CLI persists the wrapped state.
3. After merge or closure, use `loaf session archive` if the session should leave
   the active list.

Reports, councils, and handoffs have their own archive or finalization commands.
Do not use session archival as a substitute for those lifecycles.

## Hook Integration

- Session start hooks should surface recent journal entries from SQLite.
- Session-end hooks should nudge for missing durable journal entries before wrap.
- Pre-compaction hooks should ask the agent to flush facts through
  `loaf session log`.
- Post-compaction recovery should use `loaf session start` and
  `loaf session show`.

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Hand-author session markdown as source | Use `loaf session start/log/show/end` |
| Store decisions only in chat context | Log them and promote durable decisions to ADR/spec/report/docs |
| Invent status values | Cite the runtime lifecycle until SPEC-049 lands |
| Archive before extracting outcomes | Promote outcomes first, then wrap/archive |
| Batch all journal entries at the end | Log significant facts as they happen |
