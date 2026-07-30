---
change: pitch-entrypoint
id: TASK-005
title: Guidance sweep — flow vocabulary and final verification
blocked-by:
  - TASK-002
  - TASK-003
  - TASK-004
---

# TASK-005 — Guidance sweep

## Objective

The Loaf Flow vocabulary — pitch → shape → implement → ship → release, at both scales — is consistent across guidance surfaces and skill cross-references, the Durable Outputs are distilled on the branch, and the whole change verifies green.

## Scope boundaries

**In:** root `AGENTS.md` (canonical per ADR-020; `.claude/CLAUDE.md` follows as the compatibility symlink), `docs/knowledge/work-model.md`, `docs/knowledge/glossary.md`, and `docs/knowledge/task-system.md` (the shipped operating account of the pipeline and its command table), an amended-by pointer in `docs/decisions/ADR-022-change-anatomy-and-release-cohorts.md`, Related Skills sections that should name pitch (idea, research, wrap where they route entry intent), `README.md` only if it narrates the flow, the Durable Outputs distillation (knowledge doc under `docs/knowledge/`, ADR under `docs/decisions/`), the H4 end-to-end dogfood and its evidence append to `research/pitch-interview-dogfood.md`, final rebuild.

**Out:** new behavior of any kind — this task follows shipped behavior, never leads it. No CHANGELOG entry (release curates that), no version bump, no edits to the retired `.agents/AGENTS.md` path.

## Context pointers

- Contract: `shape.md` — Hypothesis (the one-front-door claim this sweep makes legible), Scope (registration and guidance bullet), Verification Contract V1–V6.
- Precedent: change-work-model TASK-006 (skills and guidance sweep) for tone and extent.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-005 — guidance sweep for the pitch entry stage"
# Grep guidance surfaces for flow narrations: rg -l "shape.*implement.*ship" AGENTS.md content/ README.md
```

## Conventions

Markdown paragraphs are single lines — never hard-wrap. Guidance documents state what is, not what is planned.

## Steps

- [x] Sweep root `AGENTS.md` for flow narration: the entry stage is pitch, briefs are authored problem-space documents, shape consumes them (`.claude/CLAUDE.md` follows via the symlink — never edit it directly).
- [x] Converge the shipped operating account: update `docs/knowledge/work-model.md` (triage-seeds-the-brief becomes the pitch front door; brief authorship and accretes-until-shaping semantics; the landing matrix), `docs/knowledge/task-system.md` (the work-record command table names capture promotion), and the glossary's affected entries, and add the amended-by pointer to ADR-022 referencing the new entry-stage ADR — one account of the pipeline, not two.
- [x] Correct `AGENTS.md`'s skill-registration guidance to match Decision 10: skills are auto-discovered from `content/skills/` at build time, and `hooks.yaml` registers hook instances only — both the "Before Committing" checklist line and the "Add skill" quick-task entry currently instruct registering every new skill in `hooks.yaml`.
- [x] Sweep skill cross-references: Related Skills sections that route entry intent name `/pitch`; no skill still presents brainstorm or explore as the user-facing front door.
- [x] Distill the Durable Outputs via reflect on the branch: the Loaf Flow knowledge doc under `docs/knowledge/` and the entry-stage ADR under `docs/decisions/` (next free ADR number at authoring time), per `shape.md` Durable Outputs.
- [ ] H4 end-to-end dogfood — a user-invoked checkpoint: with TASK-004's brief-aware shape landed, rebuild and prepare the isolated scratch project with a fresh session explicitly loaded from the branch-built plugin (`--plugin-dir` at this branch's `plugins/loaf`, per `cli/scripts/smoke-claude-code-startup.mjs`) so both ceremonies are the candidate skills rather than the installed release's, then ask the human to run `/pitch` then `/shape` on its output, confirming shape consumes the brief without re-asking problem questions; append the result and the candidate commit to `research/pitch-interview-dogfood.md`.
- [x] Run the full verification contract V1–V7 as declared in `shape.md`: `go test ./... -count=1`, `npm run build`, `loaf check`, the five-projection presence check, the explore-demotion grep, `loaf change check docs/changes/20260728-pitch-entrypoint --require-executable`, and the human-entry-guard greps.
- [x] Final rebuild; commit any remaining artifact drift with this task.

## Verification

- V1–V7 from `shape.md` all green, run from the repository root.
- A cold `rg -i "brainstorm" content/skills --files-with-matches` review shows no surviving user-facing entry framing.
- The slug never cites other work units — identity is local; provenance is in frontmatter and the change folder.
