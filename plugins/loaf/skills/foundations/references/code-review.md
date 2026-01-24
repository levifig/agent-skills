# Code Review

Give and receive feedback that improves code quality.

## Philosophy

**Reviews improve code, not ego.** The goal is better software, not proving who's right. Approach reviews with curiosity, not defensiveness.

**Reviewable PRs get reviewed.** Small, focused, well-documented changes get thoughtful reviews. Massive PRs get rubber-stamped or ignored.

**Disagreement requires evidence.** "I don't like this" isn't feedback. "This violates X principle because Y" is. Both giving and receiving feedback should be grounded in technical reasoning.

**Don't perform agreement.** When receiving feedback, verify it's correct before implementing. Blindly accepting suggestions can introduce bugs. Push back constructively when feedback seems wrong.

## Quick Reference

| Role | Focus | Key Behavior |
|------|-------|--------------|
| **Requesting** | Make it reviewable | Small scope, clear description, tested |
| **Giving** | Be specific and constructive | Explain why, suggest alternatives |
| **Receiving** | Verify before implementing | Check correctness, ask questions |

---

## Requesting Review

### Before Requesting

```
□ PR is focused (one concern, not feature creep)
□ Tests pass and cover new behavior
□ Self-reviewed (read your own diff)
□ Description explains what and why
□ No debug code, console.logs, or TODOs
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

### What Makes PRs Reviewable

| Good | Bad |
|------|-----|
| < 400 lines changed | 2000+ lines "refactor" |
| Single focused change | Multiple unrelated changes |
| Clear commit history | One giant commit |
| Explains the "why" | Just describes the "what" |
| Tests included | "Will add tests later" |

---

## Giving Review

### Types of Comments

| Type | Example | When to Use |
|------|---------|-------------|
| **Blocking** | "This will cause a null pointer exception" | Must fix before merge |
| **Suggestion** | "Consider using X for better performance" | Improvement, not required |
| **Question** | "Why this approach over Y?" | Seeking understanding |
| **Nitpick** | "Prefer `const` over `let` here" | Style, low priority |

**Label your comments** so authors know priority.

### Constructive Feedback Pattern

```
[What]: Describe the issue specifically
[Why]: Explain the concern (correctness, performance, maintainability)
[Suggestion]: Offer an alternative if you have one
```

**Bad:** "This is wrong"
**Good:** "This query runs inside a loop, causing N+1 queries. Consider eager loading: `User.includes(:posts).where(...)`"

### Review Checklist

```
□ Does the code do what the PR claims?
□ Are there obvious bugs or edge cases missed?
□ Is the approach reasonable for the problem?
□ Are tests meaningful (not just coverage theater)?
□ Is the code maintainable (readable, not clever)?
□ Any security concerns?
```

---

## Receiving Review

### Critical Principle: Verify Before Implementing

**Don't blindly accept feedback.** Reviewers can be wrong. Before implementing a suggestion:

1. **Understand it:** Make sure you understand what's being suggested
2. **Verify it:** Check if the suggestion is technically correct
3. **Test it:** If you implement, verify the change works

### When Feedback Seems Wrong

Don't perform agreement. If you disagree:

```
1. State your understanding of the suggestion
2. Explain your concern with evidence
3. Ask for clarification if needed
4. Propose an alternative if you have one
```

**Example:**
> Reviewer: "This should use async/await instead of Promises"
>
> Response: "I used Promises here because we need `Promise.all` for concurrent requests. Async/await would make these sequential, increasing latency from ~200ms to ~800ms. Happy to discuss if I'm missing something."

### Responding to Comments

| Comment Type | Response |
|--------------|----------|
| Valid fix | Implement, reply "Fixed" or "Good catch" |
| Suggestion you'll take | Implement, explain any modifications |
| Suggestion you won't take | Explain why with reasoning |
| Question | Answer thoroughly |
| Misunderstanding | Clarify, update code/docs if unclear |
| Incorrect suggestion | Respectfully explain why it's incorrect |

### Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| "You're right, will fix" (without checking) | Verify the suggestion is correct |
| Argue without evidence | Provide technical reasoning |
| Ignore comments | Respond to every comment |
| Take it personally | Focus on the code, not yourself |
| Ghost the reviewer | Communicate timeline and blockers |

---

## Integration with Loaf Workflow

| Command | Code Review Role |
|---------|-----------------|
| `/implement` | Self-review before marking complete |
| `/breakdown` | Review task scope and approach |
| `/reflect` | Note review feedback patterns |

## Review as Learning

Code review is a learning opportunity for both parties:

- **Authors learn:** Better patterns, missed edge cases, team conventions
- **Reviewers learn:** Different approaches, domain context, new techniques

Approach every review as a chance to learn something.

## Related Skills

- `verification` - Verify before claiming review feedback is addressed
- `tdd` - Tests make code more reviewable
- `foundations` - Coding standards that inform reviews
