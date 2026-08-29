# Flow Semantics

The vNext Loaf Flow is `pitch → shape → implement → ship → release`. Triage decides which captured direction enters the Flow, and orchestration coordinates bounded execution without acquiring work authority.

## Ceremonies

| Ceremony | Reads | Produces |
|----------|-------|----------|
| Pitch | Human context and relevant live tracker context | A problem narrative |
| Triage | Candidate native tracker records and problem narratives | A disposition on the native record |
| Shape | Problem narrative plus current native state | A complete work contract on the tracker record |
| Implement | Live work contract and repository state | Code, verification evidence, and tracker updates |
| Ship | Live contract, candidate change, and evidence | A quality verdict and verified tracker transition |
| Release | Already-landed work and release evidence | A release outcome recorded on native work |

## Continuity Rules

Every ceremony begins by reading the native record again. Handoffs carry its native reference, not a copied snapshot treated as authority. Mutations use the configured provider mapping, then re-read the same native record and report the observed result.

The main agent executes the common contract through the selected provider skill by default. A harness may use the optional project-manager profile only when it can enforce that connector-only boundary; otherwise execution remains with the main agent. Both routes derive behavior from the same common contract and provider selection.

If the connection is unavailable or a required provider capability is absent, preserve the narrative as conversation output or private continuity and report the gap. Do not manufacture shared work, relationships, or status outside the tracker.
