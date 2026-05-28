---
id: TASK-058
title: Slim foundations and reassign hook skill fields
spec: SPEC-014
status: done
priority: P0
created: '2026-04-04T16:41:22.302Z'
updated: '2026-04-04T16:41:22.302Z'
completed_at: '2026-04-04T16:41:22.302Z'
---

# TASK-058: Slim foundations and reassign hook skill fields

After the 4 new skills exist (TASK-057), slim the original foundations skill to its core code quality focus and update hook `skill:` fields in `config/hooks.yaml`.

## Slim foundations

Update `content/skills/foundations/SKILL.md`:
- Remove references to moved files (commits, debugging, hypothesis-tracking, test-debugging, security, security-review, documentation, documentation-review, diagrams)
- Keep: `code-style.md`, `tdd.md`, `verification.md`, `code-review.md`, `review.md`, `permissions.md`, `observability.md`, `production-readiness.md`
- Update description to reflect narrowed scope (code style, naming conventions, TDD, verification, review discipline)
- Update the Topics reference table

## Reassign hooks

Update `skill:` fields in `config/hooks.yaml` for hooks that now belong to new skills:
- `check-secrets` → `skill: security-compliance`
- `security-audit` → `skill: security-compliance`
- `validate-push` → `skill: git-workflow`
- `workflow-pre-pr` → `skill: git-workflow`
- `workflow-pre-merge` → `skill: git-workflow`
- `workflow-pre-push` → `skill: git-workflow`
- `validate-changelog` → `skill: documentation-standards`
- Other hooks: audit each and assign to the most relevant skill

## Test
- Slimmed foundations references only its retained files
- No broken reference links in any SKILL.md
- Hook `skill:` fields point to valid skill names
- `loaf build` succeeds

## Relates to
- R4
