---
change: hooks-entry-reconciliation
id: TASK-002
title: Hook catalog and entry-level reconciler
blocked-by:
  - TASK-001
blocks:
  - TASK-003
  - TASK-004
---

# TASK-002 — Hook catalog and entry-level reconciler

## Objective

The build emits a per-target hook catalog as the single identity authority, and install/upgrade converge Loaf's own hook entries per event section in Codex and Cursor `hooks.json` — add missing-and-enabled, update present-but-different, remove retired, absorb cohort-absent-as-disabled exactly once — while every non-Loaf entry survives value-identical and integrity failures preserve the file unchanged.

## Scope boundaries

**In:** Catalog emission `(target, event, hook_id, desired entry template)` from `config/hooks.yaml` through the per-target builders plus its `internal/cli` reader; the reconciler replacing the hook-projection branches of `install_plan.go` and the merge paths in `install_target.go`; the closed recognition predicate and normalization algorithm; cohort-restricted run-once absorption; integrity preconditions; crash-safe recompute-on-apply; sanitized raw fixtures; golden and matrix tests.

**Out:** Deleting the retired digest/refusal helpers, manifest-row handling, and `config check` rework (TASK-004); the verb surface (TASK-003); OpenCode/Amp plugin artifacts; any modification to non-Loaf entries under any circumstances.

## Context pointers

- Contract: `shape.md` — Decisions 1–3, 6–10, 12; Planning Contract → Hook catalog, Recognition and normalization, Absorption and migration marker, Crash safety and concurrency, Risks.
- Recognition reference: `codexHookOwnershipForOS`, `isExactCodexJournalHookCommand(Windows)`, and the trusted-executable resolution in `internal/cli` — the quoting/placeholder rules to preserve, tightened to trusted-path identity per Decision 6.
- Evidence: `research/hook-entry-classification.json` (0.2.20-predicate provenance) and the sanitized raw fixtures at `research/fixtures/codex-hooks-live.json` and `research/fixtures/cursor-hooks-live.json` — the golden-test inputs.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-002 — hook catalog + entry-level reconciler"
# Read install_plan.go planTargetAdapterArtifacts, install_target.go merge paths, build_manifest.go hook-projection branches, and config/hooks.yaml before writing the replacement.
```

## Steps

- [ ] Emit the hook catalog per target at build time, including the 0.2.20 cohort enumeration (17 Cursor entries, 1 Codex entry, by hook ID) pinned by a test against the 0.2.20 dist fixtures; update dist/build parity tests.
- [ ] Closed recognition predicate per Decision 6: Loaf-executable invocation in the three exact forms (template, trusted-path-quoted absolute from TASK-001's executable record, bare `loaf` first token) gated by catalog signature or identity-stem match (Decision 13); exact normalized manifest-recorded `hook-file` paths (shell-aware token extraction, `$HOME`/`~` expansion, target-root anchoring, quote stripping, separator normalization; no symlinks, no directory containment); the frozen legacy allowlist with prompt-prefix recognition bounded to prompt-type entries. Positive fixtures: all 17 shipped Cursor entries plus the Codex entry. Negative fixtures: all 32 legacy-generation paths plus the herdr entries.
- [ ] Deterministic pairing per Decision 13: exact-template pass, the closed signature-to-ID map, then identity-stem match for mutated own entries (stems declared per catalog ID as exact normalized shell-token sequences — never substring containment — validated mutually non-overlapping with all signatures by a build test; zero stems → foreign, multiple stems → integrity error); duplicate-owned entries converge to one; owned-but-unpaired entries are retired generations and removed. The weakened-enforcement case (`loaf check --hook check-secrets --advisory`) must pair and converge, not orphan; the boundary case (`--hook check-secrets-disabled`) must stay foreign.
- [ ] Installer integration for the trusted-executable path record: write the current resolved path at install/upgrade via TASK-001's accessor, preserving previously recorded paths per target. Tests: relocation records the new path and keeps the old, records stay target-isolated, and an entry quoting the previous path is still recognized.
- [ ] Reconcile plan: per event, compute `add`/`update`/`remove`/`absorb` actions for Loaf entries against catalog + enablement records; non-Loaf entries never classified or mutated beyond the predicate returning false.
- [ ] Per-target advisory lock per Decision 10 held from state read through file publication for reconciles and verb reprojections; contention fails actionably; interleaving tests cover the verb-versus-upgrade races.
- [ ] Cohort-restricted run-once absorption: gate on the absorption marker, detect prior installs per Decision 7, restrict to the prior version's cohort, write records + marker via TASK-001's transactional accessor, then converge. Cover the full matrix: fresh install, no-manifest legacy upgrade, normal upgrade, repeat upgrade, reinstall, downgrade-re-upgrade, and old-hook-deleted-plus-new-hook-introduced (only the former absorbs).
- [ ] Integrity preconditions fail closed preserving the file: malformed JSON, non-object top level, unsupported structural shape, symlink/non-regular destination, I/O error, concurrent modification between read and atomic write.
- [ ] Apply path: recompute actions from live state at execution time (never replay a stale plan); atomic write; post-verify by re-reading and re-running recognition, not by digest. Crash-injection tests between record commit and projection.
- [ ] Golden tests from the sanitized fixtures: every foreign entry value-identical and order-stable through reconcile; converge idempotent (second run produces zero actions).

## Verification

- `go test ./internal/cli/...` passes; goldens prove foreign-entry value-identity and idempotency; the migration matrix and integrity cases all have named tests, including Windows `commandWindows` parity.
- The live-Codex fixture reconciles to: herdr untouched, `session-start-loaf` absorbed as disabled, no drift-refusal path reachable.
