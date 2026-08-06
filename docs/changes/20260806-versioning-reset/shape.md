<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Versioning Reset

## Problem

Two months on a `2.0.0-alpha.N` treadmill — nineteen alphas, each implying 2.0 is imminent while the actual work is foundational churn. The version string makes a promise the software is not close to keeping, and it distorts release behaviour: cutting a release feels like a statement about 2.0 rather than a routine act of shipping fixes, so releases do not happen, and known field bugs sit unfixed on the daily-driver machine. Dev builds have no defined identity — today a local build reports exactly the same version string as a release, distinguishable only by the *absence* of build metadata. See `brief.md` for the full problem archaeology.

## Hypothesis

If releases carry honest major-zero numbers and every merged fix can ship as a patch-slot bump without ceremony-angst, releases become boring — the dogfooding machine gets fixes as releases instead of worktree builds, dev builds get an unmistakable identity, and stabilization milestones are reached rather than defended.

## Scope

**In**

- Version scheme flip: releases are plain `0.X.X`; this Change ships **0.2.20**, continuing the renumbered alpha line.
- Dev/canary build identity: `0.2.{unix_timestamp}` — timestamp in the patch slot, valid SemVer, sorts above all releases in the minor (deliberate: a canary machine is ahead of the latest release).
- Ceremony guardrail: the release pipeline refuses its full ceremony (packaged GitHub Release, Homebrew bump, changelog entry, release build) for a timestamp-magnitude patch version. Anything cheaper — commits, lightweight tags, prerelease-marked uploads — stays allowed.
- Forced-downgrade transit: version-comparing code paths survive `2.0.0-alpha.19 → 0.2.20`. The inventory (Planning Contract → Comparison Sites) found the gates are equality-based and pass untouched; the one genuinely inverted surface (harness-drift advice) is fixed, not worked around.
- CHANGELOG renumbering in place: `0.1.0` collapses the 1.x era into one entry, `dev.N → 0.1.N` (sequence gaps stay as historical fact), the four `pre.<timestamp>` builds → `0.1.50–0.1.53`, `alpha.N → 0.2.N`. Entry content is untouched; only headings and version-carrying reference lines renumber.
- Version citations in living docs (`docs/ARCHITECTURE.md`, `docs/STRATEGY.md`, release skill prose) renumber per the same map; ADRs stay verbatim as dated records.
- Versioning-policy ADR in `docs/decisions/`.
- The 0.2.20 release itself, riding the PR-based release flow, with the Homebrew formula updated.
- GitHub tag/release wipe: all pre-reset tags and Releases deleted (including `v1.17.4`), strictly after Homebrew points at 0.2.20; then a lightweight `v0.1.0` era marker planted at the commit `v1.17.4` pointed to.

**Out** (deferred, not rejected)

- Hooks-entry reconciliation — ships as 0.2.21 (already scoped in the journal).
- OpenCode session-start hook fix — ships as 0.2.22.
- Skills store redesign and skills audit — later 0.2.x Changes.
- Branch and worktree housekeeping (~20 diverged branches, stale worktrees) — a separate housekeeping pass.

**Cut** (explicitly rejected)

- A `-dev.{ts}` prerelease suffix for dev builds: prerelease precedence sorts dev builds *below* the latest release, making the canary machine nag "update available" perpetually.
- A fourth version part: not valid SemVer, breaks npm.
- Retroactive retagging of renumbered history: the changelog is the sole carrier; GitHub gets exactly `v0.1.0` (marker) and `v0.2.20`.
- Dev-version stamps in tracked build artifacts: the version is embedded in 232 tracked files under `dist/`, `plugins/`, and `.claude-plugin/`, and CI's artifact-sync gate (`.github/workflows/build.yml:66-81`) rejects any drift — a timestamp there would make every local `loaf build` dirty the tree. Dev identity is binary-level only.

## Observable Workflow

