---
name: python-development
description: >-
  Covers Python 3.12+ with FastAPI, Pydantic, async patterns, pytest,
  SQLAlchemy, and uv toolchain. Use when building Python services, APIs, data
  models, or tests. Provides patterns for modern Python development. Not for
  schema design (use database-...
user-invocable: false
allowed-tools: 'Read, Write, Edit, Bash, Glob, Grep'
version: 2.0.0-dev.30
---

# Python Development

Modern Python 3.12+ development with FastAPI ecosystem.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Stack Overview

## Critical Rules

### Always

- Use `async def` for I/O-bound operations
- Use Pydantic models for external input validation
- Use `pathlib.Path` for file operations
- Run mypy in CI/CD pipeline

### Never

- Block the event loop with sync I/O in async code
- Use mutable default arguments (`def foo(items=[])`)
- Skip validation on external input
- Hardcode configuration values (use pydantic-settings)

## Verification

### After Editing Python Files

**Type Checking:**
- If `mypy` is available in the project, run: `mypy --show-error-codes {files}`
- Skip test files and migrations (they often have looser types)
- For strict mode: `mypy --strict {files}`

**Linting:**
- If `ruff` is available, run: `ruff check {files}`
- To auto-fix issues: `ruff check --fix {files}`
- To format: `ruff format {files}`

**Security Scanning:**
- If `bandit` is available, run: `bandit -f txt {files}`
- Review high/medium severity findings
- Common fixes: replace assert with proper error handling, use secrets module for random tokens

**Testing:**
- If `pytest` is available:
  - For specific test file: `pytest {test_file} -v --tb=short`
  - For related module tests: `pytest tests/test_{module}/ -v`
  - For full suite: `pytest -v --tb=short`
- Check test file structure:
  - Test functions named `test_*`
  - Async tests use `@pytest.mark.asyncio`
  - `import pytest` present in test files

### Before Committing

- Run formatters on changed files (black/ruff format)
- Run type checker on modified source files
- Run security scanner if reviewing or doing thorough validation
- Ensure tests pass for modified code

### Test Structure Validation

When writing test files, verify:
- Test functions named `test_*` (not just `test`)
- Async tests have `@pytest.mark.asyncio` decorator
- `import pytest` present in test files
- Use fixtures for setup/teardown instead of bare asserts

## Quick Reference

See Stack Overview below for default technology choices.

## Topics

| Topic | Use For |
|-------|---------|
| [Core](references/core.md) | Project setup, pyproject.toml, modern Python features |
| [FastAPI](references/fastapi.md) | REST APIs, routing, dependency injection, middleware |
| [Pydantic](references/pydantic.md) | Data models, validation, settings management |
| [Async](references/async.md) | async/await, TaskGroup, context managers |
| [Types](references/types.md) | Type hints, mypy, Protocol, generics |
| [Testing](references/testing.md) | pytest, fixtures, mocking, async tests |
| [Database](references/database.md) | SQLAlchemy 2.0, Alembic migrations, transactions |
| [Data](references/data.md) | Polars, ETL pipelines, schema validation |
| [API Clients](references/api.md) | httpx, retries, rate limiting, error handling |
| [Deployment](references/deployment.md) | Docker, logging, OpenTelemetry, health checks |
| [Debugging](references/debugging.md) | pdb, structlog, pytest debugging, remote debugging |

## Stack Overview

| Layer | Default | Alternatives |
|-------|---------|--------------|
| Runtime | Python 3.12+ | - |
| Package Manager | uv | rye, poetry |
| Linter/Formatter | ruff | black + flake8 |
| Type Checker | mypy (strict) | pyright |
| Web Framework | FastAPI | Flask, Django |
| Validation | Pydantic v2 | - |
| ORM | SQLAlchemy 2.0 | - |
| Data Processing | Polars | Pandas |
| HTTP Client | httpx | aiohttp |
| Testing | pytest | - |
| Containerization | Docker | - |
