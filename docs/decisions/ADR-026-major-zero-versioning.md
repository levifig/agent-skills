---
id: ADR-026
title: "Major-zero versioning — arc-boundary releases, liberal X epochs, and timestamp dev identity"
status: Accepted
date: 2026-08-06
revised: 2026-08-15
supersedes: null
superseded_by: null
related:
  - ADR-012
  - ADR-022
  - ADR-023
---

# ADR-026: Major-zero versioning

2026-08-15: bump semantics restate as `loaf release suggest` evidence.

## Context

Loaf spent two months on a `2.0.0-alpha.N` treadmill — nineteen alphas, each implying 2.0 was imminent while the work was foundational churn. The number made a promise the software was not close to keeping, and it distorted release behaviour: cutting a release read as a statement about 2.0 rather than a routine act of shipping fixes, so releases stopped happening and known field bugs sat unfixed on the daily-driver machine. No ADR ever blessed the alpha line — it arrived in a chore commit — so this record has no predecessor.

Dev builds had no identity at all. A local build reported exactly the string a release reported, distinguishable only by the *absence* of build metadata, and that absence is not a signal this repository can trust: the Claude Code plugin marketplace serves a committed `plugins/loaf/bin/native/<platform>/loaf` at a release tag, and `npx github:levifig/loaf` compiles one during install. Both are releases that never pass through the release workflow.

