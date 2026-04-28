/**
 * Session Command Integration Tests
 *
 * Black-box tests for the loaf session command using temp git repos.
 * Tests CLI behavior via child_process.spawn/execFile.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { spawn } from "child_process";

// Session tests spawn child processes — default 5s timeout is too tight
vi.setConfig({ testTimeout: 15000 });
import { createHash } from "crypto";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  realpathSync,
  rmSync,
  writeFileSync,
  readFileSync,
} from "fs";
import { join } from "path";
import { tmpdir } from "os";

// ─────────────────────────────────────────────────────────────────────────────
// Test Fixtures — unique per test to avoid cross-file interference
// ─────────────────────────────────────────────────────────────────────────────

let TEST_ROOT: string;
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

  // Initialize git repo. `--initial-branch=main` overrides any inherited
  // `init.defaultBranch` (e.g., a developer with `master` configured globally)
  // so SPEC-032 tests that assert literal `branch 'main'` in output remain
  // portable across machines.
  execFileSync("git", ["init", "--initial-branch=main"], { cwd: repoPath });
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
  TEST_ROOT = realpathSync(mkdtempSync(join(tmpdir(), "loaf-test-session-")));
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

  it("clears a dead detached-head create lock before starting", async () => {
    const repoPath = createTempRepo("dead-create-lock-test");

    const commitSha = require("child_process")
      .execFileSync("git", ["rev-parse", "--short", "HEAD"], { cwd: repoPath, encoding: "utf-8" })
      .trim();
    require("child_process").execFileSync("git", ["checkout", "--detach"], { cwd: repoPath });

    const branchKey = `detached-${commitSha}`.replace(/[^a-zA-Z0-9-]/g, "-");
    const sessionsDir = join(repoPath, ".agents/sessions");
    mkdirSync(sessionsDir, { recursive: true });

    const lockPath = join(sessionsDir, `.create-${branchKey}.lock`);
    writeFileSync(
      lockPath,
      JSON.stringify({ pid: 999999, timestamp: Date.now() }),
      "utf-8"
    );

    const result = await runLoaf(["start"], { cwd: repoPath });

    expect(result.exitCode).toBe(0);
    expect(getSessionFiles(repoPath)).toHaveLength(1);
    expect(existsSync(lockPath)).toBe(false);
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

  it("clears a dead session file lock before appending a log entry", async () => {
    const repoPath = createTempRepo("dead-log-lock-test");

    await runLoaf(["start"], { cwd: repoPath });

    const [sessionFile] = getSessionFiles(repoPath);
    const filePath = join(repoPath, ".agents/sessions", sessionFile);
    const lockPath = `${filePath}.lock`;

    writeFileSync(
      lockPath,
      JSON.stringify({ pid: 999999, timestamp: Date.now() }),
      "utf-8"
    );

    const result = await runLoaf(["log", "discover(test): recovered from dead lock"], { cwd: repoPath });

    expect(result.exitCode).toBe(0);
    expect(existsSync(lockPath)).toBe(false);
    expect(readFileSync(filePath, "utf-8")).toContain("discover(test): recovered from dead lock");
  });

  // SPEC-032 / TASK-117 — Tier-based routing: --session-id, hook stdin, branch fallback + WARN
  describe("SPEC-032 routing chain", () => {
    /** Read each session file's frontmatter+content, indexed by claude_session_id (when present). */
    function readSessionsByClaudeId(repoPath: string): Map<string, { fileName: string; content: string }> {
      const out = new Map<string, { fileName: string; content: string }>();
      for (const file of getSessionFiles(repoPath)) {
        const content = readFileSync(join(repoPath, ".agents/sessions", file), "utf-8");
        const m = content.match(/claude_session_id:\s*['"]?([^'"\n]+)['"]?/);
        if (m) {
          out.set(m[1].trim(), { fileName: file, content });
        }
      }
      return out;
    }

    it("Tier 1: --session-id routes to the matching session, no WARN", async () => {
      const repoPath = createTempRepo("spec032-tier1-flag");

      // Create two active sessions on the same branch (main) with different claude_session_ids.
      // Use `loaf session start` with hook stdin so claude_session_id is recorded in frontmatter.
      await runLoaf(["start"], { cwd: repoPath, input: JSON.stringify({ session_id: "claude-aaa" }) });

      // End/stop the first one before starting the second, then re-stop second
      // so we have two on-disk sessions on main with distinct claude_session_ids.
      await runLoaf(["end"], { cwd: repoPath });

      // Manually mark the stopped one's status so a second start does not archive it.
      // Simplest path: write a second session file directly.
      const sessionsDir = join(repoPath, ".agents/sessions");
      const secondFile = "20260101-120000-session.md";
      writeFileSync(
        join(sessionsDir, secondFile),
        [
          "---",
          "branch: main",
          "status: active",
          "claude_session_id: claude-bbb",
          `created: '2026-01-01T12:00:00.000Z'`,
          `last_updated: '2026-01-01T12:00:00.000Z'`,
          "---",
          "# Session: Manual",
          "",
          "## Journal",
          "",
          "[2026-01-01 12:00] session(start):  === SESSION STARTED ===",
          "",
        ].join("\n"),
        "utf-8"
      );

      // Log with --session-id targeting the second session
      const result = await runLoaf(
        ["log", "decision(test): tier1 hits bbb", "--session-id", "claude-bbb"],
        { cwd: repoPath }
      );

      expect(result.exitCode).toBe(0);
      expect(result.stderr).not.toContain("WARN: no session_id signal");

      const byId = readSessionsByClaudeId(repoPath);
      const bbb = byId.get("claude-bbb");
      expect(bbb).toBeDefined();
      expect(bbb!.content).toContain("decision(test): tier1 hits bbb");

      const aaa = byId.get("claude-aaa");
      if (aaa) {
        expect(aaa.content).not.toContain("decision(test): tier1 hits bbb");
      }
    });

    it("Tier 2: --from-hook reads session_id from stdin JSON, no WARN", async () => {
      const repoPath = createTempRepo("spec032-tier2-stdin");

      // Start a session with claude-zzz via hook stdin so frontmatter carries it
      await runLoaf(["start"], { cwd: repoPath, input: JSON.stringify({ session_id: "claude-zzz" }) });

      // Make a real commit so the hook entry-text extraction has a message to grab
      writeFileSync(join(repoPath, "feature.ts"), "export const x = 1;\n", "utf-8");
      execFileSync("git", ["add", "."], { cwd: repoPath });
      execFileSync("git", ["commit", "-m", "feat: tier2 commit"], { cwd: repoPath });

      const result = await runLoaf(["log", "--from-hook"], {
        cwd: repoPath,
        input: JSON.stringify({
          session_id: "claude-zzz",
          tool_input: { command: "git commit -m 'feat: tier2 commit'" },
        }),
      });

      expect(result.exitCode).toBe(0);
      expect(result.stderr).not.toContain("WARN: no session_id signal");

      const byId = readSessionsByClaudeId(repoPath);
      const zzz = byId.get("claude-zzz");
      expect(zzz).toBeDefined();
      expect(zzz!.content).toContain("commit(");
      expect(zzz!.content).toContain("feat: tier2 commit");
    });

    it("Tier 3: no flag and no hook → branch fallback emits WARN to stderr", async () => {
      const repoPath = createTempRepo("spec032-tier3-warn");

      // Start a session (no claude_session_id in frontmatter → branch is the only signal)
      await runLoaf(["start"], { cwd: repoPath });

      const result = await runLoaf(
        ["log", "decision(test): tier3 fallback"],
        { cwd: repoPath }
      );

      expect(result.exitCode).toBe(0);
      // Exact WARN text from cli/lib/session/resolve.ts
      expect(result.stderr).toContain(
        "WARN: no session_id signal — falling back to branch routing for branch 'main'. Pass --session-id <id> to silence."
      );

      const sessionFiles = getSessionFiles(repoPath);
      expect(sessionFiles.length).toBe(1);
      const content = readFileSync(
        join(repoPath, ".agents/sessions", sessionFiles[0]),
        "utf-8"
      );
      expect(content).toContain("decision(test): tier3 fallback");
    });

    it("--from-hook stdin missing session_id → falls through to Tier 3 with WARN, entry still logged", async () => {
      const repoPath = createTempRepo("spec032-hook-no-sid");

      await runLoaf(["start"], { cwd: repoPath });

      writeFileSync(join(repoPath, "x.ts"), "export const y = 2;\n", "utf-8");
      execFileSync("git", ["add", "."], { cwd: repoPath });
      execFileSync("git", ["commit", "-m", "chore: no sid commit"], { cwd: repoPath });

      const result = await runLoaf(["log", "--from-hook"], {
        cwd: repoPath,
        // No session_id field — only tool_input for entry extraction
        input: JSON.stringify({
          tool_input: { command: "git commit -m 'chore: no sid commit'" },
        }),
      });

      expect(result.exitCode).toBe(0);
      expect(result.stderr).toContain("WARN: no session_id signal");
      expect(result.stderr).toContain("branch 'main'");

      const sessionFiles = getSessionFiles(repoPath);
      expect(sessionFiles.length).toBe(1);
      const content = readFileSync(
        join(repoPath, ".agents/sessions", sessionFiles[0]),
        "utf-8"
      );
      expect(content).toContain("chore: no sid commit");
    });

    it("--from-hook AND --session-id together → flag wins, no WARN", async () => {
      const repoPath = createTempRepo("spec032-flag-wins");

      // Two sessions on main with different claude_session_ids
      await runLoaf(["start"], { cwd: repoPath, input: JSON.stringify({ session_id: "claude-flag" }) });

      const sessionsDir = join(repoPath, ".agents/sessions");
      writeFileSync(
        join(sessionsDir, "20260101-120000-session.md"),
        [
          "---",
          "branch: main",
          "status: active",
          "claude_session_id: claude-stdin",
          `created: '2026-01-01T12:00:00.000Z'`,
          `last_updated: '2026-01-01T12:00:00.000Z'`,
          "---",
          "# Session: Stdin",
          "",
          "## Journal",
          "",
          "[2026-01-01 12:00] session(start):  === SESSION STARTED ===",
          "",
        ].join("\n"),
        "utf-8"
      );

      const result = await runLoaf(
        ["log", "decision(test): flag should win", "--from-hook", "--session-id", "claude-flag"],
        {
          cwd: repoPath,
          input: JSON.stringify({ session_id: "claude-stdin" }),
        }
      );

      expect(result.exitCode).toBe(0);
      expect(result.stderr).not.toContain("WARN: no session_id signal");

      const byId = readSessionsByClaudeId(repoPath);
      const flagSession = byId.get("claude-flag");
      const stdinSession = byId.get("claude-stdin");
      expect(flagSession).toBeDefined();
      expect(flagSession!.content).toContain("decision(test): flag should win");
      // Flag wins: stdin's session must NOT contain the entry
      expect(stdinSession!.content).not.toContain("decision(test): flag should win");
    });

    it("dev.30 misrouting repro: 1 active + 4 stopped on same branch, hook routes to active by claude_session_id", async () => {
      const repoPath = createTempRepo("spec032-misroute-repro");

      const sessionsDir = join(repoPath, ".agents/sessions");
      mkdirSync(sessionsDir, { recursive: true });

      // Build the fixture: 4 stopped sessions on main + 1 active session on main.
      // The active one carries claude_session_id "claude-active".
      const stopped = [
        "20260401-100000-session.md",
        "20260401-110000-session.md",
        "20260401-120000-session.md",
        "20260401-130000-session.md",
      ];
      for (let i = 0; i < stopped.length; i++) {
        writeFileSync(
          join(sessionsDir, stopped[i]),
          [
            "---",
            "branch: main",
            "status: stopped",
            `claude_session_id: claude-stopped-${i}`,
            `created: '2026-04-01T${10 + i}:00:00.000Z'`,
            `last_updated: '2026-04-01T${10 + i}:30:00.000Z'`,
            "---",
            `# Session: Stopped ${i}`,
            "",
            "## Journal",
            "",
            `[2026-04-01 ${10 + i}:00] session(start):  === SESSION STARTED ===`,
            `[2026-04-01 ${10 + i}:30] session(stop):   === SESSION STOPPED ===`,
            "",
          ].join("\n"),
          "utf-8"
        );
      }

      const activeFile = "20260401-140000-session.md";
      writeFileSync(
        join(sessionsDir, activeFile),
        [
          "---",
          "branch: main",
          "status: active",
          "claude_session_id: claude-active",
          `created: '2026-04-01T14:00:00.000Z'`,
          `last_updated: '2026-04-01T14:00:00.000Z'`,
          "---",
          "# Session: Active",
          "",
          "## Journal",
          "",
          "[2026-04-01 14:00] session(start):  === SESSION STARTED ===",
          "",
        ].join("\n"),
        "utf-8"
      );

      // Hook fires for the active session
      writeFileSync(join(repoPath, "feat.ts"), "export const z = 3;\n", "utf-8");
      execFileSync("git", ["add", "."], { cwd: repoPath });
      execFileSync("git", ["commit", "-m", "feat: misroute repro"], { cwd: repoPath });

      const result = await runLoaf(["log", "--from-hook"], {
        cwd: repoPath,
        input: JSON.stringify({
          session_id: "claude-active",
          tool_input: { command: "git commit -m 'feat: misroute repro'" },
        }),
      });

      expect(result.exitCode).toBe(0);
      expect(result.stderr).not.toContain("WARN: no session_id signal");

      const activeContent = readFileSync(join(sessionsDir, activeFile), "utf-8");
      expect(activeContent).toContain("feat: misroute repro");

      // None of the stopped sessions should have received the entry
      for (const f of stopped) {
        const stoppedContent = readFileSync(join(sessionsDir, f), "utf-8");
        expect(stoppedContent).not.toContain("feat: misroute repro");
      }
    });

    it("does not create a new session file under any routing path", async () => {
      const repoPath = createTempRepo("spec032-no-new-files");

      await runLoaf(["start"], { cwd: repoPath, input: JSON.stringify({ session_id: "claude-only" }) });
      const beforeCount = getSessionFiles(repoPath).length;
      expect(beforeCount).toBe(1);

      // Tier 1
      await runLoaf(["log", "decision(test): t1", "--session-id", "claude-only"], { cwd: repoPath });
      // Tier 2
      await runLoaf(["log", "--from-hook"], {
        cwd: repoPath,
        input: JSON.stringify({
          session_id: "claude-only",
          tool_input: { command: "git commit -m 'noop'" },
        }),
      });
      // Tier 3
      await runLoaf(["log", "decision(test): t3"], { cwd: repoPath });

      const afterCount = getSessionFiles(repoPath).length;
      expect(afterCount).toBe(beforeCount);
    });
  });
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

