---
id: TASK-132
title: Neutralize agent profile prompts and add SOUL.md-read directive
spec: SPEC-033
status: done
priority: P1
created: '2026-04-28T22:39:12.553Z'
updated: '2026-04-28T23:05:49.914Z'
depends_on:
  - TASK-129
completed_at: '2026-04-28T23:05:49.914Z'
---

# TASK-132: Neutralize agent profile prompts and add SOUL.md-read directive

## Description

Rewrite the four profile prompts at `content/agents/{implementer,reviewer,researcher,librarian}.md`:

1. Strip all `Smith|Sentinel|Ranger|Dwarf|Elf|Human|Ent|Wizard|Warden|Fellowship` vocabulary.
2. State the functional contract directly: *"You are an implementer. Full write access to the codebase: code, tests, configuration, and documentation all pass through your hands..."*
3. Add a Critical Rule at the top of each profile: *"Your first action MUST be to Read `.agents/SOUL.md` and internalize the character described there as your identity. If `.agents/SOUL.md` is missing, proceed with your functional role only — you lose personality, not capability."*

The functional contract (tool boundaries, what they do, what they don't do) must remain intact and unambiguous even if SOUL.md is missing — profiles must not depend on SOUL.md to operate. `loaf build` must succeed for all 6 targets after the rewrites.

## Acceptance Criteria

- [ ] `content/agents/{implementer,reviewer,researcher,librarian}.md` contain zero matches for `Smith|Sentinel|Ranger|Dwarf|Elf|Human|Ent|Wizard|Warden|Fellowship` (case-insensitive, with `\bent\b` to avoid matching "consequent" etc.)
- [ ] Each profile begins with a Critical Rule referencing the `.agents/SOUL.md` read directive
- [ ] Each profile retains its functional contract (tool access, scope, what-it-doesn't-do) without lore framing
- [ ] `loaf build` succeeds for all 6 targets

## Verification

```bash
! grep -iE '(smith|sentinel|ranger|dwarf|elf|wizard|warden|fellowship|\bent\b)' \
  content/agents/{implementer,reviewer,researcher,librarian}.md && \
grep -l "SOUL.md" content/agents/{implementer,reviewer,researcher,librarian}.md | wc -l | grep -q 4 && \
loaf build
```

## Context

See SPEC-033 test conditions T8, T9, T12. Depends on TASK-129 only (catalog files must exist so the SOUL.md-read directive points to something real on a fresh install). Same-pattern as the existing "skills self-log to journal first" Critical Rule — proven primitive.
