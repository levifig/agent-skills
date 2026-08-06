---
change: versioning-reset
id: TASK-004
title: Changelog renumbering
blocked-by:
  - TASK-001
---

# TASK-004 — Changelog renumbering

## Objective

CHANGELOG.md carries the whole history under the new scheme — `0.1.0` collapses the 1.x era, `dev.N → 0.1.N`, the four `pre.<timestamp>` builds → `0.1.50–0.1.53`, `alpha.N → 0.2.N` — and living docs cite the renumbered forms, while ADRs stay verbatim.

## Scope boundaries

**In:** `CHANGELOG.md` headings, version-carrying reference/link lines, the new `0.1.0` entry; citation renumbering in `docs/ARCHITECTURE.md:387`, `docs/STRATEGY.md:41,54`, `content/skills/release/SKILL.md:209` (and its built copies via rebuild); `target_release` retargets in `docs/changes/20260727-spec-conversion-and-guidance-sweep/change.json` and `docs/changes/20260728-receipt-tree-binding/change.json`.

**Out:** entry content (facts stay verbatim — Rabbit Holes), ADRs (append-only, keep original citations), `content/skills/git-workflow/references/commits.md` (generic SemVer knowledge, not Loaf's scheme).

## Context pointers

- Contract: `shape.md` — Decisions 6 and 12, Rabbit Holes, H1
- The map's edge: pre builds are positional (`0.1.50–53`, chronologically after `dev.49`), everything else numeral-preserving; dev-sequence gaps stay
- Retarget rule: each shipped Change points at the renumbered release it actually rode — determine by tag ancestry (`git tag --contains`) **before the wipe deletes the old tags** (open fog entry routed here)

## Acquisition

```bash
loaf journal log "skill(implement): TASK-004 — changelog renumbering per the reset map"
```

## Steps

- [ ] Renumber all CHANGELOG headings and version-carrying reference lines per the map; author the `0.1.0` entry summarizing the 1.x era (tag `v1.17.4`, predates this changelog)
- [ ] Resolve which release the spec-conversion sweep rode via tag ancestry; retarget both pinned `change.json` files to their renumbered releases
- [ ] Renumber the living-doc citations; rebuild so distributed skill copies follow
- [ ] Record the old→new heading map in the commit message body for H1 review

## Verification

- `grep -c "^## \[2\.0\.0" CHANGELOG.md` exits 1 (V5)
- `grep -c "^## \[0\.1\.0\]" CHANGELOG.md` prints 1 (V6)
- H1: reviewer confirms the 1:1 map and untouched entry content
