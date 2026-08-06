---
change: versioning-reset
id: TASK-006
title: Release and tag wipe
blocked-by:
  - TASK-001
  - TASK-002
  - TASK-003
  - TASK-004
  - TASK-005
---

# TASK-006 — Release and tag wipe

## Objective

The Change is release-ready at merge (target stamped, changelog curated, capability evidence fresh), and the post-merge runbook below takes 0.2.20 live, verifies Homebrew, then wipes the pre-reset tag surface and plants the era marker.

## Scope boundaries

**In:** this Change's `change.json` (`target_release`), `CHANGELOG.md` `[Unreleased]` curation, capability-evidence re-recording, and the post-merge operational acts.

**Out:** any repository code (all landed by TASK-001–005); branch/worktree housekeeping (out of scope for the Change).

## Context pointers

- Contract: `shape.md` — Decisions 4, 5, 7, 11; Risks (irreversible wipe, stale receipts); Definition of Done
- Evidence gate: `internal/cli/release_dry_run.go:504-516` — Go changes in this PR stale the binary-pinned smoke receipts; re-record against the rebuilt binary (the alpha.16 lesson)
- Ceremony: `loaf release --post-merge` keys `Candidate = current` by equality (`internal/cli/change_release_gate.go:241`); CI packages on the tag push and pushes `Formula/loaf.rb` to `levifig/homebrew-tap` (`.github/workflows/release.yml:100-130`)
- The stable-candidate cohort gate hard-gates Changes whose `target_release` byte-matches `0.2.20` — this Change must be complete (all boxes flipped) before the ceremony runs; that is why the runbook is prose, not checkboxes

## Acquisition

```bash
loaf journal log "skill(implement): TASK-006 — 0.2.20 release readiness and wipe runbook"
```

## Steps

- [x] Stamp `target_release: "0.2.20"` in this Change's `change.json`
- [x] Curate `[Unreleased]` so the ceremony's `0.2.20` section tells the reset story (scheme, renumber map pointer, forced-downgrade note instructing `loaf upgrade` after install)
- [x] `loaf release --dry-run --base HEAD` passes its gates against the committed `loaf change verify` receipt (bare `--dry-run` derives a 0.3.0 candidate from conventional commits; `--base HEAD` is the invocation that names 0.2.20 pre-merge)
- [x] Record the full remote tag/Release inventory (`git ls-remote --tags origin`, `gh release list`) into `research/` as the pre-wipe record

## Post-merge runbook

Ordered, irreversible steps last — run after the PR merges, verified by the post-ceremony confirmations (H4–H6):

1. `loaf release --post-merge` on main — tag `v0.2.20`, draft Release; CI packages, uploads, pushes the Homebrew formula.
2. Publish the Release; verify `brew install levifig/tap/loaf` serves 0.2.20; run `loaf upgrade` on the canary machine (markers rewrite; drift returns to `current`).
3. Only now: delete all pre-reset GitHub Releases and remote tags (43 Releases; every `v2.0.0-*` and `v1.17.4`), from the recorded inventory, via `gh api` with pagination.
4. Plant lightweight `v0.1.0` at the commit `v1.17.4` pointed to; push the tag. The push triggers the release workflow, which fails its tag/version equality check by design — a red run with no side effects (nothing before that check mutates anything); cancel or ignore it. The marker must not carry a packaged Release per Decision 7.
5. Run the post-ceremony confirmations (H4–H6); log the wrap entries.

## Verification

- Pre-merge: the `loaf change verify` receipt (V1–V7) is committed; `loaf release --dry-run --base HEAD` passes its gates; `loaf change check` is green with every Steps box (all six tasks) flipped by the readiness commit
- Post-merge: H4 (`gh release view v0.2.20`), H5 (no `v1.*`/`v2.*` refs on remote), H6 (`v0.1.0` present)
