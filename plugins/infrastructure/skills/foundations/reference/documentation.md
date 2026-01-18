# Documentation Standards

Guidelines for technical documentation, ADRs, and API docs.

## Core Principle

**Document after shipping, not before.**

API documentation reflects ONLY what is implemented and released. Future features belong in Linear issues, not docs.

## Document Hierarchy

```
docs/
├── PRD.md                # What the product should be (vision)
├── ARCHITECTURE.md       # How the product is built (design)
├── IMPLEMENTATION.md     # What the product currently is (status)
├── QUICK_REFERENCE.md    # One-page command reference
├── api/                  # API docs (implemented features only)
│   ├── openapi.yaml
│   └── endpoints/
└── decisions/            # Architecture Decision Records
    ├── ADR000-template.md
    └── ADR001-*.md
```

## Document Separation

| Document | Purpose | Updates When |
|----------|---------|--------------|
| **PRD.md** | Product vision (timeless) | Vision changes |
| **ARCHITECTURE.md** | Technical design | Architecture changes |
| **IMPLEMENTATION.md** | Current status | Features ship |
| **API docs** | Implemented endpoints | Features ship |

## Architecture Decision Records (ADRs)

### When to Write

| Write ADR | Skip ADR |
|-----------|----------|
| Technology choices | Library version updates |
| Architectural patterns | Bug fixes |
| Integration approaches | Performance tweaks |
| Security decisions | Code style changes |

### File Naming

```
docs/decisions/ADRXXX-short-descriptive-title.md
```

Example: `ADR001-postgresql-only-no-redis.md`

### Template

```markdown
# ADR-XXX: Title

**Decision Date**: YYYY-MM-DD

**Status**: Proposed | Accepted | Deprecated | Superseded

## Context

[Describe the situation and problem. What constraints exist?]

## Decision

[State the decision clearly. Use "We will..." not "It was decided..."]

## Consequences

### Positive
- [Benefit 1]

### Negative
- [Drawback 1]

## Alternatives Considered

### Alternative 1: [Name]
[Brief description and why rejected]
```

### Status Lifecycle

```
Proposed -> Accepted -> (Deprecated | Superseded)
```

## API Documentation

### The Implemented-Only Rule

```
Feature Request -> Implementation -> Tests Pass -> Release -> Update API Docs
                                                                     ^
                                                               Only here!
```

### OpenAPI Spec

```yaml
openapi: 3.1.0
info:
  title: Project API
  version: 1.2.0  # Matches actual release version
  description: |
    **Last Updated**: 2025-11-14
```

### Deprecation

```yaml
paths:
  /api/v1/legacy-endpoint:
    get:
      deprecated: true
      summary: Legacy endpoint (use /api/v2/new-endpoint)
      description: |
        **Deprecated since**: v1.5.0
        **Removal planned**: v2.0.0
```

## Micro-Changelog

Track changes within individual documents.

### Placement

Always at the **bottom** of the document:

```markdown
# Document Title

[Content...]

---

## Changelog

- 2025-11-14 - Added section on agent instructions
- 2025-11-12 - Updated architecture overview
- 2025-11-10 - Initial document creation
```

### Format Rules

- **Date format**: `YYYY-MM-DD`
- **Order**: Reverse chronological (newest first)
- **Entry format**: `- YYYY-MM-DD - Short description`
- **Single line**: Per entry

### What to Log

| DO Log | DON'T Log |
|--------|-----------|
| Section additions | Typo fixes |
| Significant updates | Formatting changes |
| Restructuring | Link updates |
| Corrections | Minor wording |
| Deprecations | |

### Header Pairing

```markdown
# Architecture Overview

**Last Updated**: 2025-11-14

[Content...]

---

## Changelog
- 2025-11-14 - ...
```

## Critical Rules

### Always

- Include Last Updated timestamp (YYYY-MM-DD)
- Add micro-changelog at document bottom
- Reference files, not inline code
- Keep docs minimal - delete cruft

### Never

- Document APIs before they ship
- Include session/council file references
- Use lengthy code samples in docs
- Add planning details to docs (use Linear)
