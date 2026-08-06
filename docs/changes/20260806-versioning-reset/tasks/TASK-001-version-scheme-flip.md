---
change: versioning-reset
id: TASK-001
title: Version scheme flip
---

# TASK-001 — Version scheme flip

## Objective

The canonical version becomes `0.2.20` and every stamped surface moves with it in one commit, keeping the release "version files" consistent as `resolveReleaseSnapshot` demands.

## Scope boundaries

**In:** `package.json`, `package-lock.json`, and everything a single `loaf build` regenerates (`dist/`, `plugins/`, `.claude-plugin/marketplace.json`).

**Out:** dev-build stamping (TASK-002), release-pipeline code (TASK-003), CHANGELOG and prose citations (TASK-004).

## Context pointers

- Contract: `shape.md` — Approach, Decisions 6 and 9, Risks (mechanical-diff review note)
- Version source: `package.json:3`; runtime read `internal/cli/version.go:119`; build injection `internal/cli/build_codex.go:549`
- Second version file: `.claude-plugin/marketplace.json` via `internal/cli/release_dry_run.go:848`

## Acquisition

```bash
loaf journal log "skill(implement): TASK-001 — version scheme flip to 0.2.20"
```

## Steps

- [x] Set `"version": "0.2.20"` in `package.json` and refresh `package-lock.json` (stale at `2.0.0-alpha.17`; no dependency changes — version fields only)
- [x] Run `npm run build` so every stamped artifact regenerates; confirm the diff touches only version stamps
- [x] Run the suite; commit as one flip commit

## Verification

- `node -p "require('./package.json').version"` → `0.2.20` (V1)
- `node -p "require('./.claude-plugin/marketplace.json').metadata.version"` → `0.2.20` (V2)
- `npm run test` exits 0 (V3)
- `git diff --stat` of the flip commit shows only `package.json`, `package-lock.json`, and generated trees
