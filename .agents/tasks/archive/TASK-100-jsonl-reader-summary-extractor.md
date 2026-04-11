---
id: TASK-100
spec: SPEC-029
title: JSONL extractor and .agents/tmp/ infrastructure
priority: P1
status: done
---

# TASK-100: JSONL extractor and `.agents/tmp/` infrastructure

## Objective

Create `cli/lib/journal/extractor.ts` — a module that reads Claude Code JSONL conversation logs (including subagent transcripts), extracts a clean conversation summary, and writes it to `.agents/tmp/`.

## Acceptance Criteria

### Extractor module (`cli/lib/journal/extractor.ts`)
- [ ] Reads JSONL line-by-line, parses each as JSON
- [ ] Filters to `user` and `assistant` types only
- [ ] Skips: progress, agent_progress, hook_progress, attachment, queue-operation, all other types
- [ ] Applies `enriched_at` timestamp cutoff (only entries after given ISO timestamp)
- [ ] From `user` entries: extracts `message.content` text
- [ ] From `assistant` entries: extracts `text` blocks (full text) and `tool_use` blocks (name + key param)
- [ ] Skips `thinking` blocks in assistant content
- [ ] For Bash tool_use: extracts `command` field
- [ ] For Read/Edit/Write tool_use: extracts `file_path` field
- [ ] Discovers `<session_id>/subagents/agent-*.jsonl` and includes with `--- Subagent: {description} ---` markers
- [ ] Reads `agent-*.meta.json` for subagent description/type
- [ ] Enforces 100KB summary cap — oldest entries dropped first, warning to stderr
- [ ] Tracks and returns latest JSONL timestamp from processed entries
- [ ] Handles malformed JSONL lines gracefully (skip + warning to stderr)
- [ ] Returns empty when no entries match cutoff

### Output format
```
[2026-04-10 14:21] User: reshape the spec for enrich-only
[2026-04-10 14:22] Assistant: Here's the reshaped spec. Key changes: ...
[2026-04-10 14:23] Tool: Edit .agents/specs/SPEC-029.md
[2026-04-10 14:25] Tool: Bash — git commit -m "feat: reshape SPEC-029"

--- Subagent: Research --agent flag resolution ---
[2026-04-10 14:30] Assistant: The --agent flag resolves from plugins → ...
```

### Interface
```typescript
interface ExtractionResult {
  summaryPath: string;          // Path to .agents/tmp/<session-id>-enrichment.txt
  latestTimestamp: string | null; // Latest JSONL entry timestamp (for watermark)
  isEmpty: boolean;             // True if no entries matched cutoff
}

function extractSummary(
  jsonlPath: string,
  projectDir: string,
  sessionId: string,
  agentsDir: string,
  since?: string             // ISO timestamp (enriched_at cutoff)
): Promise<ExtractionResult>;
```

### `.agents/tmp/` infrastructure
- [ ] Creates `.agents/tmp/` directory if it doesn't exist
- [ ] Writes summary to `.agents/tmp/<session-id>-enrichment.txt`
- [ ] `.agents/tmp/` added to `.gitignore`

### Tests
- [ ] Type filtering: only user/assistant entries pass through
- [ ] Timestamp cutoff: entries before `since` are excluded
- [ ] Thinking block skip: not included in summary
- [ ] Tool_use extraction: name + command (Bash) or file_path (Edit/Read/Write)
- [ ] Subagent discovery: finds and includes subagent/*.jsonl with markers
- [ ] 100KB cap: summary truncated, oldest entries dropped, warning emitted
- [ ] Malformed lines: skipped with warning, don't crash
- [ ] Empty result: no entries → isEmpty: true, no file written
- [ ] Latest timestamp tracking: returns correct timestamp from processed entries
- [ ] `npm run typecheck` passes

## Dependencies

None — this is the foundation module.
