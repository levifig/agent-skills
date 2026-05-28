---
id: TASK-055
title: Wire loaf cleanup CLI command with interactive mode
spec: SPEC-012
status: done
priority: P1
created: '2026-03-28T23:36:10.956Z'
updated: '2026-03-29T02:21:19.103Z'
completed_at: '2026-03-29T02:21:19.102Z'
---

# TASK-055: Wire loaf cleanup CLI command with interactive mode

## Description

Register the `loaf cleanup` command, wire it to the scanner engine, and implement the interactive action loop. This is the user-facing layer that consumes TASK-053 (prompts) and TASK-054 (scanner).

## What to do

1. Create `cli/commands/cleanup.ts`:
   - `registerCleanupCommand(program: Command): void`
   - Options: `--dry-run`, `--sessions`, `--specs`, `--plans`, `--drafts`
   - Flow:
     a. Find `.agents/` dir via `findAgentsDir()`
     b. Run scanner with filter options → `ScanResult`
     c. Print formatted summary table (colored, like `loaf task list`)
     d. If `--dry-run` or non-TTY → print and exit
     e. Otherwise, iterate actionable recommendations:
        - Show artifact path + reason
        - For deletes: show first 3 lines of frontmatter as preview
        - Prompt: Archive / Delete / Keep / Skip (per-item)
        - Execute action: `archiveTasks()`, `archiveSpecs()`, `fs.rmSync()`, etc.
     f. Final reconciliation: save updated index

2. Register in `cli/index.ts`:
   - Import `registerCleanupCommand`
   - Add to command registration block

3. Archive operations for non-task/spec artifacts (sessions, councils, reports):
   - These don't have existing archive helpers — implement simple move-to-archive + set `archived_at`/`archived_by` in frontmatter
   - Create `archive/` subdirectory on first use

4. Tests:
   - `--dry-run` produces output without prompts
   - Filter flags restrict scan scope
   - Archive operations move files correctly
   - Delete operations remove files
   - Non-TTY behaves like `--dry-run`

## Acceptance Criteria

- [ ] `loaf cleanup` registered and accessible via CLI
- [ ] `--dry-run` prints summary table and exits without prompts
- [ ] `--sessions`, `--specs`, `--plans`, `--drafts` filter to specific artifact types
- [ ] Non-TTY invocation behaves like `--dry-run`
- [ ] Per-item confirmation with Archive / Delete / Keep / Skip choices
- [ ] Delete operations show frontmatter preview before confirmation
- [ ] Archive operations move to `archive/` and set metadata in frontmatter
- [ ] Tasks/specs archived via existing `archiveTasks()` / `archiveSpecs()`
- [ ] Sessions/councils/reports archived with `archived_at` + `archived_by`
- [ ] Plans deleted (not archived) per spec rules
- [ ] `/crystallize` suggestion shown for sessions with extractable learnings

## Verification

```bash
npm run typecheck && npm run test && npx tsx cli/index.ts cleanup --dry-run
```
