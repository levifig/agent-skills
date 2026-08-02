# Subagent-Driven Development

Delegate specialized work to specialized agents.

## Contents

- Philosophy
- Quick Reference
- When to Use Subagents
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

**Fail fast, escalate early.** If a subagent hits a blocker, surface it immediately. Don't let agents spin on problems outside their expertise.

**Integration is your job.** Subagents produce components. Integration, verification, and ensuring coherence is the coordinator's responsibility.

## Quick Reference

| Profile | Specialization | Use For |
|---------|----------------|---------|
| `implementer` | Server-side logic | APIs, services, business logic (with language skill) |
| `implementer` | Client-side UI | Components, state, styling (with typescript-development) |
| `implementer` | Database | Schema, migrations, queries (with database-design) |
| `implementer` | Testing | Test suites, quality checks (with foundations) |
| `implementer` | Infrastructure | CI/CD, deployment, config (with infrastructure-management) |

## When to Use Subagents

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
[Task tool invocation]
- Subagent type: implementer
- Skills: [language skill + domain skills]
- Task: [Clear description]
- Context: [Relevant details]
```

### 5. Integrate Results

When subagent completes:
- Verify the output
- Check integration points
- Run tests
- Resolve any conflicts

## Task Definition Template

```markdown
## Subagent Task

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
- Verify subagent output
- Handle integration yourself

### Never

- Delegate tasks with unclear scope
- Assume subagent output is correct without verification
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

After subagent completes:

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
| Subagent stuck | Check if task is in their expertise; escalate if not |
| Output doesn't integrate | Clarify interface contract, retry with more context |
| Quality issues | Review task definition; was it clear enough? |
| Wrong approach | Provide more context about constraints and patterns |

## Integration with Loaf Workflow

| Command | Subagent Role |
|---------|---------------|
| `/breakdown` | Tasks become subagent assignments |
| `/implement` | May dispatch subagents for specialized work |
| `/implement` | Automatically coordinates single-task and multi-task subagent work |

## Related Skills

- `parallel-agents` - Running multiple subagents concurrently
- `orchestration` - Higher-level task coordination
- `verification` - Verifying subagent output
