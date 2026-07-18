---
change: installed-distribution-authority
created: 2026-07-18
branch: hotfix/installed-distribution-authority
---

# Installed-Distribution Authority for Identity-Bearing Commands

## Problem

`resolveLoafPackageRoot(paths ...string)` in `internal/cli/version.go` searches the working directory and project runtime root before `os.Executable()`. Loaf's semver is not baked into the binary — it is read from the nearest `package.json` whose `name` is `loaf` — so the identity of the running binary is decided by where the user is standing. An installed v2.0.0-alpha.8 executable invoked inside an older checkout reports that checkout's version, and `loaf install --upgrade` silently reinstalls the stale checkout's `dist/` content, stamping stale versions into managed markers and fenced sections. `install.go`, `config check --fix`, and Runner dispatch propagate the accidental source root; `doctor` is worse — it reads `packageVersion(runtimeRoot)` straight off the project root, so in any non-Loaf project the "installed" version it diagnoses against is that project's own `package.json` version.

## Hypothesis

Partitioning root authority — project root for project state, source-checkout root for `build`, installed-distribution root (executable provenance) for identity-bearing commands — makes an installed binary report and install only its own shipped content, regardless of the invoking directory, while local development keeps working because running a checkout's own binary makes that checkout the distribution root by construction.

## Scope

**In**

- A Runner-level installed-distribution root resolver: `os.Executable()` → `filepath.EvalSymlinks` → upward walk to the adjacent `package.json` named `loaf`. No working-directory or project-root fallback.
- `version`, `install` (including `--upgrade` and deprecation cleanup), `config check` target maintenance, and `doctor`'s reported CLI version consume the installed-distribution root.
- `build` keeps the existing working-directory/project-first source-checkout resolution, renamed so the authority split is legible in code.
- End-to-end regression coverage: a current installed distribution invoked from an older checkout reports the installed version and upgrades from the installed `dist/`.
- Tracked native binaries and generated outputs regenerated via the sanctioned build scripts only.

**Out** (deferred, not rejected)

- `setup` and `release` root resolution — both are source-checkout workflows today; revisit if a setup-from-installed-distribution ceremony appears.
- Embedding the semver into the binary via ldflags — changes the committed-binary reproducibility contract; a release/publish decision, not a hotfix.
- Recovery UX for `~/.local/bin` copies that carry no adjacent distribution tree (launcher plus `native/` runtime only). The **current** `loaf install`/`setup` binary prompt (`installLoafBinary`) still writes exactly this layout, as older Loaf versions did; under this change those binaries intentionally fail closed for `install`/`config check` with reinstall guidance and degrade `version` to identity-only output. Making that layout self-describing (or improving its recovery UX) is deferred, not rejected.

**Cut** (explicitly rejected)

- Any cwd-inferred "development mode" for installation. Local-development installs are explicit: you run the checkout's own binary (`./bin/loaf`, `npm link`, `/plugin marketplace add <checkout>`). No new opt-in flag or environment variable is introduced.
- Hardcoded install locations (Homebrew Cellar paths, `~/.local/bin`) in resolution logic — provenance comes from the executable path only.

## Observable Workflow

```text
$ cd ~/old-loaf-checkout          # checkout at 2.0.0-alpha.6
$ loaf version                    # installed 2.0.0-alpha.8 binary on PATH
loaf 2.0.0-alpha.8                # ← the binary's own version, not alpha.6

$ loaf install --upgrade          # upgrades from the installed distribution's dist/
  ✓ Cursor installed …            # markers and fenced sections stamped 2.0.0-alpha.8

$ cd ~/loaf && ./bin/loaf version # a checkout's own binary: that checkout is the
loaf 2.0.0-dev                    # distribution root — dev flow unchanged, explicit

$ loaf build                      # still builds the checkout you are standing in
```

A binary with no adjacent distribution (a bare `go build` artifact in `/tmp`, or the launcher+native copy the binary-install prompt places in `~/.local/bin`) degrades `version` to identity-only output — `0.0.0`, the Go runtime line, and the resolver's reinstall guidance, with no Targets or Content sections, which would otherwise be read from the caller's working directory — and fails `install`/`config check` closed with the same guidance instead of adopting whatever checkout surrounds the caller. This fail-closed behavior is intentional: with no provenance there is no distribution to describe or install from.

## Rabbit Holes and No-Gos

- Do not re-add any working-directory fallback to installed-distribution resolution "for convenience" — that is the bug, reintroduced.
- Do not stamp versions into binaries or change `go-build-flags.mjs` ldflags here; committed-binary reproducibility is a separate contract.
- Do not touch target adapter/manifest semantics from PR #107/#117 — only where the source `dist/` and version string come from.
- Do not grow a config surface (env var, flag) for choosing the distribution root; executable provenance is the whole model.

