---
id: SPEC-017
title: 'TDD Harness — spec-driven testing, eval accumulation, reviewer-to-R mapping'
source: SPEC-014
created: '2026-03-28T15:45:00.000Z'
status: drafting
---

# SPEC-017: TDD Harness — Spec-Driven Testing and Eval Accumulation

## Problem Statement

SPEC-014 establishes functional profiles (implementer, reviewer, researcher) and describes a TDD flow as the standard orchestration pattern. But the TDD description is philosophical, not buildable:

1. **No concrete orchestration prose.** The `/implement` skill needs to describe the TDD sequence in enough detail that the coordinator reliably follows it — which Rs to hand the test-writer, when to loop, when to stop.

2. **No eval accumulation strategy.** Each spec's tests are written, the code passes, and then... the tests sit in the project. There's no mechanism to re-run previous specs' tests as a regression suite, no tracking of which Rs map to which test files.

3. **No bootstrapping path.** The first task in a greenfield project has no test framework, no test conventions, no existing patterns to follow. The TDD flow assumes infrastructure that may not exist.

4. **No reviewer-to-R mapping.** The reviewer audits "against the spec" but the spec doesn't say how. Does the reviewer literally check each R? Does it run the tests? Does it do both?

## Strategic Alignment

- **Vision:** Fits "agent-creates, human-curates" — agents write tests from specs, humans approve the spec's Rs
- **Architecture:** Pure skill/template changes. No CLI work. Builds on SPEC-014's profile model.
- **Best practices:** Aligned with Deep Agents' eval philosophy ("every eval is a vector that shifts behavior") and Singer's binary R format from Shape Up

## Solution Direction

### The TDD Orchestration Pattern

The `/implement` skill gains a concrete TDD section that the coordinator follows for implementation work. The flow has four phases:

**Phase 1: Test Scaffolding (if needed)**

For the first task in a project or when no test framework exists:
- Coordinator spawns an implementer with: "Set up the test framework for this project. Create the test runner config, directory structure, and a single smoke test that passes."
- This is a one-time bootstrap, not part of the recurring TDD loop.
- The coordinator detects this need by checking for test runner config files (e.g., `vitest.config.ts`, `pytest.ini`, `Rakefile` with test task).

**Phase 2: Test Writing**

Coordinator reads the spec's Rs and spawns an implementer:
- Prompt includes: the specific Rs being implemented, the spec's acceptance criteria, and the project's test conventions (detected from existing tests or the test framework skill)
- Instruction: "Write failing tests that verify these Rs. Each test should map to a specific R. Do not implement production code."
- The implementer returns: test file paths and which R each test covers

**Phase 3: Implementation**

Coordinator spawns an implementer:
- Prompt includes: the test file paths, the Rs being implemented, and relevant codebase context
- Instruction: "Make these tests pass. You may add additional tests but do not modify the existing ones without flagging the issue to the coordinator."
- The implementer returns: implementation file paths, test results, any flagged test issues

**Phase 4: Review**

Coordinator spawns a reviewer:
- Prompt includes: the Rs, test file paths, implementation file paths, and the spec's acceptance criteria
- Instruction: "Audit the implementation against the spec. For each R, verify: (1) a test exists that covers it, (2) the test is meaningful (not tautological), (3) the implementation satisfies the R. Report per-R pass/fail."
- The reviewer returns: per-R verdict (✅/❌), issues found, missing coverage

**Loop:** If the reviewer finds issues, the coordinator spawns an implementer to fix them, then re-spawns the reviewer. Loop until the reviewer reports all Rs pass.

### Per-R Traceability

Every test file includes a comment or docstring mapping tests to Rs:

```python
# R3: Each new skill has SKILL.md + sidecar + references/
def test_git_workflow_skill_has_required_files():
    ...

# R5: "merge this PR" activates git-workflow
def test_merge_prompt_activates_git_workflow():
    ...
```

This mapping enables:
- The reviewer to check coverage per-R without guessing
- The eval suite to report which Rs are covered and which aren't
- Future specs to reference existing R-tagged tests

### Eval Accumulation

Tests written for a spec become part of the project's permanent test suite. The accumulation strategy:

