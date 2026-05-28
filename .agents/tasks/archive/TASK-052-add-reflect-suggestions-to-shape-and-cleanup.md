---
id: TASK-052
title: Add /reflect suggestions to /shape and /cleanup
spec: SPEC-011
status: done
priority: P2
created: '2026-03-27T23:19:09.457Z'
updated: '2026-03-27T23:26:23.582Z'
depends_on:
  - TASK-050
completed_at: '2026-03-27T23:26:23.581Z'
---

# TASK-052: Add /reflect suggestions to /shape and /cleanup

## Description

Add `/reflect` suggestions to two skills:

**Shape:** Add Step 9 after "Create Spec File" — conditionally suggest `/reflect` when the shaping session produced key decisions. Signals: `## Key Decisions` has content (primary), `traceability.decisions` has entries. No spec-lessons signal (spec was just created). Suggestion: "This shaping session produced key decisions. Consider running /reflect to update strategic docs."

**Cleanup:** In section "C. Check for Extraction Needs" (lines 48-52 within Sessions section), add `/reflect` as the recommended action for sessions with `decisions`. In section "D. Present Per Session" (lines 54-56), add "Extract & Archive → suggest `/reflect` before archiving" for sessions with extractable learnings. Note: cleanup SKILL.md was recently updated with a new Tasks section (section 2) — Sessions section is still section 1.

**Files:** `content/skills/shape/SKILL.md`, `content/skills/cleanup/SKILL.md`

**Circuit breaker:** At 75%, ship shape only and skip cleanup.

## Acceptance Criteria

- [ ] `/shape` Step 9 suggests `/reflect` after spec creation when decisions exist
- [ ] `/shape` stays silent when no decisions to extract
- [ ] `/cleanup` section C mentions `/reflect` as extraction mechanism
- [ ] `/cleanup` section D recommends `/reflect` before archiving sessions with learnings
- [ ] Suggestions are advisory only (never blocking)
- [ ] `loaf build` succeeds

## Verification

```bash
loaf build
grep -c 'reflect' content/skills/shape/SKILL.md     # should be >= 3 (existing + new)
grep -c 'reflect' content/skills/cleanup/SKILL.md    # should be >= 1
```
