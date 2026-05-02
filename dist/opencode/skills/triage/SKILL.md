---
name: triage
description: >-
  Surfaces and processes the intake queue: unresolved sparks from session
  journals and brainstorm documents, plus raw ideas awaiting evaluation. Use
  when the user asks "what sparks do I have?", "review my ideas", "triage", or
  "what's in my backlog?" Produces promoted ideas, archived discards, and
  resolve(spark) journal entries. Not for capturing new ideas (use idea) or
  shaping (use shape).
user-invocable: true
version: 2.0.0-dev.40
---

# Triage

Review and process the intake queue — sparks and raw ideas.

**Input:** $ARGUMENTS

---

## Contents
- Critical Rules
- Verification
- Quick Reference
- Process
- Resolution Formats
- Guardrails
- Related Skills

## Critical Rules

- Present everything before acting -- user decides each disposition
- Never auto-promote or auto-discard without confirmation
- Log resolutions in the source document (journal or brainstorm)
- One pass through the queue -- don't loop or re-present items

## Verification

- All presented sparks have a recorded disposition (promoted, discarded, or deferred)
- Promoted sparks have corresponding idea files in `.agents/ideas/`
- Session journals have `resolve(spark)` entries for each processed spark
- Brainstorm sparks are marked `*(promoted)*` or `*(discarded)*` in source

## Quick Reference

| Source | Unprocessed Signal | Resolution |
|--------|-------------------|------------|
| Session journal | `spark()` without matching `resolve(spark)` | Append `resolve(spark)` entry |
| Brainstorm doc | Spark not marked `*(promoted)*` or `*(discarded)*` | Mark inline in source |
| Ideas directory | `status: raw` in frontmatter | Shape, brainstorm further, or archive |

---

## Process

### Step 1: Scan Sources

Scan three sources for unprocessed items:

**1. Session journal sparks**
- Search `.agents/sessions/*.md` for `spark()` journal entries
- Exclude sparks that have a matching `resolve(spark)` entry in the same session
- Also scan `.agents/sessions/archive/*.md` for unresolved sparks from past sessions

**2. Brainstorm document sparks**
- Search `.agents/drafts/*brainstorm*.md` for `## Sparks` sections
- List sparks not marked as `*(promoted)*` or `*(discarded)*`

**3. Raw ideas**
- Search `.agents/ideas/*.md` for files with `status: raw` in frontmatter

### Step 2: Present the Queue

Show a summary table:

```
Intake Queue:
  Sparks (sessions):    3 unresolved
  Sparks (brainstorms): 1 unprocessed
  Raw ideas:            2 awaiting evaluation
  Total:                6 items
```

Then list each item with source, date, and description.

### Step 3: Process Each Item

For each item, present it and ask for disposition:

**Sparks → one of:**
- **Promote** → create idea file via the idea capture flow, log resolution
- **Discard** → log resolution with reason
- **Defer** → skip, resurface next triage

**Raw ideas → one of:**
- **Shape** → suggest running `/shape` with this idea
- **Brainstorm** → suggest running `/brainstorm` to explore further
- **Archive** → update status to `archived` with reason

### Step 4: Summarize

After processing, show what happened:

```
Triage complete:
  Promoted:  2 sparks → ideas
  Discarded: 1 spark
  Deferred:  1 spark
  Shaped:    1 idea → /shape
  Archived:  1 idea
```

---

## Resolution Formats

### Session journal sparks

When promoting:
```
- YYYY-MM-DD HH:MM resolve(spark): slug → promoted to .agents/ideas/YYYYMMDD-HHMMSS-slug.md [YYYY-MM-DD HH:MM]
```

When discarding:
```
- YYYY-MM-DD HH:MM resolve(spark): slug → discarded, reason [YYYY-MM-DD HH:MM]
```

When deferring:
```
- YYYY-MM-DD HH:MM resolve(spark): slug → deferred, reason [YYYY-MM-DD HH:MM]
```

### Brainstorm sparks

Mark inline in the source document:
- Promoted: `*(promoted to .agents/ideas/YYYYMMDD-HHMMSS-slug.md)*`
- Discarded: `*(discarded: reason)*`

### Raw ideas

Update frontmatter `status:` field:
- `shaping` — when sent to `/shape`
- `archived` — when decided not to pursue

---

## Guardrails

1. **User decides every disposition** -- present, don't decide
2. **Batch presentation, individual decisions** -- show the full queue, then process one at a time
3. **Log everything** -- no silent discards or promotions
4. **Deferred items resurface** -- they'll appear again next `/triage`

---

## Suggests Next

After triage completes, suggest `/shape` for any ideas promoted to shaping.

## Related Skills

- **idea** -- Capture a new idea (fast, minimal friction)
- **shape** -- Develop an idea into a SPEC
- **brainstorm** -- Deep exploration of a problem space (produces sparks)
- **housekeeping** -- Flags sessions with unprocessed sparks
- **reflect** -- Strategic document updates (separate from triage)
