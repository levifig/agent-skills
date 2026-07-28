---
change: change-work-model
id: TASK-010
title: Conversion cannot manufacture execution
relates-to:
  - TASK-001
---

# TASK-010 — Conversion cannot manufacture execution

## Objective

Implement the rule the Planning Contract states and V4 asserts: a sanctioned layout conversion must land with **all task checkboxes unchecked**, and a conversion carrying pre-checked boxes is a check violation. The rule exists in the document and nowhere in the code.

## Scope boundaries

**In:** conversion recognition in `internal/cli/change_lineage.go` (see the sanctioned-atomic-conversion branch near `:449`) and whichever check surface should carry the finding; the fixtures.

**Out:** Retention and retarget replay (already shipped and passing). The flip detector (`TASK-009`) — this rule is about what a conversion commit is allowed to contain, not how transitions are read.

## Context pointers

- `docs/changes/20260726-change-work-model/shape.md` — Planning Contract, "Metadata history across surfaces" (the parenthetical: pre-checked boxes are a violation because flip-grade execution must come from later delivering commits); V4.
- The originating review: `reports/20260727-201619-review-codex-r3.html` finding 2.5, accepted at shaping.
- Review board finding M5 — accepted-but-unbuilt.
- `internal/cli/change_json_test.go:169` — the existing atomic-conversion retention test, which says nothing about checkbox state and is the natural place to extend.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-010 — conversion cannot manufacture execution"
```

This change's own conversion is the reference case: `acbea950` retired `change.md` for `shape.md` and reset every task checkbox to unchecked in the same commit. That commit must stay legal.

## Steps

- [x] A conversion commit (same-commit `change.md` retirement plus `change.json` addition) carrying any checked task checkbox is reported as a violation naming the offending file.
- [x] A conversion with all boxes unchecked passes, and this change's real conversion commit is covered as the positive fixture.
- [x] Decide and document where the finding surfaces — `loaf change check` on the converted unit, the release preflight, or both — and state the reasoning in the commit body; a rule that only fires at release time arrives too late to be useful.
- [x] Fixture: a two-commit conversion still blocks with the existing retention finding (regression guard, already passing).

## Verification

- `go test ./internal/cli/ -run 'Retention|Conversion|ChangeCheck'` green.
- `loaf change check docs/changes/20260726-change-work-model` still reports zero violations.
