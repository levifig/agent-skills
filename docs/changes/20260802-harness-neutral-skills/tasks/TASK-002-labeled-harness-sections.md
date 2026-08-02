---
change: harness-neutral-skills
id: TASK-002
title: Labeled harness-section convention
blocked-by:
  - TASK-001
blocks:
  - TASK-003
  - TASK-004
---

# TASK-002 — Labeled harness-section convention

## Objective

One body can carry a fact that is genuinely product-specific, in a section labeled for the harness it describes, legible to every reader. No rendering path survives: every harness opens the same bytes.

## Scope boundaries

**In:** The authoring convention, its shape, and conversion of the known counterexamples — the allowed-tools lists in `content/skills/foundations/references/permissions.md`, the differing invocation mechanisms in `content/skills/orchestration/references/background-agents.md`, and any comparable case the inventory turns up.

**Out:** Per-target rendering of any kind — that is the thing being removed. Any general templating or conditional-content engine. Content that merely restates behaviour a model can infer, which should be neutral prose instead of a section.

## Context pointers

- Contract: `shape.md` — Decision 3, Rabbit Holes and No-Gos, Open Questions (section shape)
- The corruption is reproducible today: compare `dist/codex/skills/foundations/references/permissions.md` against its OpenCode counterpart
- Real per-harness facts that are not defects: `permissions.md:17-45` (Claude-specific slash commands and allowlists), `background-agents.md:29-61` (genuinely different mechanisms), `bootstrap/SKILL.md:472-481` (deliberately names `.claude/CLAUDE.md` because it creates the symlink), `foundations/references/review.md:44-49` (must inspect both agents-files)

## Acquisition

```bash
loaf journal log "skill(implement): TASK-002 — labeled harness-section convention"
```

## Steps

- [x] Inventory every place a fact is genuinely product-specific, and separate it from places where substitution merely papered over prose that should be neutral
- [x] Choose the section shape against the real counterexamples — subsection per harness, table, or inline conditional — weighing readability for the readers a section does not apply to
- [x] Convert the `permissions.md` allowed-tools lists and the `background-agents.md` mechanisms
- [x] Document the convention where skill authors will find it
- [x] Add `TestLabeledHarnessSectionsRenderVerbatim`, asserting every section is present with its exact strings and no tool name is duplicated

## Verification

- `go test ./internal/cli/ -run TestLabeledHarnessSectionsRenderVerbatim` passes
- The built Codex permissions reference lists `update_plan` exactly once, and no built target contains a prose phrase where a tool name belongs
- The same bytes serve every target — no build path renders this content differently per target
