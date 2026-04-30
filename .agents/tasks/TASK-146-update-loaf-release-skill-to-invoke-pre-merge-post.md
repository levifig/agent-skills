---
id: TASK-146
title: >-
  Update /loaf:release skill to invoke --pre-merge / --post-merge + chore:
  release shape
spec: SPEC-031
status: done
priority: P2
created: '2026-04-29T17:28:14.904Z'
updated: '2026-04-29T19:59:47.615Z'
depends_on:
  - TASK-142
  - TASK-144
completed_at: '2026-04-29T19:59:47.614Z'
---

# TASK-146: Update /loaf:release skill to invoke --pre-merge / --post-merge + chore: release shape

## Description

Update `content/skills/release/SKILL.md` so step 4 (curated and generated paths) invokes `loaf release --pre-merge` and step 6 invokes `loaf release --post-merge`. Update all `release: vX.Y.Z` example commits to `chore: release vX.Y.Z`. Explicitly note that manual tag, push, and GH release commands are no longer needed — the CLI flags subsume them. Implements SPEC-031 Task 10.

## Acceptance Criteria

- [ ] Step 4 in `content/skills/release/SKILL.md` invokes `loaf release --pre-merge` in both the curated-entries and auto-generated paths.
- [ ] Step 6 in `content/skills/release/SKILL.md` invokes `loaf release --post-merge`.
- [ ] All `release: vX.Y.Z` example commit subjects in the skill are replaced with `chore: release vX.Y.Z`.
- [ ] Skill explicitly states that manual `git tag`, `git push --tags`, `gh release create`, and branch-cleanup steps are no longer needed.
- [ ] `grep -n 'release:' content/skills/release/SKILL.md` returns no occurrences except inside regex-shape examples.
- [ ] Skill prose mentions the new `--pre-merge` and `--post-merge` flags as the canonical invocations.

## Verification

```bash
npm run typecheck && grep -n 'release:' content/skills/release/SKILL.md
```
