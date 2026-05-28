---
id: TASK-105
title: 'Linear-native mode: specs local, tasks Linear-only (skill refactor)'
status: done
priority: P1
created: '2026-04-21T18:49:03.893Z'
updated: '2026-04-27T22:12:05.901Z'
completed_at: '2026-04-27T22:12:05.900Z'
---

# TASK-105: Linear-native mode: specs local, tasks Linear-only (skill refactor)

## Description

Refactor spec/task orchestration skills so that when `integrations.linear.enabled` is true in `.agents/loaf.json`, specs stay local (git) while tasks live in Linear as sub-issues under a `spec`-labeled parent rollup issue. When Linear is disabled, the existing local-tasks flow is preserved unchanged.

## Acceptance Criteria

- [x] Linear-native path mints a parent rollup issue labeled `spec`, creates sub-issues with `parentId`, maps Loaf priority P0–P3 to Linear priority 1–4
- [x] Pre-flight detects exclusive-label-group conflicts
- [x] `linear_parent` + `linear_parent_url` written to spec frontmatter
- [x] No local `TASK-NNN.md` files are created in Linear-native mode
- [x] Local-tasks mode unchanged when Linear is disabled
- [x] `/loaf:implement` walks sub-issues: lowest-ID in-progress first, else unique unblocked, else `AskUserQuestion` for multi-unblocked
- [x] Hard pre-flight gate: every `blockedBy` must be completed-type or the skill refuses to start
- [x] Completion auto-closes the parent when all siblings are completed
- [x] Housekeeping reconciliation surfaces (warnings, not auto-fixes): spec ↔ Linear-parent status mismatch, orphaned `linear_parent` references, pre-Linear local task detection
- [x] `orchestration/references/linear.md` expanded with `spec` label convention, parent-vs-child structure, `mcp_server_name` config, and multi-workspace guidance

## Verification

Shipped as PR #34, commit `9324c147`, included in v2.0.0-dev.29 release. See:

- `content/skills/breakdown/SKILL.md` — Linear-native breakdown path
- `content/skills/implement/SKILL.md` — sub-issue walking + pre-flight gates
- `content/skills/housekeeping/SKILL.md` — mode-aware reconciliation
- `content/skills/orchestration/references/linear.md` — convention docs

```bash
git show 9324c147 --stat
```
