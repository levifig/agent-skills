---
change: pitch-entrypoint
id: TASK-001
title: Brief contract — shared skeleton on all three template surfaces
---

# TASK-001 — Brief contract

## Objective

The shared problem-space brief skeleton exists on all three template surfaces — shape's change-scale template (the authoring authority), its byte-identical Go-embedded projection, and bootstrap's project-scale template — with template-content tests, and the project-scale frontmatter vocabulary gains `source: pitch`.

## Scope boundaries

**In:** `content/skills/shape/templates/brief.md` (authoring authority), `internal/cli/change_brief_template.md` (byte-identical embed source), `internal/cli/change_test.go`, `content/skills/bootstrap/templates/brief.md`, rebuilt `plugins/` and `dist/` artifacts committed with the source change.

**Out:** the pitch skill itself (TASK-002), bootstrap SKILL.md prose (TASK-003), seam edits to other skills (TASK-004). No new machine fields in `change.json`, no derived-state changes, no CLI verbs.

## Context pointers

- Contract: `shape.md` — Scope (shared brief skeleton bullet), Decisions 1–3 and 7–8, Planning Contract (Template propagation).
- Authority rule: the scaffold comment in `internal/cli/change_scaffold.go` — embeds must stay byte-identical to `content/skills/shape/templates/`, gated by `TestChangeScaffoldTemplatesMatchCanonical`.
- Current surfaces: `content/skills/shape/templates/brief.md` (archeology comment worth preserving in spirit), `internal/cli/change_brief_template.md`, `content/skills/bootstrap/templates/brief.md` (frontmatter schema).

## Acquisition

```bash
loaf journal log "skill(implement): TASK-001 — brief contract across template surfaces"
# Read the three current templates and the existing template-content tests in internal/cli/change_test.go
```

## Conventions

Markdown paragraphs are single lines — never hard-wrap. Match each surface's existing comment and prose style.

## Steps

- [x] Define the skeleton in `content/skills/shape/templates/brief.md` (the authoring authority): problem statement, who has it, current alternatives, value proposition, constraints, sequencing and relationships (prose), sources and research links, open questions — problem-space only, with the archeology/supersession comment updated to the accretes-until-shaping semantics (Decision 7).
- [x] Mirror it byte-identically into `internal/cli/change_brief_template.md`; `TestChangeScaffoldTemplatesMatchCanonical` gates the match.
- [x] Add or extend template-content tests asserting the scaffolded `--brief` output carries the skeleton's section headings.
- [x] Align `content/skills/bootstrap/templates/brief.md` to the same skeleton at project altitude and add `pitch` to the `source:` frontmatter vocabulary.
- [x] Rebuild (`npm run build`) and commit rebuilt `plugins/` and `dist/` artifacts together with the source changes.

## Verification

- `go test ./... -count=1` passes, including the new template-content assertions.
- `npm run build` succeeds with no artifact drift.
- The three surfaces present the same section set (hand-check; the skeleton is defined once in `shape.md`).
- The slug never cites other work units — identity is local; provenance is in frontmatter and the change folder.
