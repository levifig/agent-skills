---
id: TASK-154
title: loaf kb glossary stabilize + propose CLI verbs
status: done
priority: P2
created: '2026-05-02T01:25:29.041Z'
updated: '2026-05-02T01:25:29.041Z'
completed_at: '2026-05-02T03:35:00.000Z'
spec: SPEC-034
depends_on:
  - TASK-151
  - TASK-152
---

# TASK-154: loaf kb glossary stabilize + propose CLI verbs

## Description

Implement `stabilize <term>` (promotes a candidate to canonical; fails fast if term not in candidates) and `propose <term> --definition <d>` (writes to candidates section). Per SPEC-034, `propose` is reserved for future use — SPEC-034 itself does not invoke it, but the verb must exist for the future `/shape` glossary evolution work (idea 20260501-231923).

## File Hints

- `cli/commands/kb.ts`
- `cli/commands/kb.test.ts`

## Acceptance Criteria

- [ ] `loaf kb glossary propose <term> --definition "<d>"` writes to candidates section
- [ ] `loaf kb glossary stabilize <term>` moves entry from candidates → canonical, preserves the candidate's definition
- [ ] `stabilize <term> --definition "<new>"` overrides definition during promotion
- [ ] `stabilize <term>` when term not in candidates: exits non-zero with "not in candidates: `<term>`"
- [ ] After `stabilize`, the term is no longer in candidates section
- [ ] Tests cover: propose + stabilize round-trip, stabilize-missing failure, definition override on stabilize

## Verification

```bash
npm test -- cli/commands/kb.test.ts
loaf kb glossary propose "Seam" --definition "Place behaviour can be altered without editing in place"
loaf kb glossary stabilize "Seam"
loaf kb glossary check "Seam"   # now canonical
```
