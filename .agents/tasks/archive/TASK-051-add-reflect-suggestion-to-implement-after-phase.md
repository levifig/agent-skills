---
id: TASK-051
title: Add /reflect suggestion to /implement AFTER phase
spec: SPEC-011
status: done
priority: P1
created: '2026-03-27T23:19:05.410Z'
updated: '2026-03-27T23:26:23.114Z'
depends_on:
  - TASK-050
completed_at: '2026-03-27T23:26:23.114Z'
---

# TASK-051: Add /reflect suggestion to /implement AFTER phase

## Description

Add a conditional `/reflect` suggestion as step 8 in the AFTER (Completion) block of `/implement` SKILL.md, after "Commit housekeeping to main."

**Detection signals** (any triggers the suggestion):
- Session body `## Key Decisions` has content (not `*(none yet)*` or empty) — **primary**
- Session frontmatter `traceability.decisions` has entries (ADRs were created)
- Linked spec (`session.spec`) has `## Lessons Learned` with content
- Session has `session.spec` set (linked to meaningful work)

**Suggestion text:** "This session produced key decisions. Consider running /reflect to update strategic docs."

Silent when no detection signals are present.

**Files:** `content/skills/implement/SKILL.md`

## Acceptance Criteria

- [ ] Step 8 added to AFTER block with conditional `/reflect` suggestion
- [ ] Detection signals documented clearly
- [ ] Suggestion is advisory only (never blocking)
- [ ] Silent when no signals present
- [ ] `loaf build` succeeds

## Verification

```bash
loaf build
grep -c 'Consider running /reflect' content/skills/implement/SKILL.md  # should be >= 1
```
