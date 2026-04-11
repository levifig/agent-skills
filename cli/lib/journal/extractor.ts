/**
 * JSONL Extractor
 *
 * Reads Claude Code JSONL conversation logs (including subagent transcripts),
 * filters to user/assistant entries, extracts a clean conversation summary,
 * and writes it to .agents/tmp/.
 *
 * This is the deterministic extraction half of SPEC-029's enrich pipeline.
 * The librarian agent handles the judgment-based enrichment step.
 */

import { readFileSync, writeFileSync, existsSync, mkdirSync, readdirSync } from "fs";
import { join, basename } from "path";

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

export interface ExtractionResult {
  /** Path to .agents/tmp/<session-id>-enrichment.txt */
  summaryPath: string;
  /** Latest JSONL entry timestamp (ISO 8601) from processed entries */
  latestTimestamp: string | null;
  /** True if no entries matched the cutoff */
  isEmpty: boolean;
}

/** A single formatted line ready for the summary */
interface SummaryLine {
  /** ISO 8601 timestamp from the JSONL entry */
  timestamp: string;
  /** Formatted output line */
  text: string;
}

// ─────────────────────────────────────────────────────────────────────────────
// Constants
// ─────────────────────────────────────────────────────────────────────────────

/** Maximum summary size in bytes */
const MAX_SUMMARY_BYTES = 100 * 1024;

/** JSONL entry types we keep */
const KEPT_TYPES = new Set(["user", "assistant"]);

/** Tools where we extract file_path as the key parameter */
const FILE_PATH_TOOLS = new Set(["Read", "Edit", "Write"]);

// ─────────────────────────────────────────────────────────────────────────────
// Public API
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Extract a conversation summary from a Claude Code JSONL log.
 *
 * Reads the main JSONL and any subagent transcripts, filters to user/assistant
 * entries, applies an optional timestamp cutoff, and writes a readable summary
 * to .agents/tmp/.
 *
 * @param jsonlPath   - Path to main <session_id>.jsonl
 * @param projectDir  - Claude Code project directory (for subagent discovery)
 * @param sessionId   - Claude session ID
 * @param agentsDir   - .agents/ directory (for writing tmp/)
 * @param since       - ISO timestamp cutoff (enriched_at) — only entries after this
 */
export async function extractSummary(
  jsonlPath: string,
  projectDir: string,
  sessionId: string,
  agentsDir: string,
  since?: string,
): Promise<ExtractionResult> {
  const tmpDir = join(agentsDir, "tmp");
  const summaryPath = join(tmpDir, `${sessionId}-enrichment.txt`);

  // Extract main conversation lines
  const mainLines = extractFromJsonl(jsonlPath, since);

  // Discover and extract subagent transcripts
  const subagentLines = extractSubagents(projectDir, sessionId, since);

  const allLines = [...mainLines, ...subagentLines];

  // If nothing matched, return empty result without writing a file
  if (allLines.length === 0) {
    return { summaryPath, latestTimestamp: null, isEmpty: true };
  }

  // Build summary text, enforcing 100KB cap.
  // latestTimestamp is computed from surviving lines (post-truncation),
  // so the watermark never advances past content the librarian actually sees.
  const summaryResult = buildSummary(mainLines, subagentLines);

  // Ensure .agents/tmp/ exists
  mkdirSync(tmpDir, { recursive: true });

  // Write summary file
  writeFileSync(summaryPath, summaryResult.text, "utf-8");

  return { summaryPath, latestTimestamp: summaryResult.latestTimestamp, isEmpty: false };
}

// ─────────────────────────────────────────────────────────────────────────────
// JSONL Parsing
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Extract formatted lines from a single JSONL file.
 * Filters to user/assistant types, applies since cutoff.
 */
function extractFromJsonl(filePath: string, since?: string): SummaryLine[] {
  if (!existsSync(filePath)) {
    return [];
  }

  const raw = readFileSync(filePath, "utf-8");
  const lines = raw.split("\n");
  const result: SummaryLine[] = [];

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim();
    if (line === "") continue;

    let entry: Record<string, unknown>;
    try {
      entry = JSON.parse(line) as Record<string, unknown>;
    } catch {
      process.stderr.write(
        `Warning: malformed JSONL at line ${i + 1} in ${filePath}, skipping\n`,
      );
      continue;
    }

    const type = entry.type as string | undefined;
    if (!type || !KEPT_TYPES.has(type)) continue;

    const timestamp = entry.timestamp as string | undefined;
    if (!timestamp) continue;

    // Apply since cutoff
    if (since && timestamp <= since) continue;

    const formatted = formatEntry(entry, timestamp);
    result.push(...formatted);
  }

  return result;
}

// ─────────────────────────────────────────────────────────────────────────────
// Entry Formatting
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Format a single JSONL entry into one or more summary lines.
 */
function formatEntry(
  entry: Record<string, unknown>,
  timestamp: string,
): SummaryLine[] {
  const ts = formatTimestamp(timestamp);
  const type = entry.type as string;
  const message = entry.message as Record<string, unknown> | undefined;
  if (!message) return [];

  if (type === "user") {
    return formatUserEntry(ts, timestamp, message);
  }

  if (type === "assistant") {
    return formatAssistantEntry(ts, timestamp, message);
  }

  return [];
}