The initial reset (2026-08-06) fixed the number but framed the minor as a "stabilization epoch" — `0.2.x` now, `0.3.0` someday, `1.0.0` far away — with every merged fix shipping as a patch bump. Two months of practice falsified that framing. The patch slot absorbed twenty-one releases including subsystem-scale work — the state-dedupe identity-fork repair (#159), per-entry hook reconciliation (#161), receipt-vouched execution — that the number filed alongside one-line fixes; the version stopped carrying information. The mechanics already disagreed with the prose: `suggestReleaseBump` maps any `feat` commit to a minor bump, so every dry-run against real work derived `0.3.0` while policy held X at 2, and the one capture-only Change folder pinned to `target_release: 0.3.0` blocked exactly that candidate. Underneath sat an unexamined premise — that a release follows every merge — which made the version a merge counter and denied a multi-Change effort any single moment of announcement: the 0.2.21 cut combined two linked Changes and had no way to say so with a number. Operator direction on record (journal spark, 2026-08-10): X marks a significant change (not necessarily breaking), Y carries patches, fixes, and improvements that do not rework the current X, and X progresses liberally — `0.98.0` is a fine number to reach.

## Decision

**Releases are plain `0.X.X`.** No prerelease suffix, no aspirational major. `1.0.0` stays a far milestone — reached deliberately, never suggested by tooling and never defended. Post-1.0, plain SemVer governs; everything below is the pre-1.0 regime.

**Pre-1.0, X marks significance and Y carries the rest.** A release bumps X when it ships a completed arc; every other release bumps Y — patches, fixes, and improvements that do not rework the current X. X is a counter of significant changes, not a promise about proximity to 1.0, and it advances as fast as significant work completes.

**The arc is the cohort.** ADR-022 already named it: the cohort — all Changes sharing a `target_release` — *is* the arc, derived and never declared as a graph. A standalone executed Change with no pin is an arc of one and bumps X at the release that includes it. Softer linkage — a tracked Intent, prose in a brief — does not move the version until it becomes a shared pin. This keeps the X-trigger decision-free and machine-derivable: the release gate already computes cohort membership and flip-grade execution evidence, so "does the unreleased range complete an arc" is a question git answers, not a judgment call at cut time. A breaking change pre-1.0 is at most an X bump; nothing suggests `1.0.0`.

**Releases decouple from merges.** Merging and releasing are separate acts. Mid-arc Changes merge to main when they are ready — each one complete and reviewed through the ship ceremony — but no release announces them until the arc completes; the arc-completing cut is the X release, and linking Changes into an arc means one significant moment, not several. Hotfix and patch Y releases stay available in the current X throughout: a Y cut from main mid-arc necessarily carries the merged arc code, which is safe to carry precisely because every merged Change is individually complete, and the changelog — not the tarball — is the announcement carrier. Arc work is announced at the arc's X release.

**Pin late; retarget as routine.** `target_release` is a release-entry declaration, not a roadmap wish: it is set when a Change is shaped and actually assembling into a cohort, never at capture. Capture-only folders stay unpinned — ADR-022 already establishes that untargeted Changes gate nothing and carry zero ceremony. When X advances past a pin, retargeting — including removal — is the routine reviewable act ADR-022 already sanctions, surfaced from history and at preflight, never blocked.

**The bump suggestion derives from arc evidence.** `suggestReleaseBump` historically read commit types — any `feat` suggested minor, a breaking marker suggested major. Under this record it derives from the same evidence the cohort gate reads: a completed arc in the unreleased range suggests X, otherwise Y, capped at minor while major is 0. Until that realignment lands, cuts state the bump explicitly per this policy. The explicit override remains the valve for the documented edge: a hotfix cut while a standalone executed Change sits unreleased in range takes `--bump patch` deliberately, and the X-worthy work waits for its own cut.

**A dev build's identity is `<major>.<minor>.<unix timestamp>`.** One predicate names it — a patch at or above `devVersionPatchFloor` (`1_000_000_000`, a magnitude the release count will never approach) is a dev build and nothing else (`isDevVersion`, `internal/cli/version.go`). A timestamp patch is valid SemVer and sorts *above* every release in the minor, which is the truth about a machine running its own build.

**The stamp is read, not linked.** The committed native binaries are asserted byte-for-byte reproducible (`cli/scripts/verify-go-artifacts.mjs`), so a build-varying `-X` ldflag would fail that assertion on every build. The executable's own modification time carries it instead, read at startup in `cmd/loaf/main.go` — an approximation of link time, since `git checkout`, an unpreserved copy, and tarball extraction all rewrite it. For a number whose only job is to say "this is not a release", when the binary arrived on this machine is close enough.

**Dev identity takes two facts, not one.** Absent release build metadata says no release pipeline built this binary; a resolved distribution root carrying `go.mod` beside the content says it is running out of the tree that did. Every shipped distribution — release archive, Homebrew keg, npm package, plugin payload — is content plus a prebuilt binary; only the checkout has the Go module. Absence alone would have told a user who installed `0.2.20` from the marketplace that they were running a dev build.

**Dev identity lives at the binary level; tracked artifacts always carry the release version.** The version is stamped into 224 tracked files under `dist/`, `plugins/`, and `.claude-plugin/`, and CI's artifact-sync gate (ADR-012) rejects any drift, so a timestamp there would make every local `loaf build` dirty the tree. The old signal inverts cleanly: dev used to be the *absence* of build metadata, and is now the *presence* of a dev stamp on the version line.

**One predicate, three readers.** The version report mints dev versions, the harness-drift classifier reads them off install markers, and the release snapshot refuses to cut a ceremony for one. There is no flag, suffix, or second source to keep in step.

**The guardrail is drawn at ceremony, not at visibility.** `guardReleaseCeremony` sits on `resolveReleaseSnapshot` (`internal/cli/change_release_gate.go`), the one derivation every release consumer reads, so dry-run, apply, and post-merge are closed by a single refusal. `.github/workflows/release.yml` mirrors it on the way in: a `resolve` job gates the release job, so pushing a timestamp tag skips the workflow cleanly instead of failing three steps later. Its bash predicate demands three numeric parts the way `parseUpgradeSemver` does — a malformed tag is not a dev build and must stay on the loud path — and its floor is pinned to the Go constant by `TestReleaseWorkflowSkipsAtTheDevVersionFloor`. Cheaper acts never pass through the snapshot and stay available: commits, lightweight tags, prerelease-marked uploads.

**Install markers carry the release version, and drift names both directions.** Install stamps `packageVersion`, never the dev stamp — comparing a marker against a build clock would report drift on every harness of every dev machine forever. A marker above the binary means either a newer binary stamped that content or the version line was renumbered underneath a binary that is current, so the advice names both remedies instead of assuming the binary is behind. A marker of timestamp magnitude outranks everything published by construction and is never read as evidence the binary is stale.

**History renumbers in place; the changelog is the sole carrier.** `CHANGELOG.md` maps 1:1 onto the new scheme with entry content untouched. GitHub keeps exactly two tags — a lightweight `v0.1.0` era marker and `v0.2.20` — and one Release.

| Old | New | Rule |
|---|---|---|
| `1.0.0` – `1.17.4` (pre-CLI era) | `0.1.0` | Collapsed into one authored anchor; `c7e7eb9d`, the era's final commit, carries the `v0.1.0` marker after the wipe |
| `2.0.0-dev.N` | `0.1.N` | Numeral-preserving, N = 1–49; the unreleased gaps (10, 21, 25, 41) stay absent |
| `2.0.0-pre.20260614235428` | `0.1.50` | Positional, chronological |
| `2.0.0-pre.20260625183349` | `0.1.51` | Positional, chronological |
| `2.0.0-pre.20260625190923` | `0.1.52` | Positional, chronological |
| `2.0.0-pre.20260625192947` | `0.1.53` | Positional, chronological |
| `2.0.0-alpha.N` | `0.2.N` | Numeral-preserving, N = 1–19 |

**Pre-reset ADRs keep their original citations.** An ADR revision is dated, and a citation resolves against the record as of its date — ADR-011's "PR #34 merged 2026-04-22 as `v2.0.0-dev.29`" stays exactly as written, and the table above is the translation key that resolves it to `0.1.29`. Living documents (`docs/ARCHITECTURE.md`, `docs/STRATEGY.md`, the release skill) renumbered their citations because they describe current practice rather than a moment.

## Consequences

The version carries information again: X counts significant arcs, Y counts maintenance within them, and reading `0.7.3` says seven significant things have shipped with the third round of polish on the latest. Releases stop being merge echoes and become deliberate arc-boundary events — fewer, and each one meaning something. The two identities stay unmistakable on sight — `0.7.3` versus `0.2.1786022455` — and the machine running its own build sorts honestly above the latest release instead of being nagged to "upgrade" to something older than what it is running.

Costs accepted. A Y release cut mid-arc ships arc code the number does not announce; the changelog carries the honesty, and the safety rests on the ship ceremony keeping every merged Change individually complete. Retargeting becomes recurring low-stakes maintenance instead of a rare event, which is what pin-late discipline exists to minimize. `suggestReleaseBump` disagrees with policy until its realignment lands, so the interim leans on explicit `--bump` — the one window where the trigger is not yet decision-free in the tooling. The dev stamp is an arrival time, not a link time, and a machine whose clock has never been set mints a patch below the floor and falls back to the release version rather than claim an identity it cannot name. Renumbered history exists only in `CHANGELOG.md`; no tag or artifact carries the new numbers for anything before `0.2.20`.

The arc-boundary scheme debuts at the next significant batch: 0.2.21 was cut honestly under the initial patch-per-merge rules and is not renumbered. Under this record, that cut — two linked Changes completing together — would have been an X bump.

Provenance: `docs/changes/20260806-versioning-reset/` shape.md Decisions 1–12 and the landed `versioning-reset` commits (initial record); journal spark(versioning) 2026-08-10 00:13 and wrap(release) 2026-08-10 09:23 (arc-boundary revision); the post-#161 dry-run reproduction of the sweep-pin collision.

## Alternatives Considered

### A `-dev.{timestamp}` prerelease suffix

SemVer precedence sorts prereleases *below* the release they annotate, so a canary machine on its own build would be told forever that an update is available. The whole point of the dev identity is that the canary is ahead.

### A fourth version part (`0.2.20.1786022455`)

Not valid SemVer, and npm rejects it. The patch slot is the only place a monotonic build number fits inside the standard.

### Injecting the stamp with `-ldflags -X`

The committed native binaries must rebuild byte-for-byte identical for the reproducibility gate; a build-varying linker flag fails that assertion on every build. Reading the executable's own mtime keeps the bytes constant.

### Stamping dev versions into tracked build artifacts

Every tracked build artifact under `dist/`, `plugins/`, and `.claude-plugin/` carries the version, and CI verifies rather than fixes them (ADR-012). Every local `loaf build` would dirty the tree with a number that changes each run.

### Treating absent release metadata as the entire dev signal

This repository ships locally built binaries through the plugin marketplace and `npx`, neither of which carries release metadata. The signal alone confused a stranger's install for a dev build — the exact confusion the scheme exists to prevent.

### Restarting at `0.2.0` with no renumbering of history

The pitched original. It leaves the changelog carrying two incompatible numbering eras with no key between them, and discards a progression that maps cleanly onto the new scheme.

### Retroactively retagging renumbered history

Roughly fifty tags would be re-minted for numbers no artifact carries, on commits nobody can install. The changelog is the honest carrier; GitHub gets the `v0.1.0` era marker so the 1.x commit stays findable, and nothing else.

### Drawing the guardrail at visibility

Refusing to let a timestamp version reach GitHub at all would block cheap, useful acts — pushing a build tag, attaching a prerelease-marked upload. What must never happen is the ceremony: changelog entry, release build, packaged Release, Homebrew bump. The line is drawn there.

### The minor as a rare stabilization epoch

The initial framing of this record: minors move rarely, every merged fix is a patch. Twenty-one patches in, the version had stopped distinguishing subsystem-scale work from typo fixes, and the tooling disagreed with the policy on every dry-run. Replaced by arc-boundary X in the 2026-08-10 revision.

### Any `feat` commit bumps X

Zero implementation work — it is what `suggestReleaseBump` historically did. But an improvement usually lands as a `feat` commit, and the operator's semantics put improvements that do not rework the current X in Y; commit-type triggering contradicts that, and X inflated by every small feature stops marking anything.

### Operator declares significance at cut time

Maximally flexible and honest to intent, but it fails the decision-free requirement: every release reopens a judgment call about what the number claims, which is the treadmill failure mode this record exists to end. The arc trigger encodes the same judgment once, at shaping, where it belongs.

### Symbolic epoch pins

Replacing literal pins with a `next-minor` symbol resolved at cut time removes staleness by construction — and breaks ADR-022's canonical-literal-form commitment, requires schema and gate changes, and builds resolution machinery for a problem pin-late discipline solves with zero code.

### Keeping release-per-merge

The unexamined premise under the initial "releases become boring" framing. It makes the version a merge counter, denies arcs a single announcement moment, and is the previous understanding the operator explicitly changed in the 2026-08-10 revision.

## Revisions

- 2026-08-06 — Initial record: plain `0.X.X` reset with the minor as a rare stabilization epoch, timestamp dev identity, ceremony guardrail, renumbered history.
- 2026-08-10 — Arc-boundary release semantics replace the stabilization-epoch framing: X bumps when a completed arc ships (arc = cohort, ADR-022), releases decouple from merges, pins are set late and retargeted as routine, and the bump suggestion realigns to arc evidence. First application of the living-record convention; prior text in git history.
