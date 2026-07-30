---
change: release-evidence-gate
id: TASK-001
title: Evidence-freshness gate in the release flow
---

# TASK-001 — Evidence-freshness gate in the release flow

## Objective

`loaf release` refuses to create a release commit or tag when any SHA-pinned capability receipt is stale against the just-rebuilt tree, on all three paths (`--pre-merge`, direct, `--post-merge`), with remediation copy that names the runner for the stale target. Projects without capability-evidence config are untouched.

## Scope boundaries

**In:** `internal/cli/release*.go` (gate function + call sites), reuse of the evidence loader in `internal/cli/target_capability_contract.go`, gate tests alongside the existing release tests.

**Out:** evidence schema or binding changes (Intent-tracked), any new CLI subcommand or hook registration, skill documentation (TASK-002), the full-suite run.

## Context pointers

- Contract: `shape.md` — Decisions 1–4, Planning Contract → Placement / Failure copy / Testing approach
- Loader: `internal/cli/target_capability_contract.go` (`LoadTargetCapabilityEvidence`, artifact-hash validation)
- House refusal style: `internal/cli/release_flow_advisory.go`
- Runner shapes for remediation copy: `cli/scripts/smoke-{claude-code,codex,opencode}-*.mjs` invocations recorded in the alpha.16/alpha.17 repair journal entries

## Acquisition

```bash
loaf journal log "skill(implement): TASK-001 — evidence-freshness gate in loaf release"
# Load: internal/cli/release*.go, internal/cli/target_capability_contract.go (loader entry points),
# existing release tests for fixture patterns
```

## Steps

- [x] Write failing tests: stale receipt refused on the apply executor (covers `--pre-merge` + direct) and as post-merge guardrail 9; fresh receipts pass; absent config no-ops; refusal asserts no commit/tag was created; copy lists the three runners and the ordering rule
- [x] Gate function: presence-probe `TargetCapabilityEvidenceRecordPath` under the release root, then `LoadTargetCapabilityEvidence` in-process (first production caller); classify absent vs invalid
- [x] Wire call sites per Planning Contract → Placement: third sibling refusal in `runReleaseApply` after the artifact loop (release_dry_run.go:491) before `git add -A`; guardrail 9 in `checkReleasePostMergeGuardrails`
- [x] Refusal copy per register: `Refusing to commit release artifacts: …` on apply; lowercase guardrail message on post-merge; static remediation block (three runner invocations + re-record-after-rebuild rule)
- [x] `npm run build` and commit any rebuilt tracked artifacts with the source change

Notes: the gate refuses on any `LoadTargetCapabilityEvidence` failure, not only hash drift — an invalid evidence file blocks a release equally; copy says "invalid or stale". For stale-receipt fixtures, copy the repo's real evidence file + receipts into the temp root and perturb a pinned artifact (the `build_test.go:981` pattern) instead of hand-building schema-valid JSON.

## Verification

- `go test ./internal/cli -run TestRelease` — exit 0
- `go test ./internal/cli -run TestTargetCapabilityEvidence` — exit 0 (gate precondition holds in-tree)
- `npm run typecheck` and `npm run test` — exit 0
