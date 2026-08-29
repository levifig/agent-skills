---
name: shape
description: Shapes a problem narrative into a bounded canonical tracker work contract. Use when work needs testable completion criteria, explicit exclusions, hierarchy, or dependencies before implementation. Produces a verified native record ready for implement.
---

# Shape

Write the complete [work contract](../../templates/work-contract.md) directly to the canonical tracker through [`project-management/v1`](../project-management/SKILL.md). The provider owns native identity and state; Loaf owns the questions and template that make the definition actionable.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules

- As the first action, emit a private `skill(shape)` continuity event when the vNext continuity capability is present. Never put invocation bookkeeping in the tracker.
- Confirm the destination, runtime capabilities, and whether a matching native record already exists before creation.
- If creation returns an ambiguous result, search and re-read native state; never repeat the create blindly.
- Preserve the problem narrative's intent while removing solution assumptions not required by constraints.
- Fill every work-contract field. Definition of done uses observable criteria; out of scope names what this work deliberately will not solve.
- Keep hierarchy and dependencies in their native relationship fields, not in comments or body prose alone.
- Decompose only when a child earns an independently verifiable definition of done. Read [Decomposition](references/decomposition.md) before changing hierarchy.
- Read back the native record, definition, hierarchy, dependencies, and current status before declaring it ready.
- The main agent and optional project-manager profile execute the same common operations; delegation never changes semantics or authority.

## Verification

- The tracker-issued native reference is retained and the final read matches the intended title and complete work contract.
- Each completion criterion names observable evidence and can be evaluated without reconstructing the pitch conversation.
- Out-of-scope boundaries prevent likely expansion rather than restating the goal.
- Parent/child and dependency relationships are confirmed through their native fields.
- Any unsupported provider feature is reported with honest fidelity; no prose substitute is presented as exact.

## Quick Reference

| Need | Common operation |
|------|------------------|
| Find existing work | `work.read` |
| Mint native identity | `work.create` |
| Write canonical body | `definition.write` |
| Set parent/child | `hierarchy.change` |
| Set blocking edge | `dependency.change` |
| Prove readiness | Read back all relevant native fields |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Decomposition | [decomposition.md](references/decomposition.md) | Deciding whether one criterion deserves its own native child record |
