---
change: release-promotion-model
id: TASK-005
title: Per-channel changelog guardrails
blocked-by:
  - TASK-002
  - TASK-004
  - TASK-008
---

# TASK-005 — Per-channel changelog guardrails

## Objective

Post-merge guardrails 5 and 6 key on the prepared version's rung: alpha and beta unchanged (version files + CHANGELOG section required); rc requires the release commit to stay within the metadata allowlist minus CHANGELOG (version files + regenerated build outputs), skips the changelog-section requirement, and GitHub Release notes fall back to an auto-generated commit list for the range; promotion (stable prepared from a cycle) validates via TASK-004's rollup checks. The rc-cut preflight warns when the range since the last rc or beta contains `feat:` commits.

## Scope boundaries

**In:** `internal/cli/release_post_merge.go` guardrails 5/6 (`:209-300`), rc notes fallback in the gh-release action, the feat-during-rc warning in the rc-cut preflight, fixtures.

**Out:** Rollup validation internals (TASK-004 — call them); CI workflow (TASK-006); the `workflow-pre-pr` and `validate-push` hooks unless a fixture proves they block a legal rc flow (if so, the minimal rung-aware carve-out only).

## Context pointers

- Contract: `shape.md` — Planning Contract → Per-channel guardrails; Decision 3.
- Guardrail 5: `release_post_merge.go:209-240`; guardrail 6: `:242-300`; gh notes: `:357-418`.
- The CI-green-at-HEAD preflight helper (Decision 11) is owned by TASK-004 as one shared check for rc cut and promotion — consume it, never reimplement it. The alpha/beta changelog sections this task validates are produced by TASK-008's notes projection.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-005 — per-channel guardrails"
# Read internal/cli/release_post_merge.go and its test file end to end.
```

## Steps

- [ ] Key guardrails 5/6 on the prepared rung; rc path: metadata-allowlist diff without CHANGELOG, no changelog-section requirement.
- [ ] rc GitHub Release notes: auto-generated commit list for the range since the previous tag.
- [ ] feat-during-rc preflight warning ("new development during rc; consider cutting a beta") on rc cuts whose range contains `feat:` commits.
- [ ] Negative fixtures per rung: rc commit touching CHANGELOG blocks; beta missing its section blocks; promotion missing collapse blocks (delegating to TASK-004 validators).

## Verification

- `go test ./internal/cli -run 'TestReleasePostMergeChannel' -count=1` green.
- Existing post-merge guardrail suite unregressed.
