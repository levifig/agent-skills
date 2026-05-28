---
id: TASK-099
title: Drafts lifecycle policy — session-linked cleanup in housekeeping
spec: SPEC-028
status: done
priority: P2
created: '2026-04-07T22:05:33.575Z'
updated: '2026-04-07T22:16:30.267Z'
completed_at: '2026-04-07T22:16:30.267Z'
---

# TASK-099: Drafts lifecycle policy — session-linked cleanup in housekeeping

## Description

Add a lifecycle policy to the housekeeping skill for state assessments in `drafts/`. State assessments are flagged for cleanup when their linked session is archived (not age-based).

**Files to modify:**
- `content/skills/housekeeping/SKILL.md` — add drafts lifecycle policy to Critical Rules, update Artifact Lifecycle table
- `content/skills/research/templates/state-assessment.md` — add `session:` field to frontmatter for linking

**Rules:**
- State assessments have `type: state-assessment` in frontmatter
- Linked session is identified by `session:` frontmatter field (if present) or inferred from the session active at creation time
- Housekeeping flags for cleanup when linked session status is `archived`
- Brainstorm drafts are NOT affected by this policy (no session-linked cleanup for brainstorms)

**Dependencies:** None (independent of TASK-097/098)

## Acceptance Criteria

- [ ] Housekeeping skill Critical Rules include drafts lifecycle policy for state assessments
- [ ] Artifact Lifecycle table includes Drafts row with trigger: "linked session archived"
- [ ] State assessment template includes `session:` frontmatter field
- [ ] Policy explicitly excludes brainstorm drafts
- [ ] Built output reflects changes across all targets

## Verification

```bash
loaf build && npm run typecheck && npm run test
# Confirm lifecycle policy in housekeeping:
grep -q 'session.*archived' content/skills/housekeeping/SKILL.md
# Confirm state-assessment template has session field:
grep -q 'session:' content/skills/research/templates/state-assessment.md
```
