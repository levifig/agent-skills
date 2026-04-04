---
id: TASK-072
title: Apply SKILL.md structural convention + verb/noun hygiene
spec: SPEC-020
status: todo
priority: P1
created: '2026-04-04T16:41:22.295Z'
updated: '2026-04-04T16:41:22.295Z'
---

# TASK-072: Apply SKILL.md structural convention + verb/noun hygiene

Restructure all SKILL.md files and apply verb/noun classification + renames.

## Scope

**Structural convention:** All ~30 SKILL.md files restructured to: Critical Rules (top, highest attention weight) -> Verification -> Quick Reference -> Topics. Everything above Topics table = always-apply content that changes model behavior. Everything in Topics = depth on demand.

**Token budgets (always-apply section):**
- Reference skills: < 500 tokens (~50 lines)
- Cross-cutting skills: < 1000 tokens (~100 lines)
- Workflow skills: < 2000 tokens (~200 lines)
- Total SKILL.md: < 5000 tokens (~500 lines)

**Verb/noun classification:** Verify all skills are correctly classified. Processes = verb-skills (user-invocable commands). Knowledge = noun-skills (references).

**Skill hygiene:**
- Rename `council-session` -> `council` (directory rename + all references in hooks.yaml, targets.yaml, sidecars)
- Flip `debugging` to `user-invocable: true` in its sidecar
- Classify all 30 skills as verb or noun

**Adherence techniques:** Add anti-rationalization, bright-line rules, forbidden output patterns, success criteria checklists where applicable per skill type.

## Verification

- [ ] Every SKILL.md follows section order: Critical Rules -> Verification -> Quick Reference -> Topics
- [ ] Every skill has non-empty Critical Rules or equivalent always-apply section
- [ ] Token budgets met per tier
- [ ] `council-session` directory renamed to `council`; all references updated
- [ ] `debugging` sidecar has `user-invocable: true`
- [ ] All skills classified verb/noun
- [ ] `loaf build` succeeds (no broken references)
