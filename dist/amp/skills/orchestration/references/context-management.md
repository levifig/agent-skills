# Context Management

Patterns for keeping long work resumable while using the project journal as the
external memory.

## Contents

- Design for Compaction
- Context Commands
- When to Clear Context
- Compaction Lifecycle
- Amp check/agent mode or new thread for Context Isolation
- Context Budget Guidelines
- Warning Signs
- Best Practices

## Design for Compaction

Compaction is a normal part of long workflows. Design any workflow expected to
span many exchanges so important state is already outside chat context.

### Compaction-First Principles

1. **The journal is external memory.** Record decisions, discoveries, blockers,
   and next actions with `loaf journal log`.
2. **Artifacts carry detail.** Specs, tasks, reports, ADRs, and commits hold rich
   detail; journal entries point to them.
3. **Amp check/agent mode or new thread absorb exploration.** Use Amp check/agent mode or new thread for broad investigation and
   return concise findings to the main context.
4. **`wrap` captures synthesis.** When meaningful work holds intentions or
   abandoned paths worth saving, write an optional `wrap` journal entry.

## Context Commands

| Command | Purpose |
|---------|---------|
| `/clear` | Start fresh conversation, reset context |
| `/compact` | Summarize and compress current context |
| `/cost` | Show token usage and cost estimates |

## When to Clear Context

Use `/clear` when:

- Starting a completely new task
- Previous task is complete
- Debugging noise is crowding out the current objective
- Switching between unrelated codebases

Avoid clearing mid-task until you have logged enough state for recovery.

## Compaction Lifecycle

```
PreCompact:
  1. Flush unrecorded decisions, discoveries, blockers, and next actions
     with `loaf journal log`
  2. Reference specs, tasks, reports, commits, and files by stable ID/path
  3. Let the hook nudge the flush

PostCompact:
  1. Read the continuity digest the resumption hook emits (or run
     `loaf journal context`)
  2. Widen with `loaf journal recent` / `loaf journal search` when more is needed
  3. Continue from the journal and linked artifacts
```

This makes compaction survivable without relying on hand-maintained markdown
state. Any state not logged or captured in a durable artifact can be lost.

## Amp check/agent mode or new thread for Context Isolation

Use Amp check/agent mode or new thread to investigate without filling the main context:

| Situation | Approach |
|-----------|----------|
| Quick file lookup | Direct read/search tool |
| Multi-file exploration | Explorer/research Amp check/agent mode or new thread |
| Implementation work | Implementer or task-focused agent |
| Long audit | Background agent with report output |

Pass stable references to Amp check/agent mode or new thread: task IDs, spec IDs, branch names, and
report paths. The harness id is attached to journal entries automatically —
there is no session alias to pass.

## Context Budget Guidelines

### Short Conversations

No special management is usually needed.

### Medium Conversations

- Log decisions as they happen.
- Delegate broad searches.
- Keep tool output scoped.

### Long Conversations

- Expect compaction.
- Keep the journal current.
- Use reports or handoffs for rich summaries rather than stuffing prose into the
  journal.

## Warning Signs

| Symptom | Likely Cause | Action |
|---------|--------------|--------|
| Repeating same mistakes | Context pollution | Log current facts, then clear or compact |
| Forgetting recent decisions | Overcrowded context | Read `loaf journal recent` and continue from the journal |
| Slow responses | Large context | Delegate exploration |
| Confusion about task | Too many pivots | Re-anchor on task/spec IDs |

## Best Practices

1. Log durable facts early with `loaf journal log`.
2. Use Amp check/agent mode or new thread for exploration-heavy work.
3. Clear between unrelated tasks.
4. Compact mid-task when the journal and artifacts are current.
5. Scope tool calls so context stays focused.
