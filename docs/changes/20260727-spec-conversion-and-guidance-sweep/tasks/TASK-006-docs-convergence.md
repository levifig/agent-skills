---
change: spec-conversion-and-guidance-sweep
id: TASK-006
title: Docs convergence
blocked-by:
  - TASK-004
blocks:
  - TASK-007
---

# TASK-006 — Docs convergence

## Objective

Every public and strategic doc describes only the Change-first flow: the compatibility-stance sentences are gone, the knowledge record reflects the post-sweep model, and ADR-013/016 carry dated revision notes.

## Scope boundaries

**In:** `README.md` (command + skill tables), root `AGENTS.md` (compat sentences, breakdown exemplar, command index), `docs/ARCHITECTURE.md` (work-records section, layout tree, Linear-native paragraph as spec/task-bound), `docs/STRATEGY.md` + `docs/VISION.md` stance sentences, `.github/PULL_REQUEST_TEMPLATE.md` legacy line, `docs/knowledge/task-system.md` (retitle/rescope or fold into `work-model.md`), `docs/knowledge/README.md` index, incidental mentions in `work-model.md`/`loaf-flow.md`/`glossary.md`/`hook-system.md`, `docs/schema/README.md` + diagrams (quarantine annotation with TASK-003), ADR-013 and ADR-016 dated in-place revision notes.

**Out:** ADR-011 (owned by `linear-native-coordination`, Decision 6). CHANGELOG (written at the arc cut). Historical evidence — old Changes, archived specs, ADR bodies' original citations (Decision 11, ADR-026 citation rule).

## Context pointers

- Contract: `shape.md` — Decisions 8 and 11, Durable Outputs

## Acquisition

```bash
loaf journal log "skill(implement): TASK-006 — docs convergence"
```

## Steps

- [ ] README/AGENTS.md/ARCHITECTURE/STRATEGY/VISION/PR template converged; hygiene pins updated in the same commits
- [ ] Knowledge record rescoped to the post-sweep model; `covers:` globs updated
- [ ] ADR-013 and ADR-016 revision notes: dated, in place, under the living-record convention

## Verification

- `go test ./cmd/loaf` green at each commit
- `git grep -n "remain supported compatibility" README.md AGENTS.md docs/` returns nothing
