---
change: change-work-model
id: TASK-015
title: One candidate version for gate and executor
blocks:
  - TASK-016
---

# TASK-015 — One candidate version for gate and executor

## Objective

Close C3-1: with `--bump` omitted, preflight gates the *current* version's cohort while the executor independently derives `suggestReleaseBump(commits)` and cuts that version ungated. After this task, exactly one candidate computation exists, derived from the effective bump (flag or suggestion), and both the gate and the executor consume it — the default invocation gates the version it actually cuts.

## Scope boundaries

**In:** `computeReleaseCandidateVersion` in `internal/cli/change_release_gate.go` (including its dead prerelease conditional — both branches return `current`); the bump/candidate derivation order in `internal/cli/release_dry_run.go` (~:443); fixtures in `internal/cli/change_release_gate_test.go`.

**Out:** The structural tier (TASK-016 — lands after this in the same lane). The `--post-merge` finalization path's candidate (already stable-of-current; leave it). Receipt logic (verify lane owns `change_verify.go`). Do not touch `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, `package.json`, or any test file other than `change_release_gate_test.go`.

## Context pointers

- Round-3 board `reports/20260728-035602-review-implementation-round-3-codex.html`, finding C3-1 (verification transcript and fix shape).
- shape.md TASK-004: "compute the candidate version first and gate on it" — the contract already says this; the no-flag path just never joined.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-015 — one candidate for gate and executor"
```

## Steps

- [x] Derive the effective bump before preflight: `--bump` when given, else `suggestReleaseBump(commits)`; compute the candidate from it once and hand the same value to the cohort gate and the version executor.
- [x] Remove the dead conditional in `computeReleaseCandidateVersion`; prerelease *candidates* keep bypassing the cohort gate exactly as today.
- [x] Fixture: current `1.0.0`, incomplete `target_release: 1.1.0` cohort, one `feat` commit, **no** `--bump` flag — preflight blocks naming 1.1.0; with the cohort complete, the same invocation proceeds to 1.1.0.
- [x] Fixture: no-flag invocation whose suggested bump lands on a prerelease candidate still bypasses.

## Verification

- `go test ./internal/cli/ -run 'ReleaseCohortGate|ReleaseCandidate'` green including the new fixtures; existing explicit-bump fixtures untouched and green.
