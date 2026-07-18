# Plan — installed-distribution-authority

Working plan and evidence log for the hotfix on `hotfix/installed-distribution-authority` (base c8d5f51a = origin/main = v2.0.0-alpha.8). See `change.md` for the contract.

## Steps

1. Inspect `internal/cli/version.go`, `install.go`, `config.go`, `doctor.go`, `build.go`, Runner dispatch in `cli.go`, the Change contract, and existing tests. *(done)*
2. Add the end-to-end regression (`cmd/loaf/installed_distribution_test.go`) and run it against base — record the failing command and output below before touching product code.
3. Implement the authority split: `internal/cli/distribution.go` (`Runner.Executable` seam + `resolveInstalledDistributionRoot`), rewire `version`/`install`/`config check`/`doctor`, rename `resolveLoafPackageRoot` → `resolveSourceCheckoutRoot` for `build`/`setup`.
4. Adapt affected unit tests (inject fixture provenance through the seam) and add focused unit coverage.
5. Falsify: temporarily revert the resolver rewiring, prove the regression fails, restore, prove it passes. Record both transcripts below.
6. Full `go test ./...`; sanctioned artifact regeneration via `npm run build`; isolated-environment smokes (temp HOME, XDG, `LOAF_DB`, git config, install targets).
7. Inspect the final diff; confirm no temporary edits, no journal commands, no global mutation.

## Evidence

### Failing before fix (step 2)

Command, run at base c8d5f51a with only the new test file added: `go test ./cmd/loaf -run 'TestInstalledDistribution' -v`

```text
=== RUN   TestInstalledDistributionVersionAuthorityFromStaleCheckout
    installed_distribution_test.go:33: version output = "\n\x1b[1mloaf\x1b[0m 1.1.1-stale\n\x1b[90mgo\x1b[0m 1.26.5\n\n\x1b[1mTargets:\x1b[0m\n  cursor  \x1b[90mdist/cursor/\x1b[0m\n\n\x1b[1mContent:\x1b[0m\n  Skills:  1\n  Agents:  0\n  Hooks:   1\n\n", want installed distribution version "9.9.9-current"
--- FAIL: TestInstalledDistributionVersionAuthorityFromStaleCheckout (1.03s)
=== RUN   TestInstalledDistributionUpgradeAuthorityFromStaleCheckout
    installed_distribution_test.go:58: .cursor/.loaf-version = "1.1.1-stale\n", want installed distribution version "9.9.9-current"
--- FAIL: TestInstalledDistributionUpgradeAuthorityFromStaleCheckout (0.66s)
=== RUN   TestInstalledDistributionCheckoutOwnBinaryReportsCheckoutVersion
--- PASS: TestInstalledDistributionCheckoutOwnBinaryReportsCheckoutVersion (0.36s)
FAIL
FAIL	github.com/levifig/loaf/cmd/loaf	2.405s
```

The installed 9.9.9-current executable reports the stale checkout's version and `install --upgrade` re-stamps `.cursor/.loaf-version` with `1.1.1-stale`. The checkout-own-binary guard passes before and after by design — it pins the explicit dev flow, not the bug.

### Falsification (step 5)

The fix was temporarily disabled by inserting a cwd-first branch at the top of `resolveInstalledDistributionRoot` (marked `FALSIFICATION-ONLY`), restoring the original working-directory authority. `go test ./cmd/loaf -run 'TestInstalledDistribution'` then failed exactly as at base:

```text
--- FAIL: TestInstalledDistributionVersionAuthorityFromStaleCheckout (1.04s)
    installed_distribution_test.go:33: version output = "…loaf 1.1.1-stale…", want installed distribution version "9.9.9-current"
--- FAIL: TestInstalledDistributionUpgradeAuthorityFromStaleCheckout (0.42s)
    installed_distribution_test.go:58: .cursor/.loaf-version = "1.1.1-stale\n", want installed distribution version "9.9.9-current"
FAIL	github.com/levifig/loaf/cmd/loaf	2.163s
```

The temporary branch was removed (verified: `grep -c FALSIFICATION internal/cli/distribution.go` → 0) and a fresh uncached run passes: `go test ./cmd/loaf -run 'TestInstalledDistribution' -count=1` → `ok github.com/levifig/loaf/cmd/loaf 1.765s`.

### Final verification (step 6)

