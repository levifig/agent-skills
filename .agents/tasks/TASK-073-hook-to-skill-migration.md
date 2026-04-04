---
id: TASK-073
title: Migrate ~20 non-blocking hooks to skill instructions
spec: SPEC-020
status: todo
priority: P1
created: '2026-04-04T16:41:22.295Z'
updated: '2026-04-04T16:41:22.295Z'
---

# TASK-073: Migrate ~20 non-blocking hooks to skill instructions

Move ~20 non-blocking advisory hooks into Verification sections of their target SKILL.md files.

## Scope

**Language hooks -> skill instructions:**
- `python-type-check`, `python-type-check-progressive` -> `python-development` Verification
- `python-ruff-lint`, `python-ruff-check` -> `python-development` Verification
- `python-pytest-validation`, `python-pytest-execution` -> `python-development` Verification
- `python-bandit-scan` -> `python-development` Verification
- `typescript-tsc-check` -> `typescript-development` Verification
- `typescript-bundle-analysis` -> `typescript-development` Verification
- `typescript-eslint-check` -> `typescript-development` Verification
- `rails-migration-safety`, `rails-migration-safety-deep` -> `ruby-development` Verification
- `rails-test-execution` -> `ruby-development` Verification
- `rails-brakeman-scan`, `rails-rubocop-check` -> `ruby-development` Verification

**Infra hooks -> skill instructions:**
- `infra-validate-k8s`, `infra-k8s-dry-run` -> `infrastructure-management` Verification
- `infra-dockerfile-lint` -> `infrastructure-management` Verification
- `infra-terraform-plan` -> `infrastructure-management` Verification

**Design hooks -> skill instructions:**
- `design-a11y-validation`, `design-a11y-audit` -> `interface-design` Verification
- `design-token-check` -> `interface-design` Verification

**Cross-cutting hooks -> skill instructions:**
- `format-check` -> `foundations` Verification
- `tdd-advisory` -> `foundations` Verification
- `validate-changelog` -> `documentation-standards` Verification
- `changelog-reminder` -> `documentation-standards` Verification

## Constraints

- Add tool-availability checks: "If `mypy` is available in the project, run it"
- Do NOT remove hook scripts yet — removal happens in TASK-082 (Phase 3)
- Do NOT remove hook entries from hooks.yaml yet — removal in TASK-079
- Instructions should be clear about when to run (after editing, before committing, etc.)

## Verification

- [ ] Each migrated hook has corresponding instruction in target skill's Verification section
- [ ] Instructions include tool-availability checks
- [ ] ~20 hooks migrated to ~8 skills
- [ ] Hook scripts still exist (removal deferred)
- [ ] `loaf build` succeeds
