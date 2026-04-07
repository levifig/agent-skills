---
id: TASK-095
title: >-
  Remove plan file concept from implement, orchestration, housekeeping, and
  config
spec: SPEC-025
status: done
priority: P2
created: '2026-04-07T10:42:34.049Z'
updated: '2026-04-07T10:53:23.509Z'
completed_at: '2026-04-07T10:53:23.508Z'
---

# TASK-095: Remove plan file concept from implement, orchestration, housekeeping, and config

## Description

Remove the dead plan file concept. No plan files have ever been created — specs serve this purpose. Delete the plan template, remove all plan file references from skills, and clean up config.

## Key Files

- `content/templates/plan.md` — DELETE
- `content/skills/implement/SKILL.md` — remove plan file refs (lines 48, 150, 154-155, 194, 202, 214)
- `content/skills/implement/references/session-management.md` — remove Plan Mode Integration section (lines 130, 152-218)
- `content/skills/orchestration/references/sessions.md` — remove plans frontmatter and Plans section (lines 125, 245-249)
- `content/skills/orchestration/references/planning.md` — remove Plan File Storage section (lines 438-550)
- `content/skills/orchestration/references/specs.md` — remove plan references (line 178)
- `content/skills/housekeeping/SKILL.md` — remove plan references (lines 26, 60)
- `config/targets.yaml` — remove plan.md from shared-templates
- `.agents/plans/` — DELETE empty directory

## Acceptance Criteria

- [ ] `content/templates/plan.md` deleted
- [ ] No plan file references in implement skill or its references
- [ ] No plan references in orchestration/housekeeping skills
- [ ] `plan.md` removed from `shared-templates` in targets.yaml
- [ ] `.agents/plans/` directory removed
- [ ] `loaf build` succeeds

## Verification

```bash
loaf build && ! test -f content/templates/plan.md && ! test -d .agents/plans
```
