---
name: triage
description: >-
  Surfaces and processes the intake queue: unresolved sparks from the project
  journal and brainstorm documents, plus raw ideas awaiting evaluation. Use when
  the user asks "what sparks do I have?", "review my ideas", "triage", or
  "what's in my backlo...
user-invocable: true
version: 2.0.0-alpha.4
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
- Use SQLite-aware CLI commands for lifecycle changes; do not edit idea/spark
  frontmatter by hand
- Log or link resolutions through `loaf spark resolve`, `loaf spark promote`,
  `loaf idea archive`, and `loaf brainstorm archive` when state is initialized
- One pass through the queue -- don't loop or re-present items

## Verification

- All presented sparks have a recorded disposition (promoted, discarded, or deferred)
- Promoted sparks have corresponding idea rows visible in `loaf idea list`
- Processed sparks no longer appear in default `loaf spark list` / triage output
- Archived ideas/brainstorms no longer appear in default triage lists
- Markdown source annotations, when present, are compatibility notes rather than
  the authoritative state transition

## Quick Reference

| Source | Unprocessed Signal | Resolution |
|--------|-------------------|------------|
| Sparks | Open spark rows from `loaf spark list` | `loaf spark promote` or `loaf spark resolve` |
| Brainstorms | Open brainstorm rows from `loaf brainstorm list` | `loaf brainstorm promote` or `loaf brainstorm archive` |
| Ideas | Open idea rows from `loaf idea list` | Shape, promote, or `loaf idea archive` |

---

## Process

### Step 1: Scan Sources

Scan state-backed queues first, falling back to Markdown compatibility sources
only when SQLite state is not initialized:

**1. Sparks**
- Run `loaf spark list` or `loaf spark list --json`
- Treat open rows as unresolved intake

**2. Brainstorms**
- Run `loaf brainstorm list` or `loaf brainstorm list --json`
- Treat open rows as brainstorm intake

**3. Ideas**
- Run `loaf idea list` or `loaf idea list --json`
- Treat open rows as idea intake

### Step 2: Present the Queue

Show a summary table:

```
Intake Queue:
  Sparks (journal):     3 unresolved
  Sparks (brainstorms): 1 unprocessed
  Raw ideas:            2 awaiting evaluation
  Total:                6 items
```

Then list each item with source, date, and description.

### Step 3: Process Each Item

For each item, present it and ask for disposition:

**Sparks → one of:**
- **Promote** → `loaf spark promote <spark> --to-idea <idea>`
- **Discard** → `loaf spark resolve <spark> --reason <reason>`
- **Defer** → skip, resurface next triage

**Raw ideas → one of:**
- **Shape** → suggest running `/loaf:shape` with this idea
- **Brainstorm** → suggest running `/loaf:brainstorm` to explore further
- **Archive** → `loaf idea archive <idea> --reason <reason>`

### Step 4: Summarize

After processing, show what happened:

```
Triage complete:
  Promoted:  2 sparks → ideas
  Discarded: 1 spark
  Deferred:  1 spark
  Shaped:    1 idea → /loaf:shape
  Archived:  1 idea
```

---

## Resolution Formats

### Sparks

When promoting:
```bash
loaf spark promote SPARK-slug --to-idea idea-slug
```

When discarding:
```bash
loaf spark resolve SPARK-slug --reason "reason"
```

When deferring:
Do nothing; open sparks remain visible in the next triage pass.

### Brainstorms

When promoting:
```bash
loaf brainstorm promote brainstorm-slug --to-idea idea-slug
```

When archiving:
```bash
loaf brainstorm archive brainstorm-slug --reason "reason"
```

### Ideas

When archiving:
```bash
loaf idea archive idea-slug --reason "reason"
```

When shaping, pass the idea to `/loaf:shape`; do not hand-edit status frontmatter to
represent lifecycle state.

---

## Guardrails

1. **User decides every disposition** -- present, don't decide
2. **Batch presentation, individual decisions** -- show the full queue, then process one at a time
3. **Log everything** -- no silent discards or promotions
4. **Deferred items resurface** -- they'll appear again next `/loaf:triage`

---

## Suggests Next

After triage completes, suggest `/loaf:shape` for any ideas promoted to shaping.

## Related Skills

- **idea** -- Capture a new idea (fast, minimal friction)
- **shape** -- Develop an idea into a SPEC
- **brainstorm** -- Deep exploration of a problem space (produces sparks)
- **housekeeping** -- Flags brainstorm drafts with unprocessed sparks before deletion
- **reflect** -- Strategic document updates (separate from triage)
