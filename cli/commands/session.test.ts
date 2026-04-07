/**
 * Session Command Integration Tests
 *
 * Black-box tests for the loaf session command using temp git repos.
 * Tests CLI behavior via child_process.spawn/execFile.
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { spawn } from "child_process";
import { createHash } from "crypto";
import {
  existsSync,
  mkdirSync,
  readdirSync,
  rmSync,
  writeFileSync,
  readFileSync,
} from "fs";
import { join } from "path";

// ─────────────────────────────────────────────────────────────────────────────
// Test Fixtures
// ─────────────────────────────────────────────────────────────────────────────

const TEST_ROOT = join(process.cwd(), ".test-session-command");
const CLI_PATH = join(process.cwd(), "dist-cli/index.js");

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

async function runLoaf(
  args: string[],
  options: { cwd: string; input?: string } = { cwd: TEST_ROOT }
): Promise<{ stdout: string; stderr: string; exitCode: number }> {
  return new Promise((resolve) => {
    const child = spawn("node", [CLI_PATH, "session", ...args], {
      cwd: options.cwd,
      stdio: ["pipe", "pipe", "pipe"],
    });

    let stdout = "";
    let stderr = "";

    child.stdout.on("data", (data) => {
      stdout += data.toString();
    });

    child.stderr.on("data", (data) => {
      stderr += data.toString();
    });

    if (options.input) {
      child.stdin.write(options.input);
      child.stdin.end();
    }

    child.on("close", (exitCode) => {
      resolve({ stdout, stderr, exitCode: exitCode ?? 0 });
    });
  });
}

function createTempRepo(name: string): string {
  const repoPath = join(TEST_ROOT, name);
  mkdirSync(repoPath, { recursive: true });
  
  // Initialize git repo
  execFileSync("git", ["init"], { cwd: repoPath });
  execFileSync("git", ["config", "user.email", "test@test.com"], { cwd: repoPath });
  execFileSync("git", ["config", "user.name", "Test User"], { cwd: repoPath });
  
  // Create initial commit
  writeFileSync(join(repoPath, "README.md"), "# Test\n", "utf-8");
  execFileSync("git", ["add", "."], { cwd: repoPath });
  execFileSync("git", ["commit", "-m", "Initial commit"], { cwd: repoPath });
  
  // Create .agents directory
  mkdirSync(join(repoPath, ".agents"), { recursive: true });
  writeFileSync(
    join(repoPath, ".agents/AGENTS.md"),
    "# Project Instructions\n",
    "utf-8"
  );
  
  return repoPath;
}

function execFileSync(cmd: string, args: string[], options: { cwd: string }) {
  require("child_process").execFileSync(cmd, args, { ...options, stdio: "pipe" });
}

function getSessionFiles(repoPath: string): string[] {
  const sessionsDir = join(repoPath, ".agents/sessions");
  if (!existsSync(sessionsDir)) return [];
  return readdirSync(sessionsDir).filter(f => f.endsWith(".md") && !f.startsWith("."));
}

function writeKnowledgeFile(repoPath: string, fileName: string, daysOld: number) {
  const knowledgeDir = join(repoPath, "docs/knowledge");
  mkdirSync(knowledgeDir, { recursive: true });

  const reviewedAt = new Date(Date.now() - daysOld * 24 * 60 * 60 * 1000)
    .toISOString()
    .slice(0, 10);

  writeFileSync(
    join(knowledgeDir, fileName),
    `---
topics:
  - session
last_reviewed: ${reviewedAt}
covers:
  - README.md
---

# Knowledge
`,
    "utf-8"
  );
}

function getKnowledgeNudgeFile(repoPath: string): string {
  const branch = require("child_process")
    .execFileSync("git", ["branch", "--show-current"], { cwd: repoPath, encoding: "utf-8" })
    .trim();
  const hash = createHash("md5").update(`${repoPath}:${branch}`).digest("hex").slice(0, 8);
  return `/tmp/loaf-kb-nudged-${hash}`;
}

// ─────────────────────────────────────────────────────────────────────────────
// Setup / Teardown
// ─────────────────────────────────────────────────────────────────────────────

beforeEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
  mkdirSync(TEST_ROOT, { recursive: true });
});

afterEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
});

// ─────────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────────

describe("session: start", () => {
  it("creates an ad-hoc session in a fresh repo with .agents/", async () => {
    const repoPath = createTempRepo("adhoc-test");
    
    const result = await runLoaf(["start"], { cwd: repoPath });
    
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("Creating new session file");
    
    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);
    
    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );
    // New compact inline format (SPEC-020)
    expect(content).toContain("## Journal");
    expect(content).toContain("session(start):  === SESSION STARTED ===");
    expect(content).toContain("status: active");
  });

  it("uses detached-<sha> branch key when in detached HEAD", async () => {
    const repoPath = createTempRepo("detached-test");
    
    // Checkout detached state
    const commitSha = require("child_process")
      .execFileSync("git", ["rev-parse", "--short", "HEAD"], { cwd: repoPath, encoding: "utf-8" })
      .trim();
    require("child_process").execFileSync("git", ["checkout", "--detach"], { cwd: repoPath });
    
    const result = await runLoaf(["start"], { cwd: repoPath });
    
    expect(result.exitCode).toBe(0);
    expect(result.stderr).toContain(`detached-${commitSha}`);
    
    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);
    
    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );
    expect(content).toContain(`branch: detached-${commitSha}`);
  });

  it.skip("two concurrent session start processes leave exactly one session file", async () => {
    const repoPath = createTempRepo("concurrent-test");
    
    // Start two session start processes simultaneously
    const [result1, result2] = await Promise.all([
      runLoaf(["start"], { cwd: repoPath }),
      runLoaf(["start"], { cwd: repoPath }),
    ]);
    
    // Both should exit 0
    expect(result1.exitCode).toBe(0);
    expect(result2.exitCode).toBe(0);
    
    // Only one session file should exist
    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);
    
    // Verify there's only ONE session start entry (not duplicate starts)
    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );
    const startMatches = content.match(/SESSION STARTED/g);
    expect(startMatches?.length ?? 0).toBe(1);
  }, 30000); // Higher timeout for concurrent operations

  it("starting after pause archives old session and creates new one", async () => {
    const repoPath = createTempRepo("new-after-pause-test");

    // Start session
    const startResult = await runLoaf(["start"], { cwd: repoPath });
    expect(startResult.exitCode).toBe(0);

    // End session (pauses it)
    const endResult = await runLoaf(["end"], { cwd: repoPath });
    expect(endResult.exitCode).toBe(0);

    // Start again — should archive old and create new
    const newResult = await runLoaf(["start"], { cwd: repoPath });
    expect(newResult.exitCode).toBe(0);
    expect(newResult.stdout).toContain("Closed previous session");
    expect(newResult.stdout).toContain("Creating new session file");

    // Only one active session file (old one archived)
    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);

    // Old session should be in archive
    const archiveDir = join(repoPath, ".agents/sessions/archive");
    expect(existsSync(archiveDir)).toBe(true);
    const archiveFiles = readdirSync(archiveDir).filter((f: string) => f.endsWith(".md"));
    expect(archiveFiles.length).toBe(1);

    // New session should have fresh start entry
    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );
    expect(content).toContain("SESSION STARTED");
    expect(content).toContain("status: active");
  });

  it("--resume flag resumes a paused session instead of creating new", async () => {
    const repoPath = createTempRepo("resume-flag-test");

    // Start session
    await runLoaf(["start"], { cwd: repoPath });

    // End session (pauses it)
    await runLoaf(["end"], { cwd: repoPath });

    // Resume with --resume flag
    const resumeResult = await runLoaf(["start", "--resume"], { cwd: repoPath });
    expect(resumeResult.exitCode).toBe(0);
    expect(resumeResult.stdout).toContain("Resuming existing session");

    // Still only one session file (same one, resumed)
    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);

    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );

    // Should have resume entry and still be active
    expect(content).toContain("SESSION RESUMED");
    expect(content).toContain("status: active");

    // No archive directory should exist (session was resumed, not archived)
    const archiveDir = join(repoPath, ".agents/sessions/archive");
    expect(existsSync(archiveDir)).toBe(false);
  });

  it("surfaces stale knowledge count from configured knowledge files", async () => {
    const repoPath = createTempRepo("stale-kb-test");

    writeKnowledgeFile(repoPath, "stale.md", 45);

    const result = await runLoaf(["start"], { cwd: repoPath });

    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("1 stale knowledge file");
    expect(result.stderr).not.toContain("KB directory not found");
  });

  it("does not emit KB directory warnings when no knowledge roots exist", async () => {
    const repoPath = createTempRepo("no-kb-test");

    const result = await runLoaf(["start"], { cwd: repoPath });

    expect(result.exitCode).toBe(0);
    expect(result.stderr).not.toContain("KB directory not found");
  });
});

describe("session: log", () => {
  it("hook-safe exit when no active session exists", async () => {
    const repoPath = createTempRepo("no-session-hook-test");
    
    // Try to log from hook without starting a session first
    // Should exit 0 (hook-safe) not 1 (error)
    const result = await runLoaf(["log", "--from-hook"], { 
      cwd: repoPath,
      input: JSON.stringify({ tool_input: { command: "git commit -m 'test commit'" } })
    });
    
    // Should exit 0 (no-op for hooks) rather than failing
    expect(result.exitCode).toBe(0);
  }, 10000);

  it("accepts nested tool.input format from hooks", async () => {
    const repoPath = createTempRepo("nested-payload-test");

    // Start a session
    await runLoaf(["start"], { cwd: repoPath });

    // Make an actual commit so git log returns the right message
    writeFileSync(join(repoPath, "feature.ts"), "export const x = 1;\n", "utf-8");
    execFileSync("git", ["add", "."], { cwd: repoPath });
    execFileSync("git", ["commit", "-m", "feat: add feature"], { cwd: repoPath });

    // Log with nested format (tool.input instead of tool_input)
    const result = await runLoaf(["log", "--from-hook"], {
      cwd: repoPath,
      input: JSON.stringify({
        tool: {
          name: "Bash",
          input: { command: "git commit -m 'feat: add feature'" }
        }
      })
    });

    expect(result.exitCode).toBe(0);

    // Verify the commit was logged
    const sessionFiles = getSessionFiles(repoPath);
    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );

    expect(content).toContain("commit(");
    expect(content).toContain("feat: add feature");
  }, 10000);

  it("two concurrent session log processes both preserve their entries", async () => {
    const repoPath = createTempRepo("concurrent-log-test");
    
    // Start a session first
    await runLoaf(["start"], { cwd: repoPath });
    
    // Two concurrent log entries
    const [result1, result2] = await Promise.all([
      runLoaf(["log", "discover(test): finding one"], { cwd: repoPath }),
      runLoaf(["log", "discover(test): finding two"], { cwd: repoPath }),
    ]);
    
    // Both should succeed
    expect(result1.exitCode).toBe(0);
    expect(result2.exitCode).toBe(0);
    
    // Both entries should be in the session file
    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);
    
    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );
    
    expect(content).toContain("discover(test): finding one");
    expect(content).toContain("discover(test): finding two");
  }, 30000);
});

describe("session: list", () => {
  it("shows active sessions", async () => {
    const repoPath = createTempRepo("list-test");
    
    await runLoaf(["start"], { cwd: repoPath });
    
    const result = await runLoaf(["list"], { cwd: repoPath });
    
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("Active Sessions");
    expect(result.stdout).toContain("1 active");
  });

  it("shows active and archived sessions with --all", async () => {
    const repoPath = createTempRepo("list-all-test");
    
    // Create and archive a session
    await runLoaf(["start"], { cwd: repoPath });
    await runLoaf(["archive"], { cwd: repoPath });
    
    // Create another active session
    // Switch to new branch
    require("child_process").execFileSync("git", ["checkout", "-b", "feature-branch"], { cwd: repoPath });
    await runLoaf(["start"], { cwd: repoPath });
    
    const result = await runLoaf(["list", "--all"], { cwd: repoPath });
    
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("Active Sessions");
    expect(result.stdout).toContain("Archived Sessions");
    expect(result.stdout).toContain("1 active, 1 archived");
  });
});

describe("session: end", () => {
  it("exits successfully with --if-active when no session exists", async () => {
    const repoPath = createTempRepo("if-active-test");

    const result = await runLoaf(["end", "--if-active"], { cwd: repoPath });

    expect(result.exitCode).toBe(0);
  });

  it("does not append another pause entry for a paused session when using --if-active", async () => {
    const repoPath = createTempRepo("if-active-paused-test");

    await runLoaf(["start"], { cwd: repoPath });
    await runLoaf(["end"], { cwd: repoPath });

    const sessionFiles = getSessionFiles(repoPath);
    const sessionPath = join(repoPath, ".agents/sessions", sessionFiles[0]);
    const before = readFileSync(sessionPath, "utf-8");

    const result = await runLoaf(["end", "--if-active"], { cwd: repoPath });

    expect(result.exitCode).toBe(0);
    expect(readFileSync(sessionPath, "utf-8")).toBe(before);
  });

  it("surfaces knowledge files flagged by the staleness nudge", async () => {
    const repoPath = createTempRepo("kb-end-test");

    await runLoaf(["start"], { cwd: repoPath });

    const nudgeFile = getKnowledgeNudgeFile(repoPath);
    writeFileSync(nudgeFile, "docs/knowledge/stale.md\n", "utf-8");

    const result = await runLoaf(["end"], { cwd: repoPath });

    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("Knowledge consolidation recommended for 1 file");
    expect(result.stdout).toContain("docs/knowledge/stale.md");
    expect(existsSync(nudgeFile)).toBe(false);
  });

  it("skips session management when agent_id is present in stdin", async () => {
    const repoPath = createTempRepo("subagent-skip-test");

    // Simulate hook JSON with agent_id (subagent)
    const hookJson = JSON.stringify({
      session_id: "sess-123",
      agent_id: "agent-abc",
      agent_type: "general-purpose",
    });

    const result = await runLoaf(["start"], { cwd: repoPath, input: hookJson });

    // Should exit 0 without creating a session
    expect(result.exitCode).toBe(0);
    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(0);
  });

  it("creates session with --force even when agent_id is present", async () => {
    const repoPath = createTempRepo("force-subagent-test");

    const hookJson = JSON.stringify({
      session_id: "sess-123",
      agent_id: "agent-abc",
    });

    const result = await runLoaf(["start", "--force"], { cwd: repoPath, input: hookJson });

    expect(result.exitCode).toBe(0);
    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);
  });

  it("writes claude_session_id to frontmatter from hook JSON", async () => {
    const repoPath = createTempRepo("session-id-tag-test");

    const hookJson = JSON.stringify({ session_id: "sess-unique-456" });
    await runLoaf(["start"], { cwd: repoPath, input: hookJson });

    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);

    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );
    expect(content).toContain("claude_session_id: sess-unique-456");
  });

  it("writes resume entries when session_id changes between conversations", async () => {
    const repoPath = createTempRepo("session-id-change-test");

    // First conversation
    const hookJson1 = JSON.stringify({ session_id: "sess-first" });
    await runLoaf(["start"], { cwd: repoPath, input: hookJson1 });

    // Second conversation (different session_id)
    const hookJson2 = JSON.stringify({ session_id: "sess-second" });
    await runLoaf(["start"], { cwd: repoPath, input: hookJson2 });

    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1); // Same file, not a new one

    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );
    // STOPPED is written by session end; RESUMED written by session start
    expect(content).toContain("claude_session_id: sess-second");
    expect(content).toContain("=== SESSION RESUMED ===");
  });

  it("does not write PAUSE when same session_id reconnects", async () => {
    const repoPath = createTempRepo("same-session-test");

    const hookJson = JSON.stringify({ session_id: "sess-same" });
    await runLoaf(["start"], { cwd: repoPath, input: hookJson });
    await runLoaf(["start"], { cwd: repoPath, input: hookJson });

    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);

    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );
    // No stop/resume — same conversation, session still active
    expect(content).not.toContain("SESSION STOPPED");
    expect(content).not.toContain("SESSION RESUMED");
  });

  it("adds STOP separator header to journal on end", async () => {
    const repoPath = createTempRepo("stop-header-test");

    await runLoaf(["start"], { cwd: repoPath });
    await runLoaf(["end"], { cwd: repoPath });

    const sessionFiles = getSessionFiles(repoPath);
    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );

    // Should contain session stop entry and conclude entry
    expect(content).toContain("=== SESSION STOPPED ===");
    expect(content).toContain("session(conclude):");
    // Should NOT contain redundant pause entry
    expect(content).not.toContain("SESSION PAUSED");
  });
});
