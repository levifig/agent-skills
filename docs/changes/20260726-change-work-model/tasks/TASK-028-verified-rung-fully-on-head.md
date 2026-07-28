---
change: change-work-model
id: TASK-028
title: The verified rung reads HEAD completely, at one pinned commit
---

# TASK-028 — The verified rung reads HEAD completely, at one pinned commit

## Objective

Close O6-1 and the coherence notes behind it: the rung's structural guard reads HEAD but its receipt check receives the working-tree node (`change_state.go:45` — on-disk `shape.md` drives the criteria digest, the working-tree folder drives the HEAD receipt lookup), so a dirty tree still moves the verdict. And every evidence read resolves the symbolic ref `HEAD` at its own git invocation, so a commit landing mid-derivation can split node reads from task reads. After this task the evidence path reads committed content exclusively, all of it at one SHA resolved once per derivation.

## Scope boundaries

**In:** `internal/cli/change_state.go` (pass the HEAD node the guard already loads into `changeReceiptStatus`; thread `outputCommand` instead of the package-level `changeEvidenceGitOutput` — the seam inconsistency); the evidence-path entry points in `internal/cli/change_release_gate.go` and `internal/cli/change_verify.go` insofar as pinning requires resolving `HEAD` to a SHA once and passing the ref down; fixtures in `internal/cli/change_state_test.go`.

**Out:** `check`'s working-tree semantics. The `complete`/`executing` rungs' working-tree derivation (documented in TASK-030, unchanged here — Decision 15 keeps checkboxes non-gating). Do not touch `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, `package.json`, or `docs/changes/20260710-*`.

## Context pointers

- Round-6 board `reports/20260728-143458-review-implementation-round-6-opus.html`, O6-1 and the pinned-SHA and seam notes.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-028 — verified rung fully on HEAD"
```

## Steps

- [ ] The rung's receipt check receives the HEAD node (content and folder) — an uncommitted criteria edit or folder rename changes nothing on the rung; fixture proves both directions.
- [ ] Each evidence derivation (gate preflight; state rung) resolves `HEAD` to a SHA once and reads every input — nodes, task files, receipt — at that SHA; a commit landing mid-derivation cannot split the inputs.
- [ ] The structural guard uses the threaded `outputCommand`; the package-level default remains only as the fallback when nil.

## Verification

- `go test ./internal/cli/ -run 'ChangeState|ReleaseCohortGate' -count=1` green including the dirty-criteria and rename fixtures.
