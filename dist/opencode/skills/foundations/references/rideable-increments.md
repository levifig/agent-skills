# Rideable Increments

Loaf treats a complete, useful operator journey as the unit of progress. A skateboard is not a broken car: each increment is a coherent product that a real rider can use safely today, deliberately narrower than what may follow.

## Doctrine

- Build vertical journeys, not completed horizontal layers. Loaf advances when a larger real journey becomes possible, not merely when another layer exists.
- Prefer the narrowest end-to-end slice that creates observable operator value. Reduce breadth, never integrity.
- Preserve security, determinism, rollback or recovery, honest failure, and data safety even in the smallest increment.
- Dogfood before generalizing. Complexity, abstraction, protocol depth, and breadth are earned by observed use rather than anticipated futures.
- Foundation work is legitimate when the current rideable increment needs it. It may span atomic commits, but it must name the immediate slice that consumes it and must not become a sequence of strategy milestones without integration.
- Atomic commits are necessary; atomic value increments are the higher-order goal.

This method sharpens existing briefs, canonical tracker work contracts, criteria, reviews, and strategy. It does not create another lifecycle, tracker, score, status, storage schema, or approval gate.

## Increment Contract

Every shaped milestone or work unit makes these concrete in its existing contract. Use natural prose and criteria; the labels below are a thinking aid, not mandatory native tracker headings.

| Element | Concrete answer |
|---------|-----------------|
| **Rider** | The real operator who receives the value, in a named context |
| **Complete Journey** | The end-to-end action they can finish without a promised future layer |
| **Entry point** | Where the rider begins: command, UI, hook, API, or documented procedure |
| **Observable Outcome** | What becomes true and how the rider can recognize it |
| **Real Dogfood** | Who will use the increment on real work, where, and with what evidence captured |
| **Safety/integrity proof** | The checks for security, determinism, recovery or rollback, honest failure, and no data loss that apply to this slice |
| **Learning sought** | The uncertainty this real use should resolve before more complexity is added |
| **Explicit Deferrals** | Breadth intentionally omitted, without weakening the delivered journey's integrity |

If these answers describe components rather than an operator journey, narrow or regroup the work until they describe something rideable.

## Slicing Method

1. Start with the rider and the newly possible outcome, then trace one complete path from entry point to observable result.
2. Remove personas, modes, configuration, scale, and polish until the path is the smallest useful version. Do not remove integrity properties.
3. Include only machinery exercised by that path. When proposed machinery has no current consumer, defer it.
4. Keep unavoidable foundation work beside the consuming slice. If a packet is only a wheel, name the immediate slice that mounts it; count product progress only when that slice works end to end.
5. Dogfood the slice on real work, capture the promised proof and learning, and let that evidence earn the next increment.

## Review Questions

Ask these during shaping, implementation checkpoints, review, shipping, and reflection:

- Is the result usable today by the named rider?
- Is it a complete small product or a fragment of a larger one?
- Did we add machinery this journey does not exercise?
- What observed evidence demanded the complexity?
- Could less deliver the same outcome without reducing integrity?
- If this is only a wheel, which immediate slice makes it rideable?

## Applying It Through the Flow

| Stage | Use the method to |
|-------|-------------------|
| **pitch** | Frame the first valuable operator journey without prematurely designing it |
| **shape** | Make the increment contract concrete on the canonical tracker record and decompose by complete slices |
| **implement** | Preserve the journey while landing atomic commits and reject unused machinery |
| **ship** | Verify that the rider can complete the journey and that proof matches the claims |
| **release** | Describe progress by the larger real journey now possible, not layers completed |
| **reflect** | Compare dogfood learning with the hypothesis before expanding or generalizing |
