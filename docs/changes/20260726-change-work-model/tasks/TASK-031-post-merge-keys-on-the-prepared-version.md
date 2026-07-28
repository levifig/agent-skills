---
change: change-work-model
id: TASK-031
title: Post-merge keys on the prepared version
---

# TASK-031 — Post-merge keys on the prepared version

## Objective

Close O6-2 minimally, restoring the alpha train without foreclosing the parked promotion model (`INTENT-20260728-release-promotion-model-…`). Post-merge serves two flows and must key on the **prepared version** — the consistent version-file version at HEAD: a prepared prerelease publishes through the valve (tag the prepared version, no stable gate); a prepared stable finalizes (gate that version's cohort, then tag the prepared version). Guardrail 4 returns: the tag always equals the version files. Round 3's bypass stays closed because stable-prepared is the gated row.

## Scope boundaries

**In:** `resolveReleaseSnapshot`'s post-merge branch in `internal/cli/change_release_gate.go` (candidate = the prepared version; gate applies only when it is stable); `internal/cli/release_post_merge.go` (changelog guardrail, tag collision, and tag use the prepared version; guardrail 4 restored as tag-equals-version-files); the V2 clause and the TASK-004 unit parenthetical in `docs/changes/20260726-change-work-model/shape.md` (exact edits below); grep shape.md for any other `--post-merge` sentence that encodes the single-flow reading and align it; fixtures in `internal/cli/change_release_gate_test.go`.

**Out:** The promotion model (parked — do not add channels, promote verbs, or changelog rollup machinery). Pre-merge paths (untouched). Do not touch `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, `package.json`.

## Context pointers

- Round-6 board O6-2 (three break points, converse hazard, provenance chain) and the visual `reports/20260728-144746-visual-post-merge-two-flows.html` — the fix table is the spec.
- The defect's provenance: the TASK-024 packet sentence "the tagged version is the preflighted candidate" was wrong for prerelease-prepared flows; this packet supersedes it.

## Shape amendment (exact, owner-approved 2026-07-28)

In V2, replace:
> the same fixture's `--post-merge` finalization is blocked until the cohort completes.

with:
> the same fixture's `--post-merge` publishes the prepared prerelease through the valve and tags exactly the prepared version; a fixture whose version files prepare the stable version has its `--post-merge` blocked until the cohort completes and, once verified, tags exactly the prepared version.

In TASK-004's unit description, replace `finalization paths — \`--bump release\` and \`--post-merge\` — gate the stable target's cohort` with `finalization paths gate the stable target's cohort — \`--bump release\` always, \`--post-merge\` when the prepared version is stable`.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-031 — post-merge keys on the prepared version"
```

## Steps

- [x] Post-merge resolves the prepared version from the consistent version files at HEAD; prerelease prepared bypasses the cohort gate, stable prepared gates that version's cohort — both tag exactly the prepared version.
- [x] Guardrail 4 restored: the tag must equal the consistent version-file version; the changelog guardrail and tag-collision check key on the prepared version.
- [x] Shape.md amended exactly as above; no other contract text changes beyond aligned `--post-merge` sentences found by grep.
- [x] Fixtures: prepared `2.0.0-alpha.15` with an open 2.0.0 cohort publishes and tags `v2.0.0-alpha.15` (the alpha-train regression); prepared `2.0.0` with the cohort open blocks, and with the cohort verified tags `v2.0.0`; a stray stable changelog section with prerelease files cannot cause a stable tag (converse hazard); the round-5 fixtures that encoded the single-flow semantics are corrected, not deleted.

## Verification

- `go test ./internal/cli/ -run 'Release|PostMerge' -count=1` green including all four fixtures; `go test ./cmd/loaf/ -count=1` green (the public-binary post-merge expectations updated if they encoded the broken semantics).
