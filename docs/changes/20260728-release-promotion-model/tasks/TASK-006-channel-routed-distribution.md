---
change: release-promotion-model
id: TASK-006
title: Channel-routed distribution
blocked-by:
  - TASK-001
---

# TASK-006 — Channel-routed distribution

## Objective

CI routes by the tag's channel: prerelease tags pass `--prerelease` on the `gh release create` fallback and update `Formula/loaf-dev.rb`; stable tags update `Formula/loaf.rb`. The formula-update script takes the target formula (and derives the Ruby class name) as a parameter instead of assuming one file.

## Scope boundaries

**In:** `.github/workflows/release.yml`, `cli/scripts/update-homebrew-formula.mjs`, any script-level tests that exist for the formula updater.

**Out:** Go code entirely; the tap repository itself (seeding `Formula/loaf-dev.rb` there is a manual H-tier step recorded in shape.md H3, not a commit in this repo).

## Context pointers

- Contract: `shape.md` — Planning Contract → CI and tap; Decision 7.
- Workflow today: `.github/workflows/release.yml` (create-fallback `:88-98` has no `--prerelease`; tap update `:100-130` unconditional).

## Acquisition

```bash
loaf journal log "skill(implement): TASK-006 — channel-routed distribution"
# Read .github/workflows/release.yml and cli/scripts/update-homebrew-formula.mjs end to end.
```

## Steps

- [ ] Derive the channel from the tag in the workflow (prerelease = tag contains `-`); pass `--prerelease` on the create-fallback for prerelease tags.
- [ ] Route the tap update: prerelease → `Formula/loaf-dev.rb`, stable → `Formula/loaf.rb`; commit message names the formula touched.
- [ ] Parameterize the Ruby class name in `update-homebrew-formula.mjs` — `--formula` already exists and is required (`:9`); the class is hardcoded as `class Loaf < Formula` (`:27`) and must emit `LoafDev` for the dev formula, since Homebrew derives the class from the file name.
- [ ] Guard the workflow against a missing `loaf-dev.rb` with a clear failure message naming the manual seeding step.

## Verification

- `node cli/scripts/update-homebrew-formula.mjs` invoked against a fixture formula file for both names produces correct version/sha/class updates (script-level check or documented manual run in the delivering commit).
- Workflow YAML parses (`gh workflow view` or actionlint if available); channel routing visible in the diff for review.
