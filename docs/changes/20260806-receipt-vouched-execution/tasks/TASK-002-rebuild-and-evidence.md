---
change: receipt-vouched-execution
id: TASK-002
title: Rebuild and evidence re-record
blocked-by:
  - TASK-001
---

# TASK-002 — Rebuild and evidence re-record

## Objective

The tracked binary and artifacts reflect TASK-001's gate change, the capability evidence is fresh against that binary, and the changelog carries the fix into the 0.2.20 section.

## Scope boundaries

**In:** rebuilt `bin/`, `dist/`, `plugins/` trees; capability-evidence receipts; `CHANGELOG.md` `[Unreleased]`; this Change's `change.json` `target_release`.

**Out:** any further Go changes (TASK-001 owns the code), the 0.2.20 ceremony itself (reset Change's runbook).

## Context pointers

- Contract: `shape.md` — Planning Contract (risks, self-application sequencing)
- Precedent: the reset Change's evidence re-record commit (`d4138ab2` pre-squash; see docs/changes/20260806-versioning-reset/research/) and its three-commit close-out lesson
- Evidence gate: `internal/cli/release_dry_run.go:504-516`

## Acquisition

```bash
loaf journal log "skill(implement): TASK-002 — rebuild, evidence re-record, 0.2.20 readiness"
```

## Steps

- [x] Stamp `target_release: "0.2.20"` in this Change's `change.json`
- [ ] `npm run build`; commit the rebuilt trees with the version-stamp-only diff check
- [ ] Re-record capability evidence against the rebuilt binary; commit
- [ ] Add the `[Unreleased]` changelog line for the gate fix
- [ ] After the final flip: run `loaf change verify docs/changes/20260806-receipt-vouched-execution` and land the receipt as the branch's last, content-free commit

## Verification

- `npm run test` exits 0 (V2)
- `loaf change check docs/changes/20260806-receipt-vouched-execution` reports state verified after the receipt commit
- `loaf release --dry-run --base HEAD` (repo binary) passes its evidence gate on the branch
