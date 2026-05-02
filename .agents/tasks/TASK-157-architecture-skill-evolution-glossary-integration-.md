---
id: TASK-157
title: '/architecture skill evolution: glossary integration, ADR template preserved'
status: done
priority: P1
created: '2026-05-02T01:25:35.857Z'
updated: '2026-05-02T01:25:35.857Z'
completed_at: '2026-05-02T03:35:00.000Z'
spec: SPEC-034
depends_on:
  - TASK-152
  - TASK-154
  - TASK-156
---

# TASK-157: /architecture skill evolution: glossary integration, ADR template preserved

## Description

Evolve `content/skills/architecture/SKILL.md` to absorb glossary side-effects. Add Critical Rules: read existing glossary at start of ADR interview; challenge fuzzy language inline; offer `loaf kb glossary stabilize` or `upsert` when load-bearing terms surface. Reference shared `templates/grilling.md`. **Critical constraint:** `templates/adr.md` MUST remain byte-identical — only SKILL.md gets new rules. The ADR template is the existing contract; glossary work is additive to the surrounding interview, not part of the artifact format.

## File Hints

- `content/skills/architecture/SKILL.md` (extend Critical Rules + Process)
- `content/skills/architecture/templates/adr.md` (UNCHANGED — verify intact)

## Acceptance Criteria

- [ ] SKILL.md has new Critical Rule: "Read `docs/knowledge/glossary.md` at interview start (via `loaf kb glossary list`); use canonical terms throughout"
- [ ] SKILL.md has new Critical Rule: "When fuzzy/drifted language surfaces, challenge inline; if a load-bearing term emerges, offer `loaf kb glossary upsert` or `stabilize`"
- [ ] SKILL.md references `templates/grilling.md` (the shared template distributed via TASK-156)
- [ ] SKILL.md description includes glossary interaction in its action-verb sentence (≤250 chars)
- [ ] `templates/adr.md` is byte-identical to its pre-change state (verifiable via `git diff HEAD -- content/skills/architecture/templates/adr.md` returning empty)
- [ ] `loaf build` succeeds; `/architecture` skill builds to all 6 targets

## Verification

```bash
git diff HEAD -- content/skills/architecture/templates/adr.md   # MUST be empty
loaf build
grep -q "loaf kb glossary" content/skills/architecture/SKILL.md
```
