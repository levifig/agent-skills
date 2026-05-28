---
id: TASK-049
title: 'Hook registration, build integration + docs'
spec: SPEC-015
status: done
priority: P1
created: '2026-03-27T16:27:43.289Z'
updated: '2026-03-27T16:45:24.486Z'
depends_on:
  - TASK-046
  - TASK-047
completed_at: '2026-03-27T16:45:24.485Z'
---

# TASK-049: Hook registration, build integration + docs

## Description

Wire everything together: register hooks, verify build, update docs. Three concerns:

### 1. Hook Registration in `config/hooks.yaml`

Add three new entries (or two if TASK-048 is skipped):

- **`workflow-pre-pr`** — PreToolUse on Bash, `blocking: true`, skill: foundations
- **`workflow-post-merge`** — PostToolUse on Bash, skill: foundations
- **`workflow-pre-push`** — PreToolUse on Bash, `blocking: false`, skill: foundations (if TASK-048 done)

Also update the `foundations` plugin-group's `related-hooks` to include the new hooks.

**Critical:** `blocking: true` on `workflow-pre-pr` is required — without it, exit 2 won't actually block and CHANGELOG enforcement is bypassed.

### 2. Build Integration

- Run `loaf build` and verify:
  - Hook scripts appear in `plugins/loaf/hooks/pre-tool/` and `plugins/loaf/hooks/post-tool/`
  - Instruction markdown files appear in `plugins/loaf/hooks/instructions/`
  - No build errors

### 3. Documentation Updates

- Update `content/skills/foundations/references/commits.md` to mention the workflow hooks
- Brief note in foundations SKILL.md reference table pointing to the hooks
- Ensure the hooks are mentioned in the "Use When" column for the commits reference

## Acceptance Criteria

- [ ] All workflow hooks registered in hooks.yaml with correct matchers and blocking flags
- [ ] `blocking: true` set on `workflow-pre-pr` (critical for exit 2 enforcement)
- [ ] foundations plugin-group updated with new hooks in `related-hooks`
- [ ] `loaf build` succeeds without errors
- [ ] Hook scripts appear in built plugin output (`plugins/loaf/`)
- [ ] Instruction files appear in built plugin output
- [ ] foundations references updated to document the workflow hooks

## Context

See SPEC-015 § "Registration in hooks.yaml" for the exact hook definitions. The `blocking: true` requirement is called out explicitly in the spec as critical.
