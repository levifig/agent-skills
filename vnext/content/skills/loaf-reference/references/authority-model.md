# Authority Model

One responsibility has one canonical owner. Loaf guides the agent; it does not sit between the agent and the service that owns shared work.

## Ownership

| Authority | Owns | Does not own |
|-----------|------|--------------|
| Tracker | Work identity, definition, definition of done, hierarchy, dependencies, workflow state, assignment, collaboration | Loaf continuity, credentials, code |
| Harness | Connection exposure, credentials, execution, model and tool boundaries | Work semantics or tracker state |
| Loaf | Flow ceremonies, skills, templates, profiles, private continuity, and temporary skill-artifact conventions | Shared work records, report lifecycle state, or connection credentials |
| Git | Code and deliberately promoted artifacts | Workflow state or private continuity |

## Consequences

- A template is an ephemeral semantic packet routed once to native tracker fields; it is not a stored work item or monolithic body copy.
- A native reference returned by the tracker remains the identity throughout the Flow.
- A comment records collaboration or evidence. It cannot replace the canonical definition, relationship, or workflow field.
- Failure to reach the tracker is a connection boundary, not permission to mint a substitute record.
- Private continuity may remember why a decision was made, but it does not become team workflow state.

## Temporary Reports

Reports are heterogeneous working files produced by skills, not Loaf state entities. A skill returns through the harness by default and persists a report only when the result must survive the immediate response: asynchronous work, a large investigation, cross-conversation use, multiple consumers, or an explicit request.

- The producing skill owns the report's structure and template. There is no universal report schema or shared report template.
- A temporary report lives at `.agents/reports/YYYYMMDDHHMMSS-slug.md`. The timestamp is UTC, the slug describes the report itself, and provenance belongs in the content when useful.
- A report has no universal status, identifier, database row, sync representation, or CLI lifecycle.
- Housekeeping reads every report individually and recommends leaving it, extracting durable conclusions and deleting it, deleting it, or moving the report itself to `docs/reports/` when it has perennial value.
- Deletion and promotion require explicit user approval. Only deliberately promoted reports become durable Git artifacts.
