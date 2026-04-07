---
name: foundations
description: >-
  Establishes code quality, commit conventions, documentation standards, and
  security patterns. Use when writing or reviewing code, running checks, or
  setting up project standards. Covers naming, TDD, verification, and review
  workflows. Not for git ...
user-invocable: false
allowed-tools: 'Read, Write, Edit, Bash, Glob, Grep'
version: 2.0.0-dev.19
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

## Test Patterns

Scenario-based fixture naming:

- `*_perfect` - Complete, valid data (happy path)
- `*_degraded` - Partial data, quality issues
- `*_chaos` - Edge cases, malformed data

Coverage target: 70% minimum across all components.