/**
 * Format a user entry. User message.content is typically a string.
 */
function formatUserEntry(
  ts: string,
  rawTimestamp: string,
  message: Record<string, unknown>,
): SummaryLine[] {
  const content = message.content;
  if (typeof content === "string") {
    return [{ timestamp: rawTimestamp, text: `[${ts}] User: ${content}` }];
  }
  // content could theoretically be an array of blocks, but empirically it's a string
  if (Array.isArray(content)) {
    const texts = content
      .filter((b: unknown) => {
        const block = b as Record<string, unknown>;
        return block.type === "text";
      })
      .map((b: unknown) => (b as Record<string, string>).text);
    if (texts.length > 0) {
      return [{ timestamp: rawTimestamp, text: `[${ts}] User: ${texts.join("\n")}` }];
    }
  }
  return [];
}

/**
 * Format an assistant entry. Content is an array of blocks (text, tool_use, thinking).
 */
function formatAssistantEntry(
  ts: string,
  rawTimestamp: string,
  message: Record<string, unknown>,
): SummaryLine[] {
  const content = message.content;
  if (!Array.isArray(content)) return [];

  const result: SummaryLine[] = [];

  for (const block of content) {
    const b = block as Record<string, unknown>;
    const blockType = b.type as string;

    if (blockType === "thinking") {
      // Skip thinking blocks
      continue;
    }

    if (blockType === "text") {
      const text = b.text as string;
      if (text) {
        result.push({ timestamp: rawTimestamp, text: `[${ts}] Assistant: ${text}` });
      }
      continue;
    }

    if (blockType === "tool_use") {
      const toolName = b.name as string;
      const input = b.input as Record<string, unknown> | undefined;
      const toolLine = formatToolUse(ts, toolName, input);
      result.push({ timestamp: rawTimestamp, text: toolLine });
      continue;
    }
  }

  return result;
}

/**
 * Format a tool_use block. Extracts key parameter based on tool name.
 */
function formatToolUse(
  ts: string,
  toolName: string,
  input?: Record<string, unknown>,
): string {
  if (toolName === "Bash" && input?.command) {
    return `[${ts}] Tool: Bash \u2014 ${input.command}`;
  }

  if (FILE_PATH_TOOLS.has(toolName) && input?.file_path) {
    return `[${ts}] Tool: ${toolName} ${input.file_path}`;
  }

  return `[${ts}] Tool: ${toolName}`;
}

// ─────────────────────────────────────────────────────────────────────────────
// Subagent Discovery
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Discover and extract subagent transcripts.
 * Looks for <session_id>/subagents/agent-*.jsonl in the project directory.
 * Returns formatted lines with subagent markers inserted.
 */
function extractSubagents(
  projectDir: string,
  sessionId: string,
  since?: string,
): SummaryLine[] {
  const subagentDir = join(projectDir, sessionId, "subagents");
  if (!existsSync(subagentDir)) {
    return [];
  }

  let files: string[];
  try {
    files = readdirSync(subagentDir).filter(
      (f) => f.startsWith("agent-") && f.endsWith(".jsonl"),
    );
  } catch {
    return [];
  }

  // Sort for deterministic output
  files.sort();

  const result: SummaryLine[] = [];

  for (const file of files) {
    const jsonlPath = join(subagentDir, file);
    const agentId = basename(file, ".jsonl");
    const description = readSubagentDescription(subagentDir, agentId);

    const lines = extractFromJsonl(jsonlPath, since);
    if (lines.length === 0) continue;

    // Insert marker line with the timestamp of the first subagent entry
    // (or a minimal timestamp if none). The marker itself uses an empty
    // timestamp so it sorts with its entries.
    const markerTimestamp = lines[0].timestamp;
    result.push({
      timestamp: markerTimestamp,
      text: `\n--- Subagent: ${description} ---`,
    });
    result.push(...lines);
  }

  return result;
}

/**
 * Read subagent description from the .meta.json file.
 * Falls back to agentType, then to the agentId.
 */
function readSubagentDescription(subagentDir: string, agentId: string): string {
  const metaPath = join(subagentDir, `${agentId}.meta.json`);

  if (!existsSync(metaPath)) {
    return agentId;
  }

  try {
    const raw = readFileSync(metaPath, "utf-8");
    const meta = JSON.parse(raw) as Record<string, unknown>;
    if (typeof meta.description === "string" && meta.description) {
      return meta.description;
    }
    if (typeof meta.agentType === "string" && meta.agentType) {
      return meta.agentType;
    }
  } catch {
    // Malformed meta.json — fall through to agentId
  }

  return agentId;
}

// ─────────────────────────────────────────────────────────────────────────────
// Summary Building
// ─────────────────────────────────────────────────────────────────────────────

/** Result from buildSummary — includes the text and the watermark-safe timestamp */
interface BuildSummaryResult {
  /** The summary text to write to disk */
  text: string;
  /** Latest timestamp from lines that survived truncation (null if empty) */
  latestTimestamp: string | null;
}

