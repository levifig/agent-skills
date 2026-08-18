<!-- brief.md is the optional archeological kickstart — the original unshaped ask.
     May accrete parked problem-space concepts while the change is captured; freezes when shape.md exists.
     Superseded by shape.md; never mechanically load-bearing.
     A brief-only folder is legal and non-executable (captured, not shaped). -->

# Linear-Native Coordination on the Change Model

## Problem Statement

Loaf's Linear integration (ADR-011) is built entirely on the retired spec/task split — specs local, tasks as sub-issues under a spec-labeled rollup — and the spec-conversion-and-guidance-sweep removes that substrate. The replacement was named in the 2026-07-17 successor order (`linear-native-coordination`) but never materialized, so the critical tracker-coordination capability has no home on the Change model. Beyond the integration gap, the internal work model itself (arc / Change / task packets / intake / dispositions) has evolved without ever being pressure-tested against the ontology of the tracker it must eventually coordinate with.

## Who Has It

The operator running Loaf across work and personal projects with Linear as the team tracker, and any team-lead persona coordinating agent work that teammates consume through Linear rather than through the repository. The pain recurs every time work shaped in Loaf needs a tracker presence — today that means the legacy spec-rollup mode or nothing.

## Current Alternatives

The legacy Linear-native mode still functions until the sweep executes, but it publishes the retired workflow's shapes. Otherwise: ad hoc Linear MCP calls with no model behind them, or manual duplication of Change state into issues.

## Value Proposition

One coordination model where Loaf's internal ontology and Linear's map onto each other deliberately — intake ↔ Triage, arc ↔ Project/Initiative, Change ↔ Issue, task packet ↔ sub-issue — so publishing, adopting, and reconciling work between the repository and the tracker is a defined operation instead of an improvisation. Operator directive (2026-08-11, sweep shaping interview): **Linear's model should inform the internal model** — this Change is a model-informing design effort, not an integration afterthought; where the mapping strains (Linear's declared Projects vs Loaf's derived arcs, Linear's mutable workflow states vs Loaf's append-only dispositions), the tension is resolved on purpose and recorded.

## Constraints

- Provider-neutral core survives: local-only projects never require Linear (inherited no-go from the intent-exploration-foundation tracker packet).
- No tracker-owned durable document bodies; Git and SQLite remain canonical per the storage trichotomy.
- No silent last-writer-wins between tracker and repository state.
- Append-only internal facts are not abandoned to mirror Linear's mutable statuses — any bending of that principle is an explicit, recorded decision.
- Owns the ADR-011 supersession (recategorization: the mode's mechanism is replaced wholesale).

## Sequencing and Relationships

Lands before `spec-conversion-and-guidance-sweep` executes (its shape.md Decision 6 records the sequencing) — restoring the 2026-07-17 successor order in which Linear coordination precedes destructive convergence. The sweep then converges guidance onto whatever model this Change lands. Candidate for deliberate arc assembly with the sweep at cut-planning time.

## Sources and Research Links

- ADR-011 (the mode being replaced) and the Linear-native touchpoints inventoried during sweep shaping (the Linear skill's Loaf issue coordination reference, implement's Linear sections, council's `linear_parent`).
- The `linear-native-coordination` successor packet in `docs/changes/20260710-journal-reliability-foundation/change.md` (purpose, inherited decisions, no-gos) and journal decisions cited there (`journal:fa885c8d…`, `journal:c3c937f0…`, `journal:d655dbd5…`).
- Sweep shaping interview 2026-08-11 (operator directive and sequencing decision).

## Open Questions

- [ ] Which side bends where the ontologies conflict — declared Project vs derived arc, mutable states vs append-only dispositions — blocking
- [ ] Publish/adopt/reconcile semantics: what operation set, what conflict surface, what agent-from-tracker bootstrap — blocking
- [ ] Granularity: does a task packet (a commit) warrant a Linear sub-issue, or is the Change/Issue the floor — deferrable
- [ ] Whether milestone/cycle concepts get any Loaf-side meaning — deferrable
