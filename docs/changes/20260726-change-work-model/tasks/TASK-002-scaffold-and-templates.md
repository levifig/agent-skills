---
change: change-work-model
id: TASK-002
title: Scaffold and templates
blocked-by:
  - TASK-001
blocks:
  - TASK-003
---

# TASK-002 — Scaffold and templates

## Objective

Make `loaf change init` emit the new anatomy (`change.json` + `shape.md` + seeded `tasks/`), with `--brief` as capture mode (`change.json` + `brief.md` only). Ship templates for the shape contract, optional brief/plan/design roles, task-file format, and `loaf change report new` with the closed kind registry.

## Scope boundaries

**In:** Init scaffold paths; `--brief` capture mode; embedded/canonical templates; `loaf change report new` naming, collision refusal, kind registry, charset/provenance/token skeleton, stdout design-language guidance.

**Out:** Full dual-layout check projections (`TASK-003`); gate/receipt (`TASK-004`/`TASK-005`); skill rewrites (`TASK-006`).

## Context pointers

- `change.md` Implementation Units TASK-002; Scope report naming + kind registry; Decision 16; V3/V9.
- Today: `internal/cli/change.go` init + `change_template.md`; `content/skills/shape/templates/`.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-002 — scaffold and templates"
rg -n "runChangeInit|changeTemplate|stampChange" internal/cli/change.go
```

## Steps

- [ ] `loaf change init` emits `change.json` + `shape.md` + seeded `tasks/`; `--brief` emits only `change.json` + `brief.md`.
- [ ] Templates cover shape contract (executable-criteria command form), brief/plan/design roles, and task-file delegation packet (`parent`/`blocks`/`blocked-by`/`relates-to`; slug never cites other work units).
- [ ] `loaf change report new` stamps `reports/YYYYMMDD-HHMMSS-<kind>-<slug>.html`, refuses collisions, prints guidance; `--kind` from closed registry (`approval`/`review`/`visual`/`audit`/`note`); unknown kinds refused with registry printed.
- [ ] Tests for scaffold, brief mode, report invariants, unknown kind; V3/V9 fixtures green.

## Verification

- Fresh `loaf change init` folder passes structural expectations for TASK-003's later check.
- `loaf change report new` creates the shell file and refuses duplicates / unknown kinds.
