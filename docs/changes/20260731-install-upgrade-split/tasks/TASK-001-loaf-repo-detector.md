---
change: install-upgrade-split
id: TASK-001
title: Tiered Loaf-repo detector
blocks:
  - TASK-002
  - TASK-003
---

# TASK-001 — Tiered Loaf-repo detector

## Objective

A non-prompting detection API that answers "is this directory a Loaf-powered repo?" with a tier and an evidence basis: `authoritative` (SQLite project record for the resolved root), `strong` (fenced `AGENTS.md` marker or `.agents/loaf.json`), `legacy` (deprecated `.agents/` folders such as `specs/`, `drafts/`, `sessions/` with no stronger signal), or `none`.

## Scope boundaries

**In:** New detector file in `internal/cli` (working name `loaf_repo_detection.go`), state-DB project lookup, file probes, tier + basis result type, unit tests per tier including mixed-signal cases.

**Out:** Any prompting or command wiring — the deploy-consent and legacy-confirmation prompts belong to TASK-002/TASK-003. No changes to `internal/project` root resolution semantics.

## Context pointers

- Contract: `shape.md` — Decisions 1, Planning Contract › Placement.
- Existing probes to reuse: fenced marker constants in `install_fenced.go`, `.agents/loaf.json` path in `install_mcp.go`, project identity in `internal/project` and `internal/state`.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-001 — tiered Loaf-repo detector"
# Read: internal/cli/install_fenced.go, internal/cli/install_mcp.go, internal/project/project.go, internal/state (project lookup)
```

## Steps

- [x] Define the tier type and detection result (tier + human-readable basis strings) in a new `internal/cli` file.
- [x] Implement probes: SQLite project record for the resolved root, fenced marker in root `AGENTS.md`, `.agents/loaf.json`, legacy `.agents/` subfolders.
- [x] Resolution order returns the strongest tier with all matched bases; detector never prompts and never writes.
- [x] Unit tests: one per tier, mixed signals resolve to the strongest, moved-repo case (fenced marker present, no SQLite record) resolves `strong`.

## Verification

- `go test ./internal/cli/ -run Detect` (or the chosen test name) passes; `go test ./...` stays green.
- Detector is pure read: running it against a temp dir leaves the dir byte-identical.
