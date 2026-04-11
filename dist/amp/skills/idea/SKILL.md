---
name: idea
description: >-
  Captures ideas into structured nuggets for later evaluation. Use when the user
  says "I have an idea" or "note this down." Also activate when a specific
  actionable concept crystallizes during conversation. For reviewing and
  processing the intake queue (sparks + raw ideas), use triage instead. Not for
  deep exploration (use brainstorm) or shaping (use shape).
version: 2.0.0-dev.27
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
- One idea per file -- keep them atomic
- No shaping here -- that's what `/shape` is for
- Log capture to session journal: `loaf session log "spark(scope): idea slug captured"`

## Verification

- Idea file created in `.agents/ideas/` with correct `YYYYMMDD-HHMMSS-slug.md` naming
- Frontmatter contains required fields (title, status: raw, created timestamp)
- If promoted from a spark, source document is marked `*(promoted)*`

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

### Step 2: Generate Idea File

Create file in `.agents/ideas/` following [idea template](templates/idea.md).

**Filename:** `{YYYYMMDD}-{HHMMSS}-{slug}.md`

### Step 3: Create and Announce

1. Generate timestamp: `date -u +"%Y-%m-%dT%H:%M:%SZ"`
2. Create the file (infer title, tags, and related links without asking)
3. Announce: `Captured: .agents/ideas/{filename}.md` with next steps

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
4. **One idea per file** -- keep them atomic
5. **No shaping here** -- that's what `/shape` is for

---

## Related Skills

- **triage** -- Review and process the intake queue (sparks + raw ideas)
- **shape** -- Develop an idea into a SPEC
- **brainstorm** -- Deep thinking on an idea or problem space
- **research** -- Investigate before capturing
