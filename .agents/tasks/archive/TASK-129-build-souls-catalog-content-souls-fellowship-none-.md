---
id: TASK-129
title: 'Build souls catalog (content/souls/{fellowship,none}/SOUL.md)'
spec: SPEC-033
status: done
priority: P0
created: '2026-04-28T22:39:12.399Z'
updated: '2026-04-28T22:56:10.943Z'
completed_at: '2026-04-28T22:56:10.942Z'
---

# TASK-129: Build souls catalog (content/souls/{fellowship,none}/SOUL.md)

## Description

Create the catalog directory structure with two souls.

- `content/souls/fellowship/SOUL.md` is a verbatim copy of the current `content/templates/soul.md` (Warden/Fellowship lore). No content changes — preserves existing identity for upgrade compatibility.
- `content/souls/none/SOUL.md` is newly authored. Describes the orchestrator role and the four profile types (implementer, reviewer, researcher, librarian) purely functionally — same role boundaries and orchestration principles as `fellowship`, but with no character names, no races, no metaphor.

Each soul includes a one-line description (frontmatter `description:` field is fine) for `loaf soul list` to consume. The current `content/templates/soul.md` stays in place for now (TASK-133 wires the SessionStart hook to read from the catalog instead).

## Acceptance Criteria

- [ ] `content/souls/fellowship/SOUL.md` exists and is byte-identical to current `content/templates/soul.md`
- [ ] `content/souls/none/SOUL.md` exists and describes 5 roles (orchestrator + implementer/reviewer/researcher/librarian) functionally
- [ ] `content/souls/none/SOUL.md` contains zero occurrences of `Smith|Sentinel|Ranger|Dwarf|Elf|Human|Ent|Wizard|Warden|Fellowship` (case-insensitive)
- [ ] Both souls expose a one-line description (frontmatter or H1 tagline) consumable by `loaf soul list`

## Verification

```bash
test -f content/souls/fellowship/SOUL.md && \
test -f content/souls/none/SOUL.md && \
diff -q content/souls/fellowship/SOUL.md content/templates/soul.md && \
! grep -qiE '(smith|sentinel|ranger|dwarf|elf|wizard|warden|fellowship|\bent\b)' content/souls/none/SOUL.md
```

## Context

See SPEC-033 for full context. Foundation for TASK-130 (`loaf soul` command), TASK-131 (install integration), TASK-132 (profile neutralization), TASK-133 (SessionStart hook). Sets the foundation for a future https://soul.md-aligned standard.