- A release build reports `0.2.20`; a local dev build reports `0.2.{unix_timestamp}` — the two are visually unmistakable.
- `loaf upgrade` on the dogfooding machine stops nagging: installed and latest agree, or the installed dev build is honestly newer and the drift advice says so correctly.
- Shipping a fix is routine: merge, release PR, `0.2.21` on GitHub, `brew upgrade loaf` serves it. No narrative weight, no hesitation.
- Attempting the release ceremony on a timestamp-magnitude version refuses with a clear message naming the guardrail; pushing such a tag to GitHub skips the release workflow instead of packaging.
- The GitHub releases page shows one release; the tags page shows `v0.1.0` and `v0.2.20`. CHANGELOG.md tells the whole renumbered history.

## Rabbit Holes and No-Gos

- **Branch/worktree housekeeping.** Out of scope by decision — the reset does not become a repository spring-clean.
- **Rewriting changelog entry content.** Renumbering touches headings and version-carrying reference lines only; the recorded facts of each release stay verbatim.
- **A general version-comparison library.** Fix the sites the inventory names; do not abstract beyond the one shared timestamp-magnitude predicate.
- **Old-tag archaeology beyond the v0.1.0 marker.** No re-pointing, no annotated tombstones, no per-era markers.
- **Fixing the field bugs this cadence exists to ship.** The hooks and OpenCode fixes are the next two releases, not riders here.
- **Editing ADRs.** The ADR log is append-only; old ADRs keep their original version citations, and the new ADR carries the translation map.

## Decisions

Provenance: pitch interview (journal `decision(release)` entries of 2026-08-06 02:35–02:42, carried in `brief.md`), shaping interview (2026-08-06, this session), shaping discovery (version-machinery inventory, this session).

1. **Plain `0.X.X` releases; dev builds `0.2.{unix_timestamp}` in the patch slot.** (Pitch.) Patch-slot timestamps sort above releases in the minor, matching canary reality. Forecloses prerelease-suffix dev identities.
2. **Guardrail drawn at ceremony, not visibility.** (Pitch.) Timestamp versions may appear on GitHub; they never trigger the release pipeline.
3. **The reset is a forced downgrade, accepted.** (Pitch.) Distribution is minimal; comparisons that would refuse the transit get forced or fixed.
4. **Wipe ordering: delete old tags only after Homebrew points at the new release.** (Pitch.) Deleting first breaks `brew install` against a formula referencing a dead tarball.
5. **The published release is inside the Definition of Done.** (Shaping interview.) "0.2.20 exists, Homebrew serves it, old tags gone" is this Change's outcome, not a follow-up ceremony.
6. **Renumbered continuity supersedes the pitched restart-at-0.2.0.** (Shaping interview.) History keeps its progression under new numbers — `0.1.0` = collapsed 1.x era, `0.1.N` = dev.N, `0.1.50–53` = pre builds, `0.2.N` = alpha.N — and the reset ships **0.2.20**, the number the next alpha would have taken. Hooks becomes 0.2.21, OpenCode 0.2.22; the next stable epoch is 0.3.0; 1.0.0 stays a far milestone.
7. **Tag surface after the wipe: `v0.1.0` marker + `v0.2.20`.** (Shaping interview.) Full wipe including `v1.17.4`; one lightweight era marker keeps the 1.x commit findable, with no packaged Release for it — the ceremony/visibility line applied to history.
8. **The ADR records policy fresh; nothing is superseded.** (Shaping discovery.) No ADR blessed the alpha line — it came from a chore commit — so the new ADR has no predecessor to flip.
9. **Dev identity lives at the binary level; tracked artifacts always carry the release version.** (Shaping discovery.) Forced by the CI artifact-sync gate over the 232 version stamps in `dist/`, `plugins/`, and `.claude-plugin/`. The existing signal inverts cleanly: today "dev" is the absence of build metadata; after this Change it is the presence of a dev stamp.
10. **Guardrail placement: the release-snapshot derivation, plus a CI early-skip.** (Shaping discovery.) `resolveReleaseSnapshot` (`internal/cli/change_release_gate.go:200`) is the single derivation every release consumer reads — refusing timestamp-magnitude candidates there covers dry-run, apply, and post-merge in one place. `.github/workflows/release.yml` gets a matching early-skip so a pushed timestamp tag never packages.
11. **This Change stamps `target_release: "0.2.20"`; task checkboxes complete pre-merge; the ceremony and wipe are a post-merge runbook inside the Definition of Done.** (Shaping.) The stable-candidate cohort gate hard-gates Changes targeting the candidate, so all Steps boxes must flip in the PR; post-merge acts are runbook prose in TASK-006, proven by the post-ceremony verification entries.
12. **Living docs renumber their version citations; shipped Changes' `target_release` fields retarget to the release they actually rode.** (Shaping.) Two `change.json` files target `2.0.0`, which never existed and is unreachable under the new scheme; records stay honest under the map.

