<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Release Evidence-Freshness Gate

## Problem

Two releases in one day (v2.0.0-alpha.16, v2.0.0-alpha.17) published tags whose CI failed on stale installed-smoke capability evidence, leaving GitHub Releases with zero assets. The staleness has two mechanical triggers and two process amplifiers: (1) `dist/opencode/plugins/hooks.ts` embeds `@version` in its generated header, so every version bump stales the OpenCode receipt's `hooks_sha256` — even with zero Go changes; (2) Go source changes rebuild `bin/native/darwin-arm64/loaf` and stale all binary-pinned receipts; (3) in alpha.17 the evidence was re-recorded minutes *before* the version-bump rebuild, so the release commit staled it immediately; (4) both incidents took the direct release door, skipping the release-PR CI that runs the contract tests against the prepared tree. Nothing in `loaf release` itself checks receipt freshness after the artifact rebuild — the canary lives only in `go test`, which no release path runs post-rebuild.

## Hypothesis

If `loaf release` validates capability receipts against the just-rebuilt tree and refuses to commit or tag on stale evidence, the zero-asset release incident becomes mechanically impossible on every path — PR-flow or direct — and the ordering rule ("re-record after the rebuild") is enforced by the tool instead of remembered by the operator.

## Scope

**In**

- An in-process evidence-freshness check — the same loader CI's contract tests exercise — invoked by `loaf release` after the artifact rebuild and before any release commit or tag, on all paths (`--pre-merge`, the direct door, and `--post-merge` tagging).
- Refusal with remediation text: the loader's mismatch error plus which `cli/scripts/smoke-*.mjs` runner re-records the stale receipt, and the ordering rule (re-record after the rebuild, then re-run the gate).
- The gate no-ops when the project has no capability-evidence config — `loaf release` stays generic for consumer projects.
- `content/skills/release/SKILL.md` states the ordering rule explicitly: re-record AFTER the `--pre-merge` rebuild, on the release branch, before pushing the release PR. Rebuilt `dist/`/`plugins/` skill copies ship in the same commits as the source change.

**Out** (deferred, not rejected)

- Decoupling receipts from binary rebuilds — the structural fix that would make re-recording rare; tracked by `INTENT-20260719-decouple-installed-smoke-evidence-from-binary-rebuilds`.
- Auto-re-record from within `loaf release` — the smoke runners need live harness CLIs, network, and model calls; far too heavy and flaky to run inside a release gate.

**Cut** (explicitly rejected)

- Any override/skip flag for the gate. An escape hatch recreates the incident under schedule pressure; the only exit is fixing the evidence.
- Shelling out to `go test` from the release command — the installed binary must gate without a Go toolchain present.

## Observable Workflow

On a healthy tree, `loaf release --pre-merge` (and the direct and `--post-merge` paths) reports the evidence check inline — records validated, then proceeds. On a stale receipt it stops before creating the release commit or tag, exits non-zero, and prints the mismatch (target, artifact path, recorded vs current SHA-256) followed by remediation: the runner invocation for that target and the reminder that re-recording must happen after the artifact rebuild. A consumer project without `config/target-capabilities.json` sees no new output and no new requirement.

## Rabbit Holes and No-Gos

- **No evidence-schema redesign.** Binding, schema, and the decoupling idea belong to the tracked Intent, not this gate.
- **No new user-facing command surface.** The gate is internal to `loaf release`; no `loaf evidence` subcommand, no new `hooks.yaml` registration in this Change.
- **No full-suite execution in the gate.** The gate validates evidence only; CI remains the owner of the whole test suite.

## Decisions

Provenance: shaped from the alpha.16/alpha.17 incident diagnosis (journal entries 2026-07-30, this conversation); scope approved by the user via the task brief pasted into the shaping session.

1. **In-process validation via the existing evidence loader, not `go test`.** The installed binary must gate on machines without a Go toolchain, and the loader *is* the code the contract tests call — parity comes from sharing the implementation, not the harness. Forecloses byte-exact CI-command parity; accepted.
2. **No override flag.** The gate's value is that it cannot be talked out of; both incidents were plausible-looking trees that would have shipped under an escape hatch.
3. **Presence-probed conditionality.** The gate activates only when the capability-evidence config exists, keeping `loaf release` usable in projects that never recorded evidence.
4. **`--post-merge` gates too.** Tagging is the last gate before publication; validating the merged tree there catches anything that landed stale through an unguarded path.

## Planning Contract

### Placement

