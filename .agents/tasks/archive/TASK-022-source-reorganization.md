---
id: TASK-022
title: Source reorganization
spec: SPEC-008
status: done
priority: P1
created: '2026-03-16T16:27:15.466Z'
updated: '2026-03-16T16:27:15.466Z'
files:
  - content/skills/
  - content/agents/
  - content/hooks/
  - content/templates/
  - config/hooks.yaml
  - config/targets.yaml
  - cli/lib/build/
verify: ls content/skills content/agents content/hooks content/templates config/
done: >-
  Content lives in content/, config in config/, build logic copied to
  cli/lib/build/
completed_at: '2026-03-16T16:27:15.466Z'
---

# TASK-022: Source reorganization

## Description

Separate distributable content from tooling by reorganizing the source directory structure. Move skills, agents, hooks, and templates to `content/`. Move config to `config/`. Copy build logic to `cli/lib/build/` (keeping as JS initially).

## Moves

| From | To |
|------|----|
| `src/skills/` | `content/skills/` |
| `src/agents/` | `content/agents/` |
| `src/hooks/` | `content/hooks/` |
| `src/templates/` | `content/templates/` |
| `src/config/hooks.yaml` | `config/hooks.yaml` |
| `src/config/targets.yaml` | `config/targets.yaml` |
| `build/*.js` | `cli/lib/build/` (copy, keep as JS) |
| `build/lib/*.js` | `cli/lib/build/lib/` (copy, keep as JS) |
| `build/targets/*.js` | `cli/lib/build/targets/` (copy, keep as JS) |

## Acceptance Criteria

- [ ] `content/` contains skills, agents, hooks, templates
- [ ] `config/` contains hooks.yaml and targets.yaml
- [ ] `cli/lib/build/` contains copied build JS files
- [ ] Internal path references updated in build files
- [ ] Old `src/` directory removed (after TASK-023 confirms build works)
- [ ] No broken file references in hooks.yaml or targets.yaml

## Context

See SPEC-008 for full context. Can run in parallel with TASK-021 (no dependency).
Circuit breaker stage: 30%.

## Work Log

<!-- Updated by session as work progresses -->
