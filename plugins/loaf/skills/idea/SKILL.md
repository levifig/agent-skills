---
name: idea
description: >-
  Captures ideas into structured nuggets for later evaluation. Use when the user
  says "I have an idea" or "note this down." Also activate when a specific
  actionable concept crystallizes during conversation. Ideas and sparks stay
  capture primitives routed through triage, which files worth-keeping items as
  backlog issues or hands them to pitch or shape. Not for problem discovery (use
  pitch), processing the intake queue (use triage), shaping a bounded issue (use
  shape), or agent-side divergent inquiry when direction is undecided (use
  explore as a technique).
user-invocable: true
argument-hint: '[idea description]'
version: 0.3.0
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
- Guardrails
- Related Skills

## Critical Rules

- Speed over completeness -- capture quickly, shape later
- 2-3 questions maximum -- don't turn capture into an interview
- Infer metadata automatically -- don't ask for tags, title, or links
- One idea per captured row/artifact -- keep them atomic
- No shaping, pitching, or filing issues here -- problem discovery is pitch; bounding is shape; dispositions are triage
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

Ideas are raw nuggets — unprocessed, unshaped, but worth remembering. The goal is **speed of capture**, not thoroughness. An idea is retained material, nothing more: filing it as a backlog issue, handing it to pitch for problem discovery, handing it to shape when already well-understood, or archiving it are triage dispositions chosen later by the user.

---

## Process

1. **Parse input.** If `$ARGUMENTS` contains the idea, capture directly. If empty, ask at most 2-3 questions: core idea, problem/opportunity, immediate constraints.
2. **Capture.** Run `loaf idea capture --title "..."` with the inferred title; log notable context with `loaf journal log`.
3. **Announce.** Report the captured alias and point at triage for disposition.

---

## Guardrails

1. **Speed over completeness** — capture quickly, disposition later
2. **2-3 questions max** — don't turn this into an interview
3. **Infer, don't ask** — metadata should be automatic
4. **One idea per captured row** — keep them atomic
5. **No lifecycle here** — no status transitions, promotion, or shaping; triage owns dispositions and the CLI performs them

---

## Related Skills

- **triage** — process the intake queue and choose dispositions (file as backlog issue, hand to pitch, or hand to shape)
- **pitch** — problem-discovery ceremony when a captured idea needs a brief before shaping
- **shape** — develop a well-understood direction into a bounded issue
- **explore** — agent technique when direction is genuinely undecided (not a user front door)