## Planning Contract

### Preconditions

- `gh` authentication — resolved during shaping (`levifig` active with `repo` + `workflow` scopes).

### Approach

The version has exactly one source (`package.json:3`) and is never compiled in: `packageVersion()` (`internal/cli/version.go:119`) re-reads it from the resolved distribution root at runtime, and `nativeBuildPackageVersion` (`internal/cli/build_codex.go:549`) injects it into build outputs. The flip is therefore one edit propagated by one rebuild: set `0.2.20` in `package.json`, refresh `package-lock.json` (stale at `alpha.17`; nothing regenerates it), and run `loaf build` so all 232 stamped artifacts and `.claude-plugin/marketplace.json` (the second release "version file" — `release_dry_run.go:848`) move in the same commit, as `resolveReleaseSnapshot` demands.

Dev identity is new machinery, not a relabel — the inventory found no live generator for the historical `-dev.N`/`-pre.<ts>` forms. A local (non-CI) binary build stamps `0.2.{unix_timestamp}` at build time via the existing build-metadata path (`cli/scripts/go-build-flags.mjs` gates on `LOAF_BUILD_COMMIT`/`LOAF_BUILD_DATE`, which only CI sets — that same signal distinguishes dev from release builds); `loaf --version` reports it; tracked content artifacts keep the release version per Decision 9. A single shared predicate — timestamp-magnitude patch ⇒ dev identity — is used by the drift classifier, the version report, and the release guardrail. Exact injection mechanics (ldflags variable vs. an equivalent build-metadata carrier) are TASK-002's call within these constraints.

The forced downgrade needs less forcing than feared: `validate-push`'s version check is equality-only (`check.go:967`), the nine post-merge guardrails are equality/existence-only, and the release CI tag check is equality-only — all pass a downgrade untouched. `bumpReleaseVersion` cannot mint `0.2.20` from `2.0.0-alpha.19`, so the flip is a manual version-file edit landing in this PR; the ceremony then runs `loaf release --post-merge`, which keys `Candidate = current` by equality. The one truly inverted surface is harness drift: installed `.loaf-version` markers (`2.0.0-alpha.19` on all four harnesses) compare *higher* than a `0.2.20` binary, classifying `binary-stale` — `doctorDetailLine` then advises upgrading the binary (wrong direction) and the SessionStart nudge stays silent. Fix: teach the classifier the dev-identity predicate and correct the advice; the one-time alpha transit itself is handled operationally (run `loaf upgrade` right after installing 0.2.20 — markers rewrite and state returns to `current`).

The currency advisory needs no change: it compares the running binary against GitHub Releases, all 43 existing Releases are prerelease-flagged (filtered for a stable binary), `v1.17.4` has no Release, and the wipe empties the field anyway.

