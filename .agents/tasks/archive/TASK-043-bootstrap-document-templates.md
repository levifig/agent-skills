---
id: TASK-043
title: Bootstrap document templates
spec: SPEC-013
status: done
priority: P2
created: '2026-03-27T02:59:48.199Z'
updated: '2026-03-27T11:02:18.218Z'
completed_at: '2026-03-27T11:02:18.217Z'
---

# TASK-043: Bootstrap document templates

## Description

Create document templates that the bootstrap skill references.

**Files:** `content/skills/bootstrap/templates/brief.md`, possibly `config/targets.yaml` (shared-templates)

**Templates:**
- `templates/brief.md` — BRIEF.md template with frontmatter schema (`source`, `original_path`, `created`) and section structure
- Check if shared `content/templates/session.md` already works for bootstrap sessions. If so, add `bootstrap` to `shared-templates` list in `targets.yaml`. If not, create `templates/session.md` specific to bootstrap.

## Acceptance Criteria

- [ ] `templates/brief.md` exists with valid frontmatter schema and section structure
- [ ] Session template is available (shared or skill-specific)
- [ ] Template links from SKILL.md resolve (no broken `templates/` paths)
- [ ] Templates follow Loaf template conventions (YAML frontmatter, clear section headings)

## Verification

```bash
ls content/skills/bootstrap/templates/
loaf build --target claude-code
# Verify templates appear in built output
```

## Context
See SPEC-013 — Brief as Artifact, Session Recording sections.
