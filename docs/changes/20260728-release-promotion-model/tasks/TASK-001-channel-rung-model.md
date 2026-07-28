---
change: release-promotion-model
id: TASK-001
title: Channel rung model in the version primitives
blocks:
  - TASK-002
  - TASK-003
  - TASK-004
  - TASK-006
---

# TASK-001 — Channel rung model in the version primitives

## Objective

The version primitives understand the ladder: a prerelease splits into label + numeric suffix, `alpha < beta < rc` orders rungs, and three new moves exist — cycle entry (core bump + `alpha.1`), rung advance (next or named rung, counter reset to `.1`), and promotion (rc → bare core). Forward-only is enforced at this layer: any computation whose result would sort at or below the current version's rung refuses with a named reason.

## Scope boundaries

**In:** `internal/cli/release_dry_run.go` version primitives (`releaseSemver`, `parseReleaseSemver`, `bumpReleaseVersion`, `releaseVersionIsPrerelease`) and new rung helpers beside them; their unit tests.

**Out:** Flag parsing, snapshot fields, help text (TASK-002); gate logic (TASK-003); promotion ceremony (TASK-004); anything under `.github/` or `content/`.

## Context pointers

- Contract: `shape.md` — Planning Contract → Rung model; Decisions 4, 5, 6, 8.
- Current primitives: `release_dry_run.go:865-902` (parse), `:827-863` (bump), `:876-879` (prerelease predicate).

## Acquisition

```bash
loaf journal log "skill(implement): TASK-001 — channel rung model"
# Read internal/cli/release_dry_run.go:820-910 and internal/cli/release_test.go before editing.
```

## Steps

- [ ] Add rung parsing: label + numeric suffix split at the last dot; recognized ladder alpha(1) < beta(2) < rc(3); unknown labels are non-rungs.
- [ ] Add the three moves: cycle entry, rung advance (next/named, `.1` reset), promotion (requires rc base, returns bare core); each refuses backward or invalid transitions with a named reason.
- [ ] Keep `--bump prerelease` iteration semantics byte-compatible for unknown labels (reset-to-`.1` behavior preserved); `releaseSemverLess` untouched.
- [ ] Unit tests under a `TestReleaseRung` prefix covering entry, iterate, advance, named skip, promote, refusal of every backward move, and unknown-label behavior.

## Verification

- `go test ./internal/cli -run 'TestReleaseRung' -count=1` green.
- `go test ./internal/cli -count=1` green (no regression in existing bump/parse tests).