`--pre-merge` and the direct door share one executor, `runReleaseApply` (`internal/cli/release_dry_run.go:347`), so a single insertion covers both: the gate joins the existing post-rebuild refusals between the artifact-command loop (`:487-491`) and `git add -A` (`:503`), as a third sibling to the virtualenv-leak (`:492-494`) and `docs/changes`-mutation (`:495-501`) checks — same contract (inspect the rebuilt tree, refuse before staging, leave the worktree for inspection). On `--post-merge` no rebuild occurs; the gate becomes **guardrail 9** in `checkReleasePostMergeGuardrails` (`internal/cli/release_post_merge.go`, after the HEAD-not-tagged check), inheriting the numbered `guardrail N failed` reporting and the injectable command runner, validating the merged tree as-is. Dry-run never rebuilds and stays gate-free.

The gate is the **first production caller** of `LoadTargetCapabilityEvidence` (`internal/cli/target_capability_contract.go:262`) — one argument, the full JSON path; it derives its anchor root internally. Presence probe: `os.Stat(filepath.Join(root, TargetCapabilityEvidenceRecordPath))`; absent → silent no-op. The root is the **release root** (the tree being released), deliberately not `resolveSourceCheckoutRoot` — the gate certifies the tree that will be committed and tagged.

### Failure copy

Two registers, matching each site's house style. Apply path: `Refusing to commit release artifacts: <loader error>; re-record after the rebuild and rerun: <runner list>` — the capitalized `Refusing to commit release artifacts:` form of its sibling refusals. Post-merge: lowercase guardrail message with em-dash remediation, surfaced as `guardrail 9 failed: …`. The loader's own error already names the stale record and artifact (target, path, recorded vs current SHA-256); the remediation block is static — the three runner invocations (`smoke-claude-code-startup.mjs`, `smoke-codex-startup.mjs`, `smoke-opencode-request-context.mjs` with the `--client/--expected-version/--receipt` shape) plus the ordering rule — rather than parsed out of the error string.

### Cascade risk

This Change touches Go source, so its own release rebuild will stale all binary-pinned receipts — the release that ships this gate must re-record all three receipts after its rebuild. The gate itself will enforce that ordering for the first time, which is also its first live proof.

### Testing approach

TDD against the release suite's existing patterns. Apply path: real-git temp-dir fixtures (extend `seedReleaseApplyRepo` or add a sibling seeder that writes a `config/target-capabilities.json` plus receipt/artifact files), drive the public surface via `Runner.Run(["release", "--bump", …, "--yes"])`, and — following the existing post-rebuild refusal tests — assert both the message and that **no release commit and no tag were created** (the gate fires after files were written to the worktree). Post-merge: the scripted-runner pattern (`releasePostMergeHappyResponses` + one overridden key) asserting `guardrail` number and message. Refusal-copy tests follow `TestReleaseGuardrailRemediation`: message contains each required phrase, does not contain forbidden advice. Cover: stale receipt refused on both executors; fresh receipts pass; absent config no-ops; remediation lists the runners and the ordering rule.

## Implementation Units

- **TASK-001 — Evidence gate in the release flow.** The shared validation function, three call sites, refusal copy, and tests. The unit most likely to change under review; leads.
- **TASK-002 — Release-skill ordering rule.** The SKILL.md ordering statement, plus the rebuilt `dist/`/`plugins/` copies committed with it.

## Verification Contract

- **V1.** Gate tests pass. Command: `go test ./internal/cli -run TestRelease`. Expect: exit 0.
- **V2.** In-tree receipts are fresh (the gate's own precondition holds in this repo). Command: `go test ./internal/cli -run TestTargetCapabilityEvidence`. Expect: exit 0.
- **V3.** Full suite green. Command: `npm run test`. Expect: exit 0.
- **V4.** Generated artifacts current, mirroring CI's drift check (native binaries excluded — PRs don't commit rebuilt binaries; releases do). Command: `npm run build && git diff --exit-code -- bin/ dist/ plugins/ .claude-plugin/ ':(exclude)bin/native/**' ':(exclude)plugins/loaf/bin/native/**'`. Expect: exit 0.

- **H1.** A reviewer confirms the refusal copy names the correct runner per target and reads as an instruction, not a stack trace; and that the SKILL.md ordering rule is unambiguous about *when* re-record happens relative to the rebuild.

## Definition of Done

- Stale evidence refuses the release on all three paths, proven by tests; fresh and absent-config trees proceed untouched.
- Remediation copy maps every SHA-pinned target to its runner.
- The release skill states the ordering rule; dist/plugins copies match source.
- Full suite green; `loaf change check` clean.

## Durable Outputs

- CHANGELOG entry for the gate.
- The canary lesson moves from journal/memory into enforced behavior — after shipping, update the release skill's incident narrative to note the gate exists (folded into TASK-002).

## Open Questions

None open. The placement unknown resolved into Planning Contract → Placement via the release-flow reconnaissance (2026-07-30, in-session).
