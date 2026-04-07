# Context Management

Patterns for managing context efficiently across sessions and agent spawns.

## Contents

- Design for Compaction
- Overview
- Context Commands
- When to Clear Context
- The 2-Correction Rule
- Context Compaction
- Session Files as Context Anchors
- Subagents for Context Isolation
- Context Budget Guidelines
- Preventing Context Bloat
- Session Context Patterns
- Warning Signs
- Best Practices

## Design for Compaction

Compaction is a normal part of long workflows, not an emergency measure. Any workflow expected to exceed 15-20 exchanges should be designed with compaction in mind from the start.

### Compaction-First Principles

1. **The journal IS external memory** — Session journal entries (decisions, discoveries, progress) survive compaction. Compaction can be lossy because all important state is already on disk.
2. **`## Current State` is the resumption context** — Keep this section handoff-ready at all times. The PreCompact hook requires writing a state summary here before compaction.
3. **Decisions go to disk, not context** — Record key decisions in the session journal immediately via `loaf session log`. Context may be compressed; journal entries persist.
4. **Subagents absorb exploration** — Investigation and exploration should happen in subagents. Only the summary returns to the main context.

### Compaction Lifecycle

```
PreCompact:
  1. Flush all unrecorded journal entries (decisions, discoveries, progress)
  2. Write condensed state summary to session file's ## Current State
  3. compact.sh writes compact(session) marker to journal

PostCompact:
  1. PostCompact nudge fires — tells model to read session file
  2. Model reads ## Current State for resumption context
  3. Model reads ## Journal for decision trail
  4. Work resumes without "where were we?"
```

This makes compaction invisible to workflow continuity. The session file's `## Current State` section is the resumption prompt — written by the model before compaction, read by the model after.

## Overview

Context is finite. Long conversations accumulate irrelevant information that degrades performance. Active context management keeps conversations focused and effective.

## Context Commands

| Command | Purpose |
|---------|---------|
| `/clear` | Start fresh conversation, reset context |
| `/compact` | Summarize and compress current context |
| `/cost` | Show token usage and cost estimates |

## When to Clear Context

### Clear Between Tasks

Use `/clear` when:

- Starting a completely new task
- Previous task is fully complete
- Context has become cluttered with failed attempts
- Switching between unrelated codebases

### Don't Clear When

- Mid-task and need previous context
- Debugging requires understanding of prior attempts
- Session file provides necessary handoff information

## The 2-Correction Rule

**If Claude makes the same mistake twice after correction, the context may be polluted.**

Signs of context pollution:
- Repeating errors you've already corrected
- Ignoring instructions you've given
- Reverting to patterns you've explicitly rejected
- Confusion about current task state

**Action:** Consider `/clear` and restart with fresh context, referencing session file for state.

## Context Compaction

Use `/compact` when:
- Conversation is long but task continues
- Need to preserve key decisions while reducing noise
- Approaching context limits

**Journal entries are compaction insurance.** Every `loaf session log` call writes state to disk that survives compaction. Don't defer journaling — entries not flushed before compaction are lost.

### What Compaction Preserves (via session file)

- Journal entries: decisions, discoveries, progress, commits
- `## Current State` summary (written by PreCompact)
- All externalized artifacts: specs, tasks, ideas, code

### What Compaction Discards

- Intermediate exploration steps
- Failed attempts and debugging noise
- Verbose tool output
- Redundant explanations

## Session Files as Context Anchors

Session files provide persistent context that survives `/clear`:

```markdown
## Current State
[Always handoff-ready summary]

## Key Decisions
- Chose X over Y because Z

## Next Steps
- Immediate action items
```

### Pattern: Clear + Resume

```
1. Update session file with current state
2. /clear to reset context
3. /resume to reload from session file
4. Continue with clean context
```

## Subagents for Context Isolation

Use subagents (Task tool) to investigate without polluting main context:

```
# Instead of exploring in main conversation:
Let me look at how auth works...
[reads 10 files, fills context]

# Use subagent for investigation:
Task(Explore, "How does authentication work in this codebase?")
[returns focused summary, main context stays clean]
```

### When to Use Subagents

| Situation | Approach |
|-----------|----------|
| Quick file lookup | Direct Read tool |
| Multi-file exploration | Task(Explore) |
| Implementation work | Task(implementer) |
| Complex investigation | Task(Plan) then implement |

## Context Budget Guidelines

### Short Conversations (< 10 exchanges)

- No management needed
- Context stays fresh naturally

### Medium Conversations (10-30 exchanges)

- Consider `/compact` at midpoint
- Delegate investigations to subagents
- Keep session file updated

### Long Conversations (30+ exchanges)

- `/compact` every 15-20 exchanges
- Heavy use of subagents for exploration
- Session file as primary state holder
- Consider `/clear` + restart if degraded

## Preventing Context Bloat

### Minimize Tool Output

```
# Instead of reading entire large files:
Read(file, limit=50)  # Read first 50 lines

# Instead of globbing everything:
Glob("src/**/*.py", path="src/auth/")  # Scoped search
```

### Focused Queries

```
# Instead of "show me all the code":
"Find where user authentication is validated"

# Instead of exploring blindly:
"What files handle the /api/users endpoint?"
```

### Progressive Disclosure

1. Get overview first (symbols, structure)
2. Drill into specific areas
3. Read full content only when needed

## Session Context Patterns

### Starting a Session

```
1. Create session file (minimal context recorded)
2. Break down task (decisions recorded)
3. Spawn first agent (isolated context)
4. Update session (state persisted)
```

### Mid-Session Context Check

Every 10-15 exchanges, assess:
- [ ] Is context still focused on current task?
- [ ] Are previous decisions still relevant?
- [ ] Has debugging noise accumulated?
- [ ] Would `/compact` help?

### Session Handoff

When pausing or handing off:
1. Update session file with complete state
2. Include "resumption notes" for context
3. Reference key files and decisions
4. Clear main context if long

## Warning Signs

| Symptom | Likely Cause | Action |
|---------|--------------|--------|
| Repeating same mistakes | Context pollution | `/clear` + restart |
| Forgetting recent decisions | Overcrowded context | `/compact` |
| Slow responses | Large context | Use subagents |
| Confusion about task | Too many pivots | Update session, `/clear` |

## Best Practices

1. **Update session files continuously** - they survive context resets
2. **Use subagents for exploration** - keep main context clean
3. **Clear between unrelated tasks** - fresh start beats polluted context
4. **Compact mid-task if needed** - preserve decisions, discard noise
5. **Monitor for pollution** - 2-correction rule catches degradation early
6. **Scope tool calls** - don't read entire codebases into context