## Decisions

Provenance: accepted during hotfix shaping against the reported stale-authority incident, 2026-07-18.

1. **Executable provenance is the only authority for installed-distribution resolution.** `os.Executable()` + `EvalSymlinks` + upward walk. Rationale: symlink evaluation makes Homebrew shims (`/opt/homebrew/bin/loaf` → Cellar) resolve to the real distribution; the walk makes release archives, npm installs, and source checkouts all work with one rule. Forecloses cwd inference entirely.
2. **The existing explicit dev opt-in is reused, not reinvented.** Running a checkout's own binary is the opt-in; executable provenance honors it naturally. No `LOAF_*` root-override variable exists today and none is added.
3. **Unresolvable provenance fails closed for mutating commands, degrades honestly for read-only ones.** `install` and `config check` error with guidance; `version` reports `0.0.0` plus the resolver's guidance and omits Targets/Content entirely (an empty root would resolve those lookups against the working directory — the bug again); `doctor` skips the fenced-version comparison rather than diagnosing against a fabricated version.
4. **`build` (and `setup`) keep source-checkout resolution byte-for-byte.** The function is renamed `resolveSourceCheckoutRoot` so the two authorities cannot be conflated again silently.
5. **Tests inject provenance through a Runner seam** (`Executable func() (string, error)`), never by mutating package state — the CLI package's tests construct Runners directly and must be able to simulate installed layouts.

## Planning Contract

### Approach

Add `internal/cli/distribution.go` with `Runner.resolveInstalledDistributionRoot()` and the `Runner.Executable` seam. Rewire `runVersion`, `runInstall`, `runConfig` (check), and `runDoctor` to it; `runDoctor` passes an empty CLI version on unresolvable provenance and `checkFencedVersion` skips on empty input. Rename `resolveLoafPackageRoot` → `resolveSourceCheckoutRoot` with unchanged behavior for `build`/`setup`. Update the ~60 affected unit-test Runner literals to inject fixture provenance.

### Risks

- Unit tests across `internal/cli` assume cwd-first resolution; each affected Runner literal needs the seam or the test silently exercises the fallback path. Mitigated by running the full package suite and fixing every failure explicitly.
- `cmd/loaf` end-to-end tests run the real binary from temp directories; any that relied on cwd resolution will surface in the suite run.
- The `~/.local/bin` copy layout (launcher + native runtime only, no package tree) — written by the **current** `install`/`setup` binary prompt as well as older Loaf versions — loses version reporting and fails mutating commands closed. Intentional fail-closed behavior, documented in Out.

### Sequencing

Regression first (fails on base), then the fix, then falsification (revert fix → regression fails → restore → passes), then suite + sanctioned rebuild of tracked artifacts. Evidence preserved in `plan.md`.

## Implementation Units

- **U1 — Regression evidence.** `cmd/loaf/installed_distribution_test.go`: a compiled current binary in a release-archive layout, invoked from a stale checkout with isolated HOME/XDG/`LOAF_DB`/git config, must report the installed version (`version`) and upgrade from the installed `dist/` (`install --upgrade`), with managed markers and fenced sections stamped current; a checkout's own binary must keep reporting the checkout version (explicit dev flow).
- **U2 — Authority split.** `distribution.go` resolver + `Runner.Executable` seam; rewire `version`/`install`/`config check`/`doctor`; rename source-checkout resolver; adjust unit tests and add focused unit coverage for the resolver and fallback behaviors.
- **U3 — Artifacts.** Regenerate tracked native binaries and build outputs via `npm run build` (sanctioned scripts) only.

## Verification Contract

- **V1.** `go test ./cmd/loaf -run TestInstalledDistribution` fails at base commit c8d5f51a (stale version reported, stale content installed) and passes with the fix.
- **V2.** Falsification: with the fix temporarily reverted, V1's regression fails again; restored, it passes. No temporary edits remain in the final diff.
- **V3.** `go test ./...` passes; `npm run build` (build:go, cli-ref, content, verify:go-artifacts) succeeds with reproducible artifacts.
- **V4.** Smoke under isolated HOME/XDG/`LOAF_DB`: installed-current-from-stale-checkout reports the current version; `install --upgrade` selects packaged content; `.loaf-version` markers and fenced sections read current; `loaf build` inside a checkout still builds that checkout.

## Definition of Done

- All Verification Contract items pass and their evidence (failing command + output, falsification transcript) is recorded in `plan.md`.
- No journal commands were run and no global installation, database, or user configuration was touched.
- The final diff contains only: the resolver and rewired consumers, the renamed source resolver, test changes, this change folder, and artifacts regenerated by sanctioned build scripts.
