---
name: debugging
description: >-
  Use for systematic bug investigation and diagnosis. Covers hypothesis-driven
  debugging workflows, multi-hypothesis tracking, language-specific debugging
  patterns (Python, TypeScript, Ruby), and test debugging (flaky tests, isolation,
  state pollution). Activate when investigating failures, debugging production
  issues, or diagnosing test problems.
---

# Systematic Debugging

Hypothesis-driven investigation for efficient and traceable bug diagnosis.

## Philosophy

**Eliminate, don't guess.** Debugging is a process of elimination, not random modification. Each change should test a specific hypothesis and provide evidence to rule it out or confirm it.

**Multiple hypotheses, ranked.** Never fixate on one theory. Generate 3-5 plausible hypotheses, rank by likelihood, and test the most likely first. Surprising evidence should trigger hypothesis revision.

**Reproduce before investigating.** A bug you cannot reproduce is a bug you cannot fix with confidence. Establish reliable reproduction steps before diving into code.

**Minimal changes, maximum information.** Each debugging step should change one variable and maximize the information gained. Shotgun debugging wastes time and obscures root causes.

## Quick Reference

| Situation | Approach | Key Technique |
|-----------|----------|---------------|
| **Unknown failure** | Generate hypotheses | List 3-5 possible causes, rank by likelihood |
| **Intermittent bug** | Isolate variables | Binary search through conditions |
| **Test flakiness** | Check state pollution | Run test in isolation, check fixtures |
| **Production issue** | Preserve evidence | Capture logs/state before attempting fixes |

## Topics

| Topic | Reference | Use For |
|-------|-----------|---------|
| Hypothesis Tracking | `references/hypothesis-tracking.md` | Multi-hypothesis workflow, session templates, investigation logs |
| Language Debugging | `references/language-debugging.md` | Python, TypeScript, Ruby debug patterns and tools |
| Test Debugging | `references/test-debugging.md` | Flaky tests, isolation, state pollution, environment differences |

## Critical Rules

### Always

- Generate multiple hypotheses before investigating
- Document each hypothesis and its current status
- Establish reproduction steps before attempting fixes
- Test one variable at a time
- Preserve evidence (logs, state, stack traces) before changes
- Update hypothesis status based on evidence
- Record failed attempts to avoid repeating them

### Never

- Make random changes hoping something works
- Fix symptoms without understanding root cause
- Assume the first hypothesis is correct
- Skip reproduction verification after a "fix"
- Delete logs or error output before analysis
- Change multiple variables simultaneously
- Ignore contradictory evidence

## Debugging Workflow

```
1. REPRODUCE: Establish reliable reproduction steps
2. HYPOTHESIZE: Generate 3-5 possible causes, rank by likelihood
3. TEST: Design experiment to test highest-likelihood hypothesis
4. EVALUATE: Analyze results, update hypothesis status
5. ITERATE: Move to next hypothesis or refine based on evidence
6. VERIFY: Confirm fix addresses root cause, not just symptoms
```

## Investigation Log Format

Keep a timestamped log of investigation steps:

```
[HH:MM] Testing H2 (stale cache)
        Action: Cleared Redis, restarted service
        Result: Still failing
        Evidence: Error occurs before cache read in stack trace
        Status: H2 RULED OUT

[HH:MM] Testing H3 (race condition)
        Action: Added mutex around shared state
        Result: SUCCESS - no failures in 100 runs
        Evidence: Stack trace showed concurrent access
        Status: H3 CONFIRMED
```

## Related Skills

- `foundations` - Test patterns, error handling, logging standards
- `python` - Python-specific debugging tools (pdb, logging)
- `typescript` - TypeScript debugging (Chrome DevTools, source maps)
- `ruby` - Ruby debugging (binding.irb, byebug)
