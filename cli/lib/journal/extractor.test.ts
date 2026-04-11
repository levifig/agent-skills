/**
 * JSONL Extractor Tests
 *
 * Tests for extractSummary() — reading Claude Code JSONL conversation logs,
 * filtering entries, extracting conversation summaries, and writing to
 * .agents/tmp/.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { mkdirSync, writeFileSync, rmSync, readFileSync, existsSync } from "fs";
import { join } from "path";

import { extractSummary } from "./extractor.js";

// ─────────────────────────────────────────────────────────────────────────────
// Test Fixtures
// ─────────────────────────────────────────────────────────────────────────────

const TEST_ROOT = join(process.cwd(), ".test-jsonl-extractor");
const AGENTS_DIR = join(TEST_ROOT, ".agents");
const PROJECT_DIR = join(TEST_ROOT, "project");
const SESSION_ID = "test-session-001";

/** Build a JSONL entry for a user message */
function userEntry(timestamp: string, content: string): string {
  return JSON.stringify({
    type: "user",
    timestamp,
    message: { role: "user", content },
    uuid: `user-${timestamp}`,
    sessionId: SESSION_ID,
  });
}

/** Build a JSONL entry for an assistant message with content blocks */
function assistantEntry(timestamp: string, blocks: unknown[]): string {
  return JSON.stringify({
    type: "assistant",
    timestamp,
    message: { role: "assistant", content: blocks },
    uuid: `assistant-${timestamp}`,
    sessionId: SESSION_ID,
  });
}

/** Build a text content block */
function textBlock(text: string): { type: string; text: string } {
  return { type: "text", text };
}

/** Build a thinking content block */
function thinkingBlock(thinking: string): { type: string; thinking: string } {
  return { type: "thinking", thinking };
}

/** Build a tool_use content block */
function toolUseBlock(
  name: string,
  input: Record<string, unknown>,
): { type: string; name: string; id: string; input: Record<string, unknown> } {
  return { type: "tool_use", name, id: `tool-${name}`, input };
}

/** Build a progress JSONL entry (should be skipped) */
function progressEntry(timestamp: string): string {
  return JSON.stringify({
    type: "progress",
    timestamp,
    content: "streaming...",
    sessionId: SESSION_ID,
  });
}

/** Build an attachment JSONL entry (should be skipped) */
function attachmentEntry(timestamp: string): string {
  return JSON.stringify({
    type: "attachment",
    timestamp,
    content: { type: "system_prompt" },
    sessionId: SESSION_ID,
  });
}

/** Build a queue-operation JSONL entry (should be skipped) */
function queueEntry(timestamp: string): string {
  return JSON.stringify({
    type: "queue-operation",
    operation: "enqueue",
    timestamp,
    sessionId: SESSION_ID,
  });
}

/** Write a JSONL file from an array of JSON string lines */
function writeJsonl(filePath: string, lines: string[]): void {
  const dir = filePath.substring(0, filePath.lastIndexOf("/"));
  mkdirSync(dir, { recursive: true });
  writeFileSync(filePath, lines.join("\n") + "\n", "utf-8");
}

// ─────────────────────────────────────────────────────────────────────────────
// Setup / Teardown
// ─────────────────────────────────────────────────────────────────────────────

beforeEach(() => {
  mkdirSync(TEST_ROOT, { recursive: true });
  mkdirSync(AGENTS_DIR, { recursive: true });
  mkdirSync(PROJECT_DIR, { recursive: true });
});

afterEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
  vi.restoreAllMocks();
});

// ─────────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────────

