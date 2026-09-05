---
id: ADR-026
title: "Major-zero versioning — arc-boundary releases, liberal X epochs, and commit-addressed dev identity"
status: Accepted
date: 2026-08-06
revised: 2026-09-05
supersedes: null
superseded_by: null
related:
  - ADR-012
  - ADR-022
  - ADR-023
---

# ADR-026: Major-zero versioning

2026-08-15: bump semantics restate as `loaf release suggest` evidence.

2026-08-18: dev identity changes from an executable-mtime timestamp to SemVer commit metadata.

2026-09-05: the toolchain stamps the commit into the binary (`-buildvcs=true`), uncommitted source reports `.dirty`, and root `bin/` leaves version control.

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

**A dev build's identity is `<package-version>+g<short-sha>`.** For example, a local build of package version `0.3.1` from commit `5a784cde…` reports `0.3.1+g5a784cd (dev build)`. The commit is immediately comparable with `git rev-parse --short=7 HEAD`, and SemVer build metadata deliberately leaves release precedence unchanged. A build from a working tree with uncommitted changes appends `.dirty` (`0.3.1+g5a784cd.dirty`), so the identity never claims a commit's source when it compiled something else.

**The commit is stamped into the binary by the toolchain.** `build-go.mjs` passes `-buildvcs=true`, so `go build` records `vcs.revision` and `vcs.modified` in the binary's build info and `cmd/loaf/main.go` reads them through `runtime/debug.ReadBuildInfo`. The identity is mechanical — it describes the bytes that were compiled — and needs no file beside the binary that could be written from one HEAD while the source on disk said another, which is exactly how `bin/.loaf-dev-commit` produced a stale label on 2026-09-05. That file is gone, and so is the byte-for-byte rebuild assertion in `verify-go-artifacts.mjs`, whose only purpose was keeping tracked binaries identical across commits: root `bin/` is no longer tracked, so nothing needs that property. `plugins/loaf/bin/` remains committed for the Claude Code marketplace until a shim that resolves an installed `loaf` replaces it. One gap needs a bridge: the pinned `go1.26.6` toolchain writes no stamp inside a linked worktree (a `.git` file), while `go1.27.1` does, and worktrees are where Loaf's issue branches live. `build-go.mjs` therefore resolves the same two facts with `git rev-parse HEAD` and `git status --porcelain` and links them as `-X main.devCommit/devModified`; the runtime prefers the toolchain stamp and falls back to the linked values, so both paths report one identity, and the fallback can be deleted once the pinned toolchain stamps worktrees. Staging still applies: `build-go.mjs` compiles requested targets into `bin/native/.staging/` and publishes the set only after every requested target compiles. A binary compiled outside any checkout reports the package version with the dev-channel suffix and no commit.

**The last successful dev build owns Loaf's user-local launcher pointer.** After every requested target compiles, `build-go.mjs` retargets `$XDG_DATA_HOME/loaf/current-dev-launcher` at the checkout's launcher. `~/.local/bin/loaf` is created only when that name is absent, as a symlink to the pointer. Release-metadata builds never claim it, `LOAF_DEV_LINK=0` is the explicit opt-out, and the build never overwrites, unlinks, or renames a real file, directory, or any other symlink at the PATH name — including a leftover checkout symlink from the previous scheme. Activation is best-effort: I/O or permission failures warn and leave the native build successful. This makes the active CLI follow whichever worktree was most recently built once the PATH name points at the pointer, without making the main checkout a privileged indirection point.

**Dev identity takes two facts, not one.** Absent release build metadata says no release pipeline built this binary; a resolved distribution root carrying `go.mod` beside the content says it is running out of the tree that did. Every shipped distribution — release archive, Homebrew keg, npm package, plugin payload — is content plus a prebuilt binary; only the checkout has the Go module. Absence alone would have told a user who installed `0.2.20` from the marketplace that they were running a dev build.

**Dev identity lives on the runtime version line; tracked artifacts always carry the release version.** The version is stamped into tracked files under `dist/`, `plugins/`, and `.claude-plugin/`, and CI's artifact-sync gate (ADR-012) rejects any drift, so a commit there would dirty the tree whenever HEAD changes. The ignored provenance file varies locally while tracked outputs remain deterministic.

**One convention, three readers.** The version report mints `+g<short-sha>`, the harness-drift classifier recognizes it if one appears in an install marker, and the release workflow skips tags carrying it. The classifier and workflow continue recognizing the old timestamp-patch identity so existing markers and tags age out safely.

**The guardrail is drawn at ceremony, not at visibility.** `.github/workflows/release.yml` validates a strict SemVer tag first, then classifies only valid identities. A malformed tag such as `v0.3+gabcdef0` fails resolve. A valid `+g<sha>` or legacy timestamp-patch identity skips packaging instead of failing downstream on a tag/version mismatch. Cheaper acts remain available: commits, lightweight tags, and prerelease-marked uploads.

**Install markers carry the release version, and drift names both directions.** Install stamps `packageVersion`, never the dev commit — comparing a marker against local build provenance would report drift on every harness of every dev machine forever. A marker above the binary means either a newer binary stamped that content or the version line was renumbered underneath a binary that is current, so the advice names both remedies instead of assuming the binary is behind. Legacy timestamp markers remain recognizable and are never read as evidence the binary is stale.

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

