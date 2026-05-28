---
id: TASK-042
title: >-
  Bootstrap skill — Completeness (session, symlinks, KB, cross-harness, next
  steps)
spec: SPEC-013
status: done
priority: P2
created: '2026-03-27T02:59:47.066Z'
updated: '2026-03-27T11:04:13.212Z'
depends_on:
  - TASK-041
completed_at: '2026-03-27T11:04:13.211Z'
---

# TASK-042: Bootstrap skill — Completeness (session, symlinks, KB, cross-harness, next steps)

## Description

Extend the bootstrap SKILL.md (from TASK-041) with remaining sections to reach full SPEC-013 coverage.

**Files:** `content/skills/bootstrap/SKILL.md` (extend existing)

**Sections to add:**
- Session recording: save the bootstrap interview as `.agents/sessions/{timestamp}-bootstrap.md` preserving decision rationale
- Symlink auto-creation: `.claude/CLAUDE.md → .agents/AGENTS.md`, `./AGENTS.md → .agents/AGENTS.md`
- KB integration: delegate to `loaf kb init` if available, otherwise scaffold `docs/knowledge/` with README
- Cross-harness fallback: Bash-based alternative instructions for non-Claude Code harnesses
- Pipeline exit: suggest `/brainstorm`, `/idea`, `/research`, `/shape`, and `loaf doctor`

## Acceptance Criteria

- [ ] Session recording section describes bootstrap session template and what to capture
- [ ] Symlink section creates both conventional symlinks without prompting
- [ ] KB integration gracefully degrades when `loaf kb init` unavailable
- [ ] Cross-harness section documents Bash-based equivalent workflow
- [ ] Pipeline exit suggests at least 2 workflow skills + `loaf doctor`
- [ ] SKILL.md covers all 10 steps from SPEC-013 skill flow
- [ ] `loaf build` succeeds

## Verification

```bash
loaf build --target claude-code
# Verify all 10 skill flow steps are present in output
```

## Context
See SPEC-013 — Session Recording, Cross-Harness Support sections. Depends on TASK-041.
