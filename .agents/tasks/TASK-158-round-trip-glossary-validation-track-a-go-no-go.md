---
id: TASK-158
title: Round-trip glossary validation (Track A go/no-go)
status: done
priority: P1
created: '2026-05-02T01:25:35.911Z'
updated: '2026-05-02T01:25:35.911Z'
completed_at: '2026-05-02T03:35:00.000Z'
spec: SPEC-034
depends_on:
  - TASK-157
---

# TASK-158: Round-trip glossary validation (Track A go/no-go)

## Description

**Track A go/no-go gate.** Manual end-to-end test of the foundation. Run a real `/architecture` session in this codebase that mentions a load-bearing term not yet in glossary; verify the skill offers and runs `loaf kb glossary upsert` (or `stabilize`). Then start a second `/architecture` session that uses an avoided alias for the same term; verify it reads the glossary at start and challenges the drift in conversation. If either step fails, iterate TASK-157 SKILL.md until both pass. Track A is not done until this test passes once.

## File Hints

- (no source files — this is a validation task)
- Journal entries via `loaf session log` capture the test outcome
- `docs/knowledge/glossary.md` will be populated as a side-effect (commit it)

## Acceptance Criteria

- [ ] First `/architecture` invocation produced at least one glossary `upsert` or `stabilize` (verifiable via `loaf kb glossary list` after, and journal entry: `decision(architecture): upserted/stabilized term <X>`)
- [ ] Second `/architecture` invocation evidence of glossary read at start (journal entry or session prose)
- [ ] Second invocation challenged use of an avoided alias (text confronting the drift in conversation)
- [ ] If either step failed, TASK-157 was iterated and this task re-run until both pass
- [ ] Final journal entry: `validate(track-a): glossary round-trip works end-to-end`

## Verification

```bash
loaf kb glossary list   # should show ≥1 canonical term after invocation 1
# Manual review of session journal for the two /architecture sessions
loaf session log "validate(track-a): glossary round-trip works end-to-end"
```
