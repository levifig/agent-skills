---
id: TASK-101
spec: SPEC-029
title: loaf session enrich CLI command + hook isolation + librarian update
priority: P1
status: done
---

# TASK-101: `loaf session enrich` CLI command + hook isolation + librarian update

## Objective

Add the `loaf session enrich` subcommand that discovers JSONL files, calls the extractor, spawns the librarian agent, and manages the `enriched_at` watermark. Also add `LOAF_ENRICHMENT` hook isolation and update the librarian profile.

## Acceptance Criteria

### CLI command
- [ ] `loaf session enrich` — enriches the active session
- [ ] `loaf session enrich <file>` — enriches a specific session file
- [ ] `--dry-run`: passes `--disallowedTools "Edit,Write"` + appends "do not edit" to prompt, captures text output to stdout
- [ ] `--model <model>`: passed through to `claude --model <model>`
- [ ] Registered as subcommand in session command group

### JSONL discovery
- [ ] Reads `claude_session_id` from session frontmatter
- [ ] Derives project directory: `${CLAUDE_CONFIG_DIR}/projects/` + cwd path with dashes
- [ ] Constructs JSONL path: `<project-dir>/<session_id>.jsonl`
- [ ] Falls back to scanning project directory if direct path fails
- [ ] Validates: `claude` binary available, `claude_session_id` exists, JSONL exists

### Agent invocation
- [ ] Spawns: `LOAF_ENRICHMENT=1 claude --agent librarian -p --no-session-persistence --permission-mode acceptEdits --max-turns 10`
- [ ] `LOAF_ENRICHMENT=1` set only on child process env via `spawn()`, never on parent
- [ ] Prompt includes: session file path + temp file path + instructions
- [ ] `--model` passed through when specified
- [ ] For `--dry-run`: adds `--disallowedTools "Edit,Write"` + behavioral instruction

### Watermark management
- [ ] `enriched_at` added to `SessionFrontmatter` interface
- [ ] On agent success (exit 0): advances `enriched_at` to `latestTimestamp` from extractor
- [ ] On agent failure (exit non-zero): does NOT advance, reports error to stderr
- [ ] On empty summary (no new entries): exits 0 without spawning agent, no watermark change
- [ ] Uses existing `readSessionFile()` + `writeFileAtomic()` for safe frontmatter writes

### Hook isolation
- [ ] `LOAF_ENRICHMENT=1` check at top of `loaf session start` action → `process.exit(0)`
- [ ] `LOAF_ENRICHMENT=1` check at top of `loaf session end` action → `process.exit(0)`
- [ ] Enrichment does NOT create spurious session files

### Librarian profile update (`content/agents/librarian.md`)
- [ ] Add "Journal enrichment" to "What You Tend" section
- [ ] Note: conversation summary in `.agents/tmp/`, not raw JSONL
- [ ] No scope constraint changes — `.agents/tmp/` is within `.agents/`

### Error handling
- [ ] `claude` binary not found → exit 1 with clear message
- [ ] `--agent librarian` not found → exit 1 with "Ensure Loaf is installed" message
- [ ] Missing `claude_session_id` in frontmatter → exit 1 with explanation
- [ ] Missing JSONL file → exit 1 with path attempted
- [ ] Agent non-zero exit → exit 1, don't advance watermark

### Build + tests
- [ ] `loaf build` succeeds
- [ ] `npm run typecheck` passes
- [ ] JSONL discovery tests: path derivation, fallback scan, error cases
- [ ] Hook isolation test: LOAF_ENRICHMENT=1 causes start/end to exit early
- [ ] Watermark test: advances to JSONL timestamp, not current time
- [ ] Watermark test: does NOT advance on failure

## Dependencies

- TASK-100 (extractor module)
