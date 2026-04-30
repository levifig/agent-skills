---
id: ADR-012
title: CI as verifier, not fixer (build artifacts)
status: Accepted
date: 2026-04-30
---

# ADR-012: CI as Verifier, Not Fixer (Build Artifacts)

## Decision

Tracked build artifacts (`dist/`, `plugins/`, `.claude-plugin/`) are committed locally alongside the source changes that produced them. CI's job is to **verify** that committed artifacts match what `npm run build` produces from source — not to **rebuild and push** them.

The `Build Distributions` workflow runs on `push` to `main` AND `pull_request` (catches drift at PR review, not only post-merge), runs `npm ci` + `npm run build`, then `git diff --quiet -- dist/ plugins/ .claude-plugin/`. If dirty: print diff stat + first 200 lines of diff with an actionable message ("Run `npm run build` locally and commit alongside source") and fail the run. No write permissions; no auto-commit; no auto-push.

## Context

The prior workflow ran on `push` to `main`, ran `npm run build`, then `git add` + `git commit` + `git push origin main` to publish freshly-built artifacts as a follow-up commit. Branch protection on `main` (PR-required + signature-required) rejected the auto-push with `GH013` every single run, leaving the CI failed and creating an ambiguous "is the failure real or just the push step?" signal.

Local discipline was already authoritative: CLAUDE.md says *"If tracked build artifacts in `dist/` or `plugins/` changed, commit them with the source changes that produced them."* The CI auto-push was redundant with that convention — and now actively conflicting with branch protection.

The convention had two reasonable evolutionary paths:

1. **Keep CI as the publisher** — disable branch protection for `main` (or grant the workflow a bypass token). Defeats the purpose of branch protection. Rejected.
2. **Make CI a verifier** — formalize the local discipline as the contract; CI fails when the contract is broken. Selected.

## Tradeoffs

- **Build determinism becomes a contract.** Any non-determinism between local and CI fails CI, caught at landing time, not silently papered over. This is a feature, not a bug — the dev.33 ship caught two real determinism issues this way (`yaml@2.8.2` vs `2.8.3` lockfile/install drift; `loaf release`'s content-only build step shipping a stale CLI bundle, see TASK-149).
- **Bundled dependency versions are part of the determinism contract.** Local `node_modules` can drift from lockfile (e.g., `yaml@2.8.3` installed locally while lockfile pins `2.8.2`); `npm ci` is required before any commit-able rebuild. The contract surfaces this rather than masking it.
- **Onboarding cost.** Contributors need the same Node version as CI and clean install discipline. Mitigated by `node-version: '22'` pinned in workflow; future improvement: `.tool-versions` for mise/asdf and a `build-clean` pre-commit hook that runs `npm run build` and warns on dirty tree.
- **No more "CI fixes the build for me" implicit safety net.** The dev who commits is responsible for the artifact bundle being current. The verifier turns this into a gate; previously it was an aspiration.

## Consequences

- The two determinism bugs caught pre-merge during the dev.33 ship validate the verify-only design over the auto-push design within a single release cycle.
- The CI status now reflects real artifact correctness, not push-permission noise.
- Failure mode is loud and self-explanatory: the diff is in the run log, the fix command is in the failure message.
- If future workflows produce artifacts the dev cannot reasonably regenerate locally (e.g., signed binaries, large model files), this ADR will need a carve-out — either by excluding those paths from the verifier or by carving out a publish-after-merge job with a dedicated bypass.

## Supersedes

None — this codifies an emerging convention into an explicit decision.

## Related

- SPEC-031 (release flow hardening; pull #41) shipped the new workflow.
- TASK-149 (build-step gap) and the yaml-lockfile drift are the two determinism bugs caught by the new verifier during dev.33.
- CLAUDE.md "If tracked build artifacts in `dist/` or `plugins/` changed, commit them with the source changes that produced them" — the local-side convention this ADR enforces in CI.
