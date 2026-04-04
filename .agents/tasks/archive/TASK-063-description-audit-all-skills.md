---
id: TASK-063
title: Audit and rewrite descriptions for all skills
spec: SPEC-014
status: done
priority: P2
created: '2026-04-04T16:41:22.302Z'
updated: '2026-04-04T16:41:22.302Z'
completed_at: '2026-04-04T16:41:22.302Z'
---

# TASK-063: Audit and rewrite descriptions for all skills

Audit and rewrite the `description` field in every skill's `SKILL.md` frontmatter to optimize context-based triggering.

## Description formula (100-300 chars, max 1024)

1. **Third-person action verb opener**: "Covers...", "Establishes...", "Coordinates..."
2. **Specific scope terms**: key terms Claude will match against
3. **User-intent phrases**: "Use when..." with natural language triggers
4. **Negative routing**: "Not for..." to disambiguate confusable skills
5. **Success criteria** (workflow skills only): "Produces..."

## Skills to audit (~27 total)

### New skills from Track A (4)
- git-workflow
- debugging
- security-compliance
- documentation-standards

### Slimmed foundations (1)
- foundations (updated scope)

### Existing reference skills (8)
- python-development
- ruby-development
- go-development
- typescript-development
- database-design
- infrastructure-management
- interface-design
- power-systems-modeling

### Existing workflow skills (13)
- orchestration, implement, council-session, research, brainstorm
- breakdown, shape, strategy, architecture, reflect
- reference-session, resume-session, bootstrap, cleanup, idea

### Other (1)
- knowledge-base

## Test
- Every skill has a description with action verb opener
- Every skill has user-intent phrases ("Use when...")
- Reference skills have negative routing ("Not for...")
- Workflow skills have success criteria ("Produces...")
- No description exceeds 1024 chars

## Relates to
- R16
