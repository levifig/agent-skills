---
change: install-upgrade-split
id: TASK-003
title: Retarget `loaf install` to onboarding
blocked-by:
  - TASK-001
  - TASK-002
blocks:
  - TASK-006
---

# TASK-003 — Retarget `loaf install` to onboarding

## Objective

`loaf install` is onboarding only: outside a Loaf repo it asks before scaffolding project files ("Deploy Loaf to this folder?"); inside one it no-ops the project part with a `loaf upgrade` suggestion; `--to <target>` remains valid for onboarding a net-new harness in either case; `--upgrade` is a hard error naming `loaf upgrade`. The consent/no-op boundary covers **every** project write: `AGENTS.md`, fenced sections, symlinks, and the MCP-recommendation writes to `.agents/loaf.json` (`runInstall` currently fires those unconditionally at `install.go:95` and `install.go:167` — both call sites go behind the gate).

## Scope boundaries

**In:** `install.go` flag parsing and flow branching on the detector, the deploy-consent prompt with deterministic non-TTY behavior, the `--upgrade` tombstone error, help-text updates, tests for all four matrix cells plus the `--to` carve-out.

**Out:** The upgrade command itself (TASK-002), doc/skill references to the removed flag (TASK-006). `config check --fix`'s direct use of `installTargetDistribution` must keep working unchanged (Planning Contract › Risks).

## Context pointers

- Contract: `shape.md` — Observable Workflow, Decision 2, Rabbit Holes (prompt hangs).
- Current behavior being replaced: unconditional `AGENTS.md` creation in `install_symlink.go` (`ensureRootInstallAgentsFile`) and fenced-section writes via `enforceInstallProjectFiles` (`install.go`).

## Acquisition

```bash
loaf journal log "skill(implement): TASK-003 — install retargeting"
export LOAF_DB="$(mktemp -d)/loaf.sqlite"
# Read: internal/cli/install.go, install_symlink.go, install_fenced.go, config.go (shared installer path)
```

## Steps

- [ ] `--upgrade` parses to a hard error: exit 1, message names `loaf upgrade`. Word the tombstone so it never contains the literal phrase `install --upgrade` (e.g. "the `--upgrade` flag was removed from install; use `loaf upgrade`") — V5's sweep grep then needs no source exception.
- [ ] Branch project enforcement on the detector: outside → consent prompt before any project write (files **and** MCP `.agents/loaf.json` writes); inside → skip all of it with the suggestion; `--to <target>` performs the harness-global install in both cases.
- [ ] Non-TTY without `-y`: report that deploy consent is required, exit cleanly, create nothing — no files, no `.agents/`.
- [ ] Update install help text to describe onboarding semantics.
- [ ] Tests: consent accepted/declined, no-op suggestion, net-new `--to`, tombstone error, non-TTY, `config check --fix` regression.

## Verification

- `go run ./cmd/loaf install --upgrade --dry-run` exits 1 and mentions `loaf upgrade` (V3); the error text does not contain the literal `install --upgrade`.
- Running install in a temp dir without consent leaves the dir empty of `AGENTS.md`, `.claude/`, and `.agents/` (V8 pins this from a real invocation).
- `go test ./...` green.
