---
id: TASK-090
title: Ad-hoc task auto-creation in /implement
spec: SPEC-027
status: done
priority: P1
created: '2026-04-07T10:10:56.366Z'
updated: '2026-04-07T10:31:23.408Z'
completed_at: '2026-04-07T10:31:23.407Z'
---

# TASK-090: Ad-hoc task auto-creation in /implement

## Description

Implement SPEC-027 Part A: when `/implement` receives free text that doesn't match any known pattern (TASK-XXX, SPEC-XXX, Linear ID), auto-create a local task and fall through to the task-coupled flow.

**Smart parsing:** Single sentence → task title. Multi-sentence → first sentence = title, remainder = acceptance criteria. Split on `. ` followed by uppercase letter only (conservative — no URL/abbreviation false positives).

**Error case:** If input matches `TASK-XXX` pattern but task doesn't exist in TASKS.json, show error with option to create from raw text. Don't silently create — the user probably has a typo.

## Key Files

- `content/skills/implement/SKILL.md` — Input Detection table (line ~85), Ad-hoc Sessions section (line ~105)

## Acceptance Criteria

- [ ] `/implement "fix the login button"` creates TASK-XXX with title "fix the login button" and proceeds to session/plan creation without user interaction
- [ ] `/implement "Fix auth flow. Tokens should rotate every 24h. Add refresh endpoint."` creates task with title "Fix auth flow" and remaining sentences as acceptance criteria
- [ ] `/implement TASK-999` (non-existent) shows error message, doesn't silently create
- [ ] Existing TASK-XXX, SPEC-XXX, and Linear ID paths unchanged
- [ ] `loaf build` succeeds

## Verification

```bash
loaf build
```

## Context

See SPEC-027 Part A. This is purely a skill content change — the auto-creation behavior is driven by updated instructions in the implement skill, which uses `loaf task create` at runtime.
