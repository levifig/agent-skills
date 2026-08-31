---
name: project-management
description: Defines the provider-neutral project-management/v1 contract for canonical tracker work. Use when a Flow ceremony must read or mutate native work through an already-configured harness connection. Not for configuring providers or storing work locally.
---

# Project Management

Use the closed [`project-management/v1`](contract.json) operation vocabulary. The main agent executes it through a provider skill that maps those semantics to the selected harness-native connection; the tracker remains canonical before, during, and after every operation.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules

- Discover only connections the harness already exposes. Never install, authenticate, or configure one.
- Select and report the exact native destination before mutation. A provider name alone is not a destination.
- Discover runtime capabilities before choosing an operation or fidelity.
- Read the destination or target record before every write, including duplicate detection before creation.
- Re-read authoritative native state after every mutation.
- Never blindly retry an ambiguous create or comment append. Re-read first and return `indeterminate` if the native result cannot be proven.
- Keep definition, hierarchy, dependency, status, and comment operations distinct. A comment is evidence or collaboration, never a substitute field.
- Return every result with operation, destination, native reference, outcome, fidelity, observed state, and verification evidence.
- Read [Record Contract](references/record-contract.md) when mapping a ceremony to native tracker fields.
- Read [Provider Modules](references/provider-modules.md) before adding or reviewing a tracker backend.

## Verification

- The selected provider mapping covers the requested operation and its required runtime capability is visible.
- Writes have both a prior authoritative read and a post-write authoritative readback.
- The observed outcome is one of `confirmed`, `unchanged`, `partial`, `failed`, or `indeterminate`.
- Fidelity is reported as `exact`, `advisory`, `manual`, or `unsupported` without promotion by assumption.
- The result names the same native destination and reference that were operated.

## Quick Reference

| Semantic area | Read | Write |
|---------------|------|-------|
| Work | `work.read` | `work.create`, `work.update` |
| Definition | `work.read` | `definition.write` |
| Hierarchy | `hierarchy.read` | `hierarchy.change` |
| Dependency | `dependency.read` | `dependency.change` |
| Status | `status.read` | `status.transition` |
| Comment | `comment.list` | `comment.append` |

Use the [work contract](templates/work-contract.md) as an ephemeral packet for routing definition fields to the native record, and use the [tracker update](templates/tracker-update.md) for evidence-bearing collaboration. A template is never a second stored work record.

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Record contract | [record-contract.md](references/record-contract.md) | Mapping Flow semantics, outcomes, or degradation to a native provider |
| Provider modules | [provider-modules.md](references/provider-modules.md) | Adding Linear, GitHub, GitLab, Gitea, or a community tracker mapping without changing core Flow |
