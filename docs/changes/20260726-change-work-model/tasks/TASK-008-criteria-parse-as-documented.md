---
change: change-work-model
id: TASK-008
title: Declared criteria parse and run as documented
blocks:
  - TASK-007
relates-to:
  - TASK-005
---

# TASK-008 — Declared criteria parse and run as documented

## Objective

Make the executable-criteria form that the shipped scaffold writes actually parse, run declared commands from the repository root, and teach the form somewhere a shaper will find it. Today a change shaped from the scaffold has zero parseable criteria, so `loaf change verify` refuses and no cohort member can satisfy the gate.

## Scope boundaries

**In:** `changeCriterionCommandRE` and `parseChangeExecutableCriteria` in `internal/cli/change_verify.go`; `runChangeCriterionCommand`'s working directory and the receipt field recording it; `internal/cli/change_shape_template.md` if the template needs to change rather than the parser; guidance in `content/skills/shape/` (SKILL.md and/or `references/cli-boundary.md`); rebuilt content targets.

**Out:** Receipt success semantics (`TASK-007`). Do not redesign the criteria syntax — Decision 14 settled that an executable V-entry carries its command and expected result; this is a parser-meets-template defect, not a format question.

## Context pointers

- `docs/changes/20260726-change-work-model/shape.md` — Decision 14, V5, V9.
- Review board finding B2 and minor m1 (`reports/20260727-234403-review-implementation-round-1.html`).
- `internal/cli/change_verify.go:21` — the regex is anchored `^\s*-\s*Command:`, while `:168` comments that inline support on the V-entry header line is intended.
- `internal/cli/change_shape_template.md:59` — the shipped inline form.
- `internal/cli/change_release_gate_test.go:53` — the sub-bullet form, currently the only one that works and documented nowhere.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-008 — criteria parse as documented"
loaf change init parse-probe   # in a throwaway repo, then run loaf change verify against the fresh scaffold
```

## Steps

- [x] Both forms parse: the inline `- **V1.** Prose. Command: \`cmd\`. Expect: exit 0.` written by the template, and the sub-bullet form already used in tests. Keep `Expect` optional as today.
- [x] Criteria commands run with the working directory at the repository root, and the receipt records the cwd it used so evidence states where the command ran.
- [x] `loaf change verify` on a freshly scaffolded change with the template's V1 left intact parses one criterion rather than refusing — a scaffold-then-verify smoke path, not just a hand-built fixture.
- [x] The working form is taught in the shape skill (and `references/cli-boundary.md` if that is the better home), including that commands run from the repository root and that H-tier entries are never gate input.
- [x] Rebuild content targets so `dist/` and `plugins/` carry the guidance; `npm run build` reports zero drift.

## Verification

- `go test ./internal/cli/ -run 'ChangeVerify'` green with a fixture per form.
- In a throwaway repo: `loaf change init x` then `loaf change verify` parses the template's criterion instead of printing "no executable criteria found".
- `go test ./...` and `npm run build` clean; V9's report invariants untouched.