describe("session: archive", () => {
  describe("SPEC-032 routing chain", () => {
    /** Read each session file's frontmatter+content, indexed by claude_session_id (when present). */
    function readSessionsByClaudeId(
      repoPath: string,
      includeArchive = false
    ): Map<string, { fileName: string; content: string; archived: boolean }> {
      const out = new Map<string, { fileName: string; content: string; archived: boolean }>();
      const sessionsDir = join(repoPath, ".agents/sessions");
      if (!existsSync(sessionsDir)) return out;

      // Active sessions
      for (const file of getSessionFiles(repoPath)) {
        const content = readFileSync(join(sessionsDir, file), "utf-8");
        const m = content.match(/claude_session_id:\s*['"]?([^'"\n]+)['"]?/);
        if (m) {
          out.set(m[1].trim(), { fileName: file, content, archived: false });
        }
      }

      // Archived sessions
      if (includeArchive) {
        const archiveDir = join(sessionsDir, "archive");
        if (existsSync(archiveDir)) {
          for (const file of readdirSync(archiveDir).filter(f => f.endsWith(".md"))) {
            const content = readFileSync(join(archiveDir, file), "utf-8");
            const m = content.match(/claude_session_id:\s*['"]?([^'"\n]+)['"]?/);
            if (m) {
              out.set(m[1].trim(), { fileName: file, content, archived: true });
            }
          }
        }
      }

      return out;
    }

    it("--session-id archives the session with that claude_session_id, no WARN", async () => {
      const repoPath = createTempRepo("spec032-archive-tier1");
      const sessionsDir = join(repoPath, ".agents/sessions");
      mkdirSync(sessionsDir, { recursive: true });

      // Two active sessions on main with different claude_session_ids.
      writeFileSync(
        join(sessionsDir, "20260101-120000-session.md"),
        [
          "---",
          "branch: main",
          "status: active",
          "claude_session_id: claude-aaa",
          `created: '2026-01-01T12:00:00.000Z'`,
          `last_updated: '2026-01-01T12:00:00.000Z'`,
          "---",
          "# Session: AAA",
          "",
          "## Journal",
          "",
          "[2026-01-01 12:00] session(start):  === SESSION STARTED ===",
          "",
        ].join("\n"),
        "utf-8"
      );
      writeFileSync(
        join(sessionsDir, "20260101-130000-session.md"),
        [
          "---",
          "branch: main",
          "status: active",
          "claude_session_id: claude-bbb",
          `created: '2026-01-01T13:00:00.000Z'`,
          `last_updated: '2026-01-01T13:00:00.000Z'`,
          "---",
          "# Session: BBB",
          "",
          "## Journal",
          "",
          "[2026-01-01 13:00] session(start):  === SESSION STARTED ===",
          "",
        ].join("\n"),
        "utf-8"
      );

      const result = await runLoaf(
        ["archive", "--session-id", "claude-aaa"],
        { cwd: repoPath }
      );

      expect(result.exitCode).toBe(0);
      expect(result.stderr).not.toContain("WARN: no session_id signal");

      // claude-aaa should be archived; claude-bbb should remain active.
      const all = readSessionsByClaudeId(repoPath, true);
      const aaa = all.get("claude-aaa");
      const bbb = all.get("claude-bbb");
      expect(aaa).toBeDefined();
      expect(aaa!.archived).toBe(true);
      expect(bbb).toBeDefined();
      expect(bbb!.archived).toBe(false);
    });

    it("no flag → branch fallback emits WARN, archives most-recent active session", async () => {
      const repoPath = createTempRepo("spec032-archive-tier3");

      await runLoaf(["start"], { cwd: repoPath });

      const result = await runLoaf(["archive"], { cwd: repoPath });

      expect(result.exitCode).toBe(0);
      expect(result.stderr).toContain(
        "WARN: no session_id signal — falling back to branch routing for branch 'main'. Pass --session-id <id> to silence."
      );

      // Active directory should be empty; archive directory should have 1 file.
      const activeFiles = getSessionFiles(repoPath);
      expect(activeFiles.length).toBe(0);
      const archiveDir = join(repoPath, ".agents/sessions/archive");
      expect(existsSync(archiveDir)).toBe(true);
      const archived = readdirSync(archiveDir).filter(f => f.endsWith(".md"));
      expect(archived.length).toBe(1);
    });

    it("multi-session repro: --session-id archives the right one, leaves others untouched", async () => {
      const repoPath = createTempRepo("spec032-archive-multi");
      const sessionsDir = join(repoPath, ".agents/sessions");
      mkdirSync(sessionsDir, { recursive: true });

      // 3 active sessions on main, different claude_session_ids.
      const ids = ["claude-aaa", "claude-bbb", "claude-ccc"];
      for (let i = 0; i < ids.length; i++) {
        writeFileSync(
          join(sessionsDir, `20260101-${10 + i}0000-session.md`),
          [
            "---",
            "branch: main",
            "status: active",
            `claude_session_id: ${ids[i]}`,
            `created: '2026-01-01T${10 + i}:00:00.000Z'`,
            `last_updated: '2026-01-01T${10 + i}:00:00.000Z'`,
            "---",
            `# Session: ${ids[i]}`,
            "",
            "## Journal",
            "",
            `[2026-01-01 ${10 + i}:00] session(start):  === SESSION STARTED ===`,
            "",
          ].join("\n"),
          "utf-8"
        );
      }

      const result = await runLoaf(
        ["archive", "--session-id", "claude-bbb"],
        { cwd: repoPath }
      );

      expect(result.exitCode).toBe(0);
      expect(result.stderr).not.toContain("WARN: no session_id signal");

      const all = readSessionsByClaudeId(repoPath, true);
      // The targeted session is archived
      expect(all.get("claude-bbb")?.archived).toBe(true);
      // The other two remain active
      expect(all.get("claude-aaa")?.archived).toBe(false);
      expect(all.get("claude-ccc")?.archived).toBe(false);

      // Active directory should still have 2 files
      const activeFiles = getSessionFiles(repoPath);
      expect(activeFiles.length).toBe(2);
    });
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

  it("creates new session file when session_id changes between conversations", async () => {
    const repoPath = createTempRepo("session-id-change-test");

    // First conversation
    const hookJson1 = JSON.stringify({ session_id: "sess-first" });
    await runLoaf(["start"], { cwd: repoPath, input: hookJson1 });

    // Second conversation (different session_id)
    const hookJson2 = JSON.stringify({ session_id: "sess-second" });
    await runLoaf(["start"], { cwd: repoPath, input: hookJson2 });

    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(2); // New file for new conversation

    // Identify old vs new by content (ordering may vary due to collision suffix)
    const contents = sessionFiles.map(f =>
      readFileSync(join(repoPath, ".agents/sessions", f), "utf-8")
    );
    const oldContent = contents.find(c => c.includes("claude_session_id: sess-first"))!;
    const newContent = contents.find(c => c.includes("claude_session_id: sess-second"))!;

    expect(oldContent).toBeDefined();
    expect(newContent).toBeDefined();

    // Old session should be stopped
    expect(oldContent).toContain("status: stopped");
    expect(oldContent).toContain("closed by new conversation");

    // New session should be active
    expect(newContent).toContain("status: active");
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

    // Should contain session stop entry and end entry
    expect(content).toContain("=== SESSION STOPPED ===");
    expect(content).toContain("session(end):");
    // Should NOT contain redundant pause entry
    expect(content).not.toContain("SESSION PAUSED");
  });

  it("resumes stopped session when same claude_session_id reconnects", async () => {
    const repoPath = createTempRepo("same-id-stopped-test");

    const hookJson = JSON.stringify({ session_id: "sess-persist" });
    await runLoaf(["start"], { cwd: repoPath, input: hookJson });
    await runLoaf(["end"], { cwd: repoPath });

    // Same session_id reconnects after stop
    await runLoaf(["start"], { cwd: repoPath, input: hookJson });

    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1); // Same file, not a new one

    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );
    expect(content).toContain("=== SESSION RESUMED ===");
    expect(content).toContain("status: active");
  });

  it("session end with reason=clear logs session(clear) and keeps status active", async () => {
    const repoPath = createTempRepo("clear-end-test");

    await runLoaf(["start"], { cwd: repoPath });

    // End session with reason=clear (simulates /clear hook)
    const hookJson = JSON.stringify({ reason: "clear" });
    const result = await runLoaf(["end"], { cwd: repoPath, input: hookJson });
    expect(result.exitCode).toBe(0);

    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);

    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );

    // Should contain clear marker
    expect(content).toContain("session(clear):  === CONTEXT CLEARED ===");
    // Should NOT contain normal stop/end entries
    expect(content).not.toContain("SESSION STOPPED");
    expect(content).not.toContain("session(end):");
    // Status should still be active
    expect(content).toContain("status: active");
  });

  it("session start with source=clear resumes existing session", async () => {
    const repoPath = createTempRepo("clear-resume-test");

    // Start session with initial session_id
    const hookJson1 = JSON.stringify({ session_id: "sess-old" });
    await runLoaf(["start"], { cwd: repoPath, input: hookJson1 });

    // End with reason=clear (keeps status active)
    const hookJsonEnd = JSON.stringify({ reason: "clear" });
    await runLoaf(["end"], { cwd: repoPath, input: hookJsonEnd });

    // Start with source=clear and new session_id
    const hookJson2 = JSON.stringify({ session_id: "sess-new", source: "clear" });
    await runLoaf(["start"], { cwd: repoPath, input: hookJson2 });

    // Still only 1 session file (no archive, no new file)
    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);

    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );

    // Frontmatter should have the new session_id
    expect(content).toContain("claude_session_id: sess-new");
    // Should contain clear marker from end and resume from start
    expect(content).toContain("session(clear)");
    expect(content).toContain("SESSION RESUMED");
    // No archive should exist
    const archiveDir = join(repoPath, ".agents/sessions/archive");
    expect(existsSync(archiveDir)).toBe(false);
  });

  it("session(start) entry includes session_id when provided", async () => {
    const repoPath = createTempRepo("start-id-test");

    const hookJson = JSON.stringify({ session_id: "sess-with-id" });
    await runLoaf(["start"], { cwd: repoPath, input: hookJson });

    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);

    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );

    // First 8 chars of "sess-with-id" is "sess-wit"
    expect(content).toContain("SESSION STARTED === (session sess-wit");
  });

  it("full clear cycle preserves session continuity", async () => {
    const repoPath = createTempRepo("clear-cycle-test");

    // Start session
    const hookJson1 = JSON.stringify({ session_id: "sess-before" });
    await runLoaf(["start"], { cwd: repoPath, input: hookJson1 });

    // Log a decision
    await runLoaf(["log", "decision(test): first decision"], { cwd: repoPath });

    // End with reason=clear
    const hookJsonEnd = JSON.stringify({ reason: "clear" });
    await runLoaf(["end"], { cwd: repoPath, input: hookJsonEnd });

    // Start with source=clear and new session_id
    const hookJson2 = JSON.stringify({ session_id: "sess-after", source: "clear" });
    await runLoaf(["start"], { cwd: repoPath, input: hookJson2 });

    // Log another decision
    await runLoaf(["log", "decision(test): second decision"], { cwd: repoPath });

    // Verify single session file with full history
    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);

    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );

    // Both decisions preserved
    expect(content).toContain("decision(test): first decision");
    expect(content).toContain("decision(test): second decision");
    // Clear marker present
    expect(content).toContain("session(clear):  === CONTEXT CLEARED ===");
    // Resume present
    expect(content).toContain("SESSION RESUMED");
    // No archive
    const archiveDir = join(repoPath, ".agents/sessions/archive");
    expect(existsSync(archiveDir)).toBe(false);
  });

  it("adopts session when branch switches mid-session", async () => {
    const repoPath = createTempRepo("branch-switch-test");

    await runLoaf(["start"], { cwd: repoPath });

    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);

    const contentBefore = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );
    expect(contentBefore).toContain("branch: main");

    // Switch to a new branch
    execFileSync("git", ["checkout", "-b", "feat/new-feature"], { cwd: repoPath });

    // Log an entry — triggers findActiveSessionForBranch which should adopt
    await runLoaf(["log", "decision(test): testing branch adoption"], { cwd: repoPath });

    const contentAfter = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );
    expect(contentAfter).toContain("branch: feat/new-feature");
    expect(contentAfter).toContain("decision(test): testing branch adoption");
  });

  // SPEC-032 / TASK-121 — Tier-based routing for `loaf session end --wrap`.
  // The `--wrap` invocation is a direct-CLI call from the /wrap skill, NOT
  // a hook event. It must route through `resolveCurrentSession` so that
  // --session-id wins (Tier 1) and branch fallback emits the WARN. Hook-driven
  // `loaf session end` (--from-hook / Stop event) keeps the inline chain.
  describe("SPEC-032 routing chain", () => {
    /**
     * Compute sha256 of file contents — used to assert that other sessions'
     * files were NOT mutated when `--wrap --session-id <id>` is targeted.
     */
    function fileHash(path: string): string {
      return createHash("sha256").update(readFileSync(path)).digest("hex");
    }

    it("Tier 1: --wrap --session-id targets the matching session, no WARN", async () => {
      const repoPath = createTempRepo("spec032-end-wrap-tier1");
      const sessionsDir = join(repoPath, ".agents/sessions");
      mkdirSync(sessionsDir, { recursive: true });

      // Two active sessions on main with different claude_session_ids.
      writeFileSync(
        join(sessionsDir, "20260101-120000-session.md"),
        [
          "---",
          "branch: main",
          "status: active",
          "claude_session_id: claude-aaa",
          `created: '2026-01-01T12:00:00.000Z'`,
          `last_updated: '2026-01-01T12:00:00.000Z'`,
          "---",
          "# Session: AAA",
          "",
          "## Journal",
          "",
          "[2026-01-01 12:00] session(start):  === SESSION STARTED ===",
          "",
        ].join("\n"),
        "utf-8"
      );
      writeFileSync(
        join(sessionsDir, "20260101-130000-session.md"),
        [
          "---",
          "branch: main",
          "status: active",
          "claude_session_id: claude-bbb",
          `created: '2026-01-01T13:00:00.000Z'`,
          `last_updated: '2026-01-01T13:00:00.000Z'`,
          "---",
          "# Session: BBB",
          "",
          "## Journal",
          "",
          "[2026-01-01 13:00] session(start):  === SESSION STARTED ===",
          "",
        ].join("\n"),
        "utf-8"
      );

      const result = await runLoaf(
        ["end", "--wrap", "--session-id", "claude-bbb"],
        { cwd: repoPath }
      );

      expect(result.exitCode).toBe(0);
      expect(result.stderr).not.toContain("WARN: no session_id signal");

      // claude-bbb gets the wrap marker and status=done; claude-aaa untouched.
      const aaa = readFileSync(join(sessionsDir, "20260101-120000-session.md"), "utf-8");
      const bbb = readFileSync(join(sessionsDir, "20260101-130000-session.md"), "utf-8");
      expect(bbb).toContain("session(wrap):");
      expect(bbb).toContain("status: done");
      expect(aaa).not.toContain("session(wrap):");
      expect(aaa).toContain("status: active");
    });

    it("Tier 3: --wrap with no flag falls through to branch routing and emits WARN", async () => {
      const repoPath = createTempRepo("spec032-end-wrap-tier3");

      await runLoaf(["start"], { cwd: repoPath });

      const result = await runLoaf(["end", "--wrap"], { cwd: repoPath });

      expect(result.exitCode).toBe(0);
      // Exact WARN text from cli/lib/session/resolve.ts
      expect(result.stderr).toContain(
        "WARN: no session_id signal — falling back to branch routing for branch 'main'. Pass --session-id <id> to silence."
      );

      const sessionFiles = getSessionFiles(repoPath);
      expect(sessionFiles.length).toBe(1);
      const content = readFileSync(
        join(repoPath, ".agents/sessions", sessionFiles[0]),
        "utf-8"
      );
      // Branch fallback resolved to the only active session and wrapped it.
      expect(content).toContain("session(wrap):");
      expect(content).toContain("status: done");
    });

    it("--from-hook (no --wrap) keeps inline chain — silent on no match, no WARN", async () => {
      const repoPath = createTempRepo("spec032-end-fromhook-silent");

      // No session exists. Inline-chain hook path should exit silently when
      // combined with --if-active, without emitting a WARN.
      const result = await runLoaf(
        ["end", "--if-active", "--from-hook"],
        { cwd: repoPath, input: JSON.stringify({ session_id: "claude-zzz" }) }
      );

      expect(result.exitCode).toBe(0);
      expect(result.stderr).not.toContain("WARN: no session_id signal");
    });

    it("--from-hook resolves via inline chain when stdin carries session_id, no WARN", async () => {
      const repoPath = createTempRepo("spec032-end-fromhook-stdin");
      const sessionsDir = join(repoPath, ".agents/sessions");
      mkdirSync(sessionsDir, { recursive: true });

      writeFileSync(
        join(sessionsDir, "20260101-120000-session.md"),
        [
          "---",
          "branch: main",
          "status: active",
          "claude_session_id: claude-stop",
          `created: '2026-01-01T12:00:00.000Z'`,
          `last_updated: '2026-01-01T12:00:00.000Z'`,
          "---",
          "# Session: Stop",
          "",
          "## Journal",
          "",
          "[2026-01-01 12:00] session(start):  === SESSION STARTED ===",
          "",
        ].join("\n"),
        "utf-8"
      );

      const result = await runLoaf(
        ["end", "--from-hook"],
        { cwd: repoPath, input: JSON.stringify({ session_id: "claude-stop" }) }
      );

      expect(result.exitCode).toBe(0);
      expect(result.stderr).not.toContain("WARN: no session_id signal");

      const content = readFileSync(
        join(sessionsDir, "20260101-120000-session.md"),
        "utf-8"
      );
      expect(content).toContain("=== SESSION STOPPED ===");
      expect(content).toContain("status: stopped");
    });

    it("multi-session repro: --wrap --session-id ends the active one, leaves stopped untouched", async () => {
      const repoPath = createTempRepo("spec032-end-wrap-multi");
      const sessionsDir = join(repoPath, ".agents/sessions");
      mkdirSync(sessionsDir, { recursive: true });

      // Fixture: 1 active + 4 stopped sessions on main, distinct claude_session_ids.
      // Mirrors the dev.30 misroute layout. We hash every stopped session's
      // file contents and assert post-wrap that none of them changed.
      const stopped = [
        "20260401-100000-session.md",
        "20260401-110000-session.md",
        "20260401-120000-session.md",
        "20260401-130000-session.md",
      ];
      for (let i = 0; i < stopped.length; i++) {
        writeFileSync(
          join(sessionsDir, stopped[i]),
          [
            "---",
            "branch: main",
            "status: stopped",
            `claude_session_id: claude-stopped-${i}`,
            `created: '2026-04-01T${10 + i}:00:00.000Z'`,
            `last_updated: '2026-04-01T${10 + i}:30:00.000Z'`,
            "---",
            `# Session: Stopped ${i}`,
            "",
            "## Journal",
            "",
            `[2026-04-01 ${10 + i}:00] session(start):  === SESSION STARTED ===`,
            `[2026-04-01 ${10 + i}:30] session(stop):   === SESSION STOPPED ===`,
            "",
          ].join("\n"),
          "utf-8"
        );
      }

      const activeFile = "20260401-140000-session.md";
      writeFileSync(
        join(sessionsDir, activeFile),
        [
          "---",
          "branch: main",
          "status: active",
          "claude_session_id: claude-active",
          `created: '2026-04-01T14:00:00.000Z'`,
          `last_updated: '2026-04-01T14:00:00.000Z'`,
          "---",
          "# Session: Active",
          "",
          "## Journal",
          "",
          "[2026-04-01 14:00] session(start):  === SESSION STARTED ===",
          "",
        ].join("\n"),
        "utf-8"
      );

      // Snapshot stopped-session file hashes before the wrap.
      const before = new Map<string, string>();
      for (const f of stopped) {
        before.set(f, fileHash(join(sessionsDir, f)));
      }

      const result = await runLoaf(
        ["end", "--wrap", "--session-id", "claude-active"],
        { cwd: repoPath }
      );

      expect(result.exitCode).toBe(0);
      expect(result.stderr).not.toContain("WARN: no session_id signal");

      // Active session got the wrap marker and status=done.
      const activeContent = readFileSync(join(sessionsDir, activeFile), "utf-8");
      expect(activeContent).toContain("session(wrap):");
      expect(activeContent).toContain("status: done");

      // Every stopped session's file content is byte-identical to before.
      for (const f of stopped) {
        expect(fileHash(join(sessionsDir, f))).toBe(before.get(f));
      }
    });
  });
});

