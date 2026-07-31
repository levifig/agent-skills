---
change: install-upgrade-split
id: TASK-002
title: New `loaf upgrade` command
blocked-by:
  - TASK-001
blocks:
  - TASK-003
  - TASK-004
  - TASK-005
---

# TASK-002 — New `loaf upgrade` command

## Objective

`loaf upgrade` exists as the canonical machine-global refresh: harness content sync from the installed distribution plus deprecation cleanup always; project-surface refresh (fenced sections, symlinks, migrations, MCP-recommendation refresh of `.agents/loaf.json`) only when the detector reports a Loaf repo; legacy-tier detection triggers the explicit "is this a Loaf project?" confirmation (yes → full path, no → global only plus an install offer). The `--dry-run`/`--json` plan surface and `-y` consent semantics move here from install, and the plan builder becomes command-aware: every apply/follow-up command it emits (JSON and human output, titles included) names `loaf upgrade`. `--to <target>` filters to already-installed targets; an uninstalled target errors with a pointer to `loaf install --to <target>`.

## Scope boundaries

**In:** `internal/cli/upgrade.go` with `runUpgrade`, dispatch in `cli.go`, help text, `agent_help.go` entry, flag parsing, the legacy-confirmation prompt with deterministic non-TTY behavior (report required consent, exit cleanly), plan surface reuse from `install_plan.go`, tests covering the matrix rows and non-TTY paths.

**Out:** Removing anything from install (TASK-003), channel detection and the currency advisory (TASK-004), doctor/session-start surfacing (TASK-005). `loaf upgrade` is NOT added to `basicCommandAuthorityPrefixes` (Decision 5) — that is a deliberate omission, not an oversight.

## Context pointers

- Contract: `shape.md` — Observable Workflow, Decisions 1/2/5/6, Planning Contract › Placement and Risks.
- Reused machinery: `installTargetDistribution` (`install_target.go`), `runInstallDeprecationCleanup` (`install_deprecations.go`), plan builder (`install_plan.go`), project-file enforcement (`install.go`, `install_fenced.go`, `install_symlink.go`).
- Open question owned here: plan JSON schema for the moved `--dry-run --json` — keep byte-compatible with the documented consumer flow in `content/skills/loaf-reference/references/maintenance.md`, or version it deliberately and update that doc in TASK-006.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-002 — loaf upgrade command"
export LOAF_DB="$(mktemp -d)/loaf.sqlite"   # isolate all manual smokes
# Read: internal/cli/install.go, install_plan.go, install_deprecations.go, cli.go dispatch
```

## Steps

- [ ] Add `upgrade` dispatch, flag parsing (`--dry-run`, `--json`, `-y`/`--yes`, `--no-yes`, `--to <target>` filter, `--help`), and help text.
- [ ] Global part: detect installed targets, sync from installed distribution, run deprecation cleanup with existing `-y` consent semantics, stamp `.loaf-version`. `--to` narrows to installed targets only; uninstalled target → error naming `loaf install --to`.
- [ ] Project part gated on detector tier: `authoritative`/`strong` proceed (print basis); `legacy` prompts for confirmation; `none` skips with a one-line note. Includes the MCP-recommendation refresh (`.agents/loaf.json`) — it is a project write and never runs outside the gate.
- [ ] Move the dry-run plan surface; make the plan builder command-aware (`install_plan.go` doc strings, follow-up commands, and human titles emit `loaf upgrade`); decide and document the schema-compat call.
- [ ] Tests: each matrix row, legacy prompt yes/no, non-TTY consent reporting, dry-run byte-stability, `--to` filter/error cases, and a plan assertion that no emitted command contains the removed flag (V6's in-process twin).

## Verification

- `go run ./cmd/loaf upgrade --help` exits 0 and names the global/project split.
- `LOAF_DB=$(mktemp -d)/t.sqlite go run ./cmd/loaf upgrade --dry-run --json` from a temp cwd emits a plan without writing anything.
- `go test ./...` green.
