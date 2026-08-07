---
change: self-carried-release-guardrail
id: TASK-001
title: Relax the release-commit diff shape
---

# TASK-001 — Relax the release-commit diff shape

## Objective

Guardrail 5 accepts a changelog-only release commit when guardrail 4 has proven every version file already equals the candidate, refuses it when that proof is absent, and refuses a missing changelog either way — with the tracked artifacts rebuilt and the capability evidence re-recorded against the resulting binary so the 0.2.20 ceremony's evidence gate stays green.

## Scope boundaries

**In:** `checkReleasePostMergeDiffFiles` and its single call site in `internal/cli/release_post_merge.go`; the relaxation test matrix in `internal/cli/release_test.go`; the rebuilt `bin/`, `dist/`, `plugins/` trees; this Change's `research/` smoke receipts and the registry/test paths that read them; the `[Unreleased]` changelog line.

**Out:** every other guardrail; the evidence-only repair detection (`releaseIsEvidenceOnlyRepairCommit`) and the ref selection it drives; the 0.2.20 ceremony itself.

## Context pointers

- Contract: `shape.md` — Decisions 1–4, Planning Contract → Approach
- Guardrail 4 (the attestation's source): `internal/cli/release_post_merge.go:133-140`
- Guardrail 5: `internal/cli/release_post_merge.go:240-277`, called at `:147`
- Test harness: `seedReleasePostMergeFiles`, `releasePostMergeHappyResponses`, `scriptedReleasePostMergeRunner` in `internal/cli/release_test.go:1358-1441`
- Repair-commit refusals that must keep failing at guardrail 5: `internal/cli/release_evidence_gate_test.go:684-755` (they fail on the missing *changelog*, not the missing version file)
- Re-record precedent: `82bbd154`, and the sibling Change's TASK-002 close-out lesson (receipt last, content-free)

## Acquisition

```bash
loaf journal log "skill(implement): TASK-001 — relax post-merge guardrail 5 for self-carrying releases"
```

## Steps

- [x] Add the `versionFilesAtCandidate` attestation to `checkReleasePostMergeDiffFiles`, make the missing-version-file abort conditional on its absence, and pass guardrail 4's own comparison at the call site
- [x] Author the matrix: changelog-only accepted under attestation, changelog-only rejected without it, missing-changelog rejected under attestation, and a guardrail-level self-carrying release passing end to end while a candidate/version-file divergence still aborts at guardrail 4
- [x] Add the `[Unreleased]` changelog line
- [x] `npm run build`; commit the rebuilt trees
- [x] Re-record all three capability smokes against the rebuilt binary into this Change's `research/`, repoint `config/target-capabilities.json` and the two test files, and build nothing afterwards
- [x] After the final flip: run `loaf change verify docs/changes/20260807-self-carried-release-guardrail` and land the receipt as the branch's last, content-free commit

## Verification

- `go test ./internal/cli -run TestReleasePostMerge -count=1` exits 0 (V1)
- `npm run test` exits 0 (V2)
- `loaf change check docs/changes/20260807-self-carried-release-guardrail` reports state verified after the receipt commit
- H1: a reviewer confirms guardrails 1–4 and 6–9 are untouched and guardrail 5's changelog demand is still unconditional
