---
change: harness-neutral-skills
id: TASK-005
title: Prefix and retirement of unprefixed names
blocked-by:
  - TASK-004
blocks:
  - TASK-006
---

# TASK-005 — Prefix and retirement of unprefixed names

## Objective

Loaf's skills occupy `loaf-`-prefixed names in the canonical store, so sharing a directory with other vendors stops being a collision risk. The unprefixed names are retired through the deprecation manifest rather than silently abandoned.

## Scope boundaries

**In:** The prefix applied at install time to the canonical store, `retired_skills` entries covering the unprefixed names, and any internal reference that resolves a skill by directory name.

**Out:** Typed command surfaces, which are generated separately and keep their bare names — OpenCode's `/shape` comes from `command/shape.md`, Claude Code's from the plugin. Whether a given skill should exist at all, or under a different name, is the skills audit.

## Context pointers

- Contract: `shape.md` — Decision 4, Open Questions (retirement window)
- The ecosystem provides no namespacing, scoping, or collision mechanism; prefixing is the only available defense
- Orca currently holds `~/.agents/skills/orchestration`, which prefixing vacates rather than contests

## Acquisition

```bash
loaf journal log "skill(implement): TASK-005 — loaf- prefix and unprefixed-name retirement"
```

## Steps

- [ ] Apply the prefix to canonical skill directory names, and to the managed-skills manifest that tracks them
- [ ] Confirm generated command surfaces are unaffected, with a test if none covers it today
- [ ] Add `retired_skills` entries for the unprefixed names, and decide the window — the standard one-release rule was written for one or two entries, not 35, so justify whatever it becomes
- [ ] Verify Loaf no longer claims `orchestration`, and that Orca's copy is left entirely alone

## Verification

- Installing into a sandbox HOME produces only `loaf-`-prefixed Loaf directories in the canonical store
- A sandbox seeded with a foreign `orchestration` directory completes install with no conflict and no modification to it
- Typed commands still resolve under their bare names
