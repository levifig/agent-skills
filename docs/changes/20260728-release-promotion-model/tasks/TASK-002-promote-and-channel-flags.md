---
change: release-promotion-model
id: TASK-002
title: Promote and channel flags on the release surface
blocked-by:
  - TASK-001
blocks:
  - TASK-005
  - TASK-007
  - TASK-008
---

# TASK-002 — Promote and channel flags on the release surface

## Objective

`loaf release` speaks the settled surface: `--promote [alpha|beta|rc|release]` (bare walks one rung; a named rung is a warned skip; `release` explicit and rc-only), `--channel <rung>` as a post-condition assertion validated after candidate computation and before any mutation (required only at cycle entry), and `--bump release` retired with an error pointing at `--promote release`. All three help surfaces agree and the stale lineage-freeze text is gone.

## Scope boundaries

**In:** Flag parsing and validation in `internal/cli/release_dry_run.go:95-202`, snapshot fields (`releaseSnapshot` at `release_dry_run.go:40`; `resolveReleaseSnapshot` at `change_release_gate.go:199` — a file TASK-003 also claims: this task touches only that function's move wiring, TASK-003 owns the gate trigger set), help text in `internal/cli/release.go`, `internal/cli/agent_help.go`, `internal/cli/cli_reference.go`; tests.

**Out:** Rung math itself (TASK-001); gate trigger changes (TASK-003); promotion preflight/ceremony beyond flag routing (TASK-004); guardrails (TASK-005); skill prose (TASK-007).

## Context pointers

- Contract: `shape.md` — Planning Contract → Surface and snapshot; Decisions 4, 5.
- Flag matrix today: `release_dry_run.go:95-202`; post-merge exclusivity `:163-200`.
- Help drift: `cli_reference.go:90` still says "during a lineage freeze"; `release.go:99` and `agent_help.go:253` differ.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-002 — promote and channel flags"
# Read internal/cli/release_dry_run.go:40-210, release.go, agent_help.go:240-270, cli_reference.go:80-110.
```

## Steps

- [ ] Parse `--promote` with optional rung/`release` value and `--channel <rung>`; extend `releaseSnapshot` with the move kind and asserted channel; `--promote` mutually exclusive with `--bump`, composes with `--dry-run`/`--yes`/`--base`/`--tag`/`--no-gh`; `--post-merge` exclusivity unchanged.
- [ ] Wire moves through `resolveReleaseSnapshot` using the TASK-001 primitives; `--channel` assertion checked against the computed candidate before any mutation; cycle entry requires a `--bump major|minor|patch --channel alpha` pair from a stable base.
- [ ] Add `--promote` and `--channel` to `--post-merge`'s incompatibility list (`release_dry_run.go:163-200`) alongside `--bump` — post-merge takes no version-shaping input; rejection test included.
- [ ] Retire `--bump release`: error with pointer to `--promote release`; remove it from the valid-bump set and every help surface.
- [ ] Rewrite the three help surfaces together — flags, moves, one shared vocabulary; delete the lineage-freeze sentence.
- [ ] Tests under a `TestReleasePromote` prefix: bare walk, named skip warning, `release` from non-rc refused, `--channel` mismatch refused pre-mutation, `--bump release` pointer error, flag-matrix rejections.

## Verification

- `go test ./internal/cli -run 'TestReleasePromote' -count=1` green.
- `grep -rn "lineage freeze" internal/cli/` returns nothing; `grep -n "bump release" internal/cli/release.go internal/cli/agent_help.go internal/cli/cli_reference.go` shows only the retirement error text.
