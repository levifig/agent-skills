---
id: TASK-094
title: Replace circuit breaker with priority order in spec template and shape skill
spec: SPEC-025
status: done
priority: P1
created: '2026-04-07T10:42:34.000Z'
updated: '2026-04-07T10:49:22.466Z'
completed_at: '2026-04-07T10:49:22.465Z'
---

# TASK-094: Replace circuit breaker with priority order in spec template and shape skill

## Description

Replace `## Circuit Breaker` with `## Priority Order` in the spec template. Remove appetite from shape skill's guardrails, interview process, and spec splitting guidance. Replace circuit breaker guidance with priority ordering + go/no-go gates.

## Key Files

- `content/skills/shape/templates/spec.md` — remove appetite frontmatter, replace circuit breaker section
- `content/skills/shape/SKILL.md` — lines 34, 37, 84, 102, 122, 140, 143

## Acceptance Criteria

- [ ] Spec template has `## Priority Order` instead of `## Circuit Breaker`
- [ ] Spec template frontmatter has no `appetite` field
- [ ] Shape skill guardrails reference priority ordering, not appetite/circuit breaker
- [ ] Shape interview process asks about priority order, not appetite
- [ ] `loaf build` succeeds

## Verification

```bash
loaf build && ! grep -i "appetite\|circuit.breaker" content/skills/shape/
```