Renumbering is a heading-and-references rewrite of `CHANGELOG.md` per the Decision 6 map, plus an authored `0.1.0` entry summarizing the 1.x era, plus citation renumbering in living docs (`docs/ARCHITECTURE.md:387`, `docs/STRATEGY.md:41,54`, `content/skills/release/SKILL.md:209`), plus retargeting the two `change.json` files pinned to `2.0.0` (`20260727-spec-conversion-and-guidance-sweep`, `20260728-receipt-tree-binding`) to the renumbered release each actually rode (determined by tag ancestry before the wipe). The generic prerelease documentation in `content/skills/git-workflow/references/commits.md` stays — it is SemVer knowledge for consumer projects, not Loaf's own scheme.

The release itself follows the flow the release-flow-guidance Change shipped: this PR carries the version flip, machinery, renumbering, and ADR; merge; `loaf release --post-merge` cuts the tag and drafts the Release; CI packages and pushes the Homebrew formula (`levifig/homebrew-tap`, generated wholesale by `cli/scripts/update-homebrew-formula.mjs`); verify `brew install`; only then run the wipe (43 Releases, ~50 remote tags) and plant `v0.1.0`. Go changes in this PR stale the binary-pinned capability receipts (the alpha.16 lesson), so evidence re-records against the rebuilt binary before release.

### Comparison Sites (inventory digest)

Full report in the shaping session journal; the load-bearing sites:

| Site | Location | Behavior across the transit |
|---|---|---|
| Currency advisory | `upgrade_advisory.go:72-116` | Unaffected; prerelease Releases filtered for stable binary |
| Harness drift classifier | `harness_drift.go:63,117-147` | **Inverted advice + silent nudge — fixed by TASK-002** |
| Release cohort gate | `change_release_gate.go:14,186` | Stable 0.2.20 hard-gates byte-matching `target_release` only |
| Candidate derivation | `change_release_gate.go:200`, `release_dry_run.go:968` | Cannot mint 0.2.20; manual edit + `--post-merge` equality |
| validate-push | `check.go:944-981` | Equality-only; downgrade passes |
| Post-merge guardrails ×9 | `release_post_merge.go` | Equality/existence-only; pass |
| Release CI tag check | `.github/workflows/release.yml:63-73` | Equality-only; passes |

### Risks

- **The flip commit is large and mechanical** — 232 artifact stamps rebuild. CI's artifact-sync gate re-verifies determinism; review focuses on `package.json`, `marketplace.json`, and the build being the only author of the rest.
- **Transition window on canary machines**: inverted drift advice persists until `loaf upgrade` runs after installing 0.2.20. Accepted; the release notes instruct.
- **The wipe is irreversible** (43 Releases, ~50 tags). The runbook orders it strictly after the Homebrew verify, uses `gh api` pagination to enumerate, and TASK-006 records the full tag list before deleting.
- **Stale capability receipts**: Go changes here stale the binary-pinned smoke receipts; the release evidence gate (`release_dry_run.go:504-516`) refuses until re-recorded. Named in TASK-006 readiness.
- **GitHub marks 0.2.20 "Latest" by date** despite `2.0.0-alpha.19` being semver-higher — desired, and moot post-wipe.

### Sequencing

TASK-001 (flip) lands first; TASK-002 (dev identity), TASK-003 (guardrail), and TASK-004 (renumbering) are independent of each other after it; TASK-005 (ADR) waits for 002/003 so it documents proven behavior; TASK-006 (release + wipe) is last — its Steps complete pre-merge, its runbook runs post-merge.

## Implementation Units

- **TASK-001 — Version scheme flip.** `package.json` → 0.2.20, `package-lock.json` refreshed, one rebuild moves all stamped artifacts, suite green.
- **TASK-002 — Dev-build identity.** `0.2.{unix_timestamp}` binary-level stamping, the shared timestamp-magnitude predicate, drift-classifier fix.
- **TASK-003 — Release ceremony guardrail.** Snapshot-level refusal, release.yml early-skip, the two scheme-pinned tests updated.
- **TASK-004 — Changelog renumbering.** The Decision 6 map applied, the `0.1.0` era entry authored, living-doc citations and the two `target_release` retargets.
- **TASK-005 — Versioning-policy ADR.** The scheme, dev identity, guardrail, renumber map, and milestone semantics as a dated record.
- **TASK-006 — Release and tag wipe.** Pre-merge readiness (target stamp, changelog curation, evidence re-record), post-merge runbook (ceremony, Homebrew verify, wipe, era marker).

