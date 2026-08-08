---
change: hooks-entry-reconciliation
id: TASK-004
title: Retire the hook-projection kind and rework config check
blocked-by:
  - TASK-002
---

# TASK-004 — Retire the hook-projection kind and rework `config check`

## Objective

The `hook-projection` artifact kind is gone — digest and drift-refusal code deleted while integrity preconditions stay — installed manifests tolerate and drop the obsolete rows, the plan surface speaks per-entry actions, and `loaf config check` diagnoses hooks with enablement-aware states.

## Scope boundaries

**In:** Deleting `targetHookProjectionDigest`, `planHookProjectionRefusal`, `targetHookProjectionIsEmpty`, `mergeHookFiles`/`mergeCodexHookFiles` and their call sites; installed-manifest reader dropping `hook-projection` rows on next write (after the absorption marker exists — never as the absorption gate); plan output wording; `config check` five-state hook diagnosis with `--fix` routed through the reconciler; dist/build parity test updates.

**Out:** The reconciler and catalog (TASK-002); `hook-file`, `instruction`, and `plugin` artifact kinds, which keep their existing semantics; the integrity preconditions, which must survive this deletion pass intact.

## Context pointers

- Contract: `shape.md` — Decisions 8, 10; Planning Contract → `loaf config check`, Approach; Cut (drift refusals only).
- Current call sites: `internal/cli/build_manifest.go`, `install_plan.go`, `install_target.go`, `config.go` (hook diagnosis around its absent-command reporting), and the refusal/conflict tests named for them.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-004 — retire hook-projection kind, rework config check"
# grep for "hook-projection" across internal/ and tests to enumerate every deletion site before editing.
```

## Steps

- [ ] Delete the drift-refusal and digest helpers and their branches; no code path may compute a hooks-file digest or emit a drift conflict. Integrity preconditions (malformed JSON, unsupported shape, symlink, I/O, concurrent modification) remain and keep their tests.
- [ ] Manifest reader: obsolete `hook-projection` rows are ignored on read and absent after the next write; row removal is sequenced after the absorption marker is durable (crash between them is covered by an injected-failure test).
- [ ] Plan surface: hook-file work reports the reconciler's per-entry actions; drift-conflict wording for hook files removed from fixtures and snapshots.
- [ ] `config check`: five states — disabled-and-correctly-absent (healthy), enabled-and-in-sync (healthy), enabled-but-stale (present but differing from the catalog template; needs reconcile), enabled-and-missing (needs reconcile), disabled-but-present (needs reprojection); foreign/unknown never reported; `--fix` converges through the reconciler, no private refresh path.
- [ ] Update dist/build parity tests for the catalog and removed digest fields.

## Verification

- `go test ./...` and `loaf build` pass with zero references to the retired kind outside historical docs and the tolerated-row reader.
- A disabled hook reports healthy-absent in `config check`; an enabled-and-missing hook is fixed by `--fix` through the reconciler.
