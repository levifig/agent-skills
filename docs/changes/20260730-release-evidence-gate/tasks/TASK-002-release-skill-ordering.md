---
change: release-evidence-gate
id: TASK-002
title: Release-skill ordering rule for evidence re-record
blocked-by:
  - TASK-001
---

# TASK-002 — Release-skill ordering rule for evidence re-record

## Objective

`content/skills/release/SKILL.md` states the ordering rule as a hard step: capability-evidence re-recording happens AFTER the `--pre-merge` artifact rebuild, on the release branch, before pushing the release PR — and notes that `loaf release` now enforces it mechanically. The rebuilt `dist/` and `plugins/` skill copies land in the same commit.

## Scope boundaries

**In:** `content/skills/release/SKILL.md` (Step 4 area + the incident narrative around lines 196–213), rebuilt distributable copies of the release skill under `dist/` and `plugins/`.

**Out:** gate code (TASK-001), other skills, hooks.yaml.

## Context pointers

- Contract: `shape.md` — Scope In (last bullet), Durable Outputs
- Current skill text: `content/skills/release/SKILL.md` lines 186–213 (preparation, artifact verification, alpha.16 narrative, PR-CI rationale)
- Why every bump stales the OpenCode receipt: `dist/opencode/plugins/hooks.ts` embeds `@version` in its generated header

## Acquisition

```bash
loaf journal log "skill(implement): TASK-002 — release skill evidence-ordering rule"
# Load: content/skills/release/SKILL.md
```

## Steps

- [x] Add the ordering rule to the preparation step: re-record after the rebuild (every bump stales the OpenCode receipt; Go changes stale all binary-pinned receipts), verify with the evidence contract tests, commit receipts with any artifact drift on the release branch
- [x] Extend the incident narrative: alpha.17 re-recorded before the bump and shipped zero assets; the release command now gates on evidence freshness after the rebuild
- [x] `loaf build` and commit rebuilt `dist/`/`plugins/` release-skill copies with the source change

## Verification

- `go test ./cmd/loaf -run TestPlanningVocabularyConverged` — exit 0 (no pinned-vocabulary regressions)
- `npm run build && git diff --exit-code -- dist plugins` — exit 0 after committing
