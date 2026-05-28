---
id: TASK-097
title: Unified report template and research skill updates
spec: SPEC-028
status: done
priority: P1
created: '2026-04-07T22:05:27.033Z'
updated: '2026-04-07T22:16:30.195Z'
completed_at: '2026-04-07T22:16:30.195Z'
---

# TASK-097: Unified report template and research skill updates

## Description

Merge the separate findings and report templates into a single unified report template, update the research skill to write directly to `.agents/reports/`, and clean up cross-references.

**Files to modify:**
- `content/skills/research/templates/report.md` — rewrite as unified template with status lifecycle
- `content/skills/research/templates/findings.md` — delete
- `content/skills/research/SKILL.md` — Topic Investigation output path, Topics table, remove promotion step
- `content/skills/housekeeping/templates/report.md` — align frontmatter with unified template

**Dependencies:** None (foundation task)

## Acceptance Criteria

- [ ] `content/skills/research/templates/report.md` has unified frontmatter: type (research|audit|analysis|council), status (draft|final|archived), source, tags
- [ ] `content/skills/research/templates/findings.md` is deleted
- [ ] Research skill Topic Investigation writes to `.agents/reports/`, not `.agents/drafts/`
- [ ] "Promotion from Draft" section removed from report template
- [ ] Housekeeping report template frontmatter matches unified template
- [ ] State assessments still write to `.agents/drafts/` (unchanged)
- [ ] Brainstorm output unchanged (still `.agents/drafts/`)

## Verification

```bash
loaf build && npm run typecheck && npm run test
# Confirm findings.md deleted:
test ! -f content/skills/research/templates/findings.md
# Confirm reports/ path in research skill:
grep -q '.agents/reports/' content/skills/research/SKILL.md
# Confirm drafts/ NOT in research Topic Investigation:
! grep 'agents/drafts.*research' content/skills/research/SKILL.md
```
