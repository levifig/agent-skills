---
change: harness-neutral-skills
id: TASK-006
title: Ownership hardening and migration
blocked-by:
  - TASK-004
---

# TASK-006 — Ownership hardening and migration

## Objective

Retirement becomes digest-aware *before* any mass retirement is activated, and existing installs converge on the new naming without Loaf ever touching something it cannot prove it owns.

## Scope boundaries

**In:** Ownership checks in `internal/cli/install_deprecations.go`, migration of prior layouts across every path a harness scans, and un-managing entries whose digest no longer matches.

**Out:** Deleting or overwriting unowned content, under any circumstance. Reversing ADR-018's relocations — `amp-skills-to-agents-home` and `opencode-skills-to-agents-home` implemented a correct decision and are preserved, aged out only on their own established window.

**Lands independently.** Shaping paired this with a mass retirement of unprefixed names; Decision 4 dropped the prefix, so there are no such entries. The hazard survives without them — four directories Loaf does not own sit in the shared store today, and the existing relocation entries already walk prior skill homes.

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

- [x] Make retirement consult the managed-skills digest manifest before removing anything, and refuse on mismatch rather than proceeding
- [ ] Run the discovery smoke that answers whether Cursor and OpenCode deduplicate a skill found through several search paths, since the answer sets how aggressive migration must be. The same smoke answers a question TASK-004 left open and documentation could not settle: whether Cursor and Amp tolerate the OpenCode-owned frontmatter keys (`subtask`, `user-invocable`) that reach them through the single canonical copy
- [x] Enumerate the paths migration sweeps from the harness search-path table, in precedence order, rather than from memory
- [x] Retire Loaf's provable copies; un-manage rather than delete where ownership is not provable, and report the two distinctly
- [x] Handle dangling symlinks at managed targets, or record the explicit deferral
- [x] Emit a before-and-after tree-hash receipt for the migration as machine evidence
- [x] Add `TestDestructiveMigrationPreservesUnowned`, exercising the real `--yes` path

## Verification

- `go test ./internal/cli/ -run TestDestructiveMigrationPreservesUnowned` passes, covering a foreign `orchestration/SKILL.md`, a manifest entry whose digest no longer matches, a dangling symlink, every prior skill home, and interruption followed by retry
- The migration receipt shows nothing unowned was modified
- On the dogfooding machine, `loaf upgrade` completes with no conflicts and no duplicate skill listings in any harness

## Landed state

Delivered at `d37297d7` after five adversarial review passes. Every data-loss finding is closed: nothing is deleted, overwritten, or moved without a digest-proven claim, deliberate refusals preserve and report rather than aborting, and real I/O errors surface instead of being flattened into "absent".

Two items are deliberately open.

**The discovery smoke is unrun.** It needs real harnesses rather than fixtures, so it is the operator's to run, not an implementer's. Its absence is why migration is conservative rather than aggressive — the conservatism is the mitigation, not a gap.

**One refusal path is knowingly incomplete.** When an ancestor of the relocation destination is itself a regular file — `${HOME}/.agents` rather than `${HOME}/.agents/skills` — the destructive receipt hash runs before any refusal classification, and it treats only `ENOENT` as non-fatal, so the upgrade aborts instead of emitting an actionable `unmanaged` report. The source is preserved either way, which makes this a wrong refusal rather than a wrong deletion. The classification is already correct in `isDeliberateMigrationDestinationPathError`; it is simply not consulted on the root probe and hash paths.
