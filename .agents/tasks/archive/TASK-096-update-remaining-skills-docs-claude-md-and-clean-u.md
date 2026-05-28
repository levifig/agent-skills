---
id: TASK-096
title: 'Update remaining skills, docs, CLAUDE.md, and clean up data'
spec: SPEC-025
status: done
priority: P2
created: '2026-04-07T10:42:34.097Z'
updated: '2026-04-07T11:03:21.619Z'
completed_at: '2026-04-07T11:03:21.618Z'
---

# TASK-096: Update remaining skills, docs, CLAUDE.md, and clean up data

## Description

Remove appetite and circuit breaker references from orchestration, breakdown, bootstrap skills, docs, and CLAUDE.md. Replace with priority ordering where appropriate. Clean up active spec frontmatter and archive the absorbed idea file.

## Key Files

- `content/skills/orchestration/SKILL.md` — lines 69, 71
- `content/skills/orchestration/references/specs.md` — appetite field, circuit breaker sections, sizing table
- `content/skills/orchestration/references/planning.md` — appetite-driven examples, circuit breaker checkpoints, betting table
- `content/skills/breakdown/SKILL.md` — lines 109, 118
- `content/skills/bootstrap/SKILL.md` and `references/interview-guide.md` — appetite refs
- `docs/ARCHITECTURE.md`, `docs/knowledge/task-system.md` — appetite/circuit breaker docs
- `.agents/AGENTS.md` (CLAUDE.md target) — circuit breaker references
- `.agents/specs/SPEC-*.md` (active only) — remove appetite from frontmatter
- `.agents/TASKS.json` — remove appetite values if present
- `.agents/ideas/20260328-replace-circuit-breaker-with-go-nogo.md` — archive (absorbed into SPEC-025)

## Acceptance Criteria

- [ ] `grep -r "appetite" content/ docs/` returns zero matches
- [ ] `grep -r "Circuit Breaker" content/skills/` returns zero matches (excluding software pattern refs)
- [ ] Active spec frontmatter has no appetite field
- [ ] Go/no-go idea file archived
- [ ] `loaf build` succeeds

## Verification

```bash
loaf build && ! grep -ri "appetite" content/ docs/ && ! grep -r "Circuit Breaker" content/skills/
```