The version carries information again: X counts significant arcs, Y counts maintenance within them, and reading `0.7.3` says seven significant things have shipped with the third round of polish on the latest. Releases stop being merge echoes and become deliberate arc-boundary events — fewer, and each one meaning something. Release and dev identities stay unmistakable on sight — `0.3.1` versus `0.3.1+g5a784cd` — without changing their SemVer precedence.

Costs accepted. A Y release cut mid-arc ships arc code the number does not announce; the changelog carries the honesty, and the safety rests on the ship ceremony keeping every merged Change individually complete. Retargeting becomes recurring low-stakes maintenance instead of a rare event, which is what pin-late discipline exists to minimize. `suggestReleaseBump` disagrees with policy until its realignment lands, so the interim leans on explicit `--bump` — the one window where the trigger is not yet decision-free in the tooling. A dev commit names the source that was compiled, with `.dirty` marking uncommitted working-tree changes; a build outside any checkout carries no stamp and falls back to the package version with the dev-channel suffix rather than claiming provenance it cannot resolve. Every local build now differs from the last, which is only acceptable because root `bin/` is untracked; `plugins/loaf/bin/` still churns on each build until the marketplace shim lands. Renumbered history exists only in `CHANGELOG.md`; no tag or artifact carries the new numbers for anything before `0.2.20`.

The revision realigns the native version path (`cmd/loaf/main.go`, `internal/cli/version.go`), local and release build scripts, the user-local dev link, the release workflow guard, version and copied-distribution tests, and this architecture overview. Legacy timestamp recognition remains deliberately isolated to compatibility readers.

The arc-boundary scheme debuts at the next significant batch: 0.2.21 was cut honestly under the initial patch-per-merge rules and is not renumbered. Under this record, that cut — two linked Changes completing together — would have been an X bump.

Provenance: `docs/changes/20260806-versioning-reset/` shape.md Decisions 1–12 and the landed `versioning-reset` commits (initial record); journal spark(versioning) 2026-08-10 00:13 and wrap(release) 2026-08-10 09:23 (arc-boundary revision); the post-#161 dry-run reproduction of the sweep-pin collision.

## Alternatives Considered

### A timestamp dev identity

The original scheme placed executable mtime in the patch slot. It was unique but not source-addressable: copying or checking out the same bytes changed their apparent identity, and recognizing whether the active binary matched HEAD still required inference. Commit metadata answers the actual dogfooding question directly.

### A fourth version part (`0.2.20.1786022455`)

Not valid SemVer, and npm rejects it. The patch slot is the only place a monotonic build number fits inside the standard.

### Injecting the commit with `-ldflags -X`

The committed native binaries must rebuild byte-for-byte identical for the reproducibility gate; a build-varying linker flag fails that assertion whenever HEAD changes. An ignored provenance file supplies local identity without changing tracked bytes.
Adopted on 2026-09-05 once root `bin/` left version control. The committed marketplace copy under `plugins/loaf/bin/` is no longer held to byte-stability, so build-varying bytes cost nothing; `-buildvcs=true` supplies the commit and dirty flag, and `build-go.mjs` links the same two values with `-X` for toolchains that do not stamp linked worktrees.

### Keeping the user-local link pinned to the main checkout

A stable main-checkout link is simple until development happens in a worktree: the binary most recently built and the binary on PATH diverge, which recreates the ambiguity commit-addressed versions were introduced to remove. Letting the last successful build own the link makes activation and identity one operation.

### Stamping dev versions into tracked build artifacts

Every tracked build artifact under `dist/`, `plugins/`, and `.claude-plugin/` carries the version, and CI verifies rather than fixes them (ADR-012). Every local `loaf build` would dirty the tree with a number that changes each run.

### Treating absent release metadata as the entire dev signal

This repository ships locally built binaries through the plugin marketplace and `npx`, neither of which carries release metadata. The signal alone confused a stranger's install for a dev build — the exact confusion the scheme exists to prevent.

### Restarting at `0.2.0` with no renumbering of history

The pitched original. It leaves the changelog carrying two incompatible numbering eras with no key between them, and discards a progression that maps cleanly onto the new scheme.

### Retroactively retagging renumbered history

Roughly fifty tags would be re-minted for numbers no artifact carries, on commits nobody can install. The changelog is the honest carrier; GitHub gets the `v0.1.0` era marker so the 1.x commit stays findable, and nothing else.

### Drawing the guardrail at visibility

Refusing to let a dev-identity version reach GitHub at all would block cheap, useful acts — pushing a build tag, attaching a prerelease-marked upload. What must never happen is the ceremony: changelog entry, release build, packaged Release, Homebrew bump. The line is drawn there.

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
- 2026-08-18 — Commit-addressed SemVer build metadata replaces executable-mtime timestamp identity; an ignored local provenance file preserves tracked-binary reproducibility.
- 2026-08-18 — Successful Git-backed dev builds atomically retarget the guarded user-local Loaf symlink, making the last built worktree active.
- 2026-08-18 — Activation moves to a Loaf-owned launcher pointer and create-exclusive PATH claim; native publication stages binaries before replacing the published set; release tags are SemVer-validated before any dev-identity skip.
- 2026-09-05 — The toolchain stamps the commit into the binary (`-buildvcs=true`) and uncommitted source reports `.dirty`; the ignored provenance file and the byte-for-byte rebuild assertion are removed, and root `bin/` is no longer tracked.