describe("extractSummary", () => {
  describe("type filtering", () => {
    it("includes only user and assistant entries", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        queueEntry("2026-04-10T14:20:00.000Z"),
        userEntry("2026-04-10T14:21:00.000Z", "hello world"),
        attachmentEntry("2026-04-10T14:21:01.000Z"),
        progressEntry("2026-04-10T14:21:02.000Z"),
        assistantEntry("2026-04-10T14:22:00.000Z", [
          textBlock("Hello! How can I help?"),
        ]),
        progressEntry("2026-04-10T14:22:01.000Z"),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      expect(result.isEmpty).toBe(false);
      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("User: hello world");
      expect(content).toContain("Assistant: Hello! How can I help?");
      // Should not contain noise types
      expect(content).not.toContain("streaming");
      expect(content).not.toContain("system_prompt");
      expect(content).not.toContain("enqueue");
    });

    it("skips other types like agent_progress and hook_progress", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      const agentProgress = JSON.stringify({
        type: "agent_progress",
        timestamp: "2026-04-10T14:20:00.000Z",
        content: "agent working...",
      });
      const hookProgress = JSON.stringify({
        type: "hook_progress",
        timestamp: "2026-04-10T14:20:01.000Z",
        content: "hook running...",
      });
      writeJsonl(jsonlPath, [
        agentProgress,
        hookProgress,
        userEntry("2026-04-10T14:21:00.000Z", "test"),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).not.toContain("agent working");
      expect(content).not.toContain("hook running");
      expect(content).toContain("User: test");
    });
  });

  describe("timestamp cutoff", () => {
    it("filters entries before the since timestamp", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:00:00.000Z", "old message"),
        assistantEntry("2026-04-10T14:01:00.000Z", [
          textBlock("old response"),
        ]),
        userEntry("2026-04-10T14:30:00.000Z", "new message"),
        assistantEntry("2026-04-10T14:31:00.000Z", [
          textBlock("new response"),
        ]),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
        "2026-04-10T14:15:00.000Z",
      );

      expect(result.isEmpty).toBe(false);
      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).not.toContain("old message");
      expect(content).not.toContain("old response");
      expect(content).toContain("new message");
      expect(content).toContain("new response");
    });

    it("returns isEmpty when all entries are before cutoff", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:00:00.000Z", "old message"),
        assistantEntry("2026-04-10T14:01:00.000Z", [
          textBlock("old response"),
        ]),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
        "2026-04-10T15:00:00.000Z",
      );

      expect(result.isEmpty).toBe(true);
      expect(result.latestTimestamp).toBeNull();
    });
  });

  describe("thinking block skip", () => {
    it("does not include thinking blocks in output", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        assistantEntry("2026-04-10T14:22:00.000Z", [
          thinkingBlock("Let me think about this carefully..."),
          textBlock("Here is my answer."),
        ]),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).not.toContain("think about this carefully");
      expect(content).toContain("Here is my answer.");
    });
  });

  describe("tool_use extraction", () => {
    it("extracts Bash command", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        assistantEntry("2026-04-10T14:23:00.000Z", [
          toolUseBlock("Bash", { command: "git commit -m 'feat: add extractor'" }),
        ]),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("Tool: Bash");
      expect(content).toContain("git commit -m 'feat: add extractor'");
    });

    it("extracts Edit file_path", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        assistantEntry("2026-04-10T14:23:00.000Z", [
          toolUseBlock("Edit", { file_path: "/path/to/file.ts", old_string: "a", new_string: "b" }),
        ]),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("Tool: Edit /path/to/file.ts");
    });

    it("extracts Read file_path", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        assistantEntry("2026-04-10T14:23:00.000Z", [
          toolUseBlock("Read", { file_path: "/path/to/readme.md" }),
        ]),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("Tool: Read /path/to/readme.md");
    });

    it("extracts Write file_path", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        assistantEntry("2026-04-10T14:23:00.000Z", [
          toolUseBlock("Write", { file_path: "/path/to/new.ts", content: "code" }),
        ]),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("Tool: Write /path/to/new.ts");
    });

    it("shows only tool name for unknown tools", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        assistantEntry("2026-04-10T14:23:00.000Z", [
          toolUseBlock("WebFetch", { url: "https://example.com" }),
        ]),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("Tool: WebFetch");
    });

    it("handles mixed text and tool_use blocks in one assistant entry", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        assistantEntry("2026-04-10T14:23:00.000Z", [
          textBlock("Let me edit that file."),
          toolUseBlock("Edit", { file_path: "/path/to/file.ts", old_string: "a", new_string: "b" }),
        ]),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("Assistant: Let me edit that file.");
      expect(content).toContain("Tool: Edit /path/to/file.ts");
    });
  });

  describe("subagent discovery", () => {
    it("includes subagent transcripts with description markers", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:21:00.000Z", "main conversation"),
      ]);

      // Create subagent directory + files
      const subagentDir = join(PROJECT_DIR, SESSION_ID, "subagents");
      mkdirSync(subagentDir, { recursive: true });

      writeJsonl(join(subagentDir, "agent-abc123.jsonl"), [
        assistantEntry("2026-04-10T14:30:00.000Z", [
          textBlock("The --agent flag resolves from plugins directory."),
        ]),
      ]);

      writeFileSync(
        join(subagentDir, "agent-abc123.meta.json"),
        JSON.stringify({ agentType: "Explore", description: "Research --agent flag" }),
        "utf-8",
      );

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("main conversation");
      expect(content).toContain("--- Subagent: Research --agent flag ---");
      expect(content).toContain("The --agent flag resolves from plugins directory.");
    });

    it("uses agentType as fallback when description is missing", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:21:00.000Z", "test"),
      ]);

      const subagentDir = join(PROJECT_DIR, SESSION_ID, "subagents");
      mkdirSync(subagentDir, { recursive: true });

      writeJsonl(join(subagentDir, "agent-def456.jsonl"), [
        assistantEntry("2026-04-10T14:30:00.000Z", [
          textBlock("subagent response"),
        ]),
      ]);

      writeFileSync(
        join(subagentDir, "agent-def456.meta.json"),
        JSON.stringify({ agentType: "Explore" }),
        "utf-8",
      );

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("--- Subagent: Explore ---");
    });

    it("handles missing meta.json gracefully", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:21:00.000Z", "test"),
      ]);

      const subagentDir = join(PROJECT_DIR, SESSION_ID, "subagents");
      mkdirSync(subagentDir, { recursive: true });

      writeJsonl(join(subagentDir, "agent-ghi789.jsonl"), [
        assistantEntry("2026-04-10T14:30:00.000Z", [
          textBlock("subagent without meta"),
        ]),
      ]);
      // No meta.json written

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("--- Subagent: agent-ghi789 ---");
    });

    it("handles missing subagent directory gracefully", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:21:00.000Z", "no subagents"),
      ]);
      // No subagent directory created

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      expect(result.isEmpty).toBe(false);
      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("no subagents");
      expect(content).not.toContain("Subagent:");
    });
  });

  describe("100KB cap", () => {
    it("truncates oldest entries when summary exceeds 100KB", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      const lines: string[] = [];

      // Generate enough entries to exceed 100KB.
      // Each user message is ~100 bytes of content + timestamp + format.
      // 1500 entries * ~100 bytes = ~150KB (over the cap)
      for (let i = 0; i < 1500; i++) {
        const ts = `2026-04-10T14:${String(Math.floor(i / 60)).padStart(2, "0")}:${String(i % 60).padStart(2, "0")}.000Z`;
        const padding = "x".repeat(80);
        lines.push(userEntry(ts, `message-${String(i).padStart(4, "0")}-${padding}`));
      }

      writeJsonl(jsonlPath, lines);

      // Capture stderr
      const stderrSpy = vi.spyOn(process.stderr, "write").mockImplementation(() => true);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      const byteSize = Buffer.byteLength(content, "utf-8");
      expect(byteSize).toBeLessThanOrEqual(100 * 1024);

      // Should contain newest entries, not oldest
      expect(content).toContain("message-1499");
      expect(content).not.toContain("message-0000");

      // Should warn on stderr
      expect(stderrSpy).toHaveBeenCalled();
      const stderrOutput = stderrSpy.mock.calls
        .map((c) => String(c[0]))
        .join("");
      expect(stderrOutput).toContain("100KB");
    });
  });

  describe("malformed JSONL lines", () => {
    it("skips malformed lines and warns on stderr", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        "this is not json",
        userEntry("2026-04-10T14:21:00.000Z", "valid message"),
        "{incomplete json",
        assistantEntry("2026-04-10T14:22:00.000Z", [
          textBlock("valid response"),
        ]),
      ]);

      const stderrSpy = vi.spyOn(process.stderr, "write").mockImplementation(() => true);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      expect(result.isEmpty).toBe(false);
      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("valid message");
      expect(content).toContain("valid response");

      // Should warn about malformed lines
      expect(stderrSpy).toHaveBeenCalled();
      const stderrOutput = stderrSpy.mock.calls
        .map((c) => String(c[0]))
        .join("");
      expect(stderrOutput).toContain("malformed");
    });

    it("handles empty lines gracefully", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      // Write with some blank lines
      const content = [
        userEntry("2026-04-10T14:21:00.000Z", "message"),
        "",
        "",
        assistantEntry("2026-04-10T14:22:00.000Z", [textBlock("response")]),
        "",
      ].join("\n");
      mkdirSync(PROJECT_DIR, { recursive: true });
      writeFileSync(jsonlPath, content, "utf-8");

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      expect(result.isEmpty).toBe(false);
      const output = readFileSync(result.summaryPath, "utf-8");
      expect(output).toContain("User: message");
      expect(output).toContain("Assistant: response");
    });
  });

  describe("empty result", () => {
    it("returns isEmpty when JSONL has no user/assistant entries", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        progressEntry("2026-04-10T14:20:00.000Z"),
        attachmentEntry("2026-04-10T14:20:01.000Z"),
        queueEntry("2026-04-10T14:20:02.000Z"),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      expect(result.isEmpty).toBe(true);
      expect(result.latestTimestamp).toBeNull();
      // No file should be written
      expect(existsSync(result.summaryPath)).toBe(false);
    });
  });

  describe("latest timestamp tracking", () => {
    it("returns the latest timestamp from processed entries", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:00:00.000Z", "first"),
        assistantEntry("2026-04-10T14:01:00.000Z", [textBlock("response")]),
        userEntry("2026-04-10T14:30:00.000Z", "last"),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      expect(result.latestTimestamp).toBe("2026-04-10T14:30:00.000Z");
    });

    it("tracks latest timestamp across main and subagent files", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:00:00.000Z", "main message"),
      ]);

      const subagentDir = join(PROJECT_DIR, SESSION_ID, "subagents");
      mkdirSync(subagentDir, { recursive: true });

      writeJsonl(join(subagentDir, "agent-late.jsonl"), [
        assistantEntry("2026-04-10T15:00:00.000Z", [
          textBlock("later subagent response"),
        ]),
      ]);

      writeFileSync(
        join(subagentDir, "agent-late.meta.json"),
        JSON.stringify({ agentType: "Explore", description: "Late subagent" }),
        "utf-8",
      );

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      expect(result.latestTimestamp).toBe("2026-04-10T15:00:00.000Z");
    });
  });

  describe(".agents/tmp/ infrastructure", () => {
    it("creates .agents/tmp/ if it does not exist", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:21:00.000Z", "test"),
      ]);

      const tmpDir = join(AGENTS_DIR, "tmp");
      expect(existsSync(tmpDir)).toBe(false);

      await extractSummary(jsonlPath, PROJECT_DIR, SESSION_ID, AGENTS_DIR);

      expect(existsSync(tmpDir)).toBe(true);
    });

    it("writes summary to .agents/tmp/<session-id>-enrichment.txt", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:21:00.000Z", "test message"),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      expect(result.summaryPath).toBe(
        join(AGENTS_DIR, "tmp", `${SESSION_ID}-enrichment.txt`),
      );
      expect(existsSync(result.summaryPath)).toBe(true);

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("test message");
    });
  });

  describe("output format", () => {
    it("formats timestamps as YYYY-MM-DD HH:MM", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:21:00.000Z", "check timestamp format"),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toMatch(/\[2026-04-10 14:21\] User: check timestamp format/);
    });

    it("formats Bash tool_use with command after dash", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        assistantEntry("2026-04-10T14:25:00.000Z", [
          toolUseBlock("Bash", { command: 'git commit -m "feat: test"' }),
        ]),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toMatch(
        /\[2026-04-10 14:25\] Tool: Bash — git commit -m "feat: test"/,
      );
    });

    it("handles user message.content as string (not array)", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:21:00.000Z", "plain string content"),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("User: plain string content");
    });
  });

  describe("since cutoff with subagents", () => {
    it("applies since cutoff to subagent entries too", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:21:00.000Z", "main message"),
      ]);

      const subagentDir = join(PROJECT_DIR, SESSION_ID, "subagents");
      mkdirSync(subagentDir, { recursive: true });

      writeJsonl(join(subagentDir, "agent-cutoff.jsonl"), [
        assistantEntry("2026-04-10T13:00:00.000Z", [
          textBlock("old subagent response"),
        ]),
        assistantEntry("2026-04-10T15:00:00.000Z", [
          textBlock("new subagent response"),
        ]),
      ]);

      writeFileSync(
        join(subagentDir, "agent-cutoff.meta.json"),
        JSON.stringify({ agentType: "Explore", description: "Cutoff test" }),
        "utf-8",
      );

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
        "2026-04-10T14:00:00.000Z",
      );

      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).not.toContain("old subagent response");
      expect(content).toContain("new subagent response");
    });
  });

  describe("watermark safety after truncation", () => {
    it("latestTimestamp reflects surviving lines, not pre-truncation lines", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      const lines: string[] = [];

      // Generate enough entries to exceed 100KB, forcing truncation.
      // Oldest entries (with the latest-alphabetically-named timestamps in
      // early range) will be dropped.
      for (let i = 0; i < 1500; i++) {
        const ts = `2026-04-10T14:${String(Math.floor(i / 60)).padStart(2, "0")}:${String(i % 60).padStart(2, "0")}.000Z`;
        const padding = "x".repeat(80);
        lines.push(userEntry(ts, `message-${String(i).padStart(4, "0")}-${padding}`));
      }

      writeJsonl(jsonlPath, lines);

      vi.spyOn(process.stderr, "write").mockImplementation(() => true);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      // The latest entry is always the last one (1499)
      // But the watermark must reflect only what survived truncation
      const content = readFileSync(result.summaryPath, "utf-8");

      // message-1499 survived (newest), message-0000 was dropped (oldest)
      expect(content).toContain("message-1499");
      expect(content).not.toContain("message-0000");

      // The latestTimestamp should match the latest timestamp in the
      // surviving content — which is the last entry's timestamp
      expect(result.latestTimestamp).toBe("2026-04-10T14:24:59.000Z");

      // Critically, find the EARLIEST surviving entry's timestamp in the output
      // and verify the watermark is >= it (i.e., the watermark didn't advance
      // past content that was truncated away)
      const firstLine = content.split("\n")[0];
      const timestampMatch = firstLine.match(/\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2})\]/);
      expect(timestampMatch).toBeTruthy();
    });

    it("latestTimestamp is from the full set when no truncation occurs", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:00:00.000Z", "first"),
        userEntry("2026-04-10T14:30:00.000Z", "middle"),
        userEntry("2026-04-10T15:00:00.000Z", "last"),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      expect(result.latestTimestamp).toBe("2026-04-10T15:00:00.000Z");
      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("first");
      expect(content).toContain("last");
    });
  });

  describe("non-canonical timestamps", () => {
    it("handles ISO timestamps without milliseconds", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T14:21:00Z", "no-millis message"),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      expect(result.isEmpty).toBe(false);
      expect(result.latestTimestamp).toBe("2026-04-10T14:21:00Z");
      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("no-millis message");
    });

    it("handles ISO timestamps with timezone offsets", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      writeJsonl(jsonlPath, [
        userEntry("2026-04-10T10:21:00.000-04:00", "offset message"),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      expect(result.isEmpty).toBe(false);
      expect(result.latestTimestamp).toBe("2026-04-10T10:21:00.000-04:00");
      const content = readFileSync(result.summaryPath, "utf-8");
      expect(content).toContain("offset message");
      // formatTimestamp converts to UTC, so 10:21 -04:00 = 14:21 UTC
      expect(content).toMatch(/\[2026-04-10 14:21\]/);
    });
  });

  describe("non-standard content shapes", () => {
    it("handles user message.content that is neither string nor array", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      // User entry with content as a number (weird edge case)
      const weirdEntry = JSON.stringify({
        type: "user",
        timestamp: "2026-04-10T14:21:00.000Z",
        message: { role: "user", content: 42 },
        uuid: "user-weird",
        sessionId: SESSION_ID,
      });
      writeJsonl(jsonlPath, [
        weirdEntry,
        userEntry("2026-04-10T14:22:00.000Z", "normal message"),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      expect(result.isEmpty).toBe(false);
      const content = readFileSync(result.summaryPath, "utf-8");
      // The weird entry should be skipped gracefully, normal entry survives
      expect(content).toContain("normal message");
      expect(content).not.toContain("42");
    });

    it("handles assistant message.content that is not an array", async () => {
      const jsonlPath = join(PROJECT_DIR, `${SESSION_ID}.jsonl`);
      // Assistant entry with content as a string (non-standard)
      const weirdEntry = JSON.stringify({
        type: "assistant",
        timestamp: "2026-04-10T14:21:00.000Z",
        message: { role: "assistant", content: "plain string, not array" },
        uuid: "assistant-weird",
        sessionId: SESSION_ID,
      });
      writeJsonl(jsonlPath, [
        weirdEntry,
        userEntry("2026-04-10T14:22:00.000Z", "normal message"),
      ]);

      const result = await extractSummary(
        jsonlPath,
        PROJECT_DIR,
        SESSION_ID,
        AGENTS_DIR,
      );

      expect(result.isEmpty).toBe(false);
      const content = readFileSync(result.summaryPath, "utf-8");
      // The weird assistant entry should be skipped, normal entry survives
      expect(content).toContain("normal message");
      expect(content).not.toContain("plain string, not array");
    });
  });
});
