---
description: >-
  Captures ideas into structured nuggets for later evaluation. Use when the user
  says "I have an idea" or "note this down." Also activate when a specific
  actionable concept crystallizes during conversation. For reviewing and
  processing the intake queue (sparks + raw ideas), use triage instead. Not for
  deep exploration (use brainstorm) or shaping (use shape).
subtask: false
version: 2.0.0-alpha.2
---

# Idea

Capture ideas quickly with minimal friction.

**Input:** $ARGUMENTS

---

## Contents
- Critical Rules
- Verification
- Quick Reference
- Purpose
- Process
- Idea Lifecycle
- Guardrails
- Related Skills

## Critical Rules

- Speed over completeness -- capture quickly, shape later
- 2-3 questions maximum -- don't turn capture into an interview
- Infer metadata automatically -- don't ask for tags, title, or links
- One idea per captured row/artifact -- keep them atomic
- No shaping here -- that's what `/shape` is for
- Capture through `loaf idea capture --title ...` when SQLite state is
  initialized; log notable context with `loaf journal log`

## Verification

- Idea appears in `loaf idea list` / `loaf idea show`
- Status is open/raw according to the active backend
- If promoted from a spark, `loaf spark promote` records the relationship

## Quick Reference

| Status | Meaning |
|--------|---------|
| `raw` | Just captured, unprocessed |
| `shaping` | Being developed via /shape or /brainstorm |
| `shaped` | Converted to SPEC, idea file archived |
| `archived` | Decided not to pursue, kept for reference |

---

## Purpose

Ideas are raw nuggets -- unprocessed, unshaped, but worth remembering. The goal is **speed of capture**, not thoroughness. Shape later via `/shape`.

---

## Process

### Step 1: Parse Input

If `$ARGUMENTS` contains the idea, capture directly.

If `$ARGUMENTS` is empty, ask **at most 2-3 questions**: core idea, problem/opportunity, immediate constraints.

### Step 2: Capture Idea

Use the CLI capture path:

```bash
loaf idea capture --title "..."
```

In SQLite-backed mode, the row is stored in SQLite and no `.agents/ideas/`
markdown file is created. The [idea template](../skills/idea/templates/idea.md) is retained
only for markdown-only compatibility and historical restore review.

### Step 3: Create and Announce

1. Generate timestamp: `date -u +"%Y-%m-%dT%H:%M:%SZ"`
2. Run `loaf idea capture --title "..."` with the inferred title
3. Announce the captured idea alias with next steps

---

## Idea Lifecycle

```
raw -> shaping -> shaped (becomes SPEC) -> archived
```

| Status | Meaning |
|--------|---------|
| `raw` | Just captured, unprocessed |
| `shaping` | Being developed via /shape or /brainstorm |
| `shaped` | Converted to SPEC, idea file archived |
| `archived` | Decided not to pursue, kept for reference |

---

## Guardrails

1. **Speed over completeness** -- capture quickly, shape later
2. **2-3 questions max** -- don't turn this into an interview
3. **Infer, don't ask** -- metadata should be automatic
4. **One idea per captured row/artifact** -- keep them atomic
5. **No shaping here** -- that's what `/shape` is for

---

## Related Skills

- **triage** -- Review and process the intake queue (sparks + raw ideas)
- **shape** -- Develop an idea into a SPEC
- **brainstorm** -- Deep thinking on an idea or problem space
- **research** -- Investigate before capturing
