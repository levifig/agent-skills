---
change: versioning-reset
id: TASK-003
title: Release ceremony guardrail
blocked-by:
  - TASK-001
---

# TASK-003 — Release ceremony guardrail

## Objective

The release pipeline refuses its full ceremony for a timestamp-magnitude patch version, at the single derivation all consumers read and in CI, while anything cheaper (commits, lightweight tags, prerelease-marked uploads) stays allowed.

## Scope boundaries

**In:** `internal/cli/change_release_gate.go` (`resolveReleaseSnapshot`), `.github/workflows/release.yml` (early-skip step), the two scheme-pinned tests (`cmd/loaf/content_hygiene_test.go:316-317`, `cmd/loaf/native_cutover_files_test.go:275,315-318`), new Go test `TestReleaseSnapshotRefusesTimestampPatch`.

**Out:** the commit-subject regex (`check.go:699` — `\d+` matching a timestamp is fine; the guardrail is upstream), dev stamping itself (TASK-002).

## Context pointers

- Contract: `shape.md` — Decisions 2 and 10, Observable Workflow (refusal message), H3
- Chokepoint: `internal/cli/change_release_gate.go:200`; candidate minting `internal/cli/release_dry_run.go:968`
- CI trigger: `.github/workflows/release.yml` fires on any `v*` tag push — without a skip, a pushed timestamp tag would fail noisily on the tag/version equality check rather than skipping cleanly
- Pinned tests: `content_hygiene_test.go` requires the literal workflow_dispatch example (`v2.0.0` form) in release.yml; `native_cutover_files_test.go` pins a `2.0.0-alpha.4` formula fixture

## Acquisition

```bash
loaf journal log "skill(implement): TASK-003 — release ceremony guardrail for timestamp versions"
```

## Steps

- [ ] Refuse timestamp-magnitude candidates in `resolveReleaseSnapshot` with a message naming the guardrail and pointing at plain `0.X.X`; author `TestReleaseSnapshotRefusesTimestampPatch` proving dry-run, apply, and post-merge are all covered
- [ ] Add the release.yml early-skip: timestamp-magnitude tag ⇒ exit the workflow cleanly before packaging; update the workflow_dispatch example to a `0.x` form
- [ ] Update `content_hygiene_test.go` to require the new example text; renumber the `native_cutover_files_test.go` formula fixture to a plain-version form consistent with the new scheme
- [ ] Suite green

## Verification

- `go test ./internal/cli -run TestReleaseSnapshotRefusesTimestampPatch -count=1` exits 0 (V4)
- `npm run test` exits 0 (V3)
- H3: reviewer walks the release.yml skip path for a timestamp tag and the proceed path for a plain tag
