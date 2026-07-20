---
name: idea
description: >-
  Captures ideas into structured nuggets for later evaluation. Use when the user
  says "I have an idea" or "note this down." Also activate when a specific
  actionable concept crystallizes during conversation. Ideas and sparks are
  capture primitives ro...
user-invocable: true
argument-hint: '[idea description]'
version: 2.0.0-alpha.12
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
- No shaping here -- that's what `/loaf:shape` is for
- Capture through `loaf idea capture --title ...` when SQLite state is
  initialized; log notable context with `loaf journal log`

## Verification

- The idea appears in `loaf idea list` and `loaf idea show <ref>` with status open
- If promoted from a spark, `loaf spark promote` recorded the relationship
- No shaping, status transition, or promotion happened here — dispositions belong to triage

## Quick Reference

| Operation | Command |
|-----------|---------|
| Capture | `loaf idea capture --title "<title>"` |
| Read back | `loaf idea show <ref>` |
| List open | `loaf idea list` |

---

## Purpose

Ideas are raw nuggets — unprocessed, unshaped, but worth remembering. The goal is **speed of capture**, not thoroughness. An idea is retained material, nothing more: tracking it as an Intent, exploring it, shaping it, or archiving it are triage dispositions chosen later by the user.

---

## Process

1. **Parse input.** If `$ARGUMENTS` contains the idea, capture directly. If empty, ask at most 2-3 questions: core idea, problem/opportunity, immediate constraints.
2. **Capture.** Run `loaf idea capture --title "..."` with the inferred title; log notable context with `loaf journal log`.
3. **Announce.** Report the captured alias and point at `/loaf:triage` for disposition.

---

## Guardrails

1. **Speed over completeness** — capture quickly, disposition later
2. **2-3 questions max** — don't turn this into an interview
3. **Infer, don't ask** — metadata should be automatic
4. **One idea per captured row** — keep them atomic
5. **No lifecycle here** — no status transitions, promotion, or shaping; triage owns dispositions and the CLI performs them

---

## Related Skills

- **triage** — process the intake queue and choose dispositions
- **explore** — divergent inquiry when an idea needs development before commitment
- **shape** — develop a chosen direction into a bounded Change
