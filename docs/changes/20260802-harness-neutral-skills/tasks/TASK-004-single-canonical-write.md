---
change: harness-neutral-skills
id: TASK-004
title: Single canonical write
blocked-by:
  - TASK-002
  - TASK-003
blocks:
  - TASK-005
  - TASK-006
---

# TASK-004 — Single canonical write

## Objective

Installation plans and performs exactly one write per skill to `~/.agents/skills`, no matter how many targets are selected or in what order, and reports a shared conflict once rather than once per target.

## Scope boundaries

**In:** `installSkillsDestination` and the four per-target install functions in `internal/cli/install_target.go`, plus the orchestration layers in `internal/cli/install.go` and `internal/cli/install_plan.go` that currently iterate targets and independently plan and sync the shared store.

**Out:** Skill naming — TASK-005. Migration and retirement — TASK-006. Claude Code, which keeps its plugin channel. Any write to `~/.config/agents/skills`: Amp reads it at higher precedence than the canonical store, so anything placed there shadows canonical updates.

## Context pointers

- Contract: `shape.md` — Decisions 1, 5, 6; Planning Contract / Placement and Risks
- ADR-018 already decided the destination from primary documentation; this task implements one writer for it, not a new destination
- All four targets are "universal" in the `vercel-labs/skills` sense — they read the canonical store directly and need no bridge
- The current defect is two-part: divergent trees, now fixed upstream, and four target-scoped writers sharing one destination. A target's tree is admitted when its digest still matches the previous manifest and is then republished as an update (`install_target.go:357`–`439`)

## Acquisition

```bash
loaf journal log "skill(implement): TASK-004 — single canonical write"
```

## Steps

- [x] Collapse skill installation to one canonical write, so selecting several targets does not repeat it
- [x] Fix the planning layer too, so dry-run plans the write once and reports a shared conflict once
- [x] Confirm no target writes skills anywhere else, and that the test-only `AmpSkillsDir` override remains test-only
- [x] Add `TestSingleCanonicalWrite`, asserting one planned write, one executed write, and one conflict report across several target orderings

## Verification

- `go test ./internal/cli/ -run TestSingleCanonicalWrite` passes
- A dry run against a sandbox HOME with a foreign `orchestration` reports that conflict once, not four times. Note `--dry-run` is a `loaf upgrade` flag, not `loaf install`
- Installing targets in more than one order leaves identical bytes
