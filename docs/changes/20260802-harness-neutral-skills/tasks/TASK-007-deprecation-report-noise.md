---
change: harness-neutral-skills
id: TASK-007
title: Deprecation report noise
---

# TASK-007 — Deprecation report noise

## Objective

The deprecation report says something only when there is something to say. Retirements already absent go unmentioned, retired paths Loaf does not own stop generating a warning on every run forever, expired entries age out, and Gemini is gone from every live surface.

Gemini is not merely an expired entry. Google discontinued the Gemini CLI in favour of AntiGravity/agy, so the retirement has no future to wait for and no reason to remain quoted in a live manifest.

## Scope boundaries

**In:** `internal/cli/install_deprecations.go` reporting behaviour, the `gemini` retired-target entry in `config/deprecations.json`, and the stale `.gitignore` comment that still lists Gemini among the tracked `dist/` targets.

**Out:** The cleanup semantics themselves — what gets removed and under what ownership proof is unchanged; that hardening is TASK-006. Any runtime state: no persistent "already reported" acknowledgement, and no version-comparison engine that prunes entries at runtime. Manifest hygiene here is authored — expired entries are deleted from the file by hand, because a self-pruning manifest is a different, stateful feature that would contradict this task's reporting-only boundary.

**Out — deliberately, not by omission:** the AI-attribution patterns in `internal/cli/check.go`, which match `gemini` alongside `claude`, `gpt`, `copilot`, `chatgpt`, `anthropic`, and `openai`. Those detect an AI co-author trailer in a commit message; they have nothing to do with Gemini as an install target, and a product being discontinued does not stop that trailer being written. Removing them would silently weaken an enforcement hook.

Also out: every historical record naming Gemini — `.agents/specs/*`, `docs/decisions/ADR-001`, `ADR-002`, `ADR-010`, `docs/reports/*`, `.agents/reports/*`, and `CHANGELOG.md`. They document decisions that were true when made. The ADR log is append-only; when circumstances change the answer is a new ADR that supersedes the old one, never a rewrite of the record.

## Context pointers

- Contract: `shape.md` — Scope / rider
- Today's output on the dogfooding machine: five `absent ...` lines for retirements with nothing to report, plus a persistent yellow `skip-unmarked target gemini at ~/.gemini` for a real Google Gemini CLI config directory Loaf will never own
- The `gemini` entry carries `since: v2.0.0-pre.20260614235428` and no window, so it defaults to one-release and expired long ago
- Gemini is already absent as a *target*, derived rather than grepped: it appears in none of `defaultBuildTargets`, `installValidTargets`, `installDisplayNames`, `config/targets.yaml`, or `content/`. The entry does not register a target; it points a deletion at `${HOME}/.gemini`
- TASK-006 already closed the deletion vector — `inventoryRetiredTargetArtifacts` treats a Gemini home's own settings and history as foreign, and `TestRunnerUpgradePreservesForeignRetiredTargetHomeWithoutReintroducingIt` proves it. Removing the entry is therefore hygiene, not the thing standing between an `upgrade --yes` and a user's files
- Removing it leaves `retired_targets` empty. That is the expected end state, not a defect, and the report must be correct with an empty list
- This unit is independent of the rest of the Change and can land at any point

## Acquisition

```bash
loaf journal log "skill(implement): TASK-007 — deprecation report noise"
```

## Steps

- [x] Drop the `missing` action from the report entirely — a retirement with nothing present is a no-op, not news
- [x] Stop reporting retired paths Loaf does not own; no state, so the choice is report always or never, and never is right for a path Loaf will never touch
- [x] Delete entries whose window has already expired from `config/deprecations.json`, including `gemini`
- [x] Correct the `.gitignore` comment that still names Gemini among the tracked `dist/` targets
- [x] Add `TestDeprecationReport`, covering all three behaviours plus the case that must still report, and an empty `retired_targets` list

## Verification

- `go test ./internal/cli/ -run TestDeprecationReport` passes: absent retirements omitted, unowned paths omitted, a retirement with something genuinely present still reported, an empty `retired_targets` handled without error, and no expired entry remains in the manifest
- `loaf upgrade --dry-run` on the dogfooding machine prints no `absent` lines and no `~/.gemini` warning
- `rg -i gemini` over tracked files returns only historical records, the `check.go` attribution patterns, and rebuilt binaries — no live target, manifest, or config surface

## Landed state

Delivered at `0cb70404`. `missing` and `unmarked` are gone from the action model and from the plan's mirror of it, so dry-run and apply still describe the same run. Every action the ownership hardening added to say Loaf declined to act still prints, each guarded by an identity-bound assertion.

The `gemini` entry is deleted and `retired_targets` is empty. One entry was deliberately **not** deleted: `cli-reference` is expired by arithmetic but still drives leftover cleanup, and nothing in this repository can prove every install has migrated. `relocations` and `externalized_skills` are likewise untouched — the first drives real migration, the second is standing guidance rather than a timed retirement. Ageing an entry out is a judgement about the installed population, which is why this manifest is authored rather than self-pruning.

Worth remembering how the review earned its keep. The regression guard protecting `relocated` asserted the bare substring `relocated`, which the `removed-stale` line satisfies because its text reads `removed stale relocated ...`. The guard would have passed with `relocated` silenced outright. Actions here share vocabulary, so an assertion on one word is not an assertion about the action — bind the identity, and prove the guard fails when the thing it guards is removed.
