# Idea Command

Capture ideas quickly with minimal friction.

**Input:** $ARGUMENTS

---

## Purpose

Ideas are raw nuggets — unprocessed, unshaped, but worth remembering.

The goal is **speed of capture**, not thoroughness. Ideas get shaped later via `/shape`.

---

## Process

### Step 1: Parse Input

If `$ARGUMENTS` contains the idea, capture it directly.

If empty or unclear, ask **at most 2-3 questions**:

1. What's the core idea? (one sentence)
2. What problem does it solve or opportunity does it create?
3. Any immediate constraints or context?

**Keep it brief.** Don't interview extensively — that's for `/shape`.

### Step 2: Generate Idea File

Create file in `.agents/ideas/` with format:

**Filename:** `{YYYYMMDD}-{slug}.md`

```markdown
---
captured: YYYY-MM-DDTHH:MM:SSZ
status: raw
tags: []
---

# [Idea Title]

## Nugget

[One paragraph capturing the core idea]

## Problem/Opportunity

[What this addresses — keep brief]

## Initial Context

[Any constraints, related work, or context mentioned during capture]

---

*Captured via /idea — shape with /shape when ready*
```

### Step 3: Infer Metadata

From the conversation, infer:

- **Title:** Clear, concise summary
- **Tags:** Optional, only if obvious (e.g., "ux", "performance", "api")
- **Related:** Link to existing ideas/specs if mentioned

**Don't ask for confirmation on metadata.** Just infer sensibly.

### Step 4: Create and Announce

1. Generate timestamp:
   ```bash
   date -u +"%Y-%m-%dT%H:%M:%SZ"
   ```

2. Create the file

3. Announce:
   ```
   Captured: .agents/ideas/{filename}.md

   When ready to develop this, use `/shape {filename}` or `/brainstorm {filename}`.
   ```

---

## Idea Lifecycle

```
raw → shaping → shaped (becomes SPEC) → archived
```

| Status | Meaning |
|--------|---------|
| `raw` | Just captured, unprocessed |
| `shaping` | Currently being developed via /shape or /brainstorm |
| `shaped` | Converted to SPEC, idea file archived |
| `archived` | Decided not to pursue, kept for reference |

---

## Directory Structure

```
.agents/
└── ideas/
    ├── 20260124-idea-command.md
    ├── 20260124-shape-workflow.md
    └── archive/
        └── 20260115-rejected-approach.md
```

---

## Guardrails

1. **Speed over completeness** — Capture quickly, shape later
2. **2-3 questions max** — Don't turn this into an interview
3. **Infer, don't ask** — Metadata should be automatic
4. **One idea per file** — Keep them atomic
5. **No shaping here** — That's what `/shape` is for

---

## Examples

### Quick capture with arguments

```
User: /idea Add keyboard shortcuts for common actions

Agent: [Creates file immediately]

Captured: .agents/ideas/20260124-keyboard-shortcuts.md

When ready to develop this, use `/shape 20260124-keyboard-shortcuts` or `/brainstorm 20260124-keyboard-shortcuts`.
```

### Capture with brief interview

```
User: /idea

Agent: What's the core idea?

User: We should have a way to batch process multiple ideas at once

Agent: What problem does this solve?

User: Right now shaping is one-at-a-time, slow when you have many ideas queued

Agent: [Creates file]

Captured: .agents/ideas/20260124-batch-idea-processing.md
```

---

## Related Commands

- `/shape` — Develop an idea into a SPEC
- `/brainstorm` — Deep thinking on an idea or problem space
- `/research` — Investigate before capturing
---
version: 1.13.1
