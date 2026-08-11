---
change: spec-conversion-and-guidance-sweep
id: TASK-007
title: Hygiene gate flip
blocked-by:
  - TASK-005
  - TASK-006
blocks:
  - TASK-008
---

# TASK-007 — Hygiene gate flip

## Objective

`TestPlanningVocabularyConverged` pins the new stance: legacy workflow references are forbidden on current-guidance surfaces, the converged sentences are required, and any straggler the new pins catch is swept in the same slice.

## Scope boundaries

**In:** Rewrite `cmd/loaf/content_hygiene_test.go` — drop the compatibility-stance requirements (including the requirement that orchestration link breakdown and the prohibition on naming this Change), add forbidden patterns (`loaf spec`, `loaf task`, breakdown-as-workflow) scoped to current-guidance surfaces, add required convergence sentences. Fix any straggler files the new pins surface. Exact strings and scoping are this task's deliverable (fog entry resolved here).

**Out:** Historical-evidence surfaces stay exempt (Decision 11): `docs/changes/`, `.agents/specs/archive/`, `CHANGELOG.md`, ADR bodies, journal renders.

## Context pointers

- Contract: `shape.md` — Decision 9, Open Questions
- Current pins: `cmd/loaf/content_hygiene_test.go:160-360`

## Acquisition

```bash
loaf journal log "skill(implement): TASK-007 — hygiene gate flip"
```

## Steps

- [ ] Compatibility pins removed; forbidden-pattern + required-sentence sets written with explicit surface scoping
- [ ] Straggler sweep: every hit from the new pins fixed or explicitly exempted as historical evidence
- [ ] Exemption list documented in the test itself

## Verification

- `go test ./cmd/loaf -run TestPlanningVocabularyConverged -v` green
- Intentionally reintroducing `loaf task` into README fails the test (falsification check, then revert)
