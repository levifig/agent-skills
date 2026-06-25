# Session Resume Patterns

Patterns for resuming work with CLI conversation history plus SQLite-backed
Loaf session state.

## Contents

- CLI Resume Flags
- Resume Methods
- When to Use Each Method
- SQLite Session as Resume Checkpoint
- Checkpoint Pattern
- Context Recovery Strategies
- Handling Stale Sessions
- Multi-Session Coordination
- Best Practices

## CLI Resume Flags

Some harnesses provide built-in conversation continuation:

| Flag | Purpose |
|------|---------|
| `--continue` | Continue the most recent conversation |
| `--resume <id>` | Resume a specific conversation by ID |
| `/resume` | In-session command to list and resume |

Use harness resume for chat history. Use `loaf session` for operational state.

## Resume Methods

### Method 1: Conversation Continue

Resume the exact conversation when you need full chat history.

### Method 2: SQLite Session Resume

Start fresh and recover operational state:

```bash
loaf session start
loaf session list --all --json
loaf session show <session-ref> --json
```

Best for clean context with task continuity.

### Method 3: Hybrid Resume

Continue the conversation, then compare it against `loaf session show` output.
Best when chat history matters but the journal may contain newer facts.

## When to Use Each Method

| Situation | Recommended Method |
|-----------|-------------------|
| Short break, same task | Conversation continue |
| Long break, clean state needed | SQLite session resume |
| Context polluted but need history | Hybrid resume |
| Different machine | SQLite session resume |
| Handoff to another person | SQLite session + handoff/report |

## SQLite Session as Resume Checkpoint

Before ending or compacting:

1. Log last action, blockers, and next steps with `loaf session log`.
2. Reference tasks, specs, reports, commits, and branch names by stable ID/path.
3. Run `loaf session end --wrap` when work is complete.

When resuming:

1. Run `loaf session start` on the branch/worktree.
2. Inspect `loaf session show <session-ref> --json`.
3. Verify branch and file state match expectations.
4. Continue from the next logged action.

## Checkpoint Pattern

For long-running tasks, create explicit journal checkpoints:

```bash
loaf session log "decision(checkpoint): schema complete at <commit>"
loaf session log "decision(checkpoint): API complete; next write tests"
```

If work goes wrong, use Git to restore code state and log the reconciliation:

```bash
loaf session log "decision(recovery): rewound to <commit>; replaying tests"
```

## Context Recovery Strategies

### Full Replay

Use `loaf session show` plus ADR/spec/report links for complex state.

### Minimal Context

Use the last few journal entries when the next action is obvious.

### Reference Chain

For multi-session work, log relationships:

```bash
loaf session log "discover(context): previous design captured in SPEC-123 and ADR-007"
```

## Handling Stale Sessions

Signs of stale session state:

- Code changed after the last journal entry
- Branch was rebased or merged
- Another task/spec superseded the work

Recovery:

1. Inspect `loaf session show`.
2. Compare with `git status`, `git log`, and relevant specs/tasks.
3. Log the drift and reconciliation plan.

## Multi-Session Coordination

When multiple sessions touch related areas, use specs/tasks/reports as the shared
coordination layer. Session journals should point to those durable artifacts
rather than becoming the only place a dependency is described.

## Best Practices

1. Log before stopping.
2. Prefer stable IDs over pasted summaries.
3. Use `loaf session show` as the operational resume surface.
4. Keep chat-history resume separate from Loaf session state.
5. Verify code state on resume.
