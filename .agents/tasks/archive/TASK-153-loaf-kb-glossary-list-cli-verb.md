---
id: TASK-153
title: loaf kb glossary list CLI verb
spec: SPEC-034
status: done
priority: P2
created: '2026-05-02T01:25:28.985Z'
updated: '2026-05-02T01:25:28.985Z'
depends_on:
  - TASK-151
completed_at: '2026-05-02T03:35:00.000Z'
---

# TASK-153: loaf kb glossary list CLI verb

## Description

Implement `loaf kb glossary list` with `--canonical | --candidates | --all` flags. Outputs human-readable list (term + definition truncated to one line) by default. Empty glossary returns clean message, not error.

## File Hints

- `cli/commands/kb.ts`
- `cli/commands/kb.test.ts`

## Acceptance Criteria

- [ ] `loaf kb glossary list` shows all entries (default behaves like `--all`)
- [ ] `loaf kb glossary list --canonical` shows only canonical-section entries
- [ ] `loaf kb glossary list --candidates` shows only candidates-section entries
- [ ] Empty glossary: prints "No glossary entries yet" and exits 0
- [ ] Output format: `<term>: <truncated definition>` (one line per entry)
- [ ] Tests cover: each flag, empty case, mixed canonical+candidates

## Verification

```bash
npm test -- cli/commands/kb.test.ts
loaf kb glossary list
loaf kb glossary list --canonical
loaf kb glossary list --candidates
```
