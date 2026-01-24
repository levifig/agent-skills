# Hypothesis Tracking

Systematic workflow for generating, testing, and tracking multiple debugging hypotheses.

## Multi-Hypothesis Workflow

### 1. Generate Hypotheses

Before investigating, brainstorm 3-5 possible causes. Consider:

- **Recent changes** - What changed since it last worked?
- **Common culprits** - Race conditions, caching, state mutation, null references
- **Environment differences** - Dev vs prod, different data, timing
- **External dependencies** - API changes, network issues, third-party services

### 2. Rank by Likelihood

Assign likelihood based on:

| Factor | Higher Likelihood | Lower Likelihood |
|--------|-------------------|------------------|
| Recency | Code changed recently | Untouched for months |
| Complexity | Complex logic, many branches | Simple, well-tested |
| Dependencies | External services involved | Self-contained |
| Symptoms | Matches known failure pattern | Novel error |

### 3. Test Systematically

For each hypothesis (highest likelihood first):

1. **Design minimal experiment** - What single change would confirm or rule out this hypothesis?
2. **Predict outcome** - What result do you expect if hypothesis is correct?
3. **Execute and observe** - Run experiment, capture results
4. **Update status** - Mark CONFIRMED, RULED OUT, or NEEDS MORE DATA

### 4. Revise Based on Evidence

When evidence contradicts expectations:

- Lower likelihood of current hypothesis
- Consider what the evidence suggests instead
- Add new hypotheses if needed
- Re-rank based on new information

## Session Template

Use this template at the start of debugging sessions:

```markdown
## Debug Investigation

**Symptom**: [What's failing - exact error message or behavior]
**First observed**: [When did this start happening]
**Reproduction**: [Steps to reliably reproduce]

### Environment
- OS/Platform:
- Version:
- Dependencies:
- Data state:

### Hypothesis Tracker

| # | Hypothesis | Likelihood | Status | Evidence |
|---|------------|------------|--------|----------|
| H1 | [Description] | High/Med/Low | TESTING/RULED OUT/CONFIRMED | [What you learned] |
| H2 | [Description] | High/Med/Low | PENDING | - |
| H3 | [Description] | High/Med/Low | PENDING | - |

### Investigation Log
[Timestamped entries below]
```

## Example Investigation

### Symptom
API returns 500 error intermittently on `/api/orders` endpoint.

### Reproduction
1. Create 10+ concurrent requests to `/api/orders`
2. ~30% return 500 errors
3. Only happens under load

### Hypothesis Tracker

| # | Hypothesis | Likelihood | Status | Evidence |
|---|------------|------------|--------|----------|
| H1 | Database connection pool exhausted | High | RULED OUT | Pool metrics show 50% utilization during failures |
| H2 | Race condition in order validation | High | TESTING | Errors correlate with concurrent creates |
| H3 | Memory pressure causing timeouts | Medium | PENDING | - |
| H4 | Third-party payment API rate limiting | Low | RULED OUT | Payment API logs show no rate limits |

### Investigation Log

```
[14:32] Testing H1 (connection pool)
        Action: Monitored pool metrics during load test
        Result: Pool never exceeded 50% capacity
        Evidence: Prometheus shows max 5/10 connections used
        Status: H1 RULED OUT - pool is not the bottleneck

[14:45] Testing H4 (payment API rate limiting)
        Action: Checked payment provider dashboard
        Result: No rate limit events in their logs
        Evidence: All payment API calls successful
        Status: H4 RULED OUT

[15:01] Testing H2 (race condition)
        Action: Added logging around order validation
        Result: Found concurrent access to shared cart state
        Evidence: Logs show two requests modifying same cart simultaneously
        Status: H2 LIKELY - need to verify with fix

[15:23] Verifying H2 fix
        Action: Added mutex around cart validation
        Result: 0 errors in 1000 concurrent requests
        Evidence: Lock prevents concurrent modification
        Status: H2 CONFIRMED - race condition was root cause
```

## Hypothesis Status Definitions

| Status | Meaning | Next Action |
|--------|---------|-------------|
| PENDING | Not yet investigated | Move to TESTING when ready |
| TESTING | Currently being investigated | Execute experiment, gather evidence |
| NEEDS MORE DATA | Inconclusive results | Design better experiment |
| RULED OUT | Evidence contradicts hypothesis | Document and move on |
| LIKELY | Evidence supports but not conclusive | Verify with fix |
| CONFIRMED | Root cause identified and verified | Document fix and close |

## Common Hypothesis Categories

### State-Related
- Stale cache returning old data
- Database transaction not committed
- In-memory state mutation
- Session/cookie corruption

### Timing-Related
- Race condition between concurrent operations
- Timeout too short for slow operations
- Clock skew between services
- Event ordering assumptions violated

### Environment-Related
- Missing environment variable
- Different library versions
- File permissions
- Network connectivity/latency

### Data-Related
- Null/undefined where unexpected
- Type mismatch (string vs number)
- Encoding issues (UTF-8, special characters)
- Data exceeds expected bounds

## Tips for Effective Hypothesis Tracking

1. **Be specific** - "Something wrong with caching" is not a hypothesis. "Redis TTL set to 0 causing immediate expiration" is.

2. **Make them testable** - Every hypothesis should have a clear experiment that could rule it out.

3. **Update in real-time** - Log entries as you go, not from memory later.

4. **Don't delete** - Ruled-out hypotheses are valuable documentation of what was tried.

5. **Share context** - If handing off, the tracker should let someone else continue without starting over.
