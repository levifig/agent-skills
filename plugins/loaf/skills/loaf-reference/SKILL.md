---
name: loaf-reference
description: >-
  Explains vNext Loaf Flow authority, artifacts, and ceremony boundaries. Use
  when deciding where work belongs or how one Flow stage hands evidence to
  another. Not for provider-specific tracker operations.
user-invocable: false
version: 0.5.0
---

# Loaf Reference

Loaf supplies the ceremony, judgment, and artifact shape. The canonical tracker supplies shared work identity and state; the current harness supplies the already-configured connection used to reach it.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules

- Treat the live native tracker record as the only shared work record.
- Use Loaf templates to write or evaluate tracker content, never to create a second authority.
- Let the harness own connection discovery, authentication, and credential handling.
- Keep code and deliberately promoted artifacts in Git; keep private continuity out of tracker fields.
- Read [Authority Model](references/authority-model.md) before assigning a new responsibility.
- Read [Flow Semantics](references/flow-semantics.md) before changing a ceremony or handoff.

## Verification

- The tracker record contains the canonical definition, relationships, and workflow state.
- Every mutation names the native destination and is verified with an authoritative read.
- No local work record, tracker proxy, or reconciliation path was introduced.
- Artifact fields match the applicable shared template exactly.

## Quick Reference

| Concern | Authority | Portable artifact |
|---------|-----------|-------------------|
| Discovery handoff | Loaf ceremony | [Problem narrative](templates/problem-narrative.md) |
| Work definition | Tracker | [Work contract](templates/work-contract.md) |
| Progress evidence | Tracker | [Tracker update](templates/tracker-update.md) |
| Service connection | Harness | None |
| Code | Git | Repository-native |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Authority | [authority-model.md](references/authority-model.md) | Deciding which system owns a responsibility |
| Flow | [flow-semantics.md](references/flow-semantics.md) | Moving work between ceremonies without changing authority |
