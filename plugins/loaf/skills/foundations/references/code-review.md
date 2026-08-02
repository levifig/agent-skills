# Code Review

## Contents
- Review Workflow
- PR Requirements
- Comment Protocol
- Receiving Review
- Integration with Loaf Workflow

Project code review conventions and workflow.

## Review Workflow

| Role | Focus | Key Behavior |
|------|-------|--------------|
| **Requesting** | Make it reviewable | Small scope, clear description, tested |
| **Giving** | Be specific and constructive | Explain why, suggest alternatives |
| **Receiving** | Verify before implementing | Check correctness, ask questions |

## PR Requirements

### Before Requesting

```
[ ] PR is focused (one concern)
[ ] Tests pass and cover new behavior
[ ] Self-reviewed (read your own diff)
[ ] Description explains what and why
[ ] No debug code, console.logs, or TODOs
```

### PR Description Template

```markdown
## Summary
[One paragraph: what this PR does and why]

## Changes
- [Specific change 1]
- [Specific change 2]

## Testing
- [How you verified this works]
- [Edge cases considered]

## Notes for Reviewer
- [Areas of uncertainty]
- [Alternative approaches considered]
```

## Comment Protocol

| Type | Example | Priority |
|------|---------|----------|
| **Blocking** | "This will cause a null pointer exception" | Must fix |
| **Suggestion** | "Consider using X for better performance" | Optional |
| **Question** | "Why this approach over Y?" | Needs answer |
| **Nitpick** | "Prefer `const` over `let` here" | Low |

**Label your comments** so authors know priority.

**Constructive feedback pattern:**
```
[What]: Describe the issue specifically
[Why]: Explain the concern (correctness, performance, maintainability)
[Suggestion]: Offer an alternative if you have one
```

## Receiving Review

**Don't blindly accept feedback.** Reviewers can be wrong. Before implementing:

1. **Understand it:** Make sure you understand the suggestion
2. **Verify it:** Check if the suggestion is technically correct
3. **Test it:** If you implement, verify the change works

| Comment Type | Response |
|--------------|----------|
| Valid fix | Implement, reply "Fixed" or "Good catch" |
| Suggestion you'll take | Implement, explain any modifications |
| Suggestion you won't take | Explain why with reasoning |
| Incorrect suggestion | Respectfully explain why with evidence |

## Integration with Loaf Workflow

| Command | Code Review Role |
|---------|-----------------|
| `/implement` | Self-review before marking complete |
| `/breakdown` | Review task scope and approach |
| `/reflect` | Note review feedback patterns |
