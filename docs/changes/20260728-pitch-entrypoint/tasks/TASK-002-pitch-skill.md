---
change: pitch-entrypoint
id: TASK-002
title: Pitch skill — problem-discovery ceremony at both scales
blocked-by:
  - TASK-001
---

# TASK-002 — Pitch skill

## Objective

The `pitch` skill exists and ships: SKILL.md with the both-scale ceremony, an interview-guide reference, the human-entry guard (Claude Code sidecar `disable-model-invocation: true` plus a behavioral Critical Rule for every target), and presence in built targets. The interview design has been dogfooded against a real intake item before the text finalizes (H1).

## Scope boundaries

**In:** `content/skills/pitch/` (SKILL.md, `SKILL.claude-code.yaml`, `references/interview-guide.md`, templates if needed), rebuilt `plugins/` and `dist/`. No `config/hooks.yaml` edits — skills are auto-discovered at build time and pitch ships no hooks (Decision 10).

**Out:** bootstrap series-prep (TASK-003), seam edits to shape/triage/explore/brainstorm (TASK-004), guidance sweep (TASK-005). Pitch never writes `shape.md`, never seeds `tasks/`, never opens PRs (Cut list).

## Context pointers

- Contract: `shape.md` — Hypothesis, Scope (skill bullets), Observable Workflow, Rabbit Holes (The Form; third interview idiom), Decisions 1, 2, 4, 9, Open Questions ([UK] interview feel).
- Interview mechanics to borrow: `content/skills/shape/references/grilling.md` (one question at a time, recommendation-first, architectural-impact ordering) and `content/skills/bootstrap/references/interview-guide.md` (anti-patterns; Excavation/Sharpening framing).
- Skill authoring conventions: root `AGENTS.md` (read via `.claude/CLAUDE.md` on Claude Code) — SKILL.md structure, description best practices (two-tier, negative routing, "Produces..."), sidecar fields.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-002 — pitch skill authoring"
# Read shape's grilling reference, bootstrap's interview guide, and two workflow SKILL.md files (shape, triage) for structure and tone
```

## Conventions

Markdown paragraphs are single lines — never hard-wrap. Follow the standard SKILL.md section order (Critical Rules, Verification, Quick Reference, Process/Topics); journal self-logging goes in Critical Rules.

## Steps

- [x] Draft `references/interview-guide.md`: problem-discovery dimensions (problem, who has it, current alternatives/competitive landscape, value proposition, constraints), applicability judgment (when competitive analysis or personas apply and when they don't), one-question-at-a-time mechanics with recommendation-first, exit criteria, and the anti-patterns adopted from bootstrap.
- [x] Draft SKILL.md: scale detection, the change-scale ceremony (slug proposal, `loaf change init <slug> --brief`, brief authoring, `target_release` stamping when known, accretion note, then the landing rules of Decision 11 — shape now: create the slug branch and hand to `/shape` for in-place promotion; park targeted: docs-only commit on the default branch; park untargeted: docs-only commit on the slug branch or remain intake state; one commit per capture, every landing preceded by explicit-path `loaf change check <folder> --json` reporting zero violations and captured state plus a direct read-back of the folder's `change.json` confirming the intended target presence or absence; pitch prepares commits and never pushes or opens PRs), the project-scale ceremony (`docs/BRIEF.md` with `source: pitch`, hand to `/bootstrap`), evidence delegation (researcher subagent; `research/` at change scale, inline links at project scale), journal self-logging, description with negative routing (not for solution shaping — use shape; not for queue processing — use triage) and "Produces..." success criteria.
- [x] Write the sidecar (`user-invocable: true`, `disable-model-invocation: true`, `argument-hint: "[idea, problem, or intake item]"`) and state the human-entry guard as a Critical Rule that binds on every target, carrying the literal sentence "Agents never initiate a pitch." so V7's projection grep is stable; Claude Code additionally enforces it mechanically via the sidecar.
- [ ] H1 dogfood — a user-invoked checkpoint, because agents never initiate a pitch: build the candidate first (`npm run build`) and prepare the isolated scratch project (temp directory with `LOAF_DB` pointed at a throwaway database — never this branch or repo) with a fresh session explicitly loaded from the branch-built plugin (`--plugin-dir` pointing at this branch's `plugins/loaf`, per the pattern in `cli/scripts/smoke-claude-code-startup.mjs`) so the invoked `/pitch` is the candidate skill, never the installed release; shortlist repo-safe candidate items from this repo's `loaf intake list`, then pause and ask the human to invoke `/pitch` and participate in the interview; the dogfood change is not retained. Capture the produced brief — redacting anything sensitive or private before it enters Git — plus the candidate commit the session loaded and what the experience changed in the guide as `research/pitch-interview-dogfood.md` in this change folder, then revise the guide and SKILL.md accordingly.
- [x] Rebuild and commit `plugins/` and `dist/` with the source.

## Verification

- The skill lands in all five target projections: `test -f plugins/loaf/skills/pitch/SKILL.md && test -f dist/opencode/skills/pitch/SKILL.md && test -f dist/cursor/skills/pitch/SKILL.md && test -f dist/codex/skills/pitch/SKILL.md && test -f dist/amp/skills/pitch/SKILL.md`.
- The human-entry guard ships, not just the file (V7's assertions): `grep -q "disable-model-invocation: true" plugins/loaf/skills/pitch/SKILL.md`, and `grep -qF "Agents never initiate a pitch."` — the complete sentence, period included, so a weakened "…unless" variant cannot pass — succeeds against the built SKILL.md in all five projections.
- `loaf check` passes (frontmatter, description constraints, artifact naming).
- `research/pitch-interview-dogfood.md` exists in this change folder and records the dogfooded brief.
- The dogfooded brief satisfies H2's cold-read test: problem, user, alternative, value nameable in one pass; zero solution-space content.
- The slug never cites other work units — identity is local; provenance is in frontmatter and the change folder.
