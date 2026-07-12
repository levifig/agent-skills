# Context Management

## Contents

- Design for Compaction
- Continuity Digest (contract v2)
- Context Commands
- When to Clear Context
- Compaction Lifecycle
- Subagents for Context Isolation
- Context Budget Guidelines
- Warning Signs
- Best Practices

Patterns for keeping long work resumable while using the project journal as external memory.

## Design for Compaction

Compaction is normal in long workflows. Design work that spans many exchanges so important state is already outside chat context.

1. **The journal is external memory.** Record decisions, discoveries, blockers, and next actions with `loaf journal log`.
2. **Artifacts carry detail.** Changes, transitional tasks, reports, ADRs, and commits hold rich detail; journal entries point to them.
3. **Subagents absorb exploration.** Use subagents for broad investigation and return concise findings to the main context.
4. **`wrap` captures synthesis.** When meaningful work holds intentions or abandoned paths worth saving, write an optional `wrap` journal entry.

## Continuity Digest (contract v2)

`loaf journal context` is the contract-v2 active-truth digest and supersedes the retired three-part summary. Read its named layers and diagnostics; an absent item and an unavailable source are different states.

| Layer | Truth and precedence |
|-------|----------------------|
| `project-synthesis` | The latest `wrap(project)` synthesis. This is the only wrap that represents project-wide synthesis. |
| `scoped-checkpoint` | The latest non-project wrap, only when `project-synthesis` has no item. It is labeled as a fallback, not promoted to project synthesis. |
| `active-lineage` | Journal evidence associated with the active Change lineage. |
| `unresolved-blockers` | Blocks without a later exact-scope unblock. |
| `deferred-intent` | Open deferred-intent decision and spark pairs. |
| `active-changes` | Git-derived active Change evidence and worktree state. |
| `branch-recency` | Recent branch entries after entries already surfaced as active truth are removed. |
| `transitional-tasks` | Open task-board records retained during the Markdown-to-native transition. |

Each returned layer includes `source_available`, `available_count`, `shown_count`, `truncated`, and `expand_command`; paginated layers also include a cursor. Treat `source_available: false` as an explicit unavailable source, never as “nothing is active.” If Change discovery is unavailable, `active-changes` and `active-lineage` are unavailable and the digest carries a diagnostic.

Use `--branch` to select `branch-recency` scope and bind state cursors. It does not override active Change provenance or reasons, which always use the actual Git branch. Use `loaf journal context --layer <name>` to inspect one layer. `--limit` accepts 1 through 100 and requires `--layer`; `--cursor` also requires `--layer` and cannot be used with the intrinsic one-item `project-synthesis` or `scoped-checkpoint` layers. Follow the returned `expand_command` exactly: a cursor is bound to its layer, project, branch, snapshot, and limit. Use `--json` for automation; the human view preserves availability, counts, truncation, diagnostics, and expansion commands.

## Context Commands

| Command | Purpose |
|---------|---------|
| `/clear` | Start fresh conversation, reset context |
| `/compact` | Summarize and compress current context |
| `/cost` | Show token usage and cost estimates |
| `loaf journal context` | Read active continuity truth and obtain exact layer expansions |

## When to Clear Context

Use `/clear` when starting a completely new task, after the previous task is complete, when debugging noise crowds out the current objective, or when switching between unrelated codebases. Avoid clearing mid-task until enough state is logged for recovery.

## Compaction Lifecycle

PreCompact:

1. Flush unrecorded decisions, discoveries, blockers, and next actions with `loaf journal log`.
2. Reference Changes, transitional tasks, reports, commits, and files by stable ID or path.
3. Let the hook nudge the flush.

PostCompact:

1. Read the continuity digest emitted by the resumption hook, or run `loaf journal context`.
2. Expand the named layer that needs more detail, or use `loaf journal recent` and `loaf journal search` for a different query.
3. Continue from the journal and linked artifacts.

This makes compaction survivable without relying on hand-maintained Markdown state. State not logged or captured in a durable artifact can be lost.

## Subagents for Context Isolation

Use subagents to investigate without filling the main context.

| Situation | Approach |
|-----------|----------|
| Quick file lookup | Direct read or search tool |
| Multi-file exploration | Explorer or research subagent |
| Implementation work | Implementer or task-focused agent |
| Long audit | Background agent with report output |

Pass stable references to subagents: Change IDs, task IDs, branch names, and report paths. The harness ID is attached to journal entries automatically; there is no session alias to pass.

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
- Use reports or handoffs for rich summaries rather than stuffing prose into the journal.

## Warning Signs

| Symptom | Likely Cause | Action |
|---------|--------------|--------|
| Repeating same mistakes | Context pollution | Log current facts, then clear or compact |
| Forgetting recent decisions | Overcrowded context | Read `loaf journal context` and expand the relevant layer |
| Slow responses | Large context | Delegate exploration |
| Confusion about task | Too many pivots | Re-anchor on Change or task IDs |

## Best Practices

1. Log durable facts early with `loaf journal log`.
2. Use subagents for exploration-heavy work.
3. Clear between unrelated tasks.
4. Compact mid-task when the journal and artifacts are current.
5. Scope tool calls so context stays focused.
