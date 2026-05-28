---
id: TASK-044
title: Build integration + end-to-end verification
spec: SPEC-013
status: done
priority: P2
created: '2026-03-27T02:59:49.324Z'
updated: '2026-03-27T11:06:31.452Z'
depends_on:
  - TASK-040
  - TASK-041
  - TASK-042
completed_at: '2026-03-27T11:06:31.451Z'
---

# TASK-044: Build integration + end-to-end verification

## Description

Final integration task: register the bootstrap skill in Loaf's build system and verify end-to-end.

**Files:** `config/hooks.yaml`, `config/targets.yaml`

**Steps:**
1. Register bootstrap skill in `hooks.yaml` plugin-groups (add to appropriate group or create one)
2. Add session template to `shared-templates` in `targets.yaml` if needed
3. `loaf build` for all targets — verify bootstrap skill in output
4. `loaf install --to all` — verify installation
5. Manual verification: `/bootstrap` appears in Claude Code `/` menu
6. Update SPEC-013 status from `approved` to `done`

## Acceptance Criteria

- [ ] Bootstrap skill registered in `hooks.yaml` plugin-groups
- [ ] `loaf build` succeeds for all targets (claude-code, opencode, cursor, codex, gemini)
- [ ] `loaf install --to all` succeeds
- [ ] `/bootstrap` appears in Claude Code skill list
- [ ] Interview guide and templates are bundled in built output
- [ ] SPEC-013 status updated to `done`

## Verification

```bash
loaf build
loaf install --to all
# Manual: open Claude Code, type /bootstrap, verify it appears
npm run typecheck
```

## Context
See SPEC-013 — full spec. Depends on TASK-040, TASK-041, TASK-042.
