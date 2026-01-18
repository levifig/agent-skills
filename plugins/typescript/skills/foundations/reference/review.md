# Code Review Checklist

Comprehensive code review checklist for ensuring code quality, maintainability, and correctness.

## Quick Review (< 5 minutes)

Use for small PRs (< 50 lines):

- [ ] Code compiles and tests pass
- [ ] Naming is clear and descriptive
- [ ] No obvious bugs or logic errors
- [ ] No hardcoded secrets or credentials

## Standard Review (5-15 minutes)

Use for typical PRs:

### Code Quality
- [ ] Functions have single responsibility
- [ ] No deeply nested logic (max 3 levels)
- [ ] No magic numbers (use named constants)
- [ ] No commented-out code
- [ ] Error messages are helpful

### Naming & Readability
- [ ] Variables describe what they hold
- [ ] Functions describe what they do
- [ ] Classes represent clear concepts
- [ ] Abbreviations are avoided or well-known

### Type Safety
- [ ] All function parameters have types
- [ ] Return types are explicit
- [ ] No `any` types (TypeScript) or missing hints (Python)
- [ ] Generics used appropriately

### Error Handling
- [ ] Errors don't fail silently
- [ ] Error messages include context
- [ ] Resources cleaned up on error
- [ ] Appropriate error types used

## Deep Review (15-30 minutes)

Use for critical paths, security-sensitive code, or architectural changes:

### Architecture
- [ ] Follows existing patterns in codebase
- [ ] Abstractions are at right level
- [ ] Dependencies flow in correct direction
- [ ] No circular dependencies
- [ ] Changes are backwards compatible

### Performance
- [ ] No N+1 queries
- [ ] Expensive operations cached when appropriate
- [ ] No unnecessary allocations in hot paths
- [ ] Pagination for large data sets

### Security
- [ ] Input validated at boundaries
- [ ] Output encoded appropriately
- [ ] SQL uses parameterized queries
- [ ] Authentication/authorization checked
- [ ] Secrets not logged or exposed

### Testing
- [ ] New code has tests
- [ ] Tests are meaningful (not just coverage)
- [ ] Edge cases covered
- [ ] Error paths tested
- [ ] Tests don't depend on execution order

### Documentation
- [ ] Public APIs documented
- [ ] Complex algorithms explained
- [ ] Non-obvious decisions commented
- [ ] README updated if needed

## Review Response Templates

### Approve

```
LGTM!

Minor suggestions (optional):
- [suggestion]

The implementation is clean and well-tested.
```

### Request Changes

```
Good progress. A few items to address:

**Must fix:**
- [blocking issue]

**Should fix:**
- [important but not blocking]

**Consider:**
- [optional improvement]

Happy to discuss any of these.
```

### Need More Context

```
I need more context to review this effectively:

- What problem is this solving?
- What alternatives were considered?
- How was this tested?

Could you update the PR description?
```

## Common Issues to Flag

### Always Flag
- Hardcoded secrets
- SQL injection vulnerabilities
- Missing input validation
- Unbounded loops or recursion
- Memory leaks
- Race conditions

### Usually Flag
- Missing error handling
- No tests for new code
- Magic numbers
- Overly complex functions
- Copy-pasted code

### Consider Flagging
- Inconsistent style
- Missing type annotations
- Verbose code that could be simplified
- Missing documentation for public APIs

## Review Mindset

1. **Assume good intent** - reviewer and author want the same thing
2. **Be specific** - point to exact lines, provide examples
3. **Explain why** - don't just say "change this", explain the reason
4. **Offer alternatives** - suggest better approaches, don't just criticize
5. **Separate blocking from optional** - be clear about what must change
6. **Timebox** - don't spend hours on minor PRs

---

*Reference: [foundations/code-style](./code-style.md) for style guidelines*
