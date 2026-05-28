---
id: TASK-029
title: '`loaf init` — project detection + skill recommendations'
spec: SPEC-007
status: done
priority: P2
created: '2026-03-16T16:27:15.461Z'
updated: '2026-03-17T00:16:46.290Z'
depends_on:
  - TASK-028
files:
  - cli/lib/detect/project.ts
  - cli/commands/init.ts
verify: loaf init (in a Python project) shows Python skill recommendations
done: '`loaf init` detects languages/frameworks and recommends relevant Loaf skills'
completed_at: '2026-03-17T00:16:46.290Z'
---

# TASK-029: loaf init — project detection + skill recommendations

## Description

Add project detection to `loaf init`: identify languages, frameworks, and map them to Loaf skill groups.

## Detection indicators

| Language | File indicators |
|----------|----------------|
| Python | `pyproject.toml`, `setup.py`, `requirements.txt`, `Pipfile` |
| TypeScript/JS | `package.json`, `tsconfig.json` |
| Ruby | `Gemfile`, `.ruby-version` |
| Go | `go.mod` |

| Framework | Indicator |
|-----------|-----------|
| Next.js | `next.config.*` |
| FastAPI | `fastapi` in pyproject.toml deps |
| Rails | `rails` in Gemfile |
| Django | `django` in requirements/pyproject |

## Skill mapping

Use `plugin-groups` from `config/hooks.yaml` to map detected stack → skill groups. E.g., Python detected → recommend `python` group (python-development, foundations).

## Acceptance Criteria

- [ ] Detects Python/TypeScript/Ruby/Go projects
- [ ] Detects common frameworks
- [ ] Maps to Loaf skill groups from hooks.yaml
- [ ] Prints detection results during `loaf init`
- [ ] Works when no project files detected (just skips)

## Context

See SPEC-007 for full context. Circuit breaker 75%.

## Work Log

<!-- Updated by session as work progresses -->
