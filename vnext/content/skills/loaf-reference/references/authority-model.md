# Authority Model

One responsibility has one canonical owner. Loaf guides the agent; it does not sit between the agent and the service that owns shared work.

## Ownership

| Authority | Owns | Does not own |
|-----------|------|--------------|
| Tracker | Work identity, definition, definition of done, hierarchy, dependencies, workflow state, assignment, collaboration | Loaf continuity, credentials, code |
| Harness | Connection exposure, credentials, execution, model and tool boundaries | Work semantics or tracker state |
| Loaf | Flow ceremonies, skills, templates, profiles, private continuity | Shared work records or connection credentials |
| Git | Code and deliberately promoted artifacts | Workflow state or private continuity |

## Consequences

- A template is an ephemeral semantic packet routed once to native tracker fields; it is not a stored work item or monolithic body copy.
- A native reference returned by the tracker remains the identity throughout the Flow.
- A comment records collaboration or evidence. It cannot replace the canonical definition, relationship, or workflow field.
- Failure to reach the tracker is a connection boundary, not permission to mint a substitute record.
- Private continuity may remember why a decision was made, but it does not become team workflow state.
