# Commit Conventions

Git commit, branch, and pull request standards.

## Commit Message Format

```
<type>: <description>

[optional body]

[optional footer]
```

### Types

| Type | Use For | Version Impact |
|------|---------|----------------|
| `feat` | New features | Minor bump |
| `fix` | Bug fixes | Patch bump |
| `refactor` | Code restructuring | None |
| `perf` | Performance improvements | Patch bump |
| `test` | Test additions/updates | None |
| `docs` | Documentation only | None |
| `chore` | Maintenance, deps, config | None |
| `ci` | CI/CD changes | None |
| `build` | Build system changes | None |

### Description Rules

- **Imperative mood**: "add feature" not "added feature"
- **Lowercase**: Start with lowercase after type
- **No period**: Don't end with a period
- **Short**: Under 72 characters
- **Focus on why**: The diff shows what

### Examples

```bash
# Good
feat: add thermal rating calculation
fix: prevent divide by zero in sag calculation
refactor: extract common validation logic

# Bad
feat: Added thermal rating calculation.  # Past tense, period
fix: Fixed the bug  # Vague, past tense
refactor: refactored code  # Redundant, past tense
```

## Commit Body

Add a body when:
- The "why" isn't obvious from title
- Trade-offs need documenting
- Implementation needs context

```
feat: add CIGRE TB 601 thermal model

Implement steady-state heat balance calculation per CIGRE TB 601.
Uses Newton-Raphson iteration for temperature convergence.

Key implementation notes:
- Natural convection below 0.5 m/s wind speed
- Film temperature for air property evaluation
- Tolerance: 0.1C for convergence
```

### What to Avoid in Body

- File lists (the diff shows this)
- Detailed code explanation
- Agent attribution
- Verbose descriptions

## Linear Integration

Use magic words in footer to link/close issues:

```
feat: add thermal rating API endpoint

Implement GET /api/towers/{id}/thermal-rating endpoint.

Closes BACK-123
```

### Keywords

| Keyword | Effect | Use For |
|---------|--------|---------|
| `Closes BACK-XXX` | Auto-closes on merge | Features, tasks |
| `Fixes BACK-XXX` | Auto-closes on merge | Bug fixes |
| `Resolves BACK-XXX` | Auto-closes on merge | Alternative |
| `Refs BACK-XXX` | Reference only | Related work |
| `Part of BACK-XXX` | Reference only | Partial work |

## Branch Naming

```
<type>/<description>
<type>/TASK-123-description
```

### Types

- `feature/` - New features
- `bugfix/` - Bug fixes
- `hotfix/` - Critical production fixes
- `release/` - Release preparation

### Rules

- Lowercase with hyphens (kebab-case)
- Short but descriptive (max 50 chars)
- Include task ID when applicable

## Pull Request Format

### Title

Same format as commit messages:

```
feat: add thermal rating calculation (#123)
```

### Description Template

```markdown
## Summary

Brief description of what this PR adds/changes and why.

## Changes

- Added thermal rating calculation endpoint
- Implemented CIGRE TB 601 heat balance model
- Added unit tests for thermal model

## Testing

- [ ] Unit tests added/updated
- [ ] Integration tests pass
- [ ] Manual testing performed

## Related Issues

Closes BACK-123
Refs BACK-124
```

## Critical Rules

### Always

- Write atomic commits (one logical change)
- Use imperative mood in messages
- Reference issue numbers when applicable
- Test before committing

### Never

- Skip commit signing (wait for user if it fails)
- Push without explicit user confirmation
- Use scoped commits (`feat(scope):` - use `feat:` instead)
- Include file lists in message
- Add agent attribution
- Mix unrelated changes
- Commit secrets or sensitive data

## Semantic Versioning

```
MAJOR.MINOR.PATCH

1.0.0 -> 1.0.1 (patch: bug fixes)
1.0.1 -> 1.1.0 (minor: new features)
1.1.0 -> 2.0.0 (major: breaking changes)
```

Breaking changes use `feat!:` or `fix!:` and include:

```
BREAKING CHANGE: Description of breaking change.
```
