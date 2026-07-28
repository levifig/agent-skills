---
change: release-promotion-model
id: TASK-003
title: Cohort gate fires at rc cut
blocked-by:
  - TASK-001
---

# TASK-003 — Cohort gate fires at rc cut

## Objective

The cohort gate's trigger set becomes: candidate is stable (byte-equal cohort match, unchanged) OR the candidate's rung is rc, in which case the cohort is every change whose `target_release` equals the candidate's core literal. Alpha and beta candidates keep the valve semantics with warnings. Post-merge keys the same logic on the prepared version's rung: rc and stable gate, alpha and beta flow.

## Scope boundaries

**In:** `internal/cli/change_release_gate.go` (trigger set, cohort selection), the post-merge gate keying in the snapshot path, fixtures in `change_release_gate_test.go`.

**Out:** Rung math (TASK-001); promotion preflight and its re-check plumbing beyond reusing the gate function (TASK-004); guardrails 5/6 (TASK-005); receipt freshness semantics (predecessor Change — do not touch `change_verify.go` freshness logic).

## Context pointers

- Contract: `shape.md` — Planning Contract → Gate placement; Decisions 2, 5.
- Current bypass: `change_release_gate.go:43-48`; cohort selection `:50-57`; post-merge keying `:232-241`.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-003 — gate fires at rc"
# Read internal/cli/change_release_gate.go and the gate sections of change_release_gate_test.go.
```

## Steps

- [ ] Extend the trigger set: rc candidates gate the cohort of their core literal; alpha/beta return valve-with-warnings as today; stable path unchanged.
- [ ] Key post-merge gating on the prepared version's rung (rc and stable gate; alpha/beta flow) without altering post-merge's no-version-input contract.
- [ ] Fixtures for every rung: alpha flows, beta flows, rc blocks on an unverified cohort member and passes on a verified one, stable unchanged, direct stable bump still gates.

## Verification

- `go test ./internal/cli -run 'TestReleaseGateRC' -count=1` green.
- `go test ./internal/cli -run 'TestReleaseCohort|TestReleasePostMerge' -count=1` green (existing gate and post-merge suites unregressed).
