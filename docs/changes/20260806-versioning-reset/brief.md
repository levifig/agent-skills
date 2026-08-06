<!-- brief.md is the optional archeological kickstart — the original unshaped ask.
     May accrete parked problem-space concepts while the change is captured; freezes when shape.md exists.
     Superseded by shape.md; never mechanically load-bearing.
     A brief-only folder is legal and non-executable (captured, not shaped). -->

# Versioning Reset

## Problem Statement

Loaf has spent two months on a `2.0.0-alpha.N` treadmill — nineteen alphas, each release implying 2.0 is imminent while the actual work is foundational churn: install topology, a coming store redesign, a skills audit that gates stability. The version string makes a promise the software is not close to keeping, and it distorts release behaviour: cutting a release feels like a statement about 2.0 rather than a routine act of shipping fixes, so releases do not happen, so known bugs (the OpenCode session-start hook, the Codex hook-file conflict) sit unfixed on the daily-driver machine while new work continues on top. There is also no defined identity for local dev builds — the machinery stamps forms like `2.0.0-dev.49` and `v2.0.0-pre.<timestamp>` ad hoc, and the upgrade currency advisory nags about the difference.

## Who Has It

The operator, on every machine running Loaf daily. The pain shows up at each `loaf upgrade` (currency advisory noise, unfixed field bugs), at each release decision (blocked on the implied weight of "2.0"), and in every conversation about what version anything is.

## Current Alternatives

Keep riding the alpha treadmill (release nothing until 2.0 is real, meaning field bugs wait months), or cut more 2.0.0-alphas (perpetuates the false promise and the twenty-plus tag pileup on GitHub). Neither produces a routine ship-fixes cadence.

## Value Proposition

Releases become boring. A `0.2.z` line says honestly that the software is in major-zero flux, every merged fix can ship as a patch-slot bump without ceremony, and the version string stops carrying narrative weight. The dogfooding machine gets fixes as releases instead of as worktree builds, dev builds get an unmistakable identity, and 1.0.0 becomes a milestone reached by stabilization rather than a number defended by hesitation.

## Constraints

- Releases published to GitHub (and flowing to Homebrew) use plain `0.X.X`, restarting at `0.2.0`. The middle number is the epoch in progress (the rework formerly aimed at 2.0); the patch slot absorbs all minors and patches; `1.0.0` when the epoch stabilizes.
- Local dev/canary/nightly builds use `0.2.{unix_timestamp}` — the timestamp in the patch slot. This is valid SemVer (no fourth part, npm-safe). Chosen over a `-dev.{ts}` prerelease suffix deliberately: prerelease precedence would sort dev builds *below* the latest release, making the dogfooding machine nag "update available" perpetually; patch-slot timestamps sort above all releases in the minor, which matches reality for a canary machine. Consequence accepted and recorded: release patches and timestamps share one number line, discriminated by magnitude.
- Guardrail required, drawn at ceremony rather than visibility: a timestamp-magnitude version may appear on GitHub (commits, lightweight tags, or prerelease-marked uploads are all acceptable when useful), but it never triggers the release pipeline — no packaged GitHub Release, no Homebrew formula bump, no changelog ceremony, no release build time spent. The release flow refuses to run its full ceremony for a timestamp patch; anything cheaper is allowed.
- The reset is a forced downgrade — installed markers everywhere say `2.0.0-alpha.19`, and `0.2.0` precedes it in SemVer terms. Accepted deliberately: distribution is minimal. Any version comparison that would refuse or misreport the transition gets forced or fixed, not worked around.
- Old GitHub tags and releases are deleted for clarity, but only after the Homebrew formula points at `0.2.0` — deleting first breaks `brew install` against a formula referencing a dead tarball.
- Version policy is recorded as an ADR in `docs/decisions/` (supersedes whatever ADR or convention blessed the 2.0.0-alpha line).
- The release rides the existing PR-based release flow; direct `--bump` remains the named exception.

## Sequencing and Relationships

This Change lands before everything else queued: `0.2.0` is the reset itself, carrying everything already merged (the harness-neutral-skills arc). Then the fix cadence begins — `0.2.1` is the hooks-entry-reconciliation Change (dissolves the Codex hook-file refusal; `loaf upgrade` goes green), `0.2.2` the OpenCode session-start hook fix, later `0.2.x` the store redesign and skills audit. The hooks and store Changes are already scoped in the journal; neither depends on this one technically, but shipping them as releases depends on this one existing.

## Sources and Research Links

- Journal entries of 2026-08-06: `skill(pitch)` and three `decision(release)` entries recording the scheme, the forced downgrade, and the release order.
- The dogfooding acceptance run (journaled 2026-08-05): the currency advisory noise (`2.0.0-alpha.19 → 2.0.0-dev.49`) and the field bugs motivating a fix cadence.
- Existing dev-build stamping: `2.0.0-dev.N` and `v2.0.0-pre.<timestamp>` forms in the release machinery — the mechanism exists; this Change re-labels it.
- The release-flow-guidance work (PR #146): state-aware guardrails and the PR-flow default this release will ride.

## Open Questions

- [ ] Which code paths compare versions (currency advisory, upgrade markers, release gate) and how each behaves across the `2.0.0-alpha.19 → 0.2.0` boundary — deferrable to shaping; the decision (force it) is made, the inventory is not.
- [ ] Whether the npm package name/version surface has consumers beyond Homebrew and the launcher (anything pinning `2.0.0-alpha.x`) — deferrable; believed none.
