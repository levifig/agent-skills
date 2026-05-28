---
id: TASK-164
title: Codex review opt-in hook for /refactor-deepen
spec: SPEC-034
status: done
priority: P3
created: '2026-05-02T01:25:49.274Z'
updated: '2026-05-02T01:25:49.274Z'
depends_on:
  - TASK-159
completed_at: '2026-05-02T03:35:00.000Z'
---

# TASK-164: Codex review opt-in hook for /refactor-deepen

## Description

At the end of `/refactor-deepen`'s grilling loop (after the PLAN file is written), if the `codex` plugin is detected as installed, offer: *"Want a Codex review of this deepening before commit?"* The skill skips the offer cleanly when codex is absent. Plugin presence detection should reuse existing Loaf detection mechanics (check `cli/lib/detect/` for the pattern). If the user accepts, route via the existing `codex:rescue` agent. Opt-in only — never on by default per SPEC-034 no-go.

## File Hints

- `content/skills/refactor-deepen/SKILL.md` (extend with post-loop step)
- `cli/lib/detect/` (reuse existing plugin-detection helpers)
- Reference: `codex:rescue` agent invocation pattern (already used in `/codex:rescue` skill)

## Acceptance Criteria

- [ ] When `codex` plugin detected, skill offers Codex review at end of grilling loop (after plan file is written)
- [ ] Offer is opt-in only — user must affirm (not auto-fire)
- [ ] When `codex` plugin absent, skill terminates cleanly without mention of Codex
- [ ] If user accepts, codex review invocation goes via `codex:rescue` agent (or equivalent)
- [ ] Plugin detection reuses existing helper from `cli/lib/detect/` rather than re-implementing
- [ ] Skill prose explains the offer is plugin-gated and opt-in

## Verification

```bash
# Manual: invoke /refactor-deepen with codex plugin installed → offer fires after plan saved
# Manual: temporarily disable codex plugin, invoke /refactor-deepen → no Codex mention in skill output
```
