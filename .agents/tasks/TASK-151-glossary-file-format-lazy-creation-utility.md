---
id: TASK-151
title: Glossary file format + lazy creation utility
status: done
priority: P1
created: '2026-05-02T01:25:28.874Z'
updated: '2026-05-02T01:25:28.874Z'
completed_at: '2026-05-02T03:35:00.000Z'
spec: SPEC-034
---

# TASK-151: Glossary file format + lazy creation utility

## Description

Build the data layer for the domain glossary KB convention. Define the file format at `docs/knowledge/glossary.md` with `type: glossary` frontmatter and four sections: `## Canonical Terms`, `## Candidates`, `## Relationships`, `## Flagged ambiguities`. Implement parser/serializer + lazy-creation utility in a new `cli/lib/kb/glossary.ts` module. **No CLI verbs in this task** — just the library that subsequent tasks consume.

## File Hints

- `cli/lib/kb/glossary.ts` (new module)
- `cli/lib/kb/glossary.test.ts` (new tests)
- `cli/lib/kb/index.ts` (export new module)

## Acceptance Criteria

- [ ] Module exports: `readGlossary()`, `writeGlossary(data)`, `parseGlossary(text)`, `serializeGlossary(data)`, `ensureGlossaryExists()`
- [ ] Frontmatter validation: requires `type: glossary`, rejects other types with explicit error
- [ ] Lazy creation: `ensureGlossaryExists()` creates `docs/knowledge/glossary.md` with empty sections only on first call; second call is a no-op
- [ ] Parse/serialize round-trip is lossless (re-serializing parsed data produces byte-identical output)
- [ ] Term entries support: term name, definition, `_Avoid_:` aliases list
- [ ] Tests cover: parse/serialize round-trip, empty-file creation, frontmatter validation, malformed file handling, idempotent ensure

## Verification

```bash
npm run typecheck
npm test -- cli/lib/kb/glossary.test.ts
```
