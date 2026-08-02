# Test-Driven Development

## Contents
- TDD Cycle
- Critical Rules
- When TDD Feels Hard
- TDD for Bug Fixes
- Integration with Loaf Workflow

Project TDD conventions and workflow.

## TDD Cycle

| Phase | Goal | Duration |
|-------|------|----------|
| **Red** | Write failing test | 2-5 min |
| **Green** | Minimal code to pass | 5-15 min |
| **Refactor** | Clean up, no new behavior | 5-10 min |

**Red:** Pick ONE behavior, write the test BEFORE implementation, run it, verify it fails for the RIGHT reason (missing behavior, not syntax error).

**Green:** Write MINIMUM code to pass. Don't optimize, don't handle untested edge cases. "Fake it till you make it" is valid.

**Refactor:** Only when green. Remove duplication, improve names, extract methods. Run tests after EACH change.

## Critical Rules

**Always:** Write test before implementation, watch it fail, write minimal code to pass, refactor only when green, keep cycles under 30 minutes, one behavior per test.

**Never:** Write implementation before test, skip the red phase, add features during green phase, refactor while tests fail, write multiple tests before any implementation, test implementation details (test behavior, not how).

## When TDD Feels Hard

| Symptom | Likely Cause | Solution |
|---------|--------------|----------|
| Can't write test first | Don't understand requirements | Clarify with `/brainstorm` or `/shape` |
| Test is too complex | Testing too much at once | Break into smaller behaviors |
| Implementation explodes | Test scope too large | Smaller test, smaller implementation |
| Refactor breaks tests | Tests coupled to implementation | Test behavior, not structure |
| Failure cause isn't obvious | Multiple possible root causes | Load the **debugging** skill for structured hypothesis tracking |

## TDD for Bug Fixes

1. **Write a test that reproduces the bug** (should fail)
2. **Fix the bug** (test passes)
3. **Refactor if needed**

This ensures the bug is understood, actually fixed, and won't regress.

If the failing test points to a non-obvious root cause, or if your first fix attempt doesn't work, load the **debugging** skill before continuing. It provides structured hypothesis tracking that prevents trial-and-error loops.

## Integration with Loaf Workflow

| Phase | TDD Role |
|-------|----------|
| `/shape` | Test conditions become TDD test cases |
| `/breakdown` | Each task should have clear test targets |
| `/implement` | Follow TDD cycle for each task |
| `/reflect` | Note TDD friction points for improvement |
