---
change: harness-neutral-skills
id: TASK-006
title: Ownership hardening and migration
blocked-by:
  - TASK-004
relates-to:
  - TASK-005
---

# TASK-006 — Ownership hardening and migration

## Objective

Retirement becomes digest-aware *before* any mass retirement is activated, and existing installs converge on the new naming without Loaf ever touching something it cannot prove it owns.

## Scope boundaries

**In:** Ownership checks in `internal/cli/install_deprecations.go`, migration of prior layouts across every path a harness scans, and un-managing entries whose digest no longer matches.

**Out:** Deleting or overwriting unowned content, under any circumstance. Reversing ADR-018's relocations — `amp-skills-to-agents-home` and `opencode-skills-to-agents-home` implemented a correct decision and are preserved, aged out only on their own established window.

**Lands atomically with TASK-005.** Ownership hardening must be in the same tree as the retirement entries it protects against.

## Context pointers

- Contract: `shape.md` — Decision 7, Planning Contract / Risks
- The hazard is concrete: retired-skill cleanup treats the mere existence of `SKILL.md` as sufficient proof of Loaf ownership and calls `os.RemoveAll` under `--yes` without consulting the digest manifest (`install_deprecations.go:192`–`227`), and the general ownership helper does the same (`install_deprecations.go:371`–`378`)
- A digest mismatch is ambiguous: `orchestration` is in Loaf's manifest but holds Orca's content, so "the user edited it" and "another vendor replaced it" are indistinguishable and both mean hands off
- Stale copies matter because harnesses scan several paths, and on Amp `~/.config/agents/skills` outranks the canonical store
- An open deferred intent covers dangling-symlink recovery diagnostics; resolve it here or re-defer it with a reason

## Acquisition

```bash
loaf journal log "skill(implement): TASK-006 — ownership hardening and migration"
```

## Steps

- [ ] Make retirement consult the managed-skills digest manifest before removing anything, and refuse on mismatch rather than proceeding
- [ ] Run the discovery smoke that answers whether Cursor and OpenCode deduplicate a skill found through several search paths, since the answer sets how aggressive migration must be
- [ ] Enumerate the paths migration sweeps from the harness search-path table, in precedence order, rather than from memory
- [ ] Retire Loaf's provable copies; un-manage rather than delete where ownership is not provable, and report the two distinctly
- [ ] Handle dangling symlinks at managed targets, or record the explicit deferral
- [ ] Emit a before-and-after tree-hash receipt for the migration as machine evidence
- [ ] Add `TestDestructiveMigrationPreservesUnowned`, exercising the real `--yes` path

## Verification

- `go test ./internal/cli/ -run TestDestructiveMigrationPreservesUnowned` passes, covering a foreign `orchestration/SKILL.md`, a manifest entry whose digest no longer matches, a dangling symlink, every prior skill home, and interruption followed by retry
- The migration receipt shows nothing unowned was modified
- On the dogfooding machine, `loaf upgrade` completes with no conflicts and no duplicate skill listings in any harness
