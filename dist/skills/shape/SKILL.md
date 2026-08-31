---
name: shape
description: Shapes a problem narrative into a bounded canonical tracker work contract. Use when work needs testable completion criteria, explicit exclusions, hierarchy, or dependencies before implementation. Produces a verified native record ready for implement.
---

# Shape

Route the ephemeral [work contract](templates/work-contract.md) once into canonical native tracker fields through [`project-management/v1`](../project-management/SKILL.md). The provider owns native identity and state; Loaf owns the questions and packet shape that make the definition actionable.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules

- As the first action, run `loaf journal log "skill(shape): <concise intent>"` against the current private local journal. If the write fails, report the failure and continue only when the work can safely proceed; never put invocation bookkeeping in the tracker.
- Confirm the destination, runtime capabilities, and whether a matching native record already exists before creation.
- If creation returns an ambiguous result, search and re-read native state; never repeat the create blindly.
- Preserve the problem narrative's intent while removing solution assumptions not required by constraints.
- Make the rideable increment concrete in the existing native definition and criteria: Rider, complete Journey, Entry point, observable Outcome, real Dogfood, Safety/integrity proof, Learning sought, and explicit Deferrals. These are required decisions, not required native headings or a new schema.
- Set the title through the native work field, then fill every definition-packet field. Definition of done uses observable criteria; out of scope names what this work deliberately will not solve.
- Keep hierarchy and dependencies in their native relationship fields, not in comments or body prose alone.
- Decompose only when a child earns an independently verifiable definition of done. Read [Decomposition](references/decomposition.md) before changing hierarchy.
- Read back the native record, definition, hierarchy, dependencies, and current status before declaring it ready.
- The main agent executes common operations through the selected provider skill; delegation never changes semantics or authority.

## Verification

- The tracker-issued native reference is retained and the final read matches the intended native title and every routed definition field.
- Each completion criterion names observable evidence and can be evaluated without reconstructing the pitch conversation.
- The named rider can complete the journey through its real entry point; dogfood, safety/integrity proof, learning sought, and deferrals are concrete without adding another work record or gate.
- Any foundation work is exercised by this journey or remains explicitly deferred; no child is only a future wheel.
- Out-of-scope boundaries prevent likely expansion rather than restating the goal.
- Parent/child and dependency relationships are confirmed through their native fields.
- Any unsupported provider feature is reported with honest fidelity; no prose substitute is presented as exact.

## Quick Reference

| Need | Common operation |
|------|------------------|
| Find existing work | `work.read` |
| Mint native identity | `work.create` |
| Change native title | `work.update` |
| Write canonical body | `definition.write` |
| Set parent/child | `hierarchy.change` |
| Set blocking edge | `dependency.change` |
| Prove readiness | Read back all relevant native fields |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Decomposition | [decomposition.md](references/decomposition.md) | Deciding whether one criterion deserves its own native child record |
