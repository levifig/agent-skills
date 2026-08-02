# Verification Before Completion

## Contents
- Philosophy
- Quick Reference
- Verification Checklist
- The Verification Command
- Verification Levels
- Critical Rules
- Verification Patterns
- Integration with Loaf Workflow
- Red Flags
- Verification Output Format
- Related Skills

Evidence before assertions. Always.

## Philosophy

**Claims require proof.** "It works" means nothing without evidence. Run the tests. Check the output. Verify the behavior. Then make the claim.

**Verify what you changed.** If you modified authentication, verify authentication works. If you added an API endpoint, call it. Don't rely on "I think it's right."

**Fresh environment, real conditions.** Code that works in your head or with cached state may fail in reality. Test in conditions that match deployment.

**False confidence is worse than uncertainty.** Saying "done" when it's not wastes everyone's time. Better to say "I think it's done but haven't verified X" than to ship broken code.

## Quick Reference

| Before... | Verify by... |
|-----------|--------------|
| Claiming "fixed" | Running the failing test, seeing it pass |
| Marking task done | Running all related tests |
| Creating PR | Running full test suite, checking build |
| Committing | Running affected tests, checking lint |

## Verification Checklist

Before claiming any work is complete:

```
□ Tests pass (not just "should pass" — actually ran them)
□ Build succeeds (compiled, bundled, no errors)
□ Linting passes (no new warnings introduced)
□ Manual verification (if applicable, actually clicked/tested)
□ Edge cases checked (empty inputs, error conditions)
□ Documentation updated (if behavior changed)
```

## The Verification Command

Always run verification commands and **read the output**:

```bash
# Don't just run — verify the result
npm test        # Check: "X tests passed, 0 failed"
npm run build   # Check: "Build completed successfully"
npm run lint    # Check: No errors or warnings
```

**Common trap:** Running the command, not reading the output, assuming success.

## Verification Levels

| Level | When | What to Verify |
|-------|------|----------------|
| **Quick** | During development | Affected tests pass |
| **Standard** | Before commit | Full test suite, lint, build |
| **Thorough** | Before PR/deploy | E2E tests, manual verification, edge cases |

## Critical Rules

### Always

- Run tests before claiming they pass
- Read command output, don't assume success
- Verify the specific behavior you changed
- Check edge cases (empty, null, error states)
- Reproduce bugs before claiming fixed

### Never

- Say "tests pass" without running them
- Assume code works because it compiles
- Skip verification because "it's a small change"
- Claim completion without evidence
- Trust cached test results

## Verification Patterns

### Bug Fix Verification

```
1. Reproduce the bug (see it fail)
2. Apply the fix
3. Verify the bug is fixed (see it pass)
4. Run related tests (no regressions)
5. Check edge cases (similar scenarios)
```

### Feature Verification

```
1. Run new tests (they should pass)
2. Run existing tests (no regressions)
3. Manual verification (actually use the feature)
4. Edge case testing (empty, error, boundary)
5. Build verification (deploys successfully)
```

### Refactor Verification

```
1. Run full test suite BEFORE refactor
2. Make refactor changes
3. Run full test suite AFTER refactor
4. Compare coverage (no decrease)
5. Behavior unchanged (same inputs → same outputs)
```

## Integration with Loaf Workflow

| Command | Verification Point |
|---------|-------------------|
| `/implement` | Before marking session complete |
| `/breakdown` | Each task has verification criteria |
| `/shape` | Test conditions define verification |
| `/reflect` | Note verification gaps discovered |

## Red Flags

Watch for these anti-patterns:

| Statement | Red Flag |
|-----------|----------|
| "Should work" | No evidence provided |
| "Tests pass" (no output shown) | Might not have run them |
| "It works on my machine" | Environment-specific success |
| "I'm pretty sure" | Uncertainty without verification |
| "Just a small change" | Small changes can have big impacts |

## Verification Output Format

When reporting completion, include evidence:

```markdown
## Verification

**Tests:**
✓ `npm test` — 47 passed, 0 failed
✓ `npm run test:e2e` — 12 passed, 0 failed

**Build:**
✓ `npm run build` — completed in 23s

**Manual:**
✓ Logged in with valid credentials
✓ Login rejected with invalid password
✓ Session persists across page refresh
```

## Related Skills

- `tdd` - Tests as verification foundation
- `debugging` - When verification fails
- `foundations` - Testing patterns and standards
