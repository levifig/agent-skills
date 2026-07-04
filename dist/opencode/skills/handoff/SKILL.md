---
name: handoff
description: >-
  Creates explicit context-transfer packets in .agents/handoffs/ for another
  agent, branch, task, or future conversation. Use when the user asks "handoff
  this," "prepare the next session," "write transfer context," or when work is
  being parked for later. Not for routine journal continuity (use orchestration)
  or an end-of-conversation checkpoint (use wrap). Produces a disposable handoff
  artifact that housekeeping deletes after confirmed deprecation.
subtask: false
version: 2.0.0-alpha.2
---

# Handoff

Create a concise transfer packet for a fresh agent or future conversation.

**Input:** $ARGUMENTS

---

## Contents
- Critical Rules
- Verification
- Quick Reference
- Process
- Lifecycle
- Related Skills

## Critical Rules

- Log invocation as the first action when possible: `loaf journal log "skill(handoff): <focus>"`
- Save handoffs to `.agents/handoffs/`, never `.agents/reports/` or an OS temp directory
- Treat `$ARGUMENTS` as the intended next-conversation focus
- Reference existing specs, tasks, ADRs, reports, commits, branches, and journal entries instead of duplicating them
- Include suggested skills for the next agent/session
- Redact secrets, PII, tokens, credentials, customer data, and private external identifiers
- Do not make handoffs canonical state; they are transfer packets
- Do not mark a handoff `deprecated` until the user or current workflow confirms it is obsolete

## Verification

- Handoff file exists at `.agents/handoffs/YYYYMMDD-HHMMSS-{slug}.md`
- Frontmatter includes `title`, `created`, `status`, `source`, `branch`, and `tags`
- `status` is `final` unless the user explicitly requested a draft
- The body includes current state, suggested skills, existing artifacts, decisions, next actions, open questions, and deprecation criteria
- Sensitive data has been redacted
- The project journal contains `handoff(slug)` if journaling is available

## Quick Reference

| Field | Value |
|-------|-------|
| Location | `.agents/handoffs/` |
| Naming | `YYYYMMDD-HHMMSS-{slug}.md` |
| Template | [templates/handoff.md](templates/handoff.md) |
| Active statuses | `draft`, `final` |
| Disposable status | `deprecated` |
| Cleanup owner | `/housekeeping` |

## Process

### Step 1: Determine Focus

Use `$ARGUMENTS` as the focus when present. If absent, infer the focus from
the current session, branch, task, or most recent user request. Ask at most one
clarifying question if the handoff target is ambiguous.

### Step 2: Gather Existing Context

Prefer live project context over conversation memory:

1. Current branch and git status
2. Recent journal entries (`loaf journal recent`, `loaf journal context`)
3. Relevant specs, tasks, ADRs, reports, and issues
4. Recent commits or diffs when they explain current state

Do not paste large source excerpts. Link the existing artifact and summarize
only what the next agent needs to act.

### Step 3: Write the Handoff

Create the file from [templates/handoff.md](templates/handoff.md).

Use `date -u +"%Y%m%d-%H%M%S"` for the filename timestamp and
`date -u +"%Y-%m-%dT%H:%M:%SZ"` for frontmatter timestamps.

Required body sections:

- Purpose
- Current State
- Suggested Skills
- Existing Artifacts
- Decisions
- Next Actions
- Open Questions and Risks
- Deprecation Criteria

### Step 4: Log and Announce

If `loaf journal log` works, log:

```bash
loaf journal log "handoff(slug): created .agents/handoffs/<filename>"
```

Then report the handoff path and the intended next action.

## Lifecycle

Handoffs are first-class but disposable:

1. `draft` — incomplete or awaiting review
2. `final` — active transfer packet
3. `deprecated` — confirmed obsolete; `/housekeeping` may delete after confirmation

Set `deprecated_at` and `deprecated_by` only when moving to `deprecated`.

## Related Skills

- **orchestration** — Maintains journal continuity and cross-agent coordination
- **wrap** — Writes an optional end-of-conversation checkpoint to the journal
- **housekeeping** — Deletes deprecated handoffs after confirmation
