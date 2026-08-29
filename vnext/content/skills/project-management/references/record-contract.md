# Record Contract

`project-management/v1` is a semantic protocol, not a transport or client. The current harness exposes connections, the selected provider skill maps native fields, and the tracker owns the resulting record.

## Operation Sequence

1. Run `connection.discover` over connections already visible to the harness.
2. Select one exact native destination; stop if the choice changes workspace or team and remains ambiguous.
3. Run `capability.discover` and select the highest honest fidelity for the requested operation.
4. Read current native state.
5. Apply one bounded mutation when requested and authorized.
6. Re-read native state and return the result envelope.

Creation includes a prior search or destination read for an existing matching record. If a create or comment request returns no authoritative identity or state, do not repeat it blindly: inspect native state and report `indeterminate` when duplication cannot be excluded.

## Result Envelope

| Field | Meaning |
|-------|---------|
| `operation` | One identifier from the closed common vocabulary |
| `destination` | Provider plus native workspace/team/project scope |
| `native_ref` | Tracker-issued identity, or empty only when no identity was observed |
| `outcome` | `confirmed`, `unchanged`, `partial`, `failed`, or `indeterminate` |
| `fidelity` | `exact`, `advisory`, `manual`, or `unsupported` |
| `observed_state` | Relevant authoritative state from the final read |
| `verification_evidence` | What read or provider response proves the outcome |

`partial` means some bounded effects were confirmed and others were not. `indeterminate` means the agent cannot prove whether a native effect occurred. Neither may be rounded up to success.

## Semantic Separation

- `definition.write` changes canonical problem or completion fields.
- `hierarchy.change` changes parent/child structure.
- `dependency.change` changes blocking or related-work edges.
- `status.transition` selects a valid native workflow state after reading available states.
- `comment.append` adds collaboration or evidence only.

When a provider or connection lacks an exact mapping, report the lower fidelity. Do not encode a relationship in prose, claim a status through a comment, or replace a canonical definition with an activity note.
