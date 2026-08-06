---
change: versioning-reset
id: TASK-002
title: Dev-build identity
blocked-by:
  - TASK-001
---

# TASK-002 — Dev-build identity

## Objective

A local (non-CI) binary build reports `0.2.{unix_timestamp}`; a shared timestamp-magnitude predicate names dev identity in one place; harness-drift classification stops giving inverted advice when a dev binary or a version downgrade is on either side.

## Scope boundaries

**In:** `cli/scripts/go-build-flags.mjs` (or the equivalent build-metadata carrier), `internal/cli/version.go`, `internal/cli/harness_drift.go`, new Go tests.

**Out:** tracked content artifacts — they always carry the release version (Decision 9, Cut list); release-pipeline refusal (TASK-003).

## Context pointers

- Contract: `shape.md` — Approach (dev identity paragraph), Decisions 1 and 9, Open Questions (injection mechanism is this task's call)
- Existing signal: `cli/scripts/go-build-flags.mjs:15-29` injects build metadata only when `LOAF_BUILD_COMMIT`/`LOAF_BUILD_DATE` are set — only CI sets them (`.github/workflows/release.yml:81-82`)
- Report surface: `internal/cli/version.go:30,70-84,119`
- Inverted surface: `internal/cli/harness_drift.go:63,117-131` (advice), `:137-147` (nudge stays silent on `binary-stale`)
- Marker write-back: `internal/cli/install_target.go:1532`

## Acquisition

```bash
loaf journal log "skill(implement): TASK-002 — dev-build identity and drift-classifier fix"
```

## Steps

- [ ] Mint `0.2.{unix_timestamp}` at build time for non-CI binary builds, using the CI-env absence signal; decide and document the carrier (ldflags variable or equivalent)
- [ ] Teach the version report to prefer the dev stamp when present; release builds keep the runtime `package.json` read
- [ ] Add the shared dev-identity predicate (timestamp-magnitude patch) where version code lives; use it in the drift classifier so a dev binary against release-marked content classifies and advises correctly
- [ ] Correct `doctorDetailLine` advice so a higher marker no longer unconditionally means "binary is stale"; cover the `2.0.0-alpha.19` marker → `0.2.20` binary transit in a test
- [ ] Go tests for predicate, stamping selection, and drift classification across dev/release/downgrade combinations

## Verification

- `npm run build:go && ./bin/loaf --version` prints a `0.2.` patch of timestamp magnitude (H2 confirms interactively)
- New Go tests pass within `npm run test` (V3)
- Drift-classification tests cover: dev binary vs release marker, release binary vs dev marker, and the alpha→0.2.20 transit
