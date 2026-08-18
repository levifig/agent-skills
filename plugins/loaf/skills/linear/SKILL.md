---
name: linear
description: >-
  Manages Linear issues, projects, cycles, documents, and team workflows through
  whichever Linear MCP server the current harness exposes. Use when the user
  asks to triage, plan, create, update, audit, or report on Linear work.
  Produces verified mutations or read-only summaries with gaps and blockers. In
  Loaf projects, preserves loaf issue authority and synchronization boundaries.
  Not for configuring or authenticating MCP servers.
user-invocable: true
argument-hint: '[goal, issue, project, or team scope]'
version: 0.3.1
---

# Linear

Use the Linear MCP already configured in the current harness. In a Loaf project, record its server name in `.agents/loaf.json`; MCP installation and authentication remain external. This skill owns Linear workspace selection and general-purpose Linear workflows, while Loaf issue execution remains governed by `loaf issue`.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Workflow
- Common Workflows
- Loaf Projects
- Update Format
- Topics

## Critical Rules

- Discover the Linear-capable MCP servers available in the current harness. Do not assume a server name, install a plugin, configure a server, or initiate authentication from this skill.
- In a Loaf project, read `integrations.linear.mcp_server_name` from `.agents/loaf.json`. When it names an available Linear MCP, use that server. When it is absent and exactly one Linear MCP is available, record the exact server name and set `integrations.linear.enabled` to `true`, preserving the rest of the file. With several candidates, ask which server belongs to the project before recording or operating. Never silently replace a different recorded name.
- Select one server before operating. Verify the workspace and team with a read; a recorded or uniquely available server name does not prove the authenticated destination. If the recorded server is unavailable or the destination remains ambiguous, stop before any mutation and report the mismatch.
- Read before writing. Resolve current workspace, team, project, cycle, workflow state, labels, users, and target records as the requested operation requires.
- Batch related work by shared destination and intent. Explain the grouping before a bulk mutation, keep each batch bounded, and never fan mutations across workspaces implicitly.
- Re-read changed records after mutation. Report successes, unchanged records, failures, gaps, and blockers; do not claim a write succeeded from the request alone.
- Preserve identifiers returned by Linear and reuse resolved IDs within the operation instead of repeatedly searching by display name.
- In a Loaf project, read `.agents/loaf.json` and follow [Loaf Issue Coordination](references/loaf-issues.md). Never use generic MCP mutations to bypass `loaf issue` identity, description, or status reconciliation.

## Verification

- The selected server's workspace and relevant team or project were verified before mutation.
- In a Loaf project with an active Linear MCP, `integrations.linear.mcp_server_name` matches the selected server; an existing different value was not replaced without user direction.
- Every mutation was preceded by a read of the affected record or destination metadata.
- Bulk work was grouped into explainable batches and did not cross an unconfirmed workspace boundary.
- Changed records were re-read or otherwise confirmed by the selected MCP.
- The final response distinguishes created, updated, unchanged, skipped, and failed items and names unresolved access, data, or configuration gaps.
- In Loaf projects, issue identity and status changes flowed through `loaf issue`; Linear comments or metadata did not silently drive Loaf status.

## Quick Reference

| Request | Read First | Then | Verify |
|---------|------------|------|--------|
| Issue triage | Team, statuses, labels, candidate issues | Rank, label, assign, or move the approved scope | Re-read affected issues |
| Cycle planning | Team, cycle, backlog, assignees | Apply the agreed issue set and assignments | Re-read cycle membership and owners |
| Project planning | Project, teams, milestones, related issues | Update project structure and related work in batches | Re-read project and changed issues |
| Documentation audit | Documents and related issues | Summarize gaps; create approved follow-up issues | Re-read created issues and links |
| Workload review | Active issues and users | Report balance; apply approved reassignment batch | Re-read assignees |
| Status update | Exact issue and valid workflow states | Update or comment within the confirmed boundary | Re-read issue state and latest activity |
| Read-only report | Relevant teams, projects, cycles, issues, or documents | Synthesize without mutation | Name missing or inaccessible scope |

## Workflow

1. **Scope the request.** Identify the intended workspace, team or project, records, time window, and whether the user wants analysis or mutation. Resolve priority, labels, cycle, assignee, and due date only when relevant.
2. **Select the MCP.** In a Loaf project, prefer the recorded `integrations.linear.mcp_server_name`. Otherwise inspect the available Linear-capable servers and record the sole candidate, or ask the user to choose among several. Validate the selected server through a minimal workspace or team read and stop before writes if the project destination remains unclear.
3. **Read context.** Fetch destination metadata and existing records. Confirm identifiers and allowed workflow values rather than guessing names.
4. **Plan batches.** Group related operations by workspace, team/project, and mutation type. Reuse the read results and state the grouping logic before a bulk change.
5. **Execute.** Apply the smallest coherent batches. Continue independent items when one item fails, but do not cascade from an unresolved prerequisite.
6. **Verify and report.** Re-read mutations, compare requested versus observed state, and summarize outcomes plus remaining gaps or blockers.

Routine, fully specified writes do not need an extra confirmation. Ask before writes when the destination is ambiguous, the request would archive or delete data, a broad batch has unclear bounds, or the operation changes ownership beyond the user's stated scope.

## Common Workflows

- **Bug triage:** read high-impact open bugs and workflow metadata, rank by evidence, then apply approved priority, labels, assignment, or status changes.
- **Cycle planning:** read backlog candidates, current load, and cycle scope; assemble a coherent set and verify membership after updates.
- **Release planning:** read the project and dependency graph, identify missing milestones or work, then create or update the approved structure in dependency-aware batches.
- **Documentation audit:** search relevant documents, compare them with current issues or projects, report gaps, and create only approved follow-up work.
- **Workload balance:** group active work by assignee, identify concentration or unowned work, and apply only the approved reassignments.
- **Stale-work updates:** read recent activity and blockers before commenting; do not invent progress from issue age alone.
- **Smart labeling:** read existing label taxonomy first, propose mappings for unlabeled issues, and create new labels only when the taxonomy truly lacks the concept.

## Loaf Projects

Linear can be both a general collaboration surface and a Loaf issue backend. `integrations.linear.enabled` records project availability, `integrations.linear.mcp_server_name` identifies the MCP selected for this project, and `issue.authority` is the execution identity contract. The server name is routing metadata, not installation or authentication state. When a Linear issue corresponds to a Loaf work unit, use `loaf issue pull`, `loaf issue push`, and `loaf issue reconcile` rather than parallel MCP edits to owned fields.

Read [Loaf Issue Coordination](references/loaf-issues.md) before creating, adopting, publishing, or reconciling a Loaf-backed Linear issue.

## Update Format

Linear progress comments are succinct, outcome-focused, and self-contained:

```markdown
## Progress

- [x] Completed outcome
- [ ] Pending outcome

## Blockers

None currently.
```

Use Markdown checkboxes for progress lists. Omit emoji, absolute paths, local agent artifacts, session or council details, and duplicated issue titles after identifiers. For a real blocker, state its impact and what is needed to unblock it; include an ETA only when one is known.

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Loaf Issue Coordination | [loaf-issues.md](references/loaf-issues.md) | Creating, adopting, publishing, or reconciling Linear-backed Loaf issues |
