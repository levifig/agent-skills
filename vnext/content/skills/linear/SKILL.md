---
name: linear
description: Maps project-management/v1 operations to Linear issue semantics through an exposed harness-native connection. Use when the selected canonical tracker is Linear. Produces verified native outcomes without configuring credentials or a provider client.
---

# Linear

Apply [`project-management/v1`](../project-management/contract.json) through a Linear-capable connection already exposed by the current harness. The [capability mapping](capabilities.json) describes semantic ceilings; the runtime connection decides which operations are actually available.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules

- Discover exposed Linear connections and verify the workspace and team before mutation. Do not assume a connection or destination from the provider name.
- Inspect runtime capabilities and downgrade unavailable mappings to `advisory`, `manual`, or `unsupported` truthfully.
- Resolve native identifiers through reads and retain the exact issue reference returned by Linear.
- Read the current issue and relevant workspace metadata before writing; re-read the issue after writing.
- Treat description, parent/children, relations, workflow state, and comments as different native semantics.
- Never install a connector, request or store credentials, construct a provider client, or bypass the harness connection.
- Never blindly repeat issue creation or comment append after an ambiguous response.
- Return the common result envelope instead of provider-shaped success claims.

## Verification

- The selected connection was observed in the current harness and its Linear workspace/team scope was read.
- Every requested operation maps to a visible runtime capability from `capabilities.json`.
- A mutation is `confirmed` only when the final native read shows the intended state.
- Missing relation, hierarchy, status, or comment capabilities are reported rather than represented in another field.
- No connection configuration, credential material, provider transport, or local work copy was created.

## Quick Reference

| Common semantic | Linear semantic | Required runtime capability |
|-----------------|-----------------|-----------------------------|
| Work | Issue | Issue read/create/update |
| Definition | Issue description | Description write |
| Hierarchy | Parent and children | Parent read/write |
| Dependency | Issue relations | Relation read/write |
| Status | Issue workflow state | State read/write |
| Comment | Issue comments | Comment read/write |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Capabilities | [capabilities.json](capabilities.json) | Discovering runtime support and maximum honest fidelity |
| Common protocol | [project-management contract](../project-management/contract.json) | Constructing an operation or result envelope |
