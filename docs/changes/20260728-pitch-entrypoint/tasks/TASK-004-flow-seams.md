---
change: pitch-entrypoint
id: TASK-004
title: Flow seams — shape consumes briefs, triage routes to pitch, explore demotes
blocked-by:
  - TASK-002
  - TASK-006
---

# TASK-004 — Flow seams

## Objective

The surrounding skills acknowledge the new entry stage: `/shape` treats an existing `brief.md` as primary input and grills solution-space only; `/triage` offers "pitch this" as a disposition; `/explore` demotes to agent-technique surface; explore and brainstorm descriptions reroute users to `/pitch`.

## Scope boundaries

**In:** `content/skills/shape/SKILL.md` (Step 1 context gathering + related skills), `content/skills/triage/SKILL.md` (dispositions), `content/skills/explore/SKILL.claude-code.yaml` (`user-invocable: false`) and SKILL.md description, `content/skills/brainstorm/SKILL.md` description, rebuilt artifacts.

**Out:** deep rewrite of explore/brainstorm content (deferred to the skills audit Intent), Exploration CLI machinery and checkpoint contract (untouched), shape's grilling references, bootstrap (TASK-003).

## Context pointers

- Contract: `shape.md` — Scope (flow seams bullet), Observable Workflow (shape-without-brief paragraph; explore-from-inside-pitch), Decisions 5 and 9, Planning Contract (Risks — demotion reversal is a one-line flip).
- Current text: shape SKILL.md Step 1 (Gather Context) and Source Inputs list; triage SKILL.md Dispositions section ("Shape" entry is the model for the new "Pitch" entry).

## Acquisition

```bash
loaf journal log "skill(implement): TASK-004 — flow seams for the pitch entry stage"
# Read the four SKILL.md files and both sidecars before editing
```

## Conventions

Markdown paragraphs are single lines — never hard-wrap. Description edits follow the two-tier structure with negative routing; keep each skill's existing voice.

## Steps

- [ ] Shape: Step 1 recognizes an existing `brief.md` — restate the problem from it, confirm rather than re-discover, and focus grilling on solution-space; when no brief exists, full narrowing as today (pitch recommended, never gated). Step 3 handles the captured folder: run `loaf change init <slug>` on it and rely on the promotion path (TASK-006) to materialize `shape.md` + `tasks/` in place — never hand-copy templates. Add pitch to recognized source inputs and Related Skills.
- [ ] Triage: add the "Pitch" disposition — items needing problem discovery hand to `/pitch`, which owns init and brief authoring; the promoted item resolves against the created change; "Shape" survives for well-understood directions, and the brief-seeding bullet in Critical Rules updates to name both doors.
- [ ] Explore: sidecar flips to `user-invocable: false`; description reroutes entry intent ("explore this" as a user ask → `/pitch`) while keeping the technique's agent-facing purpose and the untouched Exploration machinery explicit.
- [ ] Brainstorm: description reroutes to `/pitch` as the user-facing entry (it already routes to explore for the technique).
- [ ] Rebuild and commit artifacts with the source.

## Verification

- `grep -c "user-invocable: false" content/skills/explore/SKILL.claude-code.yaml` prints 1.
- `loaf check` passes (description length and phrasing constraints hold after edits).
- Task-local acceptance, each confirmed by reading the shipped diff or grepping the built output: shape's Step 1 keeps the no-brief fallback intact and Step 3 carries the promotion instruction; triage's Dispositions section offers both "Pitch" and "Shape"; explore's description routes user entry intent to `/pitch`; brainstorm's description routes to `/pitch`.
- The slug never cites other work units — identity is local; provenance is in frontmatter and the change folder.
