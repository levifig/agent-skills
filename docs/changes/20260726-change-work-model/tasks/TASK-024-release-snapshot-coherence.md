---
change: change-work-model
id: TASK-024
title: One immutable release snapshot per invocation
---

# TASK-024 — One immutable release snapshot per invocation

## Objective

Close C5-1 and C5-2 as a class, ending the snapshot-coherence treadmill (bump in round 3, candidate in round 4, the version underneath in round 5): a `loaf release` invocation resolves one immutable snapshot — version-file state, effective bump, candidate — at its start, every consumer receives it, and any actor that writes or tags first asserts the world still matches the snapshot.

## Scope boundaries

**In:** `internal/cli/release.go`, `internal/cli/release_dry_run.go`, `internal/cli/release_post_merge.go`, and `resolveReleaseCandidate`'s home in `internal/cli/change_release_gate.go` (grow it into the snapshot resolver); fixtures in `internal/cli/change_release_gate_test.go` and, if apply-path assertions live there, `internal/cli/release_test.go`.

**Out:** Bump-resolution semantics, gate tiers, receipt logic (all settled). Do not touch `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, `package.json`, or `docs/changes/20260710-*`.

## Context pointers

- Round-5 board `reports/20260728-135031-review-implementation-round-5-codex.html`, C5-1/C5-2 verifications; TASK-020 (the candidate half of this class, already landed).

## Acquisition

```bash
loaf journal log "skill(implement): TASK-024 — release snapshot coherence"
```

## Steps

- [x] A snapshot struct (version-file paths + current version, effective bump, candidate) is resolved exactly once per invocation and threaded to preflight, preview, apply, and the post-merge path — no consumer re-derives any snapshot field.
- [x] Apply asserts the version files still carry the snapshot's current version immediately before writing; drift blocks with a plain message naming both versions and the remedy (re-run release).
- [x] `--post-merge` receives the resolved snapshot from `runRelease` and its guardrails assert against it — the tagged version is the preflighted candidate or the run aborts; the post-merge-specific consistency checks keep their own abort codes.
- [x] Fixtures: a version-file commit landed between preflight and apply blocks with the drift message; the same drift on the post-merge path aborts before tagging; happy paths byte-stable.

## Verification

- `go test ./internal/cli/ -run 'Release' -count=1` green including both drift fixtures; existing candidate and post-merge fixtures untouched and green.
