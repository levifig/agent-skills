---
name: pitch
description: Conducts problem discovery before work is shaped. Use when a direction is still solution-led, ambiguous, or missing affected people and desired outcomes. Produces a complete problem narrative for shape, not a work implementation.
---

# Pitch

Discover the problem with the user, then render the [problem narrative](../../templates/problem-narrative.md). Pitch may read relevant native tracker context through [`project-management/v1`](../project-management/SKILL.md), but it does not mint a substitute work record.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules

- As the first action, run `loaf journal log "skill(pitch): <concise intent>"` against the current private local journal. If the write fails, report the failure and continue only when the work can safely proceed; never put invocation bookkeeping in the tracker.
- Ask one consequential question at a time. Include a recommendation when choices have meaningful tradeoffs.
- Stay in the problem space until affected people, current reality, desired outcome, constraints, and important unknowns are explicit.
- Seek the first rideable journey: identify the smallest useful end-to-end change for a real operator without designing it. Sequence useful outcomes, never storage, backend, API, UI, or verification layers.
- Treat a proposed solution as evidence about the problem, not as an accepted implementation.
- Read an existing native record when the user supplies a reference or when duplicate context materially affects the pitch.
- Do not create shared work during pitch. Hand the narrative and destination uncertainty to shape.
- Read [Interview Guide](references/interview-guide.md) when the initial request is solution-heavy or crosses several problem domains.

## Verification

- Every required problem-narrative field is filled with observed or explicitly uncertain information.
- The narrative explains who is affected and what changes if the problem is solved.
- The first useful operator journey is nameable, and any sequencing is expressed as outcomes rather than component layers.
- Proposed implementation details are separated from constraints and evidence.
- Existing tracker context is attributed to its native reference and has not been copied into a second work authority.
- The handoff states whether shape should create a native record or update an existing one.

## Quick Reference

| Signal | Next question |
|--------|---------------|
| Solution first | What failure or unmet need makes this solution attractive? |
| Broad audience | Who feels the problem most directly? |
| Vague success | What observable reality would be different? |
| Conflicting goals | Which outcome wins, and why? |
| Existing native reference | What does the current record already establish? |
| Component-layer sequence | What is the first complete journey a real operator could use? |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Interviewing | [interview-guide.md](references/interview-guide.md) | Narrowing a solution-heavy, ambiguous, or multi-party direction |
