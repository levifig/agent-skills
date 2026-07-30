---
change: receipt-tree-binding
id: TASK-001
title: Scope digest and the evidence boundary
blocks:
  - TASK-002
  - TASK-003
---

# TASK-001 — Scope digest and the evidence boundary

## Objective

The canonical content digest exists and is deterministic on every machine: `scopeDigest(tree, exclusions)` built from `git ls-tree -r -z --full-tree <pinned-sha>` entries (`path\0mode\0oid`), byte-sorted, excluded paths dropped by byte-exact tree-path match, domain header `loaf/change-evidence-digest\nv1\n`, SHA-256. The exclusion set ships as one exported boundary constant — `docs/changes/*/receipts/**`, `docs/changes/*/reports/**`, and the release-metadata allowlist (version files, `CHANGELOG.md`, `dist/**`, `plugins/**`, `bin/**`) — that the promotion change will import for its designation check. Per-top-level-directory sub-digests (`scope_sections`) come from the same serialization.

## Scope boundaries

**In:** New digest code beside `internal/cli/change_verify.go` (or a sibling file), the exported boundary constant, sub-digest sections, unit tests.

**Out:** Receipt schema fields and the verify write path (TASK-002); the freshness predicate (TASK-003); any gate or message changes (TASK-003/004); `git mktree` in any form (rejected — writes objects).

## Context pointers

- Contract: `shape.md` — Planning Contract → Digest construction; Decisions 1, 2, 8.
- Council board: `reports/20260728-222942-council-receipt-freshness.html` — Git Internals card for the exact plumbing rationale; Correctness card for the predicate the digest feeds.
- Pinning precedent: `internal/cli/change_state.go:76-115` (`pinEvidenceAtHEAD`).

## Acquisition

```bash
loaf journal log "skill(implement): TASK-001 — scope digest and evidence boundary"
# Read internal/cli/change_verify.go and change_state.go pinning before writing.
```

## Steps

- [x] Implement `scopeDigest` per the pinned serialization; entries from `ls-tree` only, never the filesystem.
- [x] Export the evidence-boundary constant with the three mask groups; settle the glob grammar (component-anchored vs prefix) and record the choice here for the ADR.
- [x] Emit `scope_sections`: per-top-level-directory sub-digests from the same filtered, sorted entry stream.
- [x] Tests under `TestChangeScopeDigest`: identical trees digest identically regardless of `core.quotePath`; sort independence from traversal order; mode change (100644→100755) changes the digest; excluded paths never participate; a receipts-only or reports-only commit leaves the digest unchanged; case-sensitive mask matching.

## Verification

- `go test ./internal/cli -run 'TestChangeScopeDigest' -count=1` green.
- The boundary constant is importable without cycle from the release-gate package paths the promotion change will touch.


## Glob grammar (pinned for ADR)

Component-anchored, byte-exact, case-sensitive matching over forward-slash tree paths from `git ls-tree`:
- literal segments match exactly
- `*` matches exactly one path segment
- trailing `**` matches zero or more remaining segments

Chosen over prefix-match so `dist/**` cannot swallow `distributor/…`, and over filesystem globs so matching stays quotePath/autocrlf immune.
