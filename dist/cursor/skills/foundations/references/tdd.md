# Test-Driven Development

Write the test first. Then make it pass. Then make it clean.

## Philosophy

**Tests are specifications.** A test describes what the code should do before the code exists. Writing the test first forces clarity about requirements.

**Red → Green → Refactor.** The rhythm is non-negotiable. See it fail (proves test works), make it pass (minimal implementation), clean it up (refactor with confidence).

**Small cycles, fast feedback.** Each cycle should be minutes, not hours. If you're writing lots of code before running tests, the cycle is too big.

**Tests enable refactoring.** Without tests, refactoring is gambling. With tests, you can restructure code confidently knowing behavior is preserved.

## Quick Reference

| Phase | Goal | Duration |
|-------|------|----------|
| **Red** | Write failing test | 2-5 min |
| **Green** | Minimal code to pass | 5-15 min |
| **Refactor** | Clean up, no new behavior | 5-10 min |

## The Cycle

### 1. Red: Write a Failing Test

```
- Pick ONE specific behavior to test
- Write the test BEFORE any implementation
- Run test → verify it FAILS
- Failure should be for the RIGHT reason (missing behavior, not syntax error)
```

**Good test names describe behavior:**
- `test_user_cannot_login_with_wrong_password`
- `test_empty_cart_shows_zero_total`
- `test_expired_token_returns_401`

### 2. Green: Make It Pass

```
- Write the MINIMUM code to make the test pass
- Don't optimize, don't handle edge cases yet
- "Fake it till you make it" is valid (hardcode if needed)
- Run test → verify it PASSES
```

**Resist the urge to:**
- Add features not required by the test
- Optimize prematurely
- Handle edge cases not yet tested
- Refactor while making it pass

### 3. Refactor: Clean It Up

```
- Tests pass → safe to restructure
- Remove duplication
- Improve names
- Extract methods/functions
- Run tests after EACH change
```

**Only refactor when green.** If tests fail during refactor, you've changed behavior.

## Critical Rules

### Always

- Write the test BEFORE the implementation
- Run the test and watch it fail
- Write minimal code to pass
- Refactor only when green
- Keep cycles short (under 30 minutes total)
- One behavior per test

### Never

- Write implementation before test
- Skip the "red" phase
- Add features during "green" phase
- Refactor while tests are failing
- Write multiple tests before any implementation
- Test implementation details (test behavior, not how)

## Test Structure

Use Arrange-Act-Assert (AAA):

```python
def test_user_can_update_email():
    # Arrange: Set up preconditions
    user = create_user(email="old@example.com")

    # Act: Perform the action
    user.update_email("new@example.com")

    # Assert: Verify the outcome
    assert user.email == "new@example.com"
```

## When TDD Feels Hard

| Symptom | Likely Cause | Solution |
|---------|--------------|----------|
| Can't write test first | Don't understand requirements | Clarify with `/brainstorm` or `/shape` |
| Test is too complex | Testing too much at once | Break into smaller behaviors |
| Implementation explodes | Test scope too large | Smaller test, smaller implementation |
| Refactor breaks tests | Tests coupled to implementation | Test behavior, not structure |

## TDD for Bug Fixes

1. **Write a test that reproduces the bug** (should fail)
2. **Fix the bug** (test passes)
3. **Refactor if needed**

This ensures:
- Bug is understood (test documents it)
- Bug is actually fixed (test proves it)
- Bug won't regress (test prevents it)

## Integration with Loaf Workflow

| Phase | TDD Role |
|-------|----------|
| `/shape` | Test conditions become TDD test cases |
| `/breakdown` | Each task should have clear test targets |
| `/implement` | Follow TDD cycle for each task |
| `/reflect` | Note TDD friction points for improvement |

## Related Skills

- `debugging` - When tests fail unexpectedly
- `foundations` - Test patterns, assertions, fixtures
- Language skills (`python`, `typescript`, `ruby`) - Language-specific testing tools
