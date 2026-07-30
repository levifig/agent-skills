---
change: pitch-entrypoint
id: TASK-003
title: Bootstrap consumes pitched BRIEFs and preps the change series
blocked-by:
  - TASK-001
---

# TASK-003 — Bootstrap series-prep

## Objective

Bootstrap recognizes a pitched BRIEF (`source: pitch`) as discovery-already-done and interviews for gaps only, and closes with a series-prep phase that mints the initial arc as captured promise carriers — each `loaf change init <slug> --brief`, seeded problem-space-only, bound to a coarse `target_release`, sequencing stated as prose — with user confirmation per mint; concepts the builder won't bind to even a coarse target stay BRIEF lines, sparks, or Intents.

## Scope boundaries

**In:** `content/skills/bootstrap/SKILL.md` (greenfield+brief mode, new series-prep phase), `content/skills/bootstrap/references/interview-guide.md` (series-prep granularity guidance), rebuilt artifacts.

**Out:** the pitch skill (TASK-002), operating-document population (stays as shipped — Rabbit Holes), tracker sync, roadmap planning fields. Bootstrap never auto-shapes and never creates branches for the minted series.

## Context pointers

- Contract: `shape.md` — Scope (series-prep bullet), Observable Workflow (new-project paragraph), Rabbit Holes (series-prep is not roadmap planning; document population not rewritten), Decisions 3, 6, Open Questions ([KU] series-prep granularity).
- Existing anatomy: `content/skills/bootstrap/SKILL.md` — Brief Intake, Interview Flow, Finalization (series-prep lands as a phase between Knowledge Base Scaffolding and Next Steps, or as the reshaped Next Steps — author's call, stated in the diff).
- The sanctioned parking pattern: `docs/changes/20260726-change-work-model/shape.md` — captured promise carrier.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-003 — bootstrap series-prep"
# Read bootstrap SKILL.md and its interview guide in full before editing
```

## Conventions

Markdown paragraphs are single lines — never hard-wrap. Preserve bootstrap's existing section voice and the BRIEF-is-archeological framing.

## Steps

- [x] Teach greenfield+brief mode to read `source: pitch`: discovery is done, quote-back and gap-fill only, no re-excavation.
- [x] Add the series-prep phase: enumerate the BRIEF's scoped concepts with the builder, apply the granularity guidance (when a concept earns its own captured change versus staying a BRIEF line), and for each confirmed concept run `loaf change init <slug> --brief`, seed the brief from the BRIEF's problem-space content, stamp the coarse `target_release` the builder binds, state sequencing as prose in the brief, and land it as its own docs-only commit per Decision 11's matrix — one commit per capture, never a batch, each preceded by explicit-path `loaf change check <folder> --json` reporting zero violations and captured state plus a direct read-back of the folder's `change.json` confirming the stamped target; a concept the builder won't bind to even a coarse target stays a BRIEF line, spark, or Intent instead of minting a folder.
- [x] Write the granularity guidance into bootstrap's interview guide, resolving the fog entry: a concept earns a folder when it is independently shippable and its problem can be stated without the others; otherwise it stays in the BRIEF (or becomes a spark).
- [x] Guard the phase: every mint user-confirmed, no auto-shaping, no branch creation, no priority/date/dependency fields.
- [x] H3 dogfood: run series-prep against a sample pitched BRIEF (scratch project or fixture) and confirm the minted briefs stand alone; capture the durable evidence — the minted briefs and a commit-log summary showing one docs-only commit per capture — as `research/series-prep-dogfood.md` in this change folder before the scratch project is discarded, and adjust guidance from the experience.
- [x] Rebuild and commit artifacts with the source.

## Verification

- `loaf check` passes.
- Task-local acceptance, confirmed by reading the shipped skill text: greenfield+brief mode names `source: pitch` with gap-only interviewing; the series-prep phase carries the per-mint confirmation guard, the landing matrix (coarse target required to mint + one docs-only commit per capture, no batching), the pre-landing condition (explicit-path `loaf change check` plus the `change.json` target read-back), and the no-auto-shape/no-branch guards; the granularity guidance exists in the interview guide.
- H3: each minted brief in the dogfood names its problem without reference to the session that created it, the dogfood's commit log shows one docs-only commit per capture, and `research/series-prep-dogfood.md` exists in this change folder carrying that evidence.
- The BRIEF remains archeological: series-prep reads it during bootstrap, and nothing references it afterward.
- The slug never cites other work units — identity is local; provenance is in frontmatter and the change folder.
