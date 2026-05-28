---
id: TASK-031
title: Add test infrastructure and tests for task management CLI
status: done
priority: P1
created: '2026-03-17T00:59:13.998Z'
updated: '2026-03-17T10:15:30.958Z'
completed_at: '2026-03-17T10:15:30.957Z'
---

# TASK-031: Add test infrastructure and tests for task management CLI

## Description

Set up vitest as the test framework and write tests for the SPEC-010 task management code. This is the first test infrastructure in the project — establish patterns that all future code follows.

## Scope

### Test infrastructure
- Install vitest
- Add `npm run test` script
- Configure for TypeScript + ESM (matching tsup setup)
- Add test run to the "Before Committing" checklist in CLAUDE.md

### Tests to write (priority order)

1. **parser.ts** — pure functions, highest value
   - Status normalization (aliases, case insensitivity, defaults)
   - Priority normalization
   - Date normalization (Date objects, ISO strings, bare dates, missing)
   - Task file parsing (frontmatter extraction, ID from filename, slug generation)
   - Spec file parsing
   - Edge cases: missing fields, malformed frontmatter, empty files

2. **migrate.ts** — filesystem logic with edge cases
   - buildIndexFromFiles (active + archived files, next_id computation)
   - loadIndex / saveIndex roundtrip
   - syncFrontmatterFromIndex (only writes changed files)
   - findOrphans (no double-counting, archive subdirectory support)

3. **task.ts validation** — CLI input validation
   - Slug generation from titles
   - --spec validation rejects unknown specs
   - --status validation rejects invalid statuses
   - --priority validation rejects invalid priorities
   - --depends-on validation rejects unknown task IDs

## Acceptance Criteria

- [ ] `npm run test` runs vitest and passes
- [ ] parser.ts has >90% branch coverage
- [ ] migrate.ts core functions have tests with temp directory fixtures
- [ ] CLI validation edge cases are covered
- [ ] CI-ready (no interactive prompts, deterministic)

## Verification

```bash
npm run test
```
