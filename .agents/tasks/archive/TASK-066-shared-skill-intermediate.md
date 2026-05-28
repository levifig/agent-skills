---
id: TASK-066
title: Build shared skill intermediate at dist/skills/
spec: SPEC-020
status: done
priority: P0
created: '2026-04-04T16:41:22.293Z'
updated: '2026-04-04T19:33:57.392Z'
completed_at: '2026-04-04T19:33:57.392Z'
---

# TASK-066: Build shared skill intermediate at dist/skills/

Add a pre-target build step that produces `dist/skills/` as a shared content intermediate.

## Scope

Add to `cli/commands/build.ts` a step that runs before any target build:
1. Clean `dist/skills/`
2. Call `copySkills()` from TASK-065 with:
   - Universal command substitution (unscoped `/implement`, `/resume`)
   - Base frontmatter only (no sidecar merge, no version injection)
   - Shared templates distributed per `targets.yaml` config
   - References, templates, scripts copied

The intermediate is a **staging artifact** — targets read from it instead of `content/skills/`.

## Constraints

- No `mergeFrontmatter` callback (base frontmatter only)
- No version injection (targets add it during their copy step)
- Must run before any target build, not in parallel with targets
- Existing target output unchanged (targets still read from `content/skills/` until their refactor tasks)

## Verification

- [ ] `loaf build` produces `dist/skills/` with all skills
- [ ] Skills contain base frontmatter only (no `user-invocable`, no `version`)
- [ ] Command placeholders substituted with unscoped forms
- [ ] Shared templates distributed correctly
- [ ] `npm run typecheck` and `npm run test` pass
