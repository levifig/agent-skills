# Blindspot pass — rewrite targets (shape, breakdown)

Read-only reviewer report, 2026-07-07. Questions: unknown knowns (assumed, never stated), unknown unknowns (where an agent guesses wrong), surviving value, dead weight. Findings verbatim; adjudication in `change.md` (Planning Contract → Blast-radius findings).

## Unknown knowns

- [unknown-known] shape/SKILL.md:30 — "Use AskUserQuestion throughout" assumes the tool exists and is available; no fallback for harnesses lacking it — the planned grilling interview leans harder on this same assumption.
- [unknown-known] shape/SKILL.md:67 — Step 2 assumes VISION/STRATEGY/ARCHITECTURE.md exist and knows their paths (skill uses bare names; repo keeps them under docs/); silent on what to do when strategic docs are absent (most projects).
- [unknown-known] shape/SKILL.md:45 — "rabbit holes" is load-bearing but never defined (Shape Up vocabulary); same for "go/no-go gates" (line 85). The Change template reuses "Rabbit Holes and No-Gos" verbatim, inheriting the undefined term.
- [unknown-known] breakdown/SKILL.md:84 — "separation of concerns" declared the primary principle but never operationalized; "one concern" (line 33) is taught only by anti-examples, no positive test for a concern boundary.
- [unknown-known] breakdown/SKILL.md:69 — "Completable by a single implementer (after skills narrowing)" uses "skills narrowing" as undefined jargon.

## Unknown unknowns

- [unknown-unknown] shape/SKILL.md:80 — Step 4 interview has no stop condition; an agent guesses when it has asked "enough," and the failure the skill warns against (quick capture, line 32) is the likely wrong guess. Exactly the gap the fog register + grilling interview must fill.
- [unknown-unknown] shape/SKILL.md:63 — Step 1 recognizes only "idea file, problem description, or requirement area"; the Change model's source polymorphism (journal, spark, brainstorm, Linear issue, PR conversation — pilot change.md:30,72) is invisible, so the agent won't know to pull those sources.
- [unknown-unknown] shape/SKILL.md:78 — the misalignment branch offers three actions (surface / adjust / note for /reflect) with no selector; invites the wrong guess of silently adjusting the user's idea without confirmation.
- [unknown-unknown] shape/SKILL.md:28 — no critique/self-challenge gate anywhere; an agent won't know to interrogate its own scope, CLI/skill boundary, or smuggled-in status words before finalizing (pilot change.md Critique Gate is the target behavior).
- [unknown-unknown] breakdown/SKILL.md:138 — ordering derives only from dependency graph + priority (142-143, 250-252 blockedBy); likelihood-of-change/volatility ordering has zero precedent, so an agent defaults to dependency-only order and front-loads the most volatile units.
- [unknown-unknown] breakdown/SKILL.md:129 — Step 1 assumes a numbered SPEC input exists; in a Change-model repo there is none, so the agent hunts for a spec file that was never created.

## Surviving value

- [surviving-value] breakdown/SKILL.md:87 — the Right Size Test (87-92) + Right-Sizing Rules (66-72) are the decomposition judgment Decision 5 folds into shape; the core keeper from breakdown.
- [surviving-value] breakdown/SKILL.md:34 — "every task includes its own verification" + "keep tests with the code they test" (line 36) survive as the per-unit Verification Contract discipline.
- [surviving-value] breakdown/SKILL.md:35 — "own the decisions — decide granularity/priority autonomously, don't defer" survives as the agent-owned decomposition of pilot Decision 22.
- [surviving-value] shape/SKILL.md:53 — "solution direction (not blueprint)" / "enough direction without too much constraint" (line 31) is the shaping altitude principle; survives directly.
- [surviving-value] shape/SKILL.md:137 — "note tensions, don't fix — strategy evolves via /reflect" survives as deferral of durable-doc/strategy updates to finalize (pilot D19/21d).

## Dead weight

- [dead-weight] shape/SKILL.md:92 — SPEC-NNN ID generation shell block; killed by the no-IDs No-Go — the exact allocation machinery the model exists to shed.
- [dead-weight] shape/SKILL.md:123 — the spec lifecycle state machine (drafting→approved→implementing→complete→archived) is banned by pilot D22 and V1; delete, don't port.
- [dead-weight] shape/templates/spec.md:12 — `status: drafting` frontmatter is precisely the V1-banned status field; the SPEC template is dead weight under the Change model (superseded by templates/change.md).
- [dead-weight] shape/SKILL.md:89 — Step 5 still points only at templates/spec.md; templates/change.md already exists in the folder but the skill body never references it — the new contract is orphaned from the skill that should drive it.
- [dead-weight] shape/SKILL.md:104 — the "run /breakdown SPEC-NNN" handoff (104-106) and Suggests-Next (157-159) die with the retiring breakdown skill.
- [dead-weight] breakdown/templates/task.md:6 — the TASK-XXX entity (+ `loaf task create`, task status, Local-Tasks mode 272-287) is killed by the "no new task entity" No-Go; implementation units are in-doc packets, not rows.
- [dead-weight] breakdown/SKILL.md:175 — the Linear-native parent-rollup + sub-issue machinery (175-268) is dead as breakdown's job: the Change model publishes the Change (not units) to Linear at ceremonies only, and sub-issue generation is explicitly deferred until dogfooding proves need.
