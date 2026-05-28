---
id: TASK-056
title: Update cleanup skill to reference CLI
spec: SPEC-012
status: done
priority: P2
created: '2026-03-28T23:36:12.093Z'
updated: '2026-03-29T02:21:19.733Z'
completed_at: '2026-03-29T02:21:19.733Z'
---

# TASK-056: Update cleanup skill to reference CLI

## Description

Update the `/cleanup` skill to reference `loaf cleanup` as the execution engine. The skill remains the authoritative source for cleanup *policy* (when and why), while the CLI handles *execution* (scanning, prompting, archiving). The skill should explicitly own the Linear-aware checks that the CLI doesn't cover.

## What to do

1. Add a section near the top of SKILL.md directing agents to use `loaf cleanup` for filesystem operations
2. Clarify the division: skill = policy + Linear checks, CLI = filesystem scanning + actions
3. Replace manual archive instructions with `loaf cleanup` invocations where appropriate
4. Keep the differentiated rules table as the authoritative reference
5. Note that `loaf cleanup --dry-run` can be used for assessment before taking actions

## Acceptance Criteria

- [ ] Skill references `loaf cleanup` command for filesystem operations
- [ ] Division of responsibility is clear: skill (policy + Linear) vs CLI (filesystem execution)
- [ ] Manual archive instructions replaced with CLI equivalents
- [ ] `loaf build` succeeds with updated skill

## Verification

```bash
loaf build
```