1. **Tests live in the project's standard test directory** — no special location. They're normal tests that run with the project's test runner.
2. **R-tags in comments** provide traceability back to specs but don't affect test execution.
3. **Running `npm test` / `pytest` / etc. runs ALL accumulated tests** — including those from previous specs. This is the regression suite.
4. **No separate "eval runner"** — the project's existing test infrastructure IS the eval system. No new tooling needed.
5. **The coordinator runs the full test suite** (not just the current spec's tests) before creating a PR. If previous specs' tests break, that's a regression that must be fixed.

### Reviewer Audit Protocol

The reviewer follows a structured protocol:

```
For each R in the spec:
  1. Find test(s) tagged with this R
  2. Verify test is meaningful (not just asserting true)
  3. Verify test covers the R's assertion (input → behavior → outcome)
  4. Verify implementation makes the test pass
  5. Report: R{n} ✅ or R{n} ❌ {reason}

Then:
  6. Run full test suite — check for regressions
  7. Check for untested code paths (coverage, if available)
  8. Report: overall verdict + issues list
```

### Implement Skill Changes

The `/implement` skill's "Then Execute" section gains a TDD subsection that replaces the current generic "spawn agents" pattern. The coordinator:

1. Reads the spec's Rs (binary test conditions)
2. Groups Rs by task (if working from a breakdown) or takes all Rs (if working from a spec directly)
3. Checks for test framework (Phase 1 if needed)
4. Follows Phase 2 → 3 → 4 → loop
5. Runs full test suite before PR creation

### Spec Template Changes

The spec template's Test Conditions section is renamed to **Requirements (R)** and adopts the binary format:

```markdown
## Requirements

Binary — each passes (✅) or fails (❌).

### [Group name]
- [ ] R0: [input] → [expected behavior] → [expected outcome]
- [ ] R1: [input] → [expected behavior] → [expected outcome]
```

The `/shape` skill is updated to produce Rs in this format. Council deliberation (SPEC-016) feeds into R negotiation before locking.

## Scope

### In Scope

- TDD orchestration pattern in `/implement` skill (Phases 1-4 + loop)
- Per-R traceability convention (R-tags in test comments)
- Reviewer audit protocol (per-R verdict + regression check)
- Spec template evolution (Test Conditions → Requirements with binary R format)
- `/shape` skill update to produce binary Rs
- Documentation of eval accumulation strategy (tests = project's standard test suite)
- Test framework bootstrapping detection and scaffolding

### Out of Scope

- Custom eval runner or tooling (project's test runner IS the eval system)
- Test coverage tooling or thresholds (useful but separate concern)
- Automated R-to-test mapping validation (manual convention for now)
- Changes to the profile model itself (SPEC-014)
- Council changes (SPEC-016)
- CI/CD integration for eval runs (future — project-specific)

### Rabbit Holes

- Building a custom eval framework — the project's test runner is enough. Don't reinvent pytest/vitest.
- Trying to auto-generate tests from Rs — the implementer-as-test-writer interprets Rs, it doesn't mechanically translate them. Rs are human-readable assertions, not test DSL.
- Over-engineering R-tag parsing — a comment convention is sufficient. Don't build tooling to validate R-tags at this stage.
- Trying to make the TDD flow work for non-code specs (documentation, design) — TDD applies to implementation work. Other work types have their own quality patterns.

### No-Gos

- Don't make TDD mechanically enforced — it's a standard pattern, not a gate. The coordinator follows it because the skill says to, not because the system blocks alternatives.
- Don't require 100% R coverage before shipping — the reviewer reports gaps, the coordinator and user decide whether they're acceptable.
- Don't modify existing test frameworks or conventions — the harness adapts to the project, not the other way around.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Implementer-as-test-writer writes shallow/tautological tests | Medium | Medium | Reviewer protocol explicitly checks test meaningfulness. Coordinator can re-spawn with "write more rigorous tests" |
| R-tag convention drift (tests stop being tagged) | Medium | Low | Reviewer checks for tags during audit. Convention reinforced in skill instructions. Low-cost to fix retroactively. |
| Full test suite gets slow as specs accumulate | Low | Medium | Standard engineering problem — optimize tests, parallelize, use test sharding. Not a harness concern. |
| Test framework detection fails (false negative) | Low | Low | Coordinator asks user if detection is uncertain. Simple glob checks for common config files. |

## Open Questions

- [ ] Should the spec template rename "Test Conditions" to "Requirements" globally, or keep both terms with a migration path?
- [ ] Should R-tags follow a specific format (e.g., `# R3:` vs `@requirement R3`) or is freeform comment sufficient?
- [ ] Should the reviewer run tests itself (needs Bash access, breaking read-only) or report "please run tests" to the coordinator?

## Requirements

Binary — each passes (✅) or fails (❌).

### Orchestration
- [ ] R0: `/implement` follows TDD phases (test → code → review → loop) for implementation work
- [ ] R1: Coordinator detects missing test framework and scaffolds before TDD loop
- [ ] R2: Test-writing implementer receives only Rs and spec context (not implementation details)

### Traceability
- [ ] R3: Tests include R-tag comments mapping to specific spec requirements
- [ ] R4: Reviewer reports per-R verdict (✅/❌) with reasons

### Eval accumulation
- [ ] R5: Tests from completed specs persist in the project's standard test suite
- [ ] R6: Full test suite runs before PR creation (regression check)

### Templates
- [ ] R7: Spec template uses binary R format (input → behavior → outcome)
- [ ] R8: `/shape` produces specs with binary Rs

### Build integrity
- [ ] R9: `loaf build` succeeds with updated implement and shape skills
- [ ] R10: Existing specs are not broken by template evolution (backwards compatible)

## Priority Order

1. **TDD orchestration in `/implement`** (Phases 1-4 + loop) — core value. Go/no-go: coordinator follows the TDD flow.
2. **Reviewer audit protocol** (per-R verdicts) + R-tag convention. Go/no-go: reviews produce actionable verdicts.
3. **Spec template evolution** + `/shape` updates + eval accumulation. Drop if earlier tracks deliver sufficient quality.
