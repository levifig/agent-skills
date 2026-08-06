---
change: versioning-reset
id: TASK-005
title: Versioning-policy ADR
blocked-by:
  - TASK-002
  - TASK-003
---

# TASK-005 — Versioning-policy ADR

## Objective

`docs/decisions/ADR-026-major-zero-versioning.md` records the policy as a dated decision: plain `0.X.X` releases, patch-slot timestamp dev identity, the ceremony-not-visibility guardrail, the renumbered-history map (the translation key for every old citation), and the 0.3.0 / 1.0.0 milestone semantics.

## Scope boundaries

**In:** the one new ADR file.

**Out:** editing any existing ADR (append-only log; Decision 8 — nothing is superseded), README/installation docs (touched only if they cite a version, which they do not per the inventory).

## Context pointers

- Contract: `shape.md` — Decisions 1, 2, 6, 8, 9; Durable Outputs
- Format: `loaf:architecture` skill conventions — Decision Date, status Accepted, alternatives considered (prerelease-suffix dev identity and the fourth version part, both rejected in Cut with reasons)
- Written after TASK-002/003 so it documents proven behavior, not intent

## Acquisition

```bash
loaf journal log "skill(implement): TASK-005 — ADR-026 major-zero versioning policy"
```

## Steps

- [x] Author ADR-026 covering scheme, dev identity, guardrail, renumber map, milestone semantics, and the rejected alternatives with rationale
- [x] Cross-link: ADR notes that pre-reset ADRs cite old-scheme versions and carries the translation map

## Verification

- `test -f docs/decisions/ADR-026-major-zero-versioning.md` exits 0 (V7)
- ADR review against the `documentation-standards` ADR checklist
