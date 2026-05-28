---
id: TASK-092
title: Session management policy and rename nudges
spec: SPEC-027
status: done
priority: P2
created: '2026-04-07T10:11:05.268Z'
updated: '2026-04-07T10:35:08.881Z'
completed_at: '2026-04-07T10:35:08.881Z'
---

# TASK-092: Session management policy and rename nudges

## Description

Implement SPEC-027 Part D: document session management policy and add `/rename` prompt nudges.

**Policy:** Add compact-vs-new-session decision table to the orchestration sessions reference. Covers: same-scope continuation (compact), different scope (new session), finished spec (new session), context full mid-task (auto-compact), quick unrelated question (new session).

**Rename nudges:** After session/plan creation in `/implement`, generate a descriptive name suggestion: `Suggestion: /rename {SPEC-ID}-{slug}`. Session start output suggests rename when a spec is linked to the branch.

Depends on TASK-089 (rename suggestion uses spec info from session start infrastructure).

## Key Files

- `content/skills/orchestration/references/sessions.md` — add policy table
- `content/skills/implement/SKILL.md` — add rename nudge after session creation
- `cli/commands/session.ts` — add rename suggestion to session start output

## Acceptance Criteria

- [ ] Orchestration sessions reference includes compact-vs-new-session policy table
- [ ] `/implement` suggests `/rename` with meaningful name after session creation
- [ ] `loaf session start` output suggests `/rename` when spec is linked
- [ ] `loaf build` succeeds

## Verification

```bash
loaf build
```

## Context

See SPEC-027 Part D. The rename nudge is a prompt-level suggestion, not a programmatic rename — the user types `/rename` themselves.
