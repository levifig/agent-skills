---
change: change-work-model
id: TASK-030
title: The snapshot carries its commit range; dead wrappers go
---

# TASK-030 — The snapshot carries its commit range

## Objective

Close O6-4 and the tidy notes: dry-run and apply re-run `releaseCommitsSince` after snapshot resolution (`release_dry_run.go:244/:397`), so a commit landing during the multi-second preflight yields a changelog describing history the gated bump never judged. The snapshot gains the commit list it was resolved from; every consumer uses it. The dead wrappers the class fix orphaned (`resolveReleaseCandidate`, zero callers; `computeReleaseCandidateVersion`, tests only) are removed, and the ladder's split sources get their one documenting sentence.

## Scope boundaries

**In:** the snapshot struct and its resolution in `internal/cli/change_release_gate.go`; the two consumer sites in `internal/cli/release_dry_run.go`; wrapper removal with test migration to the snapshot API; one sentence in `docs/knowledge/work-model.md`'s Derived States section stating that `complete`/`executing` derive from the working tree while `verified` derives from committed HEAD (Decision 15 keeps checkboxes non-gating, so the split is deliberate); fixtures.

**Out:** Bump/candidate semantics (settled). Post-merge (TASK-031's file). Do not touch `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, `package.json`.

## Context pointers

- Round-6 board O6-4 and the dead-wrapper and ladder-split notes.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-030 — snapshot carries its history"
```

## Steps

- [x] The snapshot carries the commits it was resolved from; preview and apply consume that list — `releaseCommitsSince` runs exactly once per invocation.
- [x] Fixture: a commit landing after resolution appears in neither the changelog nor the bump — both describe the snapshot's history.
- [x] Dead wrappers removed; their tests assert through the snapshot API.
- [x] The work-model.md sentence lands; no other doc changes.

## Verification

- `go test ./internal/cli/ -run 'Release' -count=1` green including the changelog-coherence fixture.
