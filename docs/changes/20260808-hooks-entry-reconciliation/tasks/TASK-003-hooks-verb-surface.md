---
change: hooks-entry-reconciliation
id: TASK-003
title: loaf hooks verb surface
blocked-by:
  - TASK-001
  - TASK-002
---

# TASK-003 — `loaf hooks` verb surface

## Objective

`loaf hooks list`, `loaf hooks enable <hook-id> --target <t>`, and `loaf hooks disable <hook-id> --target <t>` operate the enablement records and immediately reproject the affected file, making the verb the one discoverable disable/re-enable ceremony.

## Scope boundaries

**In:** `internal/cli/hooks.go`, dispatch registration in `cli.go`, consumption of the TASK-002 hook catalog, CLI reference regeneration, tests.

**Out:** Reconciliation semantics and catalog emission (TASK-002); any enablement scoping beyond user × target × host; TUI or interactive modes.

## Context pointers

- Contract: `shape.md` — Observable Workflow; Decisions 4, 5, 10; Planning Contract → Hook catalog, Placement.
- Command-surface conventions: an existing `runX` command in `internal/cli` for output and JSON formatting patterns.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-003 — loaf hooks list/enable/disable"
# Read cli.go dispatch and one existing multi-subcommand runX before adding runHooks.
```

## Steps

- [ ] `list`: every catalog hook for installed targets with event, effective enablement, projected-in-file state, and absorption provenance when present; tombstoned (retired) records are not listed.
- [ ] `enable`/`disable` run entirely inside the per-target lock, in Decision 10's order: acquire lock → read state → upsert the record → run the full TASK-002 reconcile → publish the file → release. Report every action taken — one entry on a converged target, plus any other drift the reconcile converged.
- [ ] Tests: round-trip enable/disable on a converged target edits exactly one entry; on a target with seeded drift the output names every action; list agrees with file state and records; `absorbed_at` survives toggles; unknown hook-id and uninstalled target fail with actionable errors.

## Verification

- `go test ./internal/cli/...` passes.
- Manual round-trip on a temp target home shows one-entry diffs in both directions.
