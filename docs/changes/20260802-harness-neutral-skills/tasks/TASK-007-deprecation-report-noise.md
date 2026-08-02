---
change: harness-neutral-skills
id: TASK-007
title: Deprecation report noise
---

# TASK-007 — Deprecation report noise

## Objective

The deprecation report says something only when there is something to say. Retirements already absent go unmentioned, retired paths Loaf does not own stop generating a warning on every run forever, expired entries age out, and the `gemini` entry is gone.

## Scope boundaries

**In:** `internal/cli/install_deprecations.go` reporting behaviour and the `gemini` retired-target entry in `config/deprecations.json`.

**Out:** The cleanup semantics themselves — what gets removed and under what ownership proof is unchanged; that hardening is TASK-006. Any runtime state: no persistent "already reported" acknowledgement, and no version-comparison engine that prunes entries at runtime. Manifest hygiene here is authored — expired entries are deleted from the file by hand, because a self-pruning manifest is a different, stateful feature that would contradict this task's reporting-only boundary.

## Context pointers

- Contract: `shape.md` — Scope / rider
- Today's output on the dogfooding machine: five `absent ...` lines for retirements with nothing to report, plus a persistent yellow `skip-unmarked target gemini at ~/.gemini` for a real Google Gemini CLI config directory Loaf will never own
- The `gemini` entry carries `since: v2.0.0-pre.20260614235428` and no window, so it defaults to one-release and expired long ago
- This unit is independent of the rest of the Change and can land at any point

## Acquisition

```bash
loaf journal log "skill(implement): TASK-007 — deprecation report noise"
```

## Steps

- [ ] Drop the `missing` action from the report entirely — a retirement with nothing present is a no-op, not news
- [ ] Stop reporting retired paths Loaf does not own; no state, so the choice is report always or never, and never is right for a path Loaf will never touch
- [ ] Delete entries whose window has already expired from `config/deprecations.json`, including `gemini`
- [ ] Add `TestDeprecationReport`, covering all three behaviours plus the case that must still report

## Verification

- `go test ./internal/cli/ -run TestDeprecationReport` passes: absent retirements omitted, unowned paths omitted, a retirement with something genuinely present still reported, and no expired entry remains in the manifest
- `loaf upgrade --dry-run` on the dogfooding machine prints no `absent` lines and no `~/.gemini` warning
