---
change: change-work-model
id: TASK-020
title: Thread the candidate value from preflight to executor
---

# TASK-020 — Thread the candidate value from preflight to executor

## Objective

Close C4-1: `runRelease` computes the candidate at `release.go:19`, and then dry-run and apply each call `computeReleaseCandidateVersion` again (`release_dry_run.go:283, :445`) — same function, freshly re-derived, each re-reading `git log`. A commit landing between derivations still splits gate from executor: the TOCTOU residue of C3-1. After this task the candidate is derived exactly once per invocation and every consumer receives that value.

## Scope boundaries

**In:** `internal/cli/release.go`, `internal/cli/release_dry_run.go`, `internal/cli/change_release_gate.go` (plumbing only — the derivation logic itself is settled); fixtures in `internal/cli/change_release_gate_test.go`.

**Out:** The bump-resolution semantics (settled by TASK-015 — flag wins, else suggestion; zero-commit and prerelease behavior unchanged). `--post-merge`'s own candidate path. Do not touch `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, `package.json`, or `docs/changes/20260710-*`.

## Context pointers

- Round-4 board `reports/20260728-125334-review-implementation-round-4-codex.html`, finding C4-1 (verification and fix shape).
- TASK-015's promise this completes: "compute the candidate from it once and hand the same value to the cohort gate and the version executor."

## Acquisition

```bash
loaf journal log "skill(implement): TASK-020 — thread the candidate value"
```

## Steps

- [ ] Derive the candidate once in `runRelease` (and once in the `--post-merge` entry if it flows through the same path) and pass the value into the dry-run and apply paths — a parameter or a resolved field on the options struct, not a re-derivation.
- [ ] The displayed bump label derives from the same single resolution, so preview text and gated candidate can never disagree.
- [ ] Fixture proving the thread: preflight derives the candidate, the test lands a new `feat:` commit via the seam before the executor path runs, and the executor still cuts preflight's candidate — asserted against the value, not the call count.

## Verification

- `go test ./internal/cli/ -run 'ReleaseCohortGate|ReleaseCandidate|ReleaseDryRun'` green including the divergence fixture; existing candidate fixtures untouched and green.
