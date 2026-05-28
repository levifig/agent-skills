---
id: TASK-159
title: '/refactor-deepen SKILL.md (Critical Rules, vocabulary, glossary integration)'
spec: SPEC-034
status: done
priority: P1
created: '2026-05-02T01:25:48.983Z'
updated: '2026-05-02T01:25:48.983Z'
depends_on:
  - TASK-156
completed_at: '2026-05-02T03:35:00.000Z'
---

# TASK-159: /refactor-deepen SKILL.md (Critical Rules, vocabulary, glossary integration)

## Description

Author `/refactor-deepen` skill folder + SKILL.md. Critical Rules must include: vocabulary discipline (Module/Interface/Implementation/Depth/Seam/Adapter/Leverage/Locality), glossary integration via CLI verbs (call `loaf kb glossary check` before naming, `upsert` when naming a deepened module), grilling protocol via shared `templates/grilling.md`, INTERFACE-DESIGN with 3-agent unprimed default. Sidecar `SKILL.claude-code.yaml` if Claude-specific fields are needed. Description ≤250 chars first sentence with action verb start and negative routing. Body remains tight; references and templates link out, not inline.

## File Hints

- `content/skills/refactor-deepen/SKILL.md` (new)
- `content/skills/refactor-deepen/SKILL.claude-code.yaml` (if needed)

## Acceptance Criteria

- [ ] `content/skills/refactor-deepen/SKILL.md` exists with sections: Critical Rules, Verification, Quick Reference, Topics
- [ ] Description ≤250 chars first sentence; starts with action verb ("Surfaces..." or similar); includes negative routing: "Not for renames, extractions, or generic restructuring (use `/implement`)."
- [ ] Critical Rules include vocabulary discipline (terms from `references/language.md`)
- [ ] Critical Rules include: call `loaf kb glossary check <term>` when surfacing terms; call `loaf kb glossary upsert` when a deepening clearly names a structural module
- [ ] Critical Rules include: skill self-logs to session journal as first action (`loaf session log "skill(refactor-deepen): ..."`)
- [ ] References shared `templates/grilling.md`
- [ ] Topics table links to `references/language.md`, `references/deepening.md`, `references/interface-design.md`, `templates/plan.md`
- [ ] `loaf build` succeeds; skill appears in registry for all 6 targets

## Verification

```bash
loaf build
grep -q "name: refactor-deepen" plugins/loaf/skills/refactor-deepen/SKILL.md
grep -q "Not for renames" plugins/loaf/skills/refactor-deepen/SKILL.md
```
