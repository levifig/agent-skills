# Background Agents

Background execution changes when work runs; it does not make every result a repository artifact.

## When to Use

Use a background agent for bounded, non-interactive work that can complete without clarification while the main conversation continues. Give it the native work reference, acceptance criteria, allowed actions, relevant paths, and required verification.

## Result Boundary

Return through the harness by default. The orchestrator can inspect and consolidate that return without creating a file.

Persist a result only when at least one condition applies:

- the harness may discard the return before it is consumed;
- another conversation or consumer needs the full result;
- the evidence is too large for a useful direct return;
- the user explicitly asks to keep it.

When a file is warranted, specify the producing skill's template and an unused path matching `.agents/reports/YYYYMMDDHHMMSS-slug.md`. Generate the 14-digit timestamp in UTC. The slug describes the report, not its tracker issue, task, agent, or work unit. Put provenance in the report body when it helps readers.

Do not add status, an agent identifier, lifecycle fields, or archive instructions. The background agent returns the path through the harness. Housekeeping later owns any disposition recommendation.

## Verification

- The agent's work is bounded and traceable to the live contract.
- The harness return names the outcome, evidence, verification, blockers, and any written path.
- A report exists only when persistence was justified.
- A written path is unique, uses UTC naming, and was not overwritten.