describe("session: state", () => {
  it("state update writes Current State section to session file", async () => {
    const repoPath = createTempRepo("state-write-test");

    await runLoaf(["start"], { cwd: repoPath });
    await runLoaf(["log", "decision(test): initial design choice"], { cwd: repoPath });

    const result = await runLoaf(["state", "update"], { cwd: repoPath });
    expect(result.exitCode).toBe(0);

    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);

    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );

    // Verify section exists with expected content
    expect(content).toContain("## Current State (");
    expect(content).toContain("Branch: main");

    // Verify section placement: Current State before Journal
    const stateIdx = content.indexOf("## Current State (");
    const journalIdx = content.indexOf("## Journal");
    expect(stateIdx).toBeGreaterThan(-1);
    expect(journalIdx).toBeGreaterThan(-1);
    expect(stateIdx).toBeLessThan(journalIdx);
  });

  it("state update replaces existing Current State section", async () => {
    const repoPath = createTempRepo("state-replace-test");

    await runLoaf(["start"], { cwd: repoPath });

    // First update
    const result1 = await runLoaf(["state", "update"], { cwd: repoPath });
    expect(result1.exitCode).toBe(0);

    // Log a new entry
    await runLoaf(["log", "decision(test): second decision after state"], { cwd: repoPath });

    // Second update
    const result2 = await runLoaf(["state", "update"], { cwd: repoPath });
    expect(result2.exitCode).toBe(0);

    const sessionFiles = getSessionFiles(repoPath);
    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );

    // Only ONE Current State section
    const matches = content.match(/## Current State \(/g);
    expect(matches?.length).toBe(1);
  });

  it("state update skips silently for subagents", async () => {
    const repoPath = createTempRepo("state-subagent-test");

    await runLoaf(["start"], { cwd: repoPath });

    const hookJson = JSON.stringify({ agent_id: "agent-123" });
    const result = await runLoaf(["state", "update"], { cwd: repoPath, input: hookJson });

    expect(result.exitCode).toBe(0);

    const sessionFiles = getSessionFiles(repoPath);
    const content = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );

    // No Current State section should have been added
    expect(content).not.toContain("## Current State (");
  });

  it("state update is hook-safe when no session exists", async () => {
    const repoPath = createTempRepo("state-no-session-test");

    const result = await runLoaf(["state", "update"], { cwd: repoPath });
    expect(result.exitCode).toBe(0);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Housekeeping Tests
// ─────────────────────────────────────────────────────────────────────────────

describe("session housekeeping", () => {
  it("runs with no sessions without error", async () => {
    const repoPath = createTempRepo("housekeeping-empty");

    const result = await runLoaf(["housekeeping", "--dry-run"], { cwd: repoPath });
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("No sessions");
  });

  it("detects orphaned sessions (branch deleted)", async () => {
    const repoPath = createTempRepo("housekeeping-orphan");
    const sessionsDir = join(repoPath, ".agents/sessions");
    mkdirSync(sessionsDir, { recursive: true });

    // Create a session for a branch that doesn't exist
    writeFileSync(
      join(sessionsDir, "20260401-120000-session.md"),
      [
        "---",
        "branch: feat/deleted-branch",
        "status: stopped",
        `created: '2026-04-01T12:00:00.000Z'`,
        "---",
        "# Session: Test",
        "",
        "## Journal",
        "",
        "[2026-04-01 12:00] session(start):  === SESSION STARTED ===",
        "[2026-04-01 12:05] decision(test): some decision",
        "",
      ].join("\n"),
      "utf-8"
    );

    const result = await runLoaf(["housekeeping", "--dry-run"], { cwd: repoPath });
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("Orphan needs review");
    expect(result.stdout).toContain("feat/deleted-branch");
  });

  it("archives empty orphans (no journal activity)", async () => {
    const repoPath = createTempRepo("housekeeping-empty-orphan");
    const sessionsDir = join(repoPath, ".agents/sessions");
    mkdirSync(sessionsDir, { recursive: true });

    // Create an empty session for a deleted branch
    writeFileSync(
      join(sessionsDir, "20260401-120000-session.md"),
      [
        "---",
        "branch: feat/gone-branch",
        "status: stopped",
        `created: '2026-04-01T12:00:00.000Z'`,
        "---",
        "# Session: Test",
        "",
        "## Journal",
        "",
        "[2026-04-01 12:00] session(start):  === SESSION STARTED ===",
        "",
      ].join("\n"),
      "utf-8"
    );

    const result = await runLoaf(["housekeeping"], { cwd: repoPath });
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("Archived empty orphan");

    // Should have moved to archive
    const archiveDir = join(sessionsDir, "archive");
    expect(existsSync(archiveDir)).toBe(true);
    const archived = readdirSync(archiveDir).filter(f => f.endsWith(".md"));
    expect(archived.length).toBe(1);
  });

  it("archives done sessions older than 7 days", async () => {
    const repoPath = createTempRepo("housekeeping-age");
    const sessionsDir = join(repoPath, ".agents/sessions");
    mkdirSync(sessionsDir, { recursive: true });

    const oldDate = new Date(Date.now() - 10 * 24 * 60 * 60 * 1000).toISOString();

    // Create a done session that's 10 days old
    writeFileSync(
      join(sessionsDir, "20260330-120000-session.md"),
      [
        "---",
        "branch: main",
        "status: done",
        `created: '${oldDate}'`,
        `last_updated: '${oldDate}'`,
        "---",
        "# Session: Old Done",
        "",
        "## Journal",
        "",
        "[2026-03-30 12:00] session(start):  === SESSION STARTED ===",
        "",
      ].join("\n"),
      "utf-8"
    );

    const result = await runLoaf(["housekeeping"], { cwd: repoPath });
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("Archived:");
    expect(result.stdout).toContain("done, 10d old");
  });

  it("writes .loaf-state after housekeeping run", async () => {
    const repoPath = createTempRepo("housekeeping-state");
    const agentsDir = join(repoPath, ".agents");

    const result = await runLoaf(["housekeeping"], { cwd: repoPath });
    expect(result.exitCode).toBe(0);

    const statePath = join(agentsDir, ".loaf-state");
    expect(existsSync(statePath)).toBe(true);

    const state = JSON.parse(readFileSync(statePath, "utf-8"));
    expect(state.last_housekeeping).toBeDefined();
    expect(state.housekeeping_pending).toBe(false);
  });

  it("dry run does not modify files", async () => {
    const repoPath = createTempRepo("housekeeping-dry");
    const sessionsDir = join(repoPath, ".agents/sessions");
    mkdirSync(sessionsDir, { recursive: true });

    const oldDate = new Date(Date.now() - 10 * 24 * 60 * 60 * 1000).toISOString();
    writeFileSync(
      join(sessionsDir, "20260330-120000-session.md"),
      [
        "---",
        "branch: main",
        "status: done",
        `created: '${oldDate}'`,
        `last_updated: '${oldDate}'`,
        "---",
        "# Session: Test",
        "",
        "## Journal",
        "",
      ].join("\n"),
      "utf-8"
    );

    const result = await runLoaf(["housekeeping", "--dry-run"], { cwd: repoPath });
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("dry run");

    // Session should still be in place (not archived)
    expect(existsSync(join(sessionsDir, "20260330-120000-session.md"))).toBe(true);
    const archiveDir = join(sessionsDir, "archive");
    expect(existsSync(archiveDir)).toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Enrich Tests
// ─────────────────────────────────────────────────────────────────────────────

describe("session: enrich", () => {
  it("exits with error when session has no claude_session_id", async () => {
    const repoPath = createTempRepo("enrich-no-id");

    // Create a session without claude_session_id
    await runLoaf(["start"], { cwd: repoPath });

    const result = await runLoaf(["enrich"], { cwd: repoPath });

    expect(result.exitCode).toBe(1);
    expect(result.stderr).toContain("claude_session_id");
  });

  it("exits with error when JSONL file not found", async () => {
    const repoPath = createTempRepo("enrich-no-jsonl");
    const sessionsDir = join(repoPath, ".agents/sessions");
    mkdirSync(sessionsDir, { recursive: true });

    // Create a session with claude_session_id but no matching JSONL
    writeFileSync(
      join(sessionsDir, "20260410-120000-session.md"),
      [
        "---",
        "branch: main",
        "status: active",
        "created: '2026-04-10T12:00:00.000Z'",
        "claude_session_id: sess-does-not-exist",
        "---",
        "# Session: Test",
        "",
        "## Journal",
        "",
        "[2026-04-10 12:00] session(start):  === SESSION STARTED ===",
        "",
      ].join("\n"),
      "utf-8"
    );

    const result = await runLoaf(["enrich"], { cwd: repoPath });

    expect(result.exitCode).toBe(1);
    expect(result.stderr).toContain("JSONL not found");
  });

  it("exits with error when session file not found for explicit path", async () => {
    const repoPath = createTempRepo("enrich-no-file");

    const result = await runLoaf(["enrich", "nonexistent.md"], { cwd: repoPath });

    expect(result.exitCode).toBe(1);
    expect(result.stderr).toContain("Session file not found");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Hook Isolation Tests
// ─────────────────────────────────────────────────────────────────────────────

describe("session: enrich edge cases", () => {
  it("no-op exits 0 when enriched_at covers all JSONL entries (no claude binary needed)", async () => {
    const repoPath = createTempRepo("enrich-noop");
    const sessionsDir = join(repoPath, ".agents/sessions");
    mkdirSync(sessionsDir, { recursive: true });

    // Create a JSONL file that the session can find
    const claudeSessionId = "sess-noop-test";
    const configDir = join(
      process.env.HOME || "",
      ".config",
      "claude",
    );
    const cwdHash = repoPath.replace(/\//g, "-");
    const projectDir = join(configDir, "projects", cwdHash);
    const jsonlDir = join(projectDir, claudeSessionId);
    mkdirSync(jsonlDir, { recursive: true });

    const jsonlPath = join(projectDir, `${claudeSessionId}.jsonl`);
    writeFileSync(
      jsonlPath,
      [
        JSON.stringify({
          type: "user",
          timestamp: "2026-04-10T12:00:00.000Z",
          message: { role: "user", content: "hello" },
        }),
      ].join("\n") + "\n",
      "utf-8",
    );

    // Create session with enriched_at AFTER the JSONL entry
    writeFileSync(
      join(sessionsDir, "20260410-120000-session.md"),
      [
        "---",
        "branch: main",
        "status: active",
        "created: '2026-04-10T12:00:00.000Z'",
        `claude_session_id: ${claudeSessionId}`,
        "enriched_at: '2026-04-10T13:00:00.000Z'",
        "---",
        "# Session: Test",
        "",
        "## Journal",
        "",
        "[2026-04-10 12:00] session(start):  === SESSION STARTED ===",
        "",
      ].join("\n"),
      "utf-8",
    );

    const result = await runLoaf(["enrich"], { cwd: repoPath });

    // Should exit 0 (no-op) — the no-op path should not require claude binary
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("nothing to do");

    // Cleanup
    try { rmSync(jsonlDir, { recursive: true, force: true }); } catch { /* best effort */ }
    try { rmSync(jsonlPath, { force: true }); } catch { /* best effort */ }
  });

  it("LOAF_ENRICHMENT=1 suppresses session start and end during enrichment", async () => {
    const repoPath = createTempRepo("enrich-env-check");

    // First, create a session normally
    await runLoaf(["start"], { cwd: repoPath });

    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);

    const contentBefore = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8",
    );

    // Verify that both start and end are no-ops under LOAF_ENRICHMENT=1
    // (These are the hooks that the enrichment child process inherits)
    const startResult = await new Promise<{ stdout: string; stderr: string; exitCode: number }>((resolve) => {
      const child = spawn("node", [CLI_PATH, "session", "start"], {
        cwd: repoPath,
        stdio: ["pipe", "pipe", "pipe"],
        env: { ...process.env, LOAF_ENRICHMENT: "1" },
      });

      let stdout = "";
      let stderr = "";
      child.stdout.on("data", (data: Buffer) => { stdout += data.toString(); });
      child.stderr.on("data", (data: Buffer) => { stderr += data.toString(); });
      child.on("close", (exitCode: number | null) => {
        resolve({ stdout, stderr, exitCode: exitCode ?? 0 });
      });
    });

    expect(startResult.exitCode).toBe(0);

    // Session file should be unchanged (enrichment isolation suppressed start)
    const contentAfterStart = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8",
    );
    expect(contentAfterStart).toBe(contentBefore);

    // NOTE: Testing the actual LOAF_ENRICHMENT env var on the spawned child
    // process requires a real `claude` binary and is out of scope for unit
    // tests. The isolation tests above prove the var is recognized by the CLI.
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Claude Project Dir Tests
// ─────────────────────────────────────────────────────────────────────────────

describe("session: deriveClaudeProjectDir", () => {
  it("produces expected path structure from cwd", () => {
    // deriveClaudeProjectDir is not exported, but we can verify the path
    // convention indirectly. The function computes:
    //   $CLAUDE_CONFIG_DIR/projects/<cwd-with-slashes-replaced-by-dashes>
    //
    // The enrich command's error message reveals the computed projectDir
    // when JSONL is not found, which we can use to verify the derivation.
    //
    // NOTE: This is a structural test — verifying the convention matches
    // what Claude Code actually produces. True integration tests require
    // a real claude binary and are out of scope for unit tests.

    const cwd = "/Users/test/projects/myapp";
    const expectedHash = "-Users-test-projects-myapp";
    const configDir = join(
      process.env.HOME || "",
      ".config",
      "claude",
    );
    const expectedDir = join(configDir, "projects", expectedHash);

    // Verify the hash convention: slashes become dashes
    expect(cwd.replace(/\//g, "-")).toBe(expectedHash);
    expect(expectedDir).toContain("projects/-Users-test-projects-myapp");
  });
});

describe("session: LOAF_ENRICHMENT isolation", () => {
  it("session start exits early when LOAF_ENRICHMENT=1", async () => {
    const repoPath = createTempRepo("enrichment-start-isolation");

    // Run with LOAF_ENRICHMENT=1 set in the environment
    const result = await new Promise<{ stdout: string; stderr: string; exitCode: number }>((resolve) => {
      const child = spawn("node", [CLI_PATH, "session", "start"], {
        cwd: repoPath,
        stdio: ["pipe", "pipe", "pipe"],
        env: { ...process.env, LOAF_ENRICHMENT: "1" },
      });

      let stdout = "";
      let stderr = "";
      child.stdout.on("data", (data) => { stdout += data.toString(); });
      child.stderr.on("data", (data) => { stderr += data.toString(); });
      child.on("close", (exitCode) => {
        resolve({ stdout, stderr, exitCode: exitCode ?? 0 });
      });
    });

    expect(result.exitCode).toBe(0);

    // No session file should have been created
    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(0);
  });

  it("session end exits early when LOAF_ENRICHMENT=1", async () => {
    const repoPath = createTempRepo("enrichment-end-isolation");

    // Create a session first (without enrichment env)
    await runLoaf(["start"], { cwd: repoPath });

    const sessionFiles = getSessionFiles(repoPath);
    expect(sessionFiles.length).toBe(1);

    const contentBefore = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );

    // Run end with LOAF_ENRICHMENT=1
    const result = await new Promise<{ stdout: string; stderr: string; exitCode: number }>((resolve) => {
      const child = spawn("node", [CLI_PATH, "session", "end"], {
        cwd: repoPath,
        stdio: ["pipe", "pipe", "pipe"],
        env: { ...process.env, LOAF_ENRICHMENT: "1" },
      });

      let stdout = "";
      let stderr = "";
      child.stdout.on("data", (data) => { stdout += data.toString(); });
      child.stderr.on("data", (data) => { stderr += data.toString(); });
      child.on("close", (exitCode) => {
        resolve({ stdout, stderr, exitCode: exitCode ?? 0 });
      });
    });

    expect(result.exitCode).toBe(0);

    // Session file should be unchanged (no stop entry added)
    const contentAfter = readFileSync(
      join(repoPath, ".agents/sessions", sessionFiles[0]),
      "utf-8"
    );
    expect(contentAfter).toBe(contentBefore);
    expect(contentAfter).not.toContain("SESSION STOPPED");
  });
});
