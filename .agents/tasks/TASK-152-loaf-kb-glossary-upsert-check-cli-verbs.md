---
id: TASK-152
title: loaf kb glossary upsert + check CLI verbs
status: done
priority: P1
created: '2026-05-02T01:25:28.929Z'
updated: '2026-05-02T01:25:28.929Z'
completed_at: '2026-05-02T03:35:00.000Z'
spec: SPEC-034
depends_on:
  - TASK-151
---

# TASK-152: loaf kb glossary upsert + check CLI verbs

## Description

Wire up `loaf kb glossary upsert` and `loaf kb glossary check` subcommands using the data layer from TASK-151. `upsert <term> --definition <d> --avoid <list>` writes to canonical section (idempotent on re-invocation). `check <term>` returns canonical definition / avoided-alias-pointing-to-X / unknown.

## File Hints

- `cli/commands/kb.ts` (extend with new `glossary` subcommand)
- `cli/commands/kb.test.ts`

## Acceptance Criteria

- [ ] `loaf kb glossary upsert <term> --definition "<d>" --avoid "<a1,a2>"` writes valid frontmatter and term entry
- [ ] Second `upsert` for same term updates the existing entry (definition, avoid list); does not duplicate
- [ ] `loaf kb glossary check <known-canonical>` returns canonical definition + aliases (exit 0)
- [ ] `loaf kb glossary check <avoided-alias>` returns "avoided alias for `<canonical>`" (exit 0)
- [ ] `loaf kb glossary check <unknown>` returns "unknown" (exit 1)
- [ ] Tests cover happy paths + malformed input + missing flags

## Verification

```bash
npm run typecheck
npm test -- cli/commands/kb.test.ts
loaf kb glossary upsert "Order intake module" --definition "Receives raw orders from external systems." --avoid "OrderHandler,OrderProcessor"
loaf kb glossary check "Order intake module"   # canonical
loaf kb glossary check "OrderHandler"           # avoided
loaf kb glossary check "Unknown term"           # unknown (exit 1)
```
