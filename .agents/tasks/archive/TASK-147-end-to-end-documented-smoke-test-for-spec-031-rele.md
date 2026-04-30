---
id: TASK-147
title: End-to-end documented smoke test for SPEC-031 release flow
spec: SPEC-031
status: done
priority: P3
created: '2026-04-29T17:28:14.960Z'
updated: '2026-04-30T14:47:49.737Z'
depends_on:
  - TASK-137
  - TASK-138
  - TASK-139
  - TASK-140
  - TASK-141
  - TASK-142
  - TASK-143
  - TASK-144
  - TASK-145
  - TASK-146
completed_at: '2026-04-30T14:47:49.736Z'
---

# TASK-147: End-to-end documented smoke test for SPEC-031 release flow

## Description

Run the end-to-end documented smoke test once after Tasks 1-10 have merged, exercising every Test Condition in SPEC-031. The smoke run executes the canonical 5-step release sequence end-to-end and confirms no ceremony commits, no commitlint rewording, and no manual CHANGELOG/version/tag/release/branch operations are required. Results are captured in the session journal for traceability. Implements SPEC-031 Task 11.

## Acceptance Criteria

- [ ] Each item in the spec's "Test Conditions" section is exercised at least once during the smoke run (path-token validate-commit pass, stub re-insertion, curated-entries preservation, chore-shape commit, commitlint compatibility, monorepo dry-run, `--version-file` override, release-only PR, post-merge guardrails, base-detection precedence, idempotency).
- [ ] No ceremony commits appear in the shipped branch (no stub-restore chore, no hand-patched CHANGELOG, no manual version edits).
- [ ] No commit message is reworded to dodge `validate-commit`; commitlint accepts the `chore: release v<semver>` commit on first try.
- [ ] `loaf release --post-merge` tags, creates GH release, pulls base, best-effort deletes branch, and exits clean.
- [ ] Smoke-run results are recorded in the active session journal with timestamps and observed behavior per Test Condition.

## Verification

```bash
# Manual 5-step procedure (Success Metric):
# 1. loaf release --pre-merge
# 2. gh pr create ...
# 3. (review)
# 4. gh pr merge --squash
# 5. loaf release --post-merge
# Procedure passes if it runs cleanly with no ceremony commits, no
# commitlint rewording, no manual CHANGELOG/version/tag/release/branch
# operations, and every Test Condition is exercised at least once.
```
