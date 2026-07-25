# Decomposition

Shaping step absorbed from the retired breakdown skill: dependency awareness, granularity judgment, and acceptance-criteria thinking, now expressed as the Change's Implementation Units and Verification Contract instead of a separate task-minting pass.

## What Survives

- **The Right Size Test** — before finalizing a unit boundary, check: Can a single implementer complete this? If no, split by concern. Does it touch multiple unrelated concerns? If yes, split by concern. Will the agent need too much context? If yes, split into smaller coherent units. Are you splitting just to have more units? If yes, merge back.
- **Right-sizing rules** — one agent type per unit (completable by a single implementer), one concern per unit (one layer, service, or component), context-appropriate (fits in model context with room for exploration), not over-fragmented (don't split what naturally belongs together).
- **Per-unit verification discipline** — every unit includes its own observable done condition. Never a separate "verify" unit; keep tests with the code they test.
- **Own the decisions** — decide granularity and unit boundaries autonomously. Ask the user only when two orderings are genuinely equally valid with different trade-offs; otherwise decide and move on.

## What Dies

Task-file minting, ID allocation, estimate fields, and ordering-for-execution as the default presentation. Implementation Units are in-document work packets — commit-boundary guides and review anchors — never tracked entities with IDs, statuses, or a persistence layer of their own.

## The New Principle: Order by Likelihood-of-Change

Breakdown ordered units for execution (dependency graph, then priority). Shaping orders units for review: data models, interfaces, and user-facing flows lead; mechanical refactors and boilerplate collapse at the bottom. The reviewer's attention at the draft-to-ready flip should land on what's most likely to need changing, not on whatever happens to run first.

Sequencing constraints that genuinely exist — this unit must land before that one — are stated in prose within the unit description or a Planning Contract subsection. Never rely on list order alone to imply a dependency; a reviewer skimming units by likelihood-of-change won't read sequencing into position.

## Authoring the Verification Contract

Split criteria into two groups, mirroring the Change template:

- **Executable** — bound to a command and an expected result; machine-checkable.
- **Human review** — what a reviewer confirms that no command can.

A criterion whose check only restates the implementation (recomputing the expected value the way the code does) is vacuous — it can never disagree with the code under test. Prefer criteria with an independent source of truth.
