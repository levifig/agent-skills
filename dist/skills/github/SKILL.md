---
name: github
description: Maps project-management/v1 operations to GitHub repository Issue semantics through an exposed harness-native connection. Use when the selected canonical tracker is GitHub. Produces verified native outcomes without configuring credentials or a provider client.
---

# GitHub

Apply [`project-management/v1`](../project-management/contract.json) through a GitHub-capable connection already exposed by the current harness. The [capability mapping](capabilities.json) describes semantic ceilings; the runtime connection decides which operations are actually available for the selected repository.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules

- Discover exposed GitHub connections and verify the exact `owner/repository` destination before mutation. Do not assume a connection or repository from the provider name or local Git remote.
- Inspect runtime capabilities and permissions for the selected repository. `connection.describe` is only the first signal because GitHub has no reliable non-mutating write-capability probe; use successful safe native reads as additional operation-level evidence. Downgrade an unavailable mapping to `advisory`, `manual`, or `unsupported` without inventing a fallback representation.
- Accept the connection's observed permission evidence without requiring one header family: classic OAuth, fine-grained credentials, and GitHub Apps can expose different permission metadata. Never inspect, print, or infer the credential itself.
- Use repository Issues as canonical work records and the Issue body as the canonical definition. Search with GitHub's `is:issue` qualifier, read candidate matches, and reject every issue-shaped object carrying a `pull_request` property before reading or mutating it as an Issue. Apply the same check to relationship results.
- Use GitHub's native sub-issue relationships for hierarchy and native issue dependencies for directed blocked-by/blocking edges. Arbitrary related edges are unsupported. Never emulate either relationship with labels, task-list prose, links, or comments.
- Orient dependency writes from the blocked Issue: when the target blocks another Issue, write the target as a `blocked_by` edge on that other Issue, then read back both the target's `blocking` collection and the other's `blocked_by` collection.
- Use the Issue's native `state` and `state_reason` for status. The native reason set is `completed`, `not_planned`, `duplicate` with `duplicate_issue_id`, `reopened`, or `null`. GitHub's open/closed model and native reasons are narrower than a configurable workflow; Projects fields and generalized related-work edges are outside this exact mapping. Never encode a richer workflow in labels or comments and call it exact.
- Before a `duplicate` transition, read the proposed canonical Issue and reject it if it is missing, ambiguous, or carries a `pull_request` property. After the write, confirm both `state_reason: duplicate` and GitHub's native `duplicate_of` relationship against the exact requested Issue identity; the reason alone is insufficient. If the selected connection cannot read that native relationship, downgrade the duplicate transition rather than claiming exact confirmation.
- Treat a reason-only change on an already-closed Issue as manual or unsupported unless the user authorized a two-step reopen and reclose. Preserve partial evidence if only one of those transitions can be confirmed.
- Respect GitHub's native hierarchy bounds: at most 100 direct sub-issues, at most eight hierarchy levels, and cross-repository sub-issues only when parent and child repositories have the same owner. A connection that cannot safely represent the requested relationship must refuse the change rather than flatten it.
- Do not infer missing capability or permission from a `404` alone; GitHub can conceal authorization failures that way. Return the observed manual, failed, or indeterminate outcome with evidence rather than exposing or probing secrets.
- Keep the title and other Issue fields, body, sub-issues, dependencies, state, and comments as distinct native semantics.
- Read the current Issue and relevant native relationships before writing; re-read the same Issue and relationship after writing.
- Never install or authenticate a connector, request or store credentials, construct a GitHub client, invoke a direct provider transport, proxy traffic through Loaf, or store a local work record or ongoing mapping.
- Never blindly repeat Issue creation or comment append after an ambiguous response. Search or re-read native state first and return `indeterminate` when duplication cannot be excluded.
- Return the common result envelope instead of GitHub-shaped success claims.

## Verification

- The selected connection was observed in the current harness and the exact GitHub repository scope was read.
- Every requested operation maps to all visible runtime capabilities named by the `before`, `execute`, and `after` phases in `capabilities.json`.
- Every target record and related record was confirmed to be an Issue rather than a pull request before mutation, and create duplicate detection used an `is:issue` search followed by candidate reads.
- A mutation is `confirmed` only when the final native read shows the intended Issue field, body, relationship, state and state reason, or comment. A duplicate transition additionally requires native `duplicate_of` readback matching the requested canonical Issue identity.
- Missing sub-issue, dependency, state-reason, or comment capabilities are reported at their observed fidelity rather than represented through labels, prose, or another field.
- No connection configuration, credential material, provider transport, local work copy, synchronization path, or persistent mapping was created.

## Quick Reference

| Common semantic | GitHub semantic | Required runtime capability phases |
|-----------------|-----------------|------------------------------------|
| Work | Repository Issue | Scope repository and reject pull-request records; search/read before create, mutate, then read back |
| Definition | Issue body | Issue and kind read, body write, then Issue and kind readback |
| Hierarchy | Native sub-issues | Parent and sub-issue reads around native sub-issue mutation |
| Dependency | Native issue dependencies | Blocking and blocked-by reads around native dependency mutation |
| Status | Issue `state` and `state_reason` | Current state and reason around native transition |
| Comment | Issue comments | Current Issue/comments before append and both reads after |

Capability identifiers in the mapping describe GitHub semantics, not universal harness tool names. The selected connection must expose equivalent capabilities at runtime. If it exposes ordinary Issue operations but not native sub-issues or dependencies, those operations remain unsupported for that connection.

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Capabilities | [capabilities.json](capabilities.json) | Discovering runtime support and maximum honest fidelity |
| Common protocol | [project-management contract](../project-management/contract.json) | Constructing an operation or result envelope |
