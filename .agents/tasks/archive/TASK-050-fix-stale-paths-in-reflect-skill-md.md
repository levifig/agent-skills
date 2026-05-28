---
id: TASK-050
title: Fix stale paths in /reflect SKILL.md
spec: SPEC-011
status: done
priority: P1
created: '2026-03-27T23:19:02.500Z'
updated: '2026-03-27T23:26:22.598Z'
completed_at: '2026-03-27T23:26:22.597Z'
---

# TASK-050: Fix stale paths in /reflect SKILL.md

## Description

Fix stale file paths in `/reflect` SKILL.md. Line 49 references `docs/specs/SPEC-*.md` but specs live at `.agents/specs/`. Verify all other paths (session paths, template links, strategic doc paths) are accurate.

**Files:** `content/skills/reflect/SKILL.md`

## Acceptance Criteria

- [ ] `docs/specs/SPEC-*.md` on line 49 changed to `.agents/specs/SPEC-*.md`
- [ ] Session path (`.agents/sessions/`) confirmed correct
- [ ] Template link (`templates/update-proposal.md`) confirmed resolving
- [ ] Strategic doc paths (VISION.md, STRATEGY.md, ARCHITECTURE.md) confirmed accurate
- [ ] `loaf build` succeeds

## Verification

```bash
loaf build
grep -c 'docs/specs/' content/skills/reflect/SKILL.md  # should be 0
grep -c '.agents/specs/' content/skills/reflect/SKILL.md  # should be >= 1
```
