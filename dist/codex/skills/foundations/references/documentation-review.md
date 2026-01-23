# Documentation Review Checklist

Quality checklist for reviewing technical documentation.

## Quick Documentation Check

For every documentation change:

- [ ] Spelling and grammar correct
- [ ] Code examples compile/run
- [ ] Links work
- [ ] No outdated information

## README Review

### Essential Sections

- [ ] **Title** - Clear project name
- [ ] **Description** - What it does in 1-2 sentences
- [ ] **Installation** - How to set up locally
- [ ] **Usage** - Basic example of how to use
- [ ] **Configuration** - Environment variables, options

### Good to Have

- [ ] **Prerequisites** - Required dependencies
- [ ] **Development** - How to contribute
- [ ] **Testing** - How to run tests
- [ ] **License** - Clear licensing info

### Example Structure

```markdown
# Project Name

Brief description of what this project does.

## Installation

\`\`\`bash
npm install project-name
\`\`\`

## Usage

\`\`\`typescript
import { feature } from 'project-name';

const result = feature({ option: 'value' });
\`\`\`

## Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `API_KEY` | Your API key | Required |
| `DEBUG` | Enable debug mode | `false` |

## Development

\`\`\`bash
git clone https://github.com/org/project
cd project
npm install
npm test
\`\`\`

## License

MIT
```

## API Documentation Review

### Endpoint Documentation

Each endpoint should include:

- [ ] **HTTP method and path** - `POST /api/users`
- [ ] **Description** - What it does
- [ ] **Authentication** - Required or not
- [ ] **Request body** - Schema with types
- [ ] **Response** - Success and error schemas
- [ ] **Examples** - Request/response examples

### Example Endpoint Doc

```markdown
## Create User

Creates a new user account.

### Request

`POST /api/users`

**Authentication:** Required (API key)

**Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `email` | string | Yes | User's email address |
| `name` | string | Yes | Display name |
| `role` | string | No | User role (default: "user") |

**Example:**

\`\`\`json
{
  "email": "user@example.com",
  "name": "John Doe",
  "role": "admin"
}
\`\`\`

### Response

**Success (201 Created):**

\`\`\`json
{
  "id": "usr_123",
  "email": "user@example.com",
  "name": "John Doe",
  "role": "admin",
  "createdAt": "2024-01-15T10:30:00Z"
}
\`\`\`

**Errors:**

| Status | Code | Description |
|--------|------|-------------|
| 400 | `invalid_email` | Email format invalid |
| 409 | `email_exists` | Email already registered |
| 401 | `unauthorized` | Missing or invalid API key |
```

## Code Comment Review

### Good Comments

- [ ] Explain **why**, not what
- [ ] Document non-obvious behavior
- [ ] Include links to relevant specs/docs
- [ ] Warn about gotchas

### Bad Comments

- [ ] Restating the code
- [ ] Outdated information
- [ ] TODO without context
- [ ] Commented-out code

### Examples

```python
# BAD: Restates code
# Increment counter by 1
counter += 1

# GOOD: Explains why
# Compensate for zero-indexed array
counter += 1

# BAD: No context
# TODO: fix this

# GOOD: Actionable TODO
# TODO(#123): Handle rate limit errors - blocked on auth refactor

# GOOD: Warns about gotcha
# Note: This returns UTC, not local time. Convert if displaying to users.
```

## Changelog Review

### Follows Keep a Changelog

- [ ] Grouped by: Added, Changed, Deprecated, Removed, Fixed, Security
- [ ] Most recent version first
- [ ] Links to version comparisons
- [ ] Dates in ISO format (YYYY-MM-DD)

### Example Changelog

```markdown
# Changelog

## [1.2.0] - 2024-01-15

### Added
- User profile endpoint
- Rate limiting on API

### Changed
- Improved error messages for validation errors

### Fixed
- Memory leak in connection pool (#123)

## [1.1.0] - 2024-01-01

...

[1.2.0]: https://github.com/org/repo/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/org/repo/compare/v1.0.0...v1.1.0
```

## Architecture Documentation Review

### ADR (Architecture Decision Record)

- [ ] **Title** - Descriptive name
- [ ] **Status** - Proposed/Accepted/Deprecated/Superseded
- [ ] **Context** - Why this decision was needed
- [ ] **Decision** - What was decided
- [ ] **Consequences** - Trade-offs and impacts

### Example ADR

```markdown
# ADR-001: Use PostgreSQL for primary database

## Status

Accepted

## Context

We need a primary database for user data. Considered options:
- PostgreSQL: Strong ACID, JSON support, mature ecosystem
- MySQL: Familiar, good performance
- MongoDB: Flexible schema, horizontal scaling

## Decision

Use PostgreSQL because:
1. Strong consistency requirements for financial data
2. JSON support for flexible metadata
3. Team expertise

## Consequences

**Positive:**
- Strong data integrity
- Rich query capabilities

**Negative:**
- Less flexible schema evolution
- Vertical scaling primarily
```

## Quality Indicators

### Good Documentation

- Clear, concise language
- Consistent formatting
- Up-to-date with code
- Tested examples
- Accessible to target audience

### Signs of Poor Documentation

- Outdated screenshots
- Broken links
- Untested code samples
- Jargon without explanation
- Missing error scenarios

---

*Reference: [Write the Docs](https://www.writethedocs.org/guide/), [Google Technical Writing](https://developers.google.com/tech-writing)*
