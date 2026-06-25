# Amp check/agent mode or new thread-Driven Development

Delegate specialized work to specialized agents.

## Contents

- Philosophy
- Quick Reference
- When to Use Amp check/agent mode or new thread
- Delegation Pattern
- Task Definition Template
- Critical Rules
- Coordination Patterns
- Integration Checklist
- Troubleshooting
- Integration with Loaf Workflow
- Related Skills

## Philosophy

**Right agent for the job.** Backend work goes to backend agents. Frontend to frontend. Database to DBA. Specialization enables depth without requiring breadth.

**Coordinate, don't micromanage.** Define clear inputs and outputs, let agents work autonomously, integrate results. Don't dictate implementation details.

**Fail fast, escalate early.** If a Amp check/agent mode or new thread hits a blocker, surface it immediately. Don't let agents spin on problems outside their expertise.

**Integration is your job.** Amp check/agent mode or new thread produce components. Integration, verification, and ensuring coherence is the coordinator's responsibility.

## Quick Reference

| Profile | Specialization | Use For |
|---------|----------------|---------|
| `implementer` | Server-side logic | APIs, services, business logic (with language skill) |
| `implementer` | Client-side UI | Components, state, styling (with typescript-development) |
| `implementer` | Database | Schema, migrations, queries (with database-design) |
| `implementer` | Testing | Test suites, quality checks (with foundations) |
| `implementer` | Infrastructure | CI/CD, deployment, config (with infrastructure-management) |

## When to Use Amp check/agent mode or new thread

### Good Fit

- Multiple specialized tasks in one session
- Clear separation of concerns
- Tasks don't require shared context
- Implementation details delegatable

### Poor Fit

- Highly coupled tasks needing constant coordination
- Exploratory work with unclear scope
- Tasks requiring deep context about each other
- Quick fixes that don't warrant delegation

## Delegation Pattern

### 1. Define the Task

Be specific about:
- **What** to implement
- **Where** (files, modules)
- **Constraints** (patterns to follow, things to avoid)
- **Success criteria** (how to verify done)

### 2. Select the Agent

Match task to specialization:

```markdown
Task: Add user validation endpoint
Profile: implementer (with python-development)
Why: Server-side API work
```

### 3. Provide Context

Include:
- Relevant spec sections
- Existing patterns to follow
- File locations
- Interface contracts

### 4. Dispatch and Monitor

```
[Amp check/agent mode or new thread invocation]
- Amp check/agent mode or new thread type: implementer
- Skills: [language skill + domain skills]
- Task: [Clear description]
- Context: [Relevant details]
```

### 5. Integrate Results

When Amp check/agent mode or new thread completes:
- Verify the output
- Check integration points
- Run tests
- Resolve any conflicts

## Task Definition Template

```markdown
## Amp check/agent mode or new thread Task

**Profile:** [implementer | reviewer | researcher]

**Objective:**
[One sentence: what to accomplish]

**Scope:**
- Files: [List of files to create/modify]
- Modules: [Relevant modules]

**Context:**
- Spec: [Link or summary]
- Patterns: [Existing patterns to follow]
- Constraints: [What to avoid]

**Interface:**
- Input: [What this component receives]
- Output: [What this component produces]

**Success Criteria:**
- [ ] [Verifiable criterion 1]
- [ ] [Verifiable criterion 2]

**Verification:**
```bash
[Command to verify]
```
```

## Critical Rules

### Always

- Match tasks to agent specialization
- Define clear success criteria
- Provide necessary context
- Verify Amp check/agent mode or new thread output
- Handle integration yourself

### Never

- Delegate tasks with unclear scope
- Assume Amp check/agent mode or new thread output is correct without verification
- Micromanage implementation details
- Dispatch without success criteria
- Skip integration testing

## Coordination Patterns

### Sequential Dependency

When Task B needs Task A's output:

```
1. Dispatch Task A (implementer)
2. Wait for completion
3. Verify Task A output
4. Dispatch Task B with Task A's output as context
```

### Parallel Independence

When tasks are independent:

```
1. Dispatch Task A (implementer with backend skills)
2. Dispatch Task B (implementer with frontend skills) [parallel]
3. Wait for both
4. Integrate results
```

### Interface Contract

When tasks share an interface:

```
1. Define interface contract
2. Dispatch Task A with contract
3. Dispatch Task B with contract [parallel]
4. Verify both implement contract correctly
5. Integration test
```

## Integration Checklist

After Amp check/agent mode or new thread completes:

```
□ Output matches success criteria
□ Code follows project patterns
□ Tests pass
□ Integration points work
□ No regressions introduced
□ Documentation updated if needed
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Amp check/agent mode or new thread stuck | Check if task is in their expertise; escalate if not |
| Output doesn't integrate | Clarify interface contract, retry with more context |
| Quality issues | Review task definition; was it clear enough? |
| Wrong approach | Provide more context about constraints and patterns |

## Integration with Loaf Workflow

| Command | Amp check/agent mode or new thread Role |
|---------|---------------|
| `/breakdown` | Tasks become Amp check/agent mode or new thread assignments |
| `/implement` | May dispatch Amp check/agent mode or new thread for specialized work |
| `/implement` | Automatically coordinates single-task and multi-task Amp check/agent mode or new thread work |

## Related Skills

- `parallel-agents` - Running multiple Amp check/agent mode or new thread concurrently
- `orchestration` - Higher-level task coordination
- `verification` - Verifying Amp check/agent mode or new thread output
