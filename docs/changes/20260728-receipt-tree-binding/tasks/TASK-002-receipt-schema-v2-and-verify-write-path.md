---
change: receipt-tree-binding
id: TASK-002
title: Receipt schema v2 and the verify write path
blocked-by:
  - TASK-001
blocks:
  - TASK-003
---

# TASK-002 — Receipt schema v2 and the verify write path

## Objective

`loaf change verify` writes `schema_version: 2` receipts bound to content: `scope_digest`, `scope_sections`, `exclusions` (verbatim), `digest_spec`, `verified_root_tree` (provenance, never gating), `tool_version`, toolchain (`go`, `os`, `arch`), `worktree_clean`; `verified_commit` kept as provenance with a non-gating struct comment; `cwd` dropped; `criteria_digest` extended to cover each criterion's text. Verify refuses to run when the tracked worktree differs from HEAD, and criteria run under `bash -c` (no login shell).

## Scope boundaries

**In:** `internal/cli/change_verify.go` write path (`runChangeVerify`, receipt struct, `changeCriteriaDigest`, `runChangeCriterionCommand`), tests.

**Out:** The freshness read path (TASK-003 — do not touch `changeReceiptStatus` here); digest internals (TASK-001 — consume them); gate messages (TASK-004).

## Context pointers

- Contract: `shape.md` — Scope → In; Decisions 5, 6; Planning Contract → Digest construction.
- Dirty-tree false-pass and criteria-text gap: council board, Audit Integrity card.
- Login-shell divergence: council board, Portability/DX card.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-002 — receipt schema v2 and verify write path"
# Read internal/cli/change_verify.go:35-170 and 390-400 before editing.
```

## Steps

- [ ] Extend the receipt struct and writer with the v2 fields; drop `cwd`; comment `verified_commit` as provenance-only.
- [ ] Extend `changeCriteriaDigest` to include criterion text; bump nothing else about its discipline.
- [ ] Refuse a dirty tracked worktree before running any criterion, with the exact message `working tree differs from HEAD; commit before verifying`.
- [ ] Change `bash -lc` to `bash -c` in `runChangeCriterionCommand`.
- [ ] Tests under `TestChangeVerifySchemaV2`: all v2 fields present and correct; no absolute path anywhere in the artifact; dirty tree refused (tracked edits, staged edits; untracked files do not refuse); criteria-text edit changes the digest.

## Verification

- `go test ./internal/cli -run 'TestChangeVerifySchemaV2' -count=1` green.
- A receipt written on this branch round-trips through TASK-003's reader once both land.
