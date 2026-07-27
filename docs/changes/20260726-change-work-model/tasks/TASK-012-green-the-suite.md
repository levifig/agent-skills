---
change: change-work-model
id: TASK-012
title: Green the suite against the rebuilt binary
---

# TASK-012 — Green the suite against the rebuilt binary

## Objective

Return `go test ./...` to green. Fifty-seven subtests across ten parent tests fail on one cause: the installed-smoke capability receipts pin the native binary SHA-256 this branch replaced when it rebuilt. Do this first — until it lands, the suite cannot serve as the signal for any other task in this batch.

## Scope boundaries

**In:** re-recording the installed-smoke evidence under `docs/changes/20260710-journal-reliability-foundation/research/` against the current binary, using the existing capability runner scripts in `cli/scripts/`.

**Out:** Any change to the capability contract, its validation rules, or the hash-binding design. This is bookkeeping the pattern was built to demand — do not weaken the check to make it pass, and do not hand-edit hashes without re-running the smokes that produce them.

## Context pointers

- `docs/changes/20260726-change-work-model/shape.md` — Risks: "Receipt expiry via criteria-digest drift is a known canary pattern that fired four times during capability work."
- Review board finding B3.
- The three receipts recording the stale hash: `claude-code-2.1.218-plugin-startup-smoke.json`, `codex-0.145.0-isolated-startup-smoke.json`, `opencode-1.18.4-isolated-request-smoke.json`.
- `cli/scripts/smoke-claude-code-startup.mjs`, `smoke-codex-startup.mjs`, `smoke-opencode-request-context.mjs`, `capability-runner-utils.mjs`.
- Prior precedent: the version-agnostic-managed-block change resolved this same blocker with a rebuild plus a receipt re-record.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-012 — green the suite"
npm run build:go
shasum -a 256 bin/native/darwin-arm64/loaf     # the hash the receipts must carry
go test ./internal/cli/ -run TargetCapability  # confirm the failure before and after
```

## Steps

- [x] Re-run each installed-smoke capability runner against the rebuilt binary so the receipts record the current SHA-256 rather than being edited by hand.
- [x] `bin/native/` and `plugins/loaf/bin/native/` carry the same binary as the receipts attest.
- [ ] `go test ./...` passes with zero failures.
- [x] If any smoke cannot be re-run on this machine (a harness not installed locally), say so explicitly in the commit body and name what CI must confirm — a skipped smoke recorded as passing is the failure this pattern exists to prevent.

## Verification

- `go test ./...` green.
- `npm run build` reports zero drift.
- `loaf check --hook render-drift` and `--hook artifact-names` pass.
