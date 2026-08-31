---
name: foundations
description: >-
  Establishes code quality, commit conventions, documentation standards, and
  security patterns. Use when writing or reviewing code, running checks, or
  setting up project standards. Covers naming, TDD, verification, and review
  workflows. Not for git workflow (use git-workflow), debugging (use debugging),
  or security audits (use security-compliance).
---

# Code Standards

Engineering foundations for consistent, high-quality code.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Available Scripts
- Naming Conventions
- Test Patterns

## Critical Rules

### Always

- Use type hints on all public functions
- Validate inputs at trust boundaries
- Deliver [rideable increments](references/rideable-increments.md): complete, useful operator journeys whose breadth is narrow without weakening integrity

### Never

- Use bare `except:` clauses
- Log sensitive data or stack traces to users

## Verification

### After Editing Files

**Format Checking:**
- For Python files: If `black` is available, run `black --check {files}`
- For TypeScript/JavaScript files: If `prettier` is available, run `prettier --check {files}`
- For Ruby files: If `standardrb` is available, run `standardrb --format quiet {files}` or if `rubocop` is available, run `rubocop --format quiet {files}`
- Auto-fix commands:
  - Python: `black {files}`
  - TypeScript/JavaScript: `prettier --write {files}`
  - Ruby: `standardrb --fix {files}` or `rubocop -a {files}`

**TDD Advisory:**
- When editing implementation files (not test/spec files):
  - Check if corresponding test file exists
  - If no test found, consider Test-Driven Development:
    1. Write a failing test first
    2. Then implement the feature
    3. Run tests until green
  - Expected test file patterns:
    - Python: `test_{name}.py` or `{name}_test.py`
    - TypeScript/JavaScript: `{name}.test.{ext}` or `{name}.spec.{ext}`
    - Ruby: `spec/{name}_spec.rb` or `test/{name}_test.rb`
    - Go: `{name}_test.go`

### Before Committing

- Run appropriate formatter on all changed files
- Run linter/type checker for the language
- Verify tests exist for new functionality
- Run test suite if tests were modified
- Ensure CHANGELOG.md is updated if needed

### Workflow Notes

- Quick fixes with existing test coverage may skip TDD advisory
- Configuration and generated files don't need tests
- Editor format-on-save helps maintain compliance automatically

## Quick Reference

| Context | Python | TypeScript |
|---------|--------|------------|
| Files | `snake_case.py` | `PascalCase.tsx` (components) |
| Functions | `snake_case` | `camelCase` |
| Classes | `PascalCase` | `PascalCase` |
| Constants | `UPPER_SNAKE` | `UPPER_SNAKE` |
| Tests | `test_<unit>_<scenario>_<result>` | `describe/it` blocks |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Code Style | [references/code-style.md](references/code-style.md) | Writing Python/TypeScript code, naming variables |
| TDD | [references/tdd.md](references/tdd.md) | Writing tests first, red/green/refactor cycle |
| Verification | [references/verification.md](references/verification.md) | Verifying work before claiming done |
| Code Review | [references/code-review.md](references/code-review.md) | Requesting or receiving code reviews |
| Review | [references/review.md](references/review.md) | Conducting structured code reviews |
| Permissions | [references/permissions.md](references/permissions.md) | Configuring tool allowlists, sandbox, agent permissions |
| Observability | [references/observability.md](references/observability.md) | Instrumenting services, logging, metrics, tracing |
| Production Readiness | [references/production-readiness.md](references/production-readiness.md) | Validating services are ready for production |
| Rideable Increments | [references/rideable-increments.md](references/rideable-increments.md) | Shaping, implementing, reviewing, or sequencing complete operator journeys |

## Available Scripts

| Script | Usage | Description |
|--------|-------|-------------|
| `scripts/check-python-style.py` | `check-python-style.py <dir>` | Check Python style (type hints, docstrings) |
| `scripts/check-test-naming.sh` | `check-test-naming.sh <dir>` | Check test file/function naming |

## Naming Conventions

| Context | Python | TypeScript |
|---------|--------|------------|
| Files | `snake_case.py` | `PascalCase.tsx` (components) |
| Functions | `snake_case` | `camelCase` |
| Classes | `PascalCase` | `PascalCase` |
| Constants | `UPPER_SNAKE` | `UPPER_SNAKE` |
| Tests | `test_<unit>_<scenario>_<result>` | `describe/it` blocks |

### Artifact Names Never Cite Their Work Unit

Name an artifact for what it is, never for the work unit that produced it. The directory or Change that contains it already records that provenance, so repeating it in the filename inverts the reference and rots the moment the work closes: a work unit points at its artifacts, artifacts never point back.

| Instead of | Write |
|------------|-------|
| `u8-claude-smoke.mjs` | `smoke-claude-code-startup.mjs` |
| `report-spec-053-taxonomy-signoff.md` | `taxonomy-signoff.md` |
| `TASK-074-sidecar-audit-report.md` | `sidecar-audit.md` |

Provenance belongs in a front-matter field such as `source:`, where it can be read and updated, not in a name that has to be renamed to stay true.

Two look-alikes are correct and stay. A **version** is identity, not reference: `claude-code-2.1.218-plugin-startup-smoke.json`. A **timestamp** records when, not which work unit: `20260620-214448-skills-audit.md`. And a numbered record living in the directory that owns it *is* that entity, so `.agents/specs/SPEC-042-slug.md` and `docs/decisions/ADR-007-slug.md` are its own name rather than a citation.

`loaf check --hook artifact-names` enforces this at commit time over tracked artifacts, grandfathering anything already `final` or `archived`.

## Test Patterns

Scenario-based fixture naming:

- `*_perfect` - Complete, valid data (happy path)
- `*_degraded` - Partial data, quality issues
- `*_chaos` - Edge cases, malformed data

Coverage target: 70% minimum across all components.
