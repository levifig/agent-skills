---
id: TASK-237
title: Prove SQLite state does not store secrets
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T21:57:50Z'
updated: '2026-05-28T22:00:03Z'
completed_at: '2026-05-28T22:00:03Z'
depends_on:
  - TASK-236
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-237-sqlite-secret-boundary-proof.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./...
done: >-
  Tests prove the SQLite schema and Go-native state runtime preserve the
  secret boundary: backend mappings store only external identity metadata, and
  no native SQLite command/runtime source introduces Linear token, API key,
  password, refresh token, credential, or auth-storage terms.
---

# TASK-237: Prove SQLite state does not store secrets

## Description

Close the SPEC-040 secret-storage test condition with explicit guardrail tests.
The schema already rejects secret-looking column names; this task tightens the
proof around backend mappings and the Go-native SQLite command/runtime source.

This is verification hardening only. It should not add auth behavior, Linear
OAuth behavior, or any new storage columns.

## Acceptance Criteria

- [x] `backend_mappings` stores only external identity/sync metadata columns.
- [x] The initial schema still rejects token/password/API-key/secret/credential
  style columns across all tables.
- [x] Go-native SQLite state/CLI source does not introduce Linear token, API
  key, password, refresh token, credential, or auth-storage terms.
- [x] The proof covers state runtime code and native CLI dispatch code.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
```
