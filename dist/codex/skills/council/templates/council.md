# Council File Template

**Location:** `.agents/councils/YYYYMMDD-HHMMSS-<topic-slug>.md`

```yaml
---
council:
  topic: "[Clear decision description]"
  created: "[ISO timestamp]"
  status: in_progress
  composition:
    - [agent1]
    - [agent2]
    - [agent3]
    - [agent4]
    - [agent5]
  session_reference: "[.agents/sessions/FILE.md if applicable]"
  linear_issue: "[ISSUE-ID if applicable]"
---

# Council: [Topic]

## Decision Question

[Clear, specific question being decided]

## Options

### Option 1: [Name]
[Brief description]

### Option 2: [Name]
[Brief description]

### Option 3: [Name]
[Brief description (if applicable)]

## Context

[Background, constraints, requirements]

- Expected scale/load:
- Current stack:
- Team expertise:
- Other relevant context:

## Agent Perspectives

[To be filled during deliberation]

## Synthesis

[To be filled after collecting perspectives]

## Decision

[To be filled after user approval]

---

## Deliberation Log

### [Timestamp] - Council Convened
Agents: [list]
Composition selected by orchestrator.
```
