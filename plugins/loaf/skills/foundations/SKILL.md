---
name: foundations
description: >-
  Use for code quality and engineering standards. Covers code style conventions,
  conventional commit messages, documentation standards (ADRs, API docs,
  changelogs), and security patterns (input validation, secrets management,
  OWASP). Activate for code review, commit preparation, documentation writing,
  or security considerations.
allowed-tools: 'Read, Write, Edit, Bash, Glob, Grep'
---

# Code Standards

Engineering foundations for consistent, secure, and well-documented code.

## Philosophy

**Code speaks first.** Well-structured code with clear names needs fewer comments. When comments are necessary, they explain WHY, not WHAT. Documentation lives close to code but separate from implementation details.

**Commits tell a story.** Each commit represents one coherent change. Messages use imperative mood and focus on intent. The git log should read like a narrative of the project's evolution.

**Security by default.** Every input is untrusted. Every error is generic to users. Every secret is externalized. Defense in depth, not security theater.

## Quick Reference

| Standard | Key Rule | Example |
|----------|----------|---------|
| **Code Style** | Type hints required, structured logging | `async def fetch(id: UUID) -> Result` |
| **Commits** | `<type>: <description>`, imperative mood | `feat: add thermal rating API` |
| **Documentation** | Document after shipping, not before | API docs reflect implemented endpoints only |
| **Security** | Validate all inputs at trust boundaries | Pydantic models at API layer |

## Topics

| Topic | Reference | Use For |
|-------|-----------|---------|
| Code Style | `reference/code-style.md` | Python/TypeScript conventions, naming, patterns |
| Commits | `reference/commits.md` | Commit messages, branches, PRs, Linear integration |
| Diagrams | `reference/diagrams.md` | Mermaid syntax, when to diagram, storage patterns |
| Documentation | `reference/documentation.md` | ADRs, API docs, changelogs |
| Security | `reference/security.md` | Threat modeling, secrets, compliance |
| Permissions | `reference/permissions.md` | Tool allowlists, sandbox config, agent permissions |

## Available Scripts

| Script | Usage | Description |
|--------|-------|-------------|
| `scripts/check-commit-msg.sh` | `check-commit-msg.sh <file>` | Validate commit message format |
| `scripts/check-python-style.py` | `check-python-style.py <dir>` | Check Python style (type hints, docstrings) |
| `scripts/check-test-naming.sh` | `check-test-naming.sh <dir>` | Check test file/function naming |
| `scripts/validate-adr.py` | `validate-adr.py <file>` | Validate ADR structure |
| `scripts/check-changelog-format.sh` | `check-changelog-format.sh <file>` | Check micro-changelog format |
| `scripts/check-secrets.sh` | `check-secrets.sh <dir>` | Scan for hardcoded secrets |
| `scripts/validate-compliance.py` | `validate-compliance.py <file>` | Validate security checklist completion |

## Critical Rules

### Always

- Use type hints on all public functions
- Write atomic commits (one logical change)
- Use imperative mood in commit messages
- Validate inputs at trust boundaries
- Log security events (without secrets)
- Include micro-changelog at document bottom

### Never

- Commit secrets, passwords, or API keys
- Document APIs before they ship
- Use bare `except:` clauses
- Force push to main/master
- Log sensitive data or stack traces to users
- Skip commit signing without explicit permission

## Naming Conventions

| Context | Python | TypeScript |
|---------|--------|------------|
| Files | `snake_case.py` | `PascalCase.tsx` (components) |
| Functions | `snake_case` | `camelCase` |
| Classes | `PascalCase` | `PascalCase` |
| Constants | `UPPER_SNAKE` | `UPPER_SNAKE` |
| Tests | `test_<unit>_<scenario>_<result>` | `describe/it` blocks |

## Commit Types

| Type | Use For | Version Impact |
|------|---------|----------------|
| `feat` | New features | Minor bump |
| `fix` | Bug fixes | Patch bump |
| `refactor` | Code restructuring | None |
| `docs` | Documentation only | None |
| `test` | Test additions/updates | None |
| `chore` | Maintenance, deps | None |
| `perf` | Performance improvements | Patch bump |

## Test Patterns

Follow AAA (Arrange-Act-Assert) and scenario-based fixture naming:

- `*_perfect` - Complete, valid data (happy path)
- `*_degraded` - Partial data, quality issues
- `*_chaos` - Edge cases, malformed data

Coverage target: 70% minimum across all components.

## Security Mindset

For every feature, ask:

1. How could this be exploited?
2. What happens if input is malicious?
3. What if authenticated but not authorized?
4. What if the system is partially compromised?

## Related Skills

- `infrastructure-patterns` - Container security, deployment hardening
- `database-patterns` - SQL injection prevention, connection security
