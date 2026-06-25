# Background Agents

Background agents handle low-priority, long-running, or non-interactive work
while the user continues with other tasks.

## Contents

- When to Use Background Agents
- Spawning Background Agents
- Tracking
- Result Retrieval
- Workflow Example
- Anti-Patterns
- Integration Points

## When to Use Background Agents

| Appropriate | Not Appropriate |
|-------------|-----------------|
| Security audits | Interactive debugging |
| Code coverage analysis | User-facing questions |
| Large-scale refactoring reports | Time-sensitive fixes |
| Documentation audits | Work needing user decisions mid-task |
| Dependency vulnerability scans | Blocking tasks for current work |

Good candidates have clear completion criteria, can run without clarification,
and produce a report or durable artifact.

## Spawning Background Agents

### Amp

Use the Amp check/agent mode or new thread with `run_in_background: true`:

```python
Task(
    agent_type="background-runner",
    prompt="""
    Run full security audit on backend codebase.

    Scope:
    - src/api/
    - src/services/

    Write report to: .agents/reports/YYYYMMDD-HHMMSS-security-audit.md
    Active session: SESSION-ALIAS if available from loaf session list/show
    """,
    run_in_background=True
)
```

### Cursor

Background agents are configured via the `is_background: true` YAML property.
When spawning, specify the report destination and any task/spec/session IDs:

```
@background-runner Run security audit on backend codebase.
Write report to .agents/reports/.
Reference TASK-123 and the active session alias if available.
```

## Tracking

Track background work with durable references:

1. Log the spawn with `loaf session log "todo(background): started <id> for <task>"`.
2. Ask the background agent to write a report under `.agents/reports/`.
3. When complete, log `discover(background): <id> wrote <report>`.
4. Process findings into tasks, specs, ADRs, or report verdicts as appropriate.

Use a stable ID such as `bg-YYYYMMDD-HHMMSS-description` in the prompt and
journal entries.

## Result Retrieval

Background agents write results to `.agents/reports/` with enough metadata to
identify the source task and report status. In SQLite-backed projects, use
`loaf report list`, `loaf report show`, and `loaf report archive` when report
state is available.

## Workflow Example

1. Orchestrator identifies non-blocking security audit work.
2. Orchestrator logs the background spawn in the active session.
3. Background agent writes `.agents/reports/YYYYMMDD-HHMMSS-auth-security.md`.
4. Orchestrator reviews the report, creates follow-up tasks, and logs the
   outcome.
5. Report state is finalized or archived through the report lifecycle.

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Use for blocking work | Keep blocking work in foreground |
| Spawn without tracking | Log the spawn and require a report path |
| Ignore completed results | Process reports into tasks, findings, or decisions |
| Use for interactive tasks | Reserve for autonomous work |
| Spawn many concurrent background agents | Limit concurrency to avoid resource contention |
| Skip result location in prompt | Always specify where output belongs |

## Integration Points

- `loaf session log` records spawn and completion facts.
- `loaf session show` exposes recent background-work journal entries.
- `loaf report` commands own durable report lifecycle when available.
- `/wrap` should mention unprocessed background reports before ending a session.
