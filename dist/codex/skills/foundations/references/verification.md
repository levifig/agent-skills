# Verification Reference

Pre-completion checks to ensure work is actually complete before claiming it.

## When to Verify

| Trigger | What to Check |
|---------|---------------|
| Before claiming "done" | All relevant checks pass |
| Before creating commits | Tests, types, lint |
| Before creating PRs | Full verification suite |
| After making changes | The specific thing changed |

## What to Verify

| Check | Purpose | Priority |
|-------|---------|----------|
| Tests pass | Behavior correctness | Required |
| Build succeeds | Code compiles/bundles | Required |
| Type checks pass | Type safety | Required |
| Linting passes | Code standards | Required |
| Manual verification | UI/UX correctness | When applicable |

## Common Verification Commands

### By Language/Framework

| Stack | Commands |
|-------|----------|
| **Python** | `pytest`, `mypy .`, `ruff check .` |
| **TypeScript/JS** | `npm test`, `npm run build`, `npm run lint`, `tsc --noEmit` |
| **Ruby** | `rails test`, `rubocop` |
| **Go** | `go test ./...`, `go vet ./...` |
| **Rust** | `cargo test`, `cargo clippy` |

### General Commands

```bash
# Check git status for unexpected changes
git status

# Build the project
npm run build    # Node projects
make build       # Make-based projects

# Run all checks (if available)
npm run check    # Often combines lint + type + test
make check
```

### Project-Specific

Always check `package.json`, `Makefile`, or equivalent for project-specific commands:

```bash
# Common script names to look for
npm run test
npm run lint
npm run typecheck
npm run build
npm run check
```

## Verification Mindset

### Evidence Before Assertions

```bash
# Wrong: Claiming without running
"Tests pass and the build succeeds."

# Right: Run, then report
npm test && npm run build
# Output shows success, then claim completion
```

### Verify What You Changed

If you modified a calculation:
- Run tests covering that calculation
- Check edge cases for that calculation
- Verify output format if applicable

If you modified an API endpoint:
- Hit the endpoint manually or via tests
- Check error cases
- Verify response format

### Check Edge Cases

| Change Type | Edge Cases to Verify |
|-------------|---------------------|
| Input validation | Empty, null, malformed, boundary values |
| Calculations | Zero, negative, very large, precision |
| String handling | Empty, unicode, injection attempts |
| Collections | Empty, single item, large sets |

## Verification Checklist

Before any "done" claim:

- [ ] `git status` shows expected changes only
- [ ] Tests pass (run them, don't assume)
- [ ] Build succeeds (run it, don't assume)
- [ ] Type checks pass (if applicable)
- [ ] Lint passes (if applicable)
- [ ] Changed functionality manually verified (if applicable)

## Examples

### Completing a Feature

```bash
# 1. Check what changed
git status
git diff

# 2. Run verification suite
npm test
npm run build
npm run lint

# 3. Manual check if UI/behavior change
# Open the app, test the feature

# 4. Only then claim completion
```

### Fixing a Bug

```bash
# 1. Write/update test that reproduces the bug
npm test -- --grep "specific test"

# 2. Fix the bug

# 3. Verify the fix
npm test -- --grep "specific test"  # Should pass now
npm test                            # Full suite still passes

# 4. Verify no regressions
npm run build
```

## Critical Rules

### Always

- Run verification commands before any "done" claim
- Check `git status` for unexpected changes
- Verify the specific thing you changed works
- Report actual command output, not assumptions

### Never

- Claim "tests pass" without running them
- Assume the build still works after changes
- Skip verification "because it's a small change"
- Claim completion based on what should work