## Verification Contract

- **V1.** The version source is flipped. Command: `node -p "require('./package.json').version"`. Expect: exit 0 and contains `0.2.20`.
- **V2.** The second version file moved with it. Command: `node -p "require('./.claude-plugin/marketplace.json').metadata.version"`. Expect: exit 0 and contains `0.2.20`.
- **V3.** The suite is green. Command: `npm run test`. Expect: exit 0.
- **V4.** The guardrail refuses timestamp-magnitude candidates. Command: `go test ./internal/cli -run TestReleaseSnapshotRefusesTimestampPatch -count=1`. Expect: exit 0.
- **V5.** No pre-reset headings remain in the changelog. Command: `grep -c "^## \[2\.0\.0" CHANGELOG.md`. Expect: exit 1.
- **V6.** The 1.x era anchor exists. Command: `grep -c "^## \[0\.1\.0\]" CHANGELOG.md`. Expect: exit 0 and contains `1`.
- **V7.** The policy ADR exists. Command: `test -f docs/decisions/ADR-026-major-zero-versioning.md`. Expect: exit 0.

Human review (H-tier):

- **H1.** A reviewer confirms the renumbered CHANGELOG maps 1:1 to the old headings (numeral-preserving for dev/alpha, positional for the pre builds) with entry content untouched.
- **H2.** A reviewer builds locally and confirms `loaf --version` reports a timestamp-magnitude patch, and that drift advice against a release-marked install reads correctly in both directions.
- **H3.** A reviewer confirms the release workflow's early-skip path: a timestamp tag exits cleanly without packaging, a plain tag proceeds.

Post-ceremony operator confirmations run only after TASK-006's post-merge runbook, performed by the operator who ran it. They are unsatisfiable before merge, so they stay human-tier and never gate the pre-merge verify receipt:

- **H4.** The operator confirms the release is live: `gh release view v0.2.20` exits 0.
- **H5.** The operator confirms the pre-reset tags are gone from the remote: `git ls-remote --tags origin | grep -cE "refs/tags/v(1\.|2\.)"` exits 1.
- **H6.** The operator confirms the era marker survives the wipe: `git ls-remote --tags origin refs/tags/v0.1.0` shows exactly one ref.

## Definition of Done

- V1–V7 pass and the `loaf change verify` receipt is committed pre-merge; the post-ceremony confirmations (H4–H6) pass after the runbook.
- The 0.2.20 GitHub Release is live and `brew install levifig/tap/loaf` serves it.
- All pre-reset tags and Releases are gone from GitHub; `v0.1.0` marks the 1.x-era commit.
- A fresh local build reports `0.2.{unix_timestamp}` and a canary machine's `loaf upgrade` reports clean afterward.
- The versioning-policy ADR is merged.

## Durable Outputs

- The versioning-policy ADR (`docs/decisions/ADR-026-major-zero-versioning.md`): the 0.X.X scheme, patch-slot timestamp dev identity, the ceremony guardrail, the renumbered-history map, and the 0.3.0 / 1.0.0 milestone semantics.
- CHANGELOG.md as the renumbered historical record.

## Open Questions

<!-- Fog register: tag entries [KU]/[UK]/[UU] with a route. Tags are convention, never parsed by check. -->

- [KU] Exact dev-version injection mechanism (ldflags variable vs. equivalent build-metadata carrier) → TASK-002, within Decision 9's constraints.
- [KU] Which renumbered release the spec-conversion sweep actually rode (tag ancestry check, must run before the wipe) → TASK-004.
