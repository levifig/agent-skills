---
id: TASK-163
title: Sentinel vocabulary test (Track B go/no-go)
spec: SPEC-034
status: done
priority: P1
created: '2026-05-02T01:25:49.208Z'
updated: '2026-05-02T01:25:49.208Z'
depends_on:
  - TASK-159
  - TASK-160
  - TASK-161
  - TASK-162
completed_at: '2026-05-02T03:35:00.000Z'
---

# TASK-163: Sentinel vocabulary test (Track B go/no-go)

## Description

**Track B go/no-go gate.** Manual end-to-end validation of vocabulary discipline. Pick one demonstrably shallow Loaf module (suggested candidate: `cli/lib/install/` — multiple small files passing through to file-system primitives). Invoke `/refactor-deepen` on it. Manually grade the output: count occurrences of correct terms (Module, Interface, Seam, Adapter, Depth, Leverage, Locality) vs drifted terms (boundary, service, component, layer, API, signature). **Pass:** zero drifted-term occurrences in any candidate description. **Fail:** iterate `references/language.md` and SKILL.md Critical Rules in TASK-159 until pass. Track B is not done until this test passes.

## File Hints

- `.agents/plans/<YYYYMMDD-HHMMSS>-<slug>.md` (the artifact produced by the test invocation — commit it). Shipped as `.agents/plans/20260502-033000-cli-lib-install-deepening-sentinel.md`.
- `.agents/sessions/<file>` (journal capturing the test result)
- Iteration target: `content/skills/refactor-deepen/SKILL.md` Critical Rules

## Acceptance Criteria

- [x] One full `/refactor-deepen` invocation completed against a real Loaf module (`cli/lib/install/`, deepening `ensureSymlink`)
- [x] PLAN file produced and committed at `.agents/plans/<YYYYMMDD-HHMMSS>-<slug>.md`
- [x] Manual vocabulary grading of the PLAN file body: count drifted-term occurrences
- [x] **Pass condition:** zero drifted-term occurrences in body (no "boundary/service/component/layer/API/signature" where source taxonomy has a precise term)
- [x] Journal entry: `validate(refactor-deepen): vocabulary sentinel pass`

## Verification

```bash
# Body-only grade (excludes the Sentinel grade table that names drifted terms by design)
awk '/^## Sentinel Vocabulary Test/{exit} {print}' .agents/plans/20260502-033000-*.md \
  | grep -owE "boundary|service|component|layer|API|signature" \
  | sort | uniq -c   # all six counts must be zero
```
