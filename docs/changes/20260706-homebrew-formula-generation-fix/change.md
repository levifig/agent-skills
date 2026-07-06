---
change: homebrew-formula-generation-fix
created: 2026-07-06
branch: homebrew-formula-generation-fix
---

<!-- Frontmatter must open the file at byte one. No status-like frontmatter: readiness is derived from the executable sections below. -->

# Homebrew Formula Generation Fix

## Problem

The `v2.0.0-alpha.4` Homebrew tap update exposed that `cli/scripts/update-homebrew-formula.mjs` emits an explicit `version` line while also building URLs with Ruby `#{version}` interpolation. Homebrew audit treats that as a redundant explicit version because it can infer the version from the release URL, so a future generated formula would repeat the same tap CI failure we manually repaired.

## Hypothesis

If the generator emits literal versioned release URLs and omits the explicit `version` line, Homebrew can infer the package version from each URL and the generated formula will match the audit-safe shape we want to keep in the tap.

## Scope

**In**

- Update `cli/scripts/update-homebrew-formula.mjs` to generate literal release asset URLs for each supported Homebrew target.
- Remove the generated explicit `version` declaration.
- Add regression coverage that executes the `.mjs` script and pins the output against independent expected formula properties.

**Out** (deferred, not rejected)

- Automating tap PR creation or tap CI polling.
- Reworking release scripts into Go.
- Changing package archive names or release asset layout.

**Cut** (explicitly rejected)

- Reintroducing Ruby interpolation in formula URLs.
- Adding a Node test framework or runtime dependency for this small script.

## Observable Workflow

During release, `node cli/scripts/update-homebrew-formula.mjs --formula <path> --checksums checksums.txt --version <semver>` rewrites the formula so each platform block has a concrete GitHub release URL containing that version. The formula has no explicit `version` declaration, leaving Homebrew to infer the version from the URL.

## Rabbit Holes and No-Gos

- Do not broaden this into a full release pipeline redesign.
- Do not mutate the live tap from tests.
- Do not test by recomputing the same URL template in the same way the implementation does; the regression must look for fixed, literal expected strings.

## Decisions

Provenance: dogfooding the `alpha.4` release failure and user direction to keep the updater as `.mjs`.

1. **Keep the updater as `.mjs`.** The script is already release-adjacent, dependency-free, and easy to execute from npm scripts; moving it now would add churn without improving the failure mode.
2. **Use Go test coverage for the Node script.** Existing release-script coverage in `cmd/loaf/native_cutover_files_test.go` already shells out to Node, so the regression belongs beside neighboring release tooling tests.
3. **Assert literal output, not reconstructed behavior.** The test must pin the absence of `version "` and `#{version}`, plus the presence of concrete `v2.0.0-alpha.4` release URLs, so it cannot pass by repeating the implementation algorithm.

## Planning Contract

### Implementation

Patch `formula(versionValue, repoValue, values)` so generated URL strings interpolate `versionValue` at generation time and no longer emit `version "${versionValue}"`.

### Coverage

Add a Go test that creates a temp checksum file with all four Homebrew targets, runs `node cli/scripts/update-homebrew-formula.mjs`, reads the generated formula, and checks:

- no explicit `version "` line appears;
- no `#{version}` interpolation appears;
- all four expected concrete GitHub release URLs appear;
- all four expected checksum values appear;
- the script reports a successful update.

## Implementation Units

- **U1 - Generator output.** Change the `.mjs` formula template to produce Homebrew-audit-safe formula output for prerelease versions.
- **U2 - Regression coverage.** Add focused script execution coverage in the existing native cutover test file.

## Verification Contract

<!-- Executable (machine-checkable): -->

- **V1.** `go test ./cmd/loaf -run TestHomebrewFormulaUpdaterGeneratesAuditSafePrereleaseURLs` passes.
- **V2.** `go test ./...` passes.
- **V3.** `loaf change check --require-executable docs/changes/20260706-homebrew-formula-generation-fix` passes.

<!-- Human review: -->

- **H1.** Reviewer confirms the generated formula shape matches the repaired tap formula from the `v2.0.0-alpha.4` release.

## Definition of Done

- The formula updater can generate a prerelease formula without Homebrew's redundant-version audit issue.
- The regression test fails against the old `version`/`#{version}` output and passes against the new literal URL output.
- No new dependencies or broad release-flow changes are introduced.

## Durable Outputs

No new durable spec or ADR is expected. This is a narrow release tooling bug fix; the durable output is the executable regression test.

## Open Questions

- None for this change.

## Source Inputs

- `v2.0.0-alpha.4` release dogfooding: Homebrew tap audit failed until the formula was manually changed to remove explicit `version` and use literal asset URLs.
- User direction in this conversation: keep the updater as `.mjs` and add coverage.
