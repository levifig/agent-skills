---
id: TASK-156
title: Shared content/templates/grilling.md + targets.yaml distribution
status: done
priority: P1
created: '2026-05-02T01:25:35.806Z'
updated: '2026-05-02T01:25:35.806Z'
completed_at: '2026-05-02T03:35:00.000Z'
spec: SPEC-034
---

# TASK-156: Shared content/templates/grilling.md + targets.yaml distribution

## Description

Author `content/templates/grilling.md` as a shared interview-protocol template. Port the relentless-interview / decision-tree / per-question-recommendation / explore-when-can-answer mechanics from Matt Pocock's `grill-with-docs` SKILL.md. Add Loaf-specific rules: read existing glossary at start, challenge term drift inline, surface candidate terms during interview. **Does NOT own mutation policy** — that's per-skill (handled in SKILL.md of each consuming skill via CLI verbs). Register in `targets.yaml` `shared-templates` for distribution to `architecture` and `refactor-deepen` only — NOT `shape` (per deferred-evolution decision in idea 20260501-231923).

## File Hints

- `content/templates/grilling.md` (new)
- `config/targets.yaml` (extend `shared-templates`)
- Source reference: https://github.com/mattpocock/skills/tree/main/skills/engineering/grill-with-docs/SKILL.md

## Acceptance Criteria

- [ ] `content/templates/grilling.md` exists with proper Markdown structure
- [ ] Includes the four-rule mechanic: relentless interview, walk decision tree, recommend per question, explore when answerable
- [ ] Includes the Loaf-specific rules: read glossary at start, challenge drift inline, surface candidates
- [ ] Does NOT include mutation policy or specific CLI verbs (those live in consuming skill SKILL.md)
- [ ] `config/targets.yaml` `shared-templates` distributes `grilling.md` to `architecture` and `refactor-deepen`
- [ ] `shape` is NOT in the distribution list
- [ ] `loaf build` succeeds; `plugins/loaf/skills/architecture/templates/grilling.md` and `plugins/loaf/skills/refactor-deepen/templates/grilling.md` exist
- [ ] `plugins/loaf/skills/shape/templates/grilling.md` does NOT exist

## Verification

```bash
loaf build
ls plugins/loaf/skills/architecture/templates/grilling.md
ls plugins/loaf/skills/refactor-deepen/templates/grilling.md
[ ! -f plugins/loaf/skills/shape/templates/grilling.md ] && echo "shape correctly omitted" || echo "FAIL: shape should not have grilling.md"
```
