---
change: release-promotion-model
id: TASK-004
title: Designation-commit promotion mechanics
blocked-by:
  - TASK-001
  - TASK-008
blocks:
  - TASK-005
  - TASK-007
---

# TASK-004 — Designation-commit promotion mechanics

## Objective

`--promote release` runs the full promotion preflight and produces the designation commit path: locate the highest `vX.Y.Z-rc.N` tag for the candidate core (refuse when none exists — stable is a promoted rc), verify `git diff <last-rc-tag>..HEAD --name-only` stays within the release-metadata allowlist (version files, CHANGELOG.md, regenerated build outputs — see the Planning Contract), re-check the cohort gate, emit rollup seed material (the cycle's landed `release-notes.md` fragments per TASK-008's collection, `shape.md` Problem/Hypothesis lines as warned fallback, the cycle's prerelease sections), and validate the resulting CHANGELOG mechanically: `## [X.Y.Z]` present with items, the cycle's prerelease sections collapsed away.

## Scope boundaries

**In:** New promotion preflight beside `internal/cli/release_post_merge.go` / `release_dry_run.go`, seed-material emission, CHANGELOG validation helpers, tests with fixture repos.

**Out:** The gate function internals (TASK-003 — call it, don't change it); guardrails 5/6 for non-promotion channels (TASK-005) — except the CI-green-at-HEAD preflight helper, which this task owns as one shared check covering BOTH rc cut and promotion (Decision 11; TASK-005 consumes, never reimplements); the notes projection and changelog write path (TASK-008); skill ceremony prose (TASK-007); receipt freshness (predecessor Change).

## Context pointers

- Contract: `shape.md` — Planning Contract → Promotion mechanics; Decisions 1, 2, 3, 9; Rabbit Holes (rollup is judged, never generated).
- Changelog plumbing today: `release_dry_run.go:904-970` (read), `:1127-1166` (write); guardrail 6 extraction `release_post_merge.go:242-300`.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-004 — designation-commit promotion"
# Read internal/cli/release_post_merge.go, release_dry_run.go changelog sections, change_release_gate.go entry points.
```

## Steps

- [ ] Last-rc location by tag for the candidate core; refusal with a named reason when no rc exists.
- [ ] Designation diff check: the release-metadata allowlist since the last rc tag — importing receipt-tree-binding's `releaseMetadataAllowlist` specifically (the allowlist alone, never the full digest-exclusion set, which also masks receipts/ and reports/), never redefined here — else block listing offending paths with the "cut another rc" remedy.
- [ ] CI-green-at-HEAD assertion for rc cut and promotion: refuse to tag without a successful CI run at the exact HEAD SHA (`gh run list --commit <sha>`), blocking with a named remedy (Decision 11).
- [ ] Promotion preflight composes: designation check + cohort gate re-check + rollup seed emission (decide stdout block vs file here and record the choice in this file's Verification notes).
- [ ] Mechanical rollup validation: stable section present with items, cycle prerelease sections absent; wire into the promotion apply path and post-merge guardrail path for stable-prepared versions.
- [ ] Write the reaction artifact: a mock 2.0.0 rollup seeded from the live cohort, committed under `research/` for H4 judgment.
- [ ] Tests under a `TestReleaseDesignation` prefix: diff restriction (positive + offending-path negative), no-rc refusal, rollup validation positives and negatives, collapse detection against the defined collapse set (rung-labeled sections of the candidate core; dev/pre history untouched), CI-green-at-HEAD refusal (no successful run at HEAD → block with the named remedy).

## Verification

- `go test ./internal/cli -run 'TestReleaseDesignation' -count=1` green.
- The seed-emission choice is recorded here once made, resolving the shape.md open question.
