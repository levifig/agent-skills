---
change: harness-neutral-skills
id: TASK-006
title: Migration and symlink-aware ownership
blocked-by:
  - TASK-005
---

# TASK-006 — Migration and symlink-aware ownership

## Objective

Existing installs converge on the new layout without Loaf ever touching something it cannot prove it owns. Stale Loaf copies are retired from every path a harness scans; foreign and vendor-replaced directories are left alone and un-managed.

## Scope boundaries

**In:** Migration of prior layouts, the ownership model, un-managing entries whose content no longer matches, and the `amp-skills-to-agents-home` and `opencode-skills-to-agents-home` relocations this Change reverses.

**Out:** Any path outside the harness search paths this Change enumerates. Deleting or overwriting unowned content, under any circumstance.

## Context pointers

- Contract: `shape.md` — Decisions 1 and 5, Planning Contract / Risks, Rabbit Holes and No-Gos
- A hash mismatch is ambiguous: `orchestration` is in Loaf's manifest but holds Orca's content, so "modified" and "replaced by another vendor" are indistinguishable and both mean hands off
- Open deferred intent on dangling-symlink recovery diagnostics should be resolved here or explicitly re-deferred with a reason
- The stale-copy risk is concrete: Cursor scans `~/.agents/skills` and `~/.cursor/skills`; OpenCode scans `~/.agents/skills`, `~/.config/opencode/skills`, and `~/.claude/skills`

## Acquisition

```bash
loaf journal log "skill(implement): TASK-006 — migration and symlink-aware ownership"
```

## Steps

- [ ] Run the blindspot pass on harness discovery: do harnesses scanning several paths deduplicate a skill found in more than one, or list it twice? The answer sets how aggressive migration must be
- [ ] Enumerate the paths migration must sweep, derived from the harness search-path table rather than from memory
- [ ] Retire Loaf's own stale copies where ownership is provable; un-manage rather than delete where it is not, and report both distinctly
- [ ] Make ownership symlink-aware, so a link into the canonical store is recognised as owned without re-hashing
- [ ] Handle dangling symlinks at managed targets, or record the explicit deferral
- [ ] Retire the two relocations this Change reverses
- [ ] Add `TestMigrationPreservesUnownedSkills`

## Verification

- `go test ./internal/cli/ -run TestMigrationPreservesUnownedSkills` passes
- A sandbox seeded with foreign skills, a vendor-replaced managed entry, and a dangling symlink migrates without touching any of the three
- On the dogfooding machine, `loaf upgrade` completes with no conflicts and no duplicate skill listings in any harness