/**
 * Build the final summary text from main and subagent lines.
 * Enforces 100KB cap by dropping oldest entries first.
 * Returns both the text and the latest timestamp from surviving lines,
 * so the watermark only advances past content the librarian actually sees.
 */
function buildSummary(
  mainLines: SummaryLine[],
  subagentLines: SummaryLine[],
): BuildSummaryResult {
  // Combine: main conversation first, then subagent sections
  const allLines = [...mainLines, ...subagentLines];
  const allText = allLines.map((l) => l.text).join("\n");

  const byteSize = Buffer.byteLength(allText, "utf-8");
  if (byteSize <= MAX_SUMMARY_BYTES) {
    return {
      text: allText + "\n",
      latestTimestamp: findLatestTimestamp(allLines),
    };
  }

  // Over the cap — drop oldest main entries first, keeping subagent sections intact
  process.stderr.write(
    `Warning: summary exceeds 100KB (${Math.round(byteSize / 1024)}KB), truncating oldest entries\n`,
  );

  return truncateToFit(mainLines, subagentLines);
}

/**
 * Truncate the summary to fit within MAX_SUMMARY_BYTES.
 * Strategy: drop oldest main entries first (they're least likely to contain
 * unlogged decisions). If still over, drop oldest subagent entries.
 * Returns both the text and the latest timestamp from surviving lines.
 */
function truncateToFit(
  mainLines: SummaryLine[],
  subagentLines: SummaryLine[],
): BuildSummaryResult {
  // Start by trying with all subagent lines and progressively fewer main lines
  const subagentText = subagentLines.map((l) => l.text).join("\n");
  const subagentSize = Buffer.byteLength(subagentText, "utf-8");

  // If subagent content alone exceeds the cap, truncate subagent lines too
  if (subagentSize > MAX_SUMMARY_BYTES) {
    const allCombined = [...mainLines, ...subagentLines];
    const surviving = truncateLines(allCombined);
    return {
      text: surviving.text + "\n",
      latestTimestamp: surviving.latestTimestamp,
    };
  }

  // Budget for main lines = total cap minus subagent content minus newline
  const mainBudget = MAX_SUMMARY_BYTES - subagentSize - (subagentLines.length > 0 ? 1 : 0);

  // Drop oldest main entries until we fit
  let startIdx = 0;
  while (startIdx < mainLines.length) {
    const candidateText = mainLines
      .slice(startIdx)
      .map((l) => l.text)
      .join("\n");
    const candidateSize = Buffer.byteLength(candidateText, "utf-8");

    if (candidateSize <= mainBudget) {
      const survivingMain = mainLines.slice(startIdx);
      const survivingAll = [...survivingMain, ...subagentLines];
      const parts: string[] = [];
      if (startIdx < mainLines.length) {
        parts.push(candidateText);
      }
      if (subagentLines.length > 0) {
        parts.push(subagentText);
      }
      return {
        text: parts.join("\n") + "\n",
        latestTimestamp: findLatestTimestamp(survivingAll),
      };
    }

    startIdx++;
  }

  // Edge case: even with no main lines, subagent content fits
  return {
    text: subagentText + "\n",
    latestTimestamp: findLatestTimestamp(subagentLines),
  };
}

/**
 * Simple line-by-line truncation: drop from the beginning until under budget.
 * Returns both the surviving text and the latest timestamp from surviving lines.
 */
function truncateLines(lines: SummaryLine[]): { text: string; latestTimestamp: string | null } {
  let startIdx = 0;
  while (startIdx < lines.length) {
    const surviving = lines.slice(startIdx);
    const text = surviving.map((l) => l.text).join("\n");
    if (Buffer.byteLength(text + "\n", "utf-8") <= MAX_SUMMARY_BYTES) {
      return { text, latestTimestamp: findLatestTimestamp(surviving) };
    }
    startIdx++;
  }
  return { text: "", latestTimestamp: null };
}

// ─────────────────────────────────────────────────────────────────────────────
// Utilities
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Find the latest ISO timestamp across all summary lines.
 */
function findLatestTimestamp(lines: SummaryLine[]): string | null {
  let latest: string | null = null;
  for (const line of lines) {
    if (!line.timestamp) continue;
    if (!latest || line.timestamp > latest) {
      latest = line.timestamp;
    }
  }
  return latest;
}

/**
 * Convert an ISO 8601 timestamp to "YYYY-MM-DD HH:MM" format.
 * Uses UTC to avoid timezone-dependent test failures.
 */
function formatTimestamp(iso: string): string {
  const d = new Date(iso);
  const year = d.getUTCFullYear();
  const month = String(d.getUTCMonth() + 1).padStart(2, "0");
  const day = String(d.getUTCDate()).padStart(2, "0");
  const hours = String(d.getUTCHours()).padStart(2, "0");
  const minutes = String(d.getUTCMinutes()).padStart(2, "0");
  return `${year}-${month}-${day} ${hours}:${minutes}`;
}
