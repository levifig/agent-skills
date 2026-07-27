---
change: change-work-model
id: TASK-006
title: Skills and guidance sweep
blocked-by:
  - TASK-003
relates-to:
  - TASK-002
  - TASK-004
  - TASK-005
---

# TASK-006 — Skills and guidance sweep

## Objective

Update `shape`, `implement`, and `triage` (and guidance in AGENTS.md / CLAUDE.md) to the shipped anatomy and vocabulary — never ahead of behavior. Shape teaches problem-boundary and vertical-slice tests; implement picks task files and flips boxes in delivering commits; triage seeds briefs.

## Scope boundaries

**In:** Skill and guidance content aligned to shipped CLI behavior; vocabulary sweep across templates.

**Out:** New CLI verbs; tracker adapters; served projections.

## Context pointers

- Decision 17; TASK-006 in change.md; H1 (delegation packet feel).

## Acquisition

```bash
loaf journal log "skill(implement): TASK-006 — skills and guidance sweep"
```

## Steps

- [x] `shape` produces role-named documents; teaches boundary and vertical-slice tests.
- [x] `implement` routes via task files and checkbox-in-commit discipline.
- [x] `triage` promotes intake by seeding `brief.md`.
- [x] Guidance follows shipped behavior only; rebuild targets show zero drift if content is distributed.

## Verification

- Content builds clean; H1 satisfied by delegating at least one task from its packet alone.
