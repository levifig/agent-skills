---
change: install-upgrade-split
id: TASK-004
title: Install-channel detection and currency advisory
blocked-by:
  - TASK-002
---

# TASK-004 — Install-channel detection and currency advisory

## Objective

`loaf upgrade` knows how the running binary was installed and appends a best-effort advisory line when a newer release exists, per the converged contract in shape.md › Planning Contract › Currency advisory contract: channels are Homebrew (keg path / `INSTALL_RECEIPT.json`), npm (global tree; package name read from the launcher's `package.json`), and dev checkout (distribution root in a git worktree) — anything else is unknown and silent. Source of truth is GitHub Releases for all channels (prerelease binaries compare against latest-including-prereleases; stable against stable only), comparison is semver with prerelease ordering, budget is a one-second hard total, and every failure degrades to no advisory with otherwise identical output and exit code. Advisory text: current version, available version, exact command (`brew upgrade loaf`, `npm update -g <package>`, `git pull && npm run build`). Never executes anything.

## Scope boundaries

**In:** Channel-provenance resolution alongside `resolveInstalledDistributionRoot` (`distribution.go`), a currency check with a short hard timeout, the advisory output line, tests with fake keg/npm/dev layouts and a stubbed check.

**Out:** Executing upgrades or re-exec (deferred in Scope Out), any blocking or exit-code impact from the check — advisory only, upgrade's result is identical with the network down.

## Context pointers

- Contract: `shape.md` — Decision 3, Planning Contract › Currency advisory contract, Rabbit Holes (blocking on the network), Cut (network-gated behavior).
- The former open question (source, comparison, timeout) is resolved in the contract subsection above — implement it, don't re-decide it.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-004 — channel detection and currency advisory"
# Read: internal/cli/distribution.go; inspect a real keg: ls /opt/homebrew/Cellar/loaf/*/
```

## Steps

- [x] Implement channel detection from the resolved binary/distribution path; unknown channels degrade to no advisory.
- [x] Implement the GitHub Releases check behind an interface so tests stub it; enforce the one-second budget; prerelease-vs-stable comparison rule; treat every failure as "no advisory".
- [x] Emit the advisory line with current version, available version, and the exact channel command.
- [x] Tests: three channel fixtures, golden output for stale/current/offline per channel, unparseable-version silence, timeout respected.

## Verification

- With network disabled (or the check stubbed to fail), `loaf upgrade` output and exit code are identical to the check never existing.
- Channel fixtures resolve correctly under `go test ./internal/cli/`.
