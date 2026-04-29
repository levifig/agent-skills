---
id: TASK-134
title: Skill prose audit + ARCHITECTURE.md rewrite (strip fellowship vocab)
spec: SPEC-033
status: done
priority: P1
created: '2026-04-28T22:39:12.670Z'
updated: '2026-04-28T23:25:36.397Z'
depends_on:
  - TASK-132
completed_at: '2026-04-28T23:25:36.397Z'
---

# TASK-134: Skill prose audit + ARCHITECTURE.md rewrite

## Description

Two coordinated cleanups, both downstream of profile neutralization (TASK-132):

1. **Skill prose audit.** Grep `content/skills/` for `Smith|Sentinel|Ranger|Warden|Fellowship` references in agent-facing prose. Confirmed leak in `content/skills/orchestration/references/councils.md` (5 occurrences); spot-check other skill files surfaced by the grep. Replace with profile types (implementer/reviewer/researcher/librarian). Council vocabulary becomes functional: *"Councils convene implementers and researchers for deliberation; reviewers join only after, to verify the outcome."*

2. **ARCHITECTURE.md rewrite.** The "Agent Model: Functional Profiles" section currently centers the Warden as canonical. Rewrite to:
   - Describe the orchestrator role generically (the configurable soul defines the character)
   - Add a `### Soul Catalog` subsection explaining the catalog mechanism, the two shipped souls (fellowship/none), and the `loaf soul` CLI
   - Move character descriptions (Smith/Sentinel/etc.) into a fellowship-specific reference paragraph, framed as "the `fellowship` soul names them..."

3. **Knowledge file refresh.** `docs/knowledge/skill-architecture.md` mentions the 4 profiles by lore name in the table. Update to lead with functional types, with fellowship names as a secondary column tied to the `fellowship` soul.

Out of scope: every prose mention across the project. Focus on agent-facing skill content (orchestration councils + linear/sessions references) and architecture docs. Other prose drift can be cleaned up over time.

## Acceptance Criteria

- [ ] `content/skills/orchestration/references/councils.md` uses profile types only — zero `Smith|Sentinel|Ranger|Warden|Fellowship` matches
- [ ] Other agent-facing skill files surfaced by grep are cleaned up where reasonable (judgement call; documentation-only leaks acceptable)
- [ ] `docs/ARCHITECTURE.md` "Agent Model: Functional Profiles" section describes the configurable orchestrator + has a `### Soul Catalog` subsection
- [ ] `docs/knowledge/skill-architecture.md` updated to lead with functional types, fellowship as secondary column
- [ ] `loaf build` succeeds (no broken references after edits)

## Verification

```bash
! grep -lE 'Smith|Sentinel|Ranger|Warden|Fellowship' \
  content/skills/orchestration/references/councils.md && \
grep -q 'Soul Catalog' docs/ARCHITECTURE.md && \
loaf build
```

## Context

See SPEC-033 test condition T13. Depends on TASK-132 — the new neutralized profiles are the language model for this audit. Closes Track 2.
