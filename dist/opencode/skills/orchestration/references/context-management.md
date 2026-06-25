# Context Management

Patterns for keeping long work resumable while using SQLite-backed session
journals as the external memory.

## Contents

- Design for Compaction
- Context Commands
- When to Clear Context
- Compaction Lifecycle
- subtask agent for Context Isolation
- Context Budget Guidelines
- Warning Signs
- Best Practices

## Design for Compaction

Compaction is a normal part of long workflows. Design any workflow expected to
span many exchanges so important state is already outside chat context.

### Compaction-First Principles

1. **The journal is external memory.** Record decisions, discoveries, blockers,
   and next actions with `loaf session log`.
2. **Artifacts carry detail.** Specs, tasks, reports, ADRs, and commits hold rich
   detail; journal entries point to them.
3. **subtask agent absorb exploration.** Use subtask agent for broad investigation and
   return concise findings to the main context.
4. **`wrap` closes the loop.** End meaningful work with `loaf session end --wrap`
   so the session lifecycle matches the journal.

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
  2. Reference specs, tasks, reports, commits, and files by stable ID/path
  3. Let the hook persist the compact marker

PostCompact:
  1. Run or inspect `loaf session start` output for the active branch
  2. Use `loaf session show <session-ref> --json` when more context is needed
  3. Continue from the journal and linked artifacts
```

This makes compaction survivable without relying on hand-maintained markdown
state. Any state not logged or captured in a durable artifact can be lost.

## subtask agent for Context Isolation

Use subtask agent to investigate without filling the main context:

| Situation | Approach |
|-----------|----------|
| Quick file lookup | Direct read/search tool |
| Multi-file exploration | Explorer/research subtask agent |
| Implementation work | Implementer or task-focused agent |
| Long audit | Background agent with report output |

Pass stable references to subtask agent: task IDs, spec IDs, branch names, report
paths, and the active session alias when available.

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
  session journal.

## Warning Signs

| Symptom | Likely Cause | Action |
|---------|--------------|--------|
| Repeating same mistakes | Context pollution | Log current facts, then clear or compact |
| Forgetting recent decisions | Overcrowded context | Inspect `loaf session show` and continue from journal |
| Slow responses | Large context | Delegate exploration |
| Confusion about task | Too many pivots | Re-anchor on task/spec/session IDs |

## Best Practices

1. Log durable facts early with `loaf session log`.
2. Use subtask agent for exploration-heavy work.
3. Clear between unrelated tasks.
4. Compact mid-task when the journal and artifacts are current.
5. Scope tool calls so context stays focused.
