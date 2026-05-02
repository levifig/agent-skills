---
id: TASK-162
title: templates/plan.md + filename convention + .agents/plans/ lazy creation
status: done
priority: P2
created: '2026-05-02T01:25:49.154Z'
updated: '2026-05-02T01:25:49.154Z'
completed_at: '2026-05-02T03:35:00.000Z'
spec: SPEC-034
depends_on:
  - TASK-159
---

# TASK-162: templates/plan.md + filename convention + .agents/plans/ lazy creation

> **Note (post-implementation revision):** Original acceptance criteria specified
> a `PLAN-NNN` sequential-ID scheme. After SPEC-034 review, the convention
> switched to filename-as-identity using `YYYYMMDD-HHMMSS-{slug}.md` to match
> Loaf's existing temporal-record pattern (sessions, ideas, drafts, councils).
> Acceptance criteria below reflect the shipped convention.

## Description

Author the minimal PLAN artifact template at `content/skills/refactor-deepen/templates/plan.md`. Document the filename convention (`YYYYMMDD-HHMMSS-{slug}.md` — temporal-record naming, parallel to sessions/ideas/drafts/councils) and lazy `.agents/plans/` directory creation. Plans are write-once snapshots — a re-deepening of the same module produces a new file rather than updating an existing one.

## File Hints

- `content/skills/refactor-deepen/templates/plan.md` (new)
- Skill prose in TASK-159's SKILL.md will reference this template

## Acceptance Criteria

- [x] Template includes minimal sections: candidate, dependency category (per `references/deepening.md`), proposed deepened module, what survives in tests, rejected alternatives
- [x] Frontmatter schema: `title`, `created` (ISO 8601 UTC, must match filename timestamp), `status: drafting`, `spec: <SPEC-NNN if related, else null>`, optional `related:`. **No `id` field** — the filename is the identity.
- [x] Filename convention documented: `.agents/plans/<YYYYMMDD-HHMMSS>-<slug>.md`, generated via `date -u +%Y%m%d-%H%M%S`
- [x] Lazy `.agents/plans/` creation rule documented: skill creates directory only on first plan write
- [x] If template file >100 lines, include `## Contents` TOC
- [x] `loaf build` distributes the template

## Verification

```bash
loaf build
ls plugins/loaf/skills/refactor-deepen/templates/plan.md
grep -q "YYYYMMDD-HHMMSS" content/skills/refactor-deepen/templates/plan.md
grep -qv "PLAN-NNN" content/skills/refactor-deepen/templates/plan.md
```
