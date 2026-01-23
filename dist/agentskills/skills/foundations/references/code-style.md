# Code Style

Universal coding principles that apply across all languages. For syntax and tooling, see language-specific skills.

## Philosophy

**Code speaks first.** Well-structured code with clear names needs fewer comments. When comments are necessary, they explain WHY, not WHAT.

**Consistency matters.** Use consistent naming, formatting, and organization patterns within each language. Follow language-specific conventions.

**Adapt, don't impose.** These are universal principles. Adapt them to each language's idioms rather than forcing patterns that don't fit.

## Code Quality

**Readability over cleverness.** Prioritize maintainability over DRYness or clever abstractions. Three similar lines are often better than a premature abstraction.

**Single responsibility.** Keep files focused. Long files signal a need to split. Each module should do one thing well.

**Simplicity first.** Prefer simple structures over complex ones. Use existing implementations over reinventing.

## Logging

Use structured logging with context fields. Structured fields are queryable in log aggregators; string interpolation buries data in unstructured text. Consistent field names enable cross-service correlation.

| Always | Never |
|--------|-------|
| Use key-value context fields | Interpolate data into message strings |
| Include correlation IDs for tracing | Log sensitive data (PII, secrets) |
| Log at appropriate levels (info, warn, error) | Use print statements in production |

## Error Handling

| Always | Never |
|--------|-------|
| Catch specific exceptions for known failure modes | Use bare/generic catches that hide bugs |
| Chain exceptions to preserve stack traces | Swallow errors silently |
| Include context identifiers for debugging | Expose internal details in user-facing errors |
| Fail fast on unexpected errors | Catch and ignore without logging |

## Comment Policy

### Hierarchy

1. **Clear Code** - Self-documenting names and structure (preferred)
2. **Contextual Comments** - Short docstrings, WHY comments
3. **Documentation Files** - ADRs, guides (last resort)

### Comment Content

| Always | Never |
|--------|-------|
| Explain WHY a non-obvious approach was chosen | Restate what code obviously does |
| Document regulatory/compliance requirements | Leave commented-out dead code |
| Note workarounds for library bugs (with links) | Explain magic numbers (use named constants) |
| Include ticket references in TODOs | Write comments that will become stale |

## API Documentation

| Always | Never |
|--------|-------|
| Document only implemented, released endpoints | Document planned/future endpoints in specs |
| Update docs after features ship | Write API docs before implementation |
| Keep OpenAPI/Swagger specs matching production | Allow spec drift from reality |

## Test Principles

**Structure with AAA.** Organize all tests with clear Arrange-Act-Assert separation. Each section should be visually distinct.

**Parameterize repetition.** When testing multiple inputs with the same logic, use parameterized/table-driven tests rather than duplicating test bodies.

**Mirror source structure.** Organize tests to reflect source code organization. Separate unit, integration, and e2e tests.

| Always | Never |
|--------|-------|
| One assertion concept per test | Test multiple unrelated behaviors together |
| Use descriptive test names stating scenario and expectation | Use vague names like `test_function` |
| Test edge cases and error paths | Only test happy paths |
| Keep tests fast and isolated | Let tests depend on execution order |
| Use fixtures for reusable test data | Duplicate setup across tests |

## Naming Conventions

Follow language-specific conventions consistently:

| Context | Python | TypeScript | Rails | Go |
|---------|--------|------------|-------|-----|
| Files | `snake_case.py` | `PascalCase.tsx` | `snake_case.rb` | `snake_case.go` |
| Functions | `snake_case` | `camelCase` | `snake_case` | `PascalCase`/`camelCase` |
| Classes/Types | `PascalCase` | `PascalCase` | `PascalCase` | `PascalCase` |
| Constants | `UPPER_SNAKE` | `UPPER_SNAKE` | `UPPER_SNAKE` | `UPPER_SNAKE` |
| Tests | `test_<unit>_<scenario>` | `describe/it` blocks | `test_<unit>_<scenario>` | `Test<Unit><Scenario>` |

## Implementation

See language-specific skills for syntax and tooling:
- `python` skill for Python/FastAPI/pytest
- `typescript` skill for TypeScript/React/Vitest
- `rails` skill for Ruby/Rails/Minitest
