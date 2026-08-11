---
change: spec-conversion-and-guidance-sweep
id: TASK-005
title: Skills convergence
blocked-by:
  - TASK-004
blocks:
  - TASK-007
---

# TASK-005 — Skills convergence

## Objective

The breakdown skill is deleted and no shipped skill, template, agent profile, or reference routes anyone toward the retired surface; the regenerated CLI reference and rebuilt target mirrors carry the converged content.

## Scope boundaries

**In:** Delete `content/skills/breakdown/` (4 files). Rewrite legacy references in: implement (SKILL.md, sidecar argument-hint, batch-orchestration, branch-and-completion), housekeeping (SKILL.md, report template), orchestration (SKILL.md breakdown link + local-tasks, linear touchpoints that are spec/task-bound, background-agents, journal, script-surface, parallel-agents, subagent-development, context-management), foundations (SKILL.md exemplar, code-review/tdd/verification rows), git-workflow commits reference, council frontmatter example, reflect (SKILL.md, sidecar), research (SKILL.md, templates), refactor-deepen (SKILL.md, plan template `spec:` field), wrap, documentation-standards, shape templates (pr.md legacy line, task.md slug rule stays), pitch interview-guide, librarian agent profile. Regenerate loaf-reference from the TASK-002 generator state. Rebuild and commit `dist/` + `plugins/` mirrors.

**Out:** Prose improvements beyond reference removal/redirection (Cut: no skill audit). Public docs (TASK-006). Linear guidance that the landed `linear-native-coordination` model owns — converge only what is spec/task-bound, per implement-preflight's re-check (Decision 6). Watch: wide but mechanical — split by skill cluster if one rebuild-and-commit cycle proves too large (sanctioned).

## Context pointers

- Contract: `shape.md` — Decision 6 and 11, Rabbit Holes ("Guidance rewrite scope creep")
- Inventory: the content-surface table in this shaping session's inventory (40 files)

## Acquisition

```bash
loaf journal log "skill(implement): TASK-005 — skills convergence"
```

## Steps

- [ ] Delete breakdown; orchestration's required link updated with the hygiene pin in the same commit
- [ ] Rewrite each referencing skill/template/agent file: remove or redirect to the Change flow
- [ ] Regenerate loaf-reference; contract test green
- [ ] `loaf build`; commit mirrors with sources

## Verification

- `npm run build` green; `git grep -l "loaf spec\|loaf task\|breakdown" content/ | grep -v infracost` returns nothing unexpected
- Routing sanity: shape/implement/housekeeping descriptions carry no dangling "use breakdown" pointers
