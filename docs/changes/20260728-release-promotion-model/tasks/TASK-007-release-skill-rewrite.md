---
change: release-promotion-model
id: TASK-007
title: Release skill rewrite for the channel model
blocked-by:
  - TASK-002
  - TASK-004
  - TASK-008
---

# TASK-007 — Release skill rewrite for the channel model

## Objective

`content/skills/release/SKILL.md` documents the CLI that actually ships: channel state in context detection, the ladder and its ceremonies (`--bump` iterates, `--promote` advances, `--channel` asserts), the `/promote` flow with the designation commit and rollup judgment, channel suggestion via AskUserQuestion fed by mechanical signals (`loaf change list --target`, receipt states, time since last release), and the post-merge steps that today's skill omits entirely — including the cohort gate, `target_release`, and receipts, none of which the current skill mentions.

## Scope boundaries

**In:** `content/skills/release/SKILL.md` and its references/templates; `content/skills/loaf-reference/SKILL.md` release lines; rebuilt outputs under `plugins/` and `dist/` via `npm run build`.

**Out:** Go code; CI; any other skill beyond the loaf-reference release lines. Prose must not reimplement CLI logic — describe judgment and ceremony, point at commands.

## Context pointers

- Contract: `shape.md` — Planning Contract → Skill; Decisions 3, 4, 9; Observable Workflow for the canonical command flows.
- Current skill: `content/skills/release/SKILL.md` (291 lines, predates the gate); reaction artifact from TASK-004 in `research/` for the rollup ceremony's shape.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-007 — release skill rewrite"
# Read content/skills/release/SKILL.md and the shipped release help (loaf release -h) before writing.
```

## Steps

- [ ] Rewrite the skill: context detection includes current channel and cohort state; steps cover cycle entry, iteration, advance, rc cut (gate fires), promotion ceremony (designation commit, rollup judgment against seed material, mechanical validation), post-merge finalize, and distribution expectations per channel.
- [ ] Channel suggestion pattern: mechanical signals gathered first, human choice via AskUserQuestion with the agent's recommendation — never an automatic channel decision.
- [ ] Release-notes authoring discipline: every change writes `release-notes.md` while context is hot; the skill teaches the fragment's register (user-facing, grouped bullets) and the exact no-impact form TASK-008 defines — a note whose entire content is `No user-facing impact.` — as the escape's one carrier (never a change.json field or PR-body text; the skill teaches, never redefines).
- [ ] Update `loaf-reference` release lines; no flag or command appears that the shipped CLI does not accept.
- [ ] `npm run build`; commit rebuilt `plugins/` and `dist/` outputs with the source.

## Verification

- Every command and flag named in the skill exists in `loaf release -h` output (H2 review).
- `loaf build` and `npm run build` succeed; rebuilt outputs committed.
- The journal self-logging instruction survives in the rewritten skill's Critical Rules.
