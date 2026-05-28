---
id: TASK-039
title: knowledge-base skill
spec: SPEC-009
status: done
priority: P2
created: '2026-03-24T19:29:16Z'
updated: '2026-03-24T19:29:16Z'
files:
  - content/skills/knowledge-base/SKILL.md
  - content/skills/knowledge-base/SKILL.claude-code.yaml
  - content/skills/knowledge-base/references/frontmatter-schema.md
  - content/skills/knowledge-base/templates/knowledge-file.md
  - config/hooks.yaml
verify: loaf build
done: >-
  knowledge-base skill exists with SKILL.md, sidecar, frontmatter reference, and
  knowledge file template. Registered in plugin-groups in hooks.yaml. loaf build
  includes it in output.
completed_at: '2026-03-24T19:29:16Z'
---

# TASK-039: knowledge-base skill

## Description

Create the knowledge-base skill that provides agent guidance for creating, updating,
and reviewing knowledge files. This is pure content — no code changes to the CLI.
Can be done in parallel with any other task.

## Acceptance Criteria

- [ ] `content/skills/knowledge-base/SKILL.md`:
  - Name: `knowledge-base`
  - Description starts with action verb, includes user-intent phrases
  - Covers: creating knowledge files, updating stale content, reviewing process,
    frontmatter requirements, naming conventions
  - References table linking to reference docs
  - Negative routing: "Not for retrieval/search (use QMD directly)"
- [ ] `SKILL.claude-code.yaml` sidecar: `user-invocable: false` (pure reference)
- [ ] `references/frontmatter-schema.md`: full schema documentation with required
  vs optional fields, valid values, examples
- [ ] `templates/knowledge-file.md`: scaffold template with standard frontmatter
  and section structure
- [ ] Registered in `config/hooks.yaml` plugin-groups (appropriate group)
- [ ] `loaf build` succeeds and includes the skill in output

## Context

See SPEC-009 for full context. Follow skill development guidelines in CLAUDE.md:
action-verb descriptions, sidecar for Claude-specific fields, reference table with
"Use When" column. See `docs/decisions/ADR-004-knowledge-naming-convention.md` for
naming conventions to include in the skill.
