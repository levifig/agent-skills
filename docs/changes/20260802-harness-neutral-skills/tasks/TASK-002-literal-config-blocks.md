---
change: harness-neutral-skills
id: TASK-002
title: Literal config block mechanism
blocked-by:
  - TASK-001
---

# TASK-002 — Literal config block mechanism

## Objective

Fenced content a harness or user consumes verbatim renders its exact per-target string, authored explicitly rather than produced by find-replace. This is the one sanctioned exception to neutrality, and it is deliberately the smallest mechanism that works.

## Scope boundaries

**In:** The mechanism itself, and the known offender — the allowed-tools lists in `content/skills/foundations/references/permissions.md` that currently render `update_plan, update_plan` for Codex and a doubled prose phrase for OpenCode.

**Out:** Any general templating or conditional-content engine. If the mechanism starts attracting feature requests, stop and revisit the shape.

## Context pointers

- Contract: `shape.md` — Decision 3, Rabbit Holes and No-Gos
- The corruption is reproducible today: build and compare `dist/codex/skills/foundations/references/permissions.md` against its OpenCode counterpart

## Acquisition

```bash
loaf journal log "skill(implement): TASK-002 — literal config block mechanism"
```

## Steps

- [ ] Inventory every fenced block across the corpus whose content is consumed verbatim rather than read as description
- [ ] Choose the mechanism — per-target authored variants selected at build time is the presumed answer; justify anything more elaborate
- [ ] Convert the `permissions.md` allowed-tools lists, and any other block the inventory turns up
- [ ] Add `TestLiteralConfigBlocksRenderVerbatim`, asserting exact per-target strings and specifically that no tool name is duplicated

## Verification

- `go test ./internal/cli/ -run TestLiteralConfigBlocksRenderVerbatim` passes
- The built Codex permissions reference lists `update_plan` once, and no built target contains a prose phrase where a tool name belongs