- `go test ./... -count=1` — entire suite passes; `npm run typecheck` passes; `gofmt -l internal/cli cmd/loaf` clean; zero remaining `resolveLoafPackageRoot` references.
- `npm run build` (sanctioned scripts) succeeded end to end — build:go, build:cli-ref, build:content (`bin/loaf build` resolving this source checkout), verify:go-artifacts ("present and synchronized"). Tracked artifact deltas match repo convention (#107/#117 precedent): only `bin/native/darwin-arm64/loaf` and `plugins/loaf/bin/native/darwin-arm64/loaf`.
- Live smoke with the freshly built binary under `env -i` isolation (temp HOME, XDG_CONFIG_HOME/XDG_DATA_HOME, `LOAF_DB`, GIT_CONFIG_GLOBAL/SYSTEM, minimal PATH): installed alpha.8 layout invoked with cwd inside an alpha.6 checkout reported `loaf 2.0.0-alpha.8`; `install --upgrade --yes` wrote `.cursor/.loaf-version` = `2.0.0-alpha.8`, installed the packaged skill content (`# Current Foundations`, not the stale checkout's), and stamped the project fenced section `v2.0.0-alpha.8` only.
- Explicit dev flow: the checkout's own `bin/native/darwin-arm64/loaf version` from a foreign working directory reports the checkout's version (executable provenance, no cwd inference); pinned permanently by `TestInstalledDistributionCheckoutOwnBinaryReportsCheckoutVersion`.
- `loaf change check docs/changes/20260718-installed-distribution-authority` (fresh binary, isolated `LOAF_DB`): no violations, executable: yes.
- No journal commands were run; no global installation, database, or user configuration was read or mutated; no commits, pushes, or dependency changes were made.

### Capability-evidence regeneration (step 6, continued)

The full suite exposed the expected capability-evidence gate after the sanctioned rebuild: the three retained U8 installed-smoke receipts pinned `native_binary_sha256` `c4e684cb2b9f717444001b0dc5a333bb94e124ccdf9c3201141f9958a208142e`, while the rebuilt candidate is `f41c6a4c5268f02bd594471ffe324f10196258bf31c0faa31431e7bd3fcba16d`; `LoadTargetCapabilityEvidence` re-hashes current candidate artifacts against receipts, so `go test ./internal/cli` failed on hash drift by design, not from a product defect. Per coordinator authorization the receipts were regenerated through the sanctioned writers only — no hash was hand-edited.

- Pinned harness CLIs provisioned in disposable npm prefixes with an isolated npm cache (`npm install -g --prefix /tmp/loaf-u8-pins-*/{claude,codex,opencode} --cache /tmp/loaf-u8-pins-*/npm-cache`): `@anthropic-ai/claude-code@2.1.207`, `@openai/codex@0.144.1`, `opencode-ai@1.17.18`; verified `claude --version` → `2.1.207 (Claude Code)`, `codex --version` → `codex-cli 0.144.1`, `opencode --version` → `1.17.18`. Globally installed newer versions (claude 2.1.214, codex 0.144.5, opencode 1.18.3) were left untouched; pinned identities in receipts and `config/target-capabilities.json` required no change, so no decision gate was tripped.
- Sanctioned runners rerun with the pinned CLI first on `PATH`: `node cli/scripts/u8-claude-smoke.mjs` → `exit_code 0, assistant_marker_match true`; `node cli/scripts/u8-codex-smoke.mjs` → `exit_code 0, assistant_marker_match true`; `node cli/scripts/u8-opencode-smoke.mjs` → `exit_code 0, assistant_marker_match true, plugin_loaded true, root_session_lookup_proven true, cleanup_succeeded true`. Each runner rebuilt the candidate itself (`npm run build:go` + `bin/loaf build --target <target>`) and ran the harness in its own disposable repo with isolated `LOAF_DB` (plus isolated `CODEX_HOME` for Codex and isolated HOME/XDG for OpenCode).
- Reproducibility held: after all three runner-internal rebuilds, `bin/native/darwin-arm64/loaf` and `plugins/loaf/bin/native/darwin-arm64/loaf` still hash `f41c6a4c5268f02bd594471ffe324f10196258bf31c0faa31431e7bd3fcba16d`.
- Receipt diffs are exactly minimal: in each of the three receipt JSONs only `native_binary_sha256` (`c4e684cb…` → `f41c6a4c…`), `timestamp`, and the random `marker` changed; invocation shapes, pinned versions, and hooks hashes are byte-identical.
- Gate cleared: `go test ./internal/cli -run 'TargetCapability|InstalledSmoke' -count=1` → ok; `go test ./cmd/loaf -run 'TestInstalledDistribution' -count=1` → ok; full `go test ./... -count=1` → ok for cmd/loaf, internal/cli, internal/project, internal/state.
- No journal commands were run; no global installation, database, or user configuration was mutated; disposable prefixes were removed after verification.

### Independent-review follow-up (2026-07-18, post-approval findings)

Three review findings addressed mechanically within the authority contract; all prior hotfix work, falsification evidence, and receipt integrity preserved.

1. **`version` degraded output no longer reads the working directory (MEDIUM).** `runVersion` previously zeroed the root on unresolvable provenance and let `builtTargets`/`countSkillDirs`/`countAgentFiles`/`countHookEntries` join paths onto the empty root — relative lookups against the caller's cwd, so a stale checkout's Targets/Content rendered in the output. Targets and Content now render only from the resolved installed-distribution root; with no provenance the output ends at identity (`0.0.0` + go line) plus the resolver's reinstall guidance. Development behavior is unchanged and stays provenance-derived: a checkout's own binary resolves that checkout as its distribution (`TestInstalledDistributionCheckoutOwnBinaryReportsCheckoutVersion` still passes untouched).
   - Regression tests: `TestRunnerVersionOmitsTargetsAndContentWithoutDistribution` (internal/cli, `t.Chdir` into a fully populated stale checkout) and `TestInstalledDistributionBareBinaryVersionOmitsCheckoutTargetsAndContent` (cmd/loaf, real bare binary with no adjacent distribution invoked with cwd inside a stale checkout).
   - Falsification: with the guard disabled (`if false && rootErr != nil`, `FALSIFICATION-ONLY` marker), both tests fail showing the stale checkout's `Targets: cursor dist/cursor/`, `Skills: 1`, `Hooks: 1`; marker removed (`grep -c FALSIFICATION internal/cli/version.go` → 0) and both pass at `-count=1`.
   - `TestPublicBinaryDispatchesStateVersionAndReleasePreflightNatively` and `TestPublicBinaryDispatchesVersionFlagNatively` had exercised the cwd fallback (bare binary, repo-root cwd); they now run the shared binary from a distribution-shaped fixture, keeping their native-dispatch assertions intact.
2. **Docs corrected (MEDIUM).** `change.md` Out, Observable Workflow, Decision 3, and Risks now state that the **current** `loaf install`/`setup` binary prompt (`installLoafBinary`) writes the launcher+native-only `~/.local/bin` layout — not only older Loaf versions — and that fail-closed `install`/`config check` plus identity-only degraded `version` output for that layout is intentional.
3. **Shared test binary cleanup (LOW).** `cmd/loaf` gained a `TestMain` that removes the shared compiled binary's temp directory after `m.Run` returns — deterministic, race-free (the package has no `t.Parallel`), and never during tests. Verified: a fresh package run leaves zero new `loaf-installed-dist-*` directories in TMPDIR; 14 leaked directories from earlier runs were removed.

Verification after the fixes:

- `gofmt -l internal/cli cmd/loaf` clean; `go vet ./internal/cli ./cmd/loaf` clean.
- `npm run build` succeeded end to end (build:go, cli-ref, content, verify:go-artifacts "present and synchronized"); tracked artifact deltas remain only the two native binaries, both now sha256 `5b76b90e835834bea3f4208cb8fa4e344951e43ab90f986837f09ac3d6eb2519` (previous review pass `f41c6a4c…`, base `c4e684cb…`).
- Pinned CLIs re-provisioned in fresh disposable prefixes (`npm install -g --prefix /tmp/loaf-u8-pins-*/{claude,codex,opencode} --cache /tmp/loaf-u8-pins-*/npm-cache`): `claude --version` → `2.1.207 (Claude Code)`, `codex --version` → `codex-cli 0.144.1`, `opencode --version` → `1.17.18`. Globally installed newer versions were left untouched.
- U8 smokes rerun via the sanctioned runners with the pinned CLI first on `PATH`: `node cli/scripts/u8-claude-smoke.mjs` → `exit_code 0, assistant_marker_match true`; `node cli/scripts/u8-codex-smoke.mjs` → `exit_code 0, assistant_marker_match true`; `node cli/scripts/u8-opencode-smoke.mjs` → `exit_code 0, assistant_marker_match true, plugin_loaded true, root_session_lookup_proven true, cleanup_succeeded true`. Reproducibility held: both binaries still hash `5b76b90e…` after the runner-internal rebuilds.
- Receipt diffs vs HEAD exactly minimal: in each of the three receipt JSONs only `native_binary_sha256` (`c4e684cb…` → `5b76b90e…`), `timestamp`, and the random `marker` changed; pinned identities, receipt names, invocation shapes, and capability classifications are byte-identical. No hash was hand-edited.
- Gates: `go test ./internal/cli -run 'TargetCapability|InstalledSmoke' -count=1` → ok; `go test ./cmd/loaf -run 'TestInstalledDistribution' -count=1` → ok; full `go test ./... -count=1` → ok (cmd/loaf 30.3s, internal/cli 51.2s, internal/project 0.5s, internal/state 14.6s).
- No journal commands were run; no global installation, database, or user configuration was read or mutated; disposable pin prefixes were removed after verification.
