---
id: TASK-057
title: Create 4 focused skills from foundations decomposition
spec: SPEC-014
status: done
priority: P0
created: '2026-04-04T16:41:22.302Z'
updated: '2026-04-04T16:41:22.302Z'
completed_at: '2026-04-04T16:41:22.302Z'
---

# TASK-057: Create 4 focused skills from foundations decomposition

Create 4 new skills by extracting references from the monolithic foundations skill. Each new skill gets a `SKILL.md`, `.claude-code.yaml` sidecar, and a `references/` directory with the moved files.

## New Skills

### 1. `git-workflow`
- **References**: `commits.md` (moved from foundations)
- **Description focus**: Branching strategy, commit conventions, PR creation, squash merge workflow
- **Sidecar**: `user-invocable: false`

### 2. `debugging`
- **References**: `debugging.md`, `hypothesis-tracking.md`, `test-debugging.md`
- **Description focus**: Systematic debugging, hypothesis tracking, flaky test investigation
- **Sidecar**: `user-invocable: false`

### 3. `security-compliance`
- **References**: `security.md`, `security-review.md`
- **Description focus**: Threat modeling, secrets management, compliance checks
- **Sidecar**: `user-invocable: false`

### 4. `documentation-standards`
- **References**: `documentation.md`, `documentation-review.md`, `diagrams.md`
- **Description focus**: ADRs, API docs, changelogs, Mermaid diagrams
- **Sidecar**: `user-invocable: false`

## For each skill:
1. Create `content/skills/{name}/SKILL.md` with proper frontmatter (action verb opener, user-intent phrases, negative routing)
2. Create `content/skills/{name}/SKILL.claude-code.yaml` with `user-invocable: false`
3. Create `content/skills/{name}/references/` directory
4. Move (git mv) reference files from `content/skills/foundations/references/`

## Test
- Each new skill directory has SKILL.md + sidecar + references/
- Reference files are moved, not copied (no duplicates)
- `loaf build` still succeeds

## Relates to
- R3, R5, R6
