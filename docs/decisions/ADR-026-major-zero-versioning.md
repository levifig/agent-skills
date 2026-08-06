---
id: ADR-026
title: "Major-zero versioning — plain 0.X.X releases, timestamp dev identity, and a ceremony guardrail"
status: Accepted
date: 2026-08-06
supersedes: null
superseded_by: null
related:
  - ADR-012
  - ADR-022
---

# ADR-026: Major-zero versioning

## Context

Loaf spent two months on a `2.0.0-alpha.N` treadmill — nineteen alphas, each implying 2.0 was imminent while the work was foundational churn. The number made a promise the software was not close to keeping, and it distorted release behaviour: cutting a release read as a statement about 2.0 rather than a routine act of shipping fixes, so releases stopped happening and known field bugs sat unfixed on the daily-driver machine. No ADR ever blessed the alpha line — it arrived in a chore commit — so this record has no predecessor to supersede.

Dev builds had no identity at all. A local build reported exactly the string a release reported, distinguishable only by the *absence* of build metadata, and that absence is not a signal this repository can trust: the Claude Code plugin marketplace serves a committed `plugins/loaf/bin/native/<platform>/loaf` at a release tag, and `npx github:levifig/loaf` compiles one during install. Both are releases that never pass through the release workflow.

## Decision

**Releases are plain `0.X.X`.** No prerelease suffix, no aspirational major. The patch slot counts releases within a minor, so any merged fix ships as a patch bump with no narrative weight. The minor is the stabilization epoch: the current one is `0.2.x` (this reset ships `0.2.20`, the number the next alpha would have taken), the next is `0.3.0`, and `1.0.0` stays a far milestone to be reached rather than defended.

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

**Pre-reset ADRs keep their original citations.** The log is append-only, and an ADR is a dated record of what was decided when — ADR-011's "PR #34 merged 2026-04-22 as `v2.0.0-dev.29`" stays exactly as written, and the table above is the translation key that resolves it to `0.1.29`. Living documents (`docs/ARCHITECTURE.md`, `docs/STRATEGY.md`, the release skill) renumbered their citations because they describe current practice rather than a moment.

## Consequences

Releases become boring, which is the point: a merged fix is a patch bump, a release PR, and a Homebrew upgrade, with no argument about what the number claims. The two identities are unmistakable on sight — `0.2.20` versus `0.2.1786022455` — and the machine running its own build sorts honestly above the latest release instead of being nagged to "upgrade" to something older than what it is running.

Costs accepted. The reset is a forced downgrade: markers written by `2.0.0-alpha.19` sit above a `0.2.20` binary until `loaf upgrade` runs once, so the transition window shows content-stale advice that is correct but startling. GitHub will mark `0.2.20` "Latest" by date despite `2.0.0-alpha.19` being semver-higher — desired, and moot after the tag wipe. The dev stamp is an arrival time, not a link time, and a machine whose clock has never been set mints a patch below the floor and falls back to the release version rather than claim an identity it cannot name. Renumbered history exists only in `CHANGELOG.md`; no tag or artifact carries the new numbers for anything before `0.2.20`.

Provenance: `docs/changes/20260806-versioning-reset/` shape.md Decisions 1–12; landed commits on `versioning-reset` for the version flip, dev identity, ceremony guardrail, and renumbering.

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
