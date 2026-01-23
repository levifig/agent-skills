# Background Agents

Background agents handle low-priority, long-running, or non-interactive work that can run independently while the user continues with other tasks.

## When to Use Background Agents

| Appropriate | Not Appropriate |
|-------------|-----------------|
| Security audits | Interactive debugging |
| Code coverage analysis | User-facing questions |
| Large-scale refactoring reports | Time-sensitive fixes |
| Codebase-wide linting reports | Tasks requiring immediate feedback |
| Documentation audits | Work needing user decisions mid-task |
| Dependency vulnerability scans | Blocking tasks for current work |

**Good candidates:**
- Results can be processed later
- No user interaction required
- Task is well-defined with clear completion criteria
- Output is a report or artifact, not a conversation

**Poor candidates:**
- Needs clarification mid-task
- Time-sensitive (user waiting for result)
- Requires real-time coordination with other agents

## Spawning Background Agents

### Claude Code

Use the Task tool with `run_in_background: true`:

```python
Task(
    subagent_type="background-runner",
    prompt="""
    Run full security audit on backend codebase.

    Scope:
    - src/api/
    - src/services/

    Write report to: .agents/reports/YYYYMMDD-HHMMSS-security-audit.md

    Session: .agents/sessions/20260123-143000-auth-feature.md
    """,
    run_in_background=True
)
```

### Cursor

Background agents are configured via the `is_background: true` YAML property. When spawning:

```
@background-runner Run security audit on backend codebase.
Write report to .agents/reports/
```

## Background Agent Tracking

Track background agents in session frontmatter:

```yaml
background_agents:
  - id: "bg-20260123-143000-security-scan"
    agent: background-runner
    task: "Full security audit of backend"
    status: running  # running | completed | failed
    result_location: null
  - id: "bg-20260123-144500-coverage"
    agent: background-runner
    task: "Test coverage analysis"
    status: completed
    result_location: ".agents/reports/20260123-144500-coverage-report.md"
```

### Status Values

| Status | Meaning |
|--------|---------|
| `running` | Agent still executing |
| `completed` | Work finished, results available |
| `failed` | Agent encountered error |

### ID Convention

Background agent IDs follow: `bg-YYYYMMDD-HHMMSS-<description>`

Generate with:
```bash
echo "bg-$(date -u +"%Y%m%d-%H%M%S")-<description>"
```

## Result Retrieval

Background agents write results to `.agents/reports/` with standard frontmatter:

```yaml
---
report:
  title: "Security Audit Report"
  type: background-agent-output
  status: unprocessed  # unprocessed | processed | archived
  created: "2026-01-23T14:30:00Z"
  background_agent_id: "bg-20260123-143000-security-scan"
  session_reference: "20260123-140000-auth-feature.md"
---
```

### Processing Results

1. SessionStart hook alerts user to completed background work
2. User or PM reviews report in `.agents/reports/`
3. PM updates session frontmatter:
   - Change `status` from `running` to `completed`
   - Set `result_location` to report path
4. Process findings as needed (spawn agents, create issues)
5. Set report `status` to `processed`

## Workflow Example

### 1. PM Identifies Low-Priority Work

During auth feature implementation, PM identifies need for security audit but it is not blocking current work.

### 2. PM Spawns Background Agent

```python
Task(
    subagent_type="background-runner",
    prompt="""
    Run comprehensive security audit on auth implementation.

    Files to audit:
    - src/auth/endpoints.py
    - src/auth/token.py
    - src/auth/middleware.py

    Check for:
    - OWASP Top 10 vulnerabilities
    - Secrets in code
    - SQL injection risks
    - Authentication bypasses

    Write report to: .agents/reports/20260123-143000-auth-security.md

    Session: .agents/sessions/20260123-140000-auth-feature.md
    """,
    run_in_background=True
)
```

### 3. PM Updates Session Frontmatter

```yaml
background_agents:
  - id: "bg-20260123-143000-auth-security"
    agent: background-runner
    task: "Auth security audit"
    status: running
    result_location: null
```

### 4. Work Continues

PM and other agents continue with main implementation while background agent works.

### 5. Session Resumes Later

SessionStart hook detects completed background work:

```
# Background Work Completed

The following background agents have completed:

- **bg-20260123-143000-auth-security** (background-runner)
  - Task: Auth security audit
  - Result: .agents/reports/20260123-143000-auth-security.md

Review reports and update session frontmatter after processing.
```

### 6. Results Processed

PM reads report, creates issues for findings, updates session.

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Use for blocking work | Keep blocking work in foreground |
| Spawn without tracking | Always update session frontmatter |
| Ignore completed results | Process results when alerted |
| Use for interactive tasks | Reserve for autonomous work |
| Spawn many concurrent background agents | Limit to 2-3 to avoid resource contention |
| Skip result location in prompt | Always specify where to write output |

## Integration Points

### SessionStart Hook

Checks session frontmatter for `background_agents` with `status: completed`. Alerts user to review results.

### PreCompact Hook

Includes background agent state in preservation. Context-archiver captures:
- Active background agent list
- Current status of each
- Result locations for completed agents

### Session Frontmatter

Full template with background agents:

```yaml
---
session:
  title: "Feature implementation"
  status: in_progress
  # ... other session fields ...

background_agents:
  - id: "bg-20260123-143000-security"
    agent: background-runner
    task: "Security audit"
    status: completed
    result_location: ".agents/reports/20260123-143000-security.md"
  - id: "bg-20260123-150000-coverage"
    agent: background-runner
    task: "Coverage analysis"
    status: running
    result_location: null
---
```
