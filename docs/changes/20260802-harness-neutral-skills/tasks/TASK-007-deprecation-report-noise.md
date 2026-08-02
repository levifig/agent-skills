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

**Out:** The cleanup semantics themselves — what gets removed and under what ownership proof is unchanged. This is a reporting fix, not a behaviour change.

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
- [ ] Stop warning on every run about retired paths Loaf does not own; report once at most, or not at all
- [ ] Age out entries whose window has expired, so the manifest is self-pruning rather than accumulating
- [ ] Remove the `gemini` retired-target entry
- [ ] Add `TestDeprecationReportOmitsAbsent`

## Verification

- `go test ./internal/cli/ -run TestDeprecationReportOmitsAbsent` passes
- `loaf upgrade --dry-run` on the dogfooding machine prints no `absent` lines and no `~/.gemini` warning
- A retirement with something genuinely present still reports it
