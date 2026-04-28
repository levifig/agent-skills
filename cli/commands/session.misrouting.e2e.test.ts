/**
 * SPEC-032 E2E misrouting smoke test (TASK-119)
 *
 * Regression gate for the v2.0.0-dev.30 misrouting bug recorded in commit
 * `81b1808a chore: record session journals with dev.30 post-merge wrap
 * (#misrouted)`. That release shipped with `loaf session log` writing to the
 * wrong session file because the routing helper considered branch only.
 *
 * Fixture mirrors the live state observed during shaping:
 *
 *   1 active session  + 4 stopped sessions  on `main`
 *   ├── distinct claude_session_id per file
 *   └── each stopped file pre-seeded with a unique sentinel entry so a
 *       content hash can prove they remain byte-for-byte untouched.
 *
 * The test then exercises both the post-spec correct path (Tier 2 hook →
 * route by claude_session_id) and the degraded Tier 3 path (no signal →
 * branch fallback + stderr WARN). If a future change reintroduces
 * branch-only routing, *one* of the four stopped-file hashes will drift and
 * this test will fail.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { spawn } from "child_process";

// E2E tests spawn child processes — default 5s is too tight on slow CI runners
vi.setConfig({ testTimeout: 15000 });

import { createHash } from "crypto";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "fs";
import { execFileSync } from "child_process";
import { join } from "path";
import { tmpdir } from "os";

// ─────────────────────────────────────────────────────────────────────────────
// Test scaffolding
// ─────────────────────────────────────────────────────────────────────────────

let TEST_ROOT: string;
const CLI_PATH = join(process.cwd(), "dist-cli/index.js");

async function runLoaf(
  args: string[],
  options: { cwd: string; input?: string }
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

    if (options.input !== undefined) {
      child.stdin.write(options.input);
    }
    child.stdin.end();

    child.on("close", (exitCode) => {
      resolve({ stdout, stderr, exitCode: exitCode ?? 0 });
    });
  });
}

function createTempRepo(name: string): string {
  const repoPath = join(TEST_ROOT, name);
  mkdirSync(repoPath, { recursive: true });

  // Force `main` so SPEC-032 tests that assert `branch 'main'` literally are
  // portable across developers with `init.defaultBranch=master` configured.
  execFileSync("git", ["init", "--initial-branch=main"], {
    cwd: repoPath,
    stdio: "pipe",
  });
  execFileSync("git", ["config", "user.email", "test@test.com"], {
    cwd: repoPath,
    stdio: "pipe",
  });
  execFileSync("git", ["config", "user.name", "Test User"], {
    cwd: repoPath,
    stdio: "pipe",
  });

  writeFileSync(join(repoPath, "README.md"), "# Test\n", "utf-8");
  execFileSync("git", ["add", "."], { cwd: repoPath, stdio: "pipe" });
  execFileSync("git", ["commit", "-m", "Initial commit"], {
    cwd: repoPath,
    stdio: "pipe",
  });

  mkdirSync(join(repoPath, ".agents"), { recursive: true });
  writeFileSync(
    join(repoPath, ".agents/AGENTS.md"),
    "# Project Instructions\n",
    "utf-8"
  );
  return repoPath;
}

/** sha256 of a file's bytes — used to assert "not modified, byte-for-byte". */
function hashFile(filePath: string): string {
  return createHash("sha256")
    .update(readFileSync(filePath))
    .digest("hex");
}

function getActiveSessionFiles(repoPath: string): string[] {
  const sessionsDir = join(repoPath, ".agents/sessions");
  if (!existsSync(sessionsDir)) return [];
  return readdirSync(sessionsDir).filter(
    (f) => f.endsWith(".md") && !f.startsWith(".")
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Setup / Teardown
// ─────────────────────────────────────────────────────────────────────────────

beforeEach(() => {
  TEST_ROOT = realpathSync(mkdtempSync(join(tmpdir(), "loaf-test-misroute-")));
});

afterEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
});

// ─────────────────────────────────────────────────────────────────────────────
// Fixture
// ─────────────────────────────────────────────────────────────────────────────

interface FixtureSession {
  fileName: string;
  filePath: string;
  claudeSessionId: string;
  status: "active" | "stopped";
  /** sha256 captured immediately after the fixture is written. */
  initialHash: string;
}

interface Fixture {
  active: FixtureSession;
  stopped: FixtureSession[];
}

/**
 * Build the dev.30 fixture: 1 active + 4 stopped sessions on `main`, each
 * with a distinct claude_session_id and a unique sentinel entry so we can
 * detect any modification via content hash.
 */
function buildMisroutingFixture(repoPath: string): Fixture {
  const sessionsDir = join(repoPath, ".agents/sessions");
  mkdirSync(sessionsDir, { recursive: true });

  const stopped: FixtureSession[] = [];
  for (let i = 0; i < 4; i++) {
    const fileName = `20260401-${String(10 + i).padStart(2, "0")}0000-session.md`;
    const filePath = join(sessionsDir, fileName);
    const claudeSessionId = `claude-stopped-${i}`;
    const sentinel = `discover(stopped-${i}): pre-existing entry that must not move`;

    writeFileSync(
      filePath,
      [
        "---",
        "branch: main",
        "status: stopped",
        `claude_session_id: ${claudeSessionId}`,
        `created: '2026-04-01T${String(10 + i).padStart(2, "0")}:00:00.000Z'`,
        `last_updated: '2026-04-01T${String(10 + i).padStart(2, "0")}:30:00.000Z'`,
        `last_entry: '2026-04-01T${String(10 + i).padStart(2, "0")}:30:00.000Z'`,
        "---",
        `# Session: Stopped ${i}`,
        "",
        "## Journal",
        "",
        `[2026-04-01 ${String(10 + i).padStart(2, "0")}:00] session(start):  === SESSION STARTED ===`,
        `[2026-04-01 ${String(10 + i).padStart(2, "0")}:15] ${sentinel}`,
        `[2026-04-01 ${String(10 + i).padStart(2, "0")}:30] session(stop):   === SESSION STOPPED ===`,
        "",
      ].join("\n"),
      "utf-8"
    );

    stopped.push({
      fileName,
      filePath,
      claudeSessionId,
      status: "stopped",
      initialHash: hashFile(filePath),
    });
  }

  const activeFileName = "20260401-140000-session.md";
  const activeFilePath = join(sessionsDir, activeFileName);
  const activeSessionId = "claude-active";

  writeFileSync(
    activeFilePath,
    [
      "---",
      "branch: main",
      "status: active",
      `claude_session_id: ${activeSessionId}`,
      "created: '2026-04-01T14:00:00.000Z'",
      "last_updated: '2026-04-01T14:00:00.000Z'",
      "last_entry: '2026-04-01T14:00:00.000Z'",
      "---",
      "# Session: Active",
      "",
      "## Journal",
      "",
      "[2026-04-01 14:00] session(start):  === SESSION STARTED ===",
      "[2026-04-01 14:01] discover(active): pre-existing entry on the active session",
      "",
    ].join("\n"),
    "utf-8"
  );

  return {
    active: {
      fileName: activeFileName,
      filePath: activeFilePath,
      claudeSessionId: activeSessionId,
      status: "active",
      initialHash: hashFile(activeFilePath),
    },
    stopped,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────────

describe("SPEC-032 dev.30 misrouting regression (TASK-119)", () => {
  it("hook with active session_id routes ONLY to the active session — 4 stopped files unmodified (content-hash gate)", async () => {
    const repoPath = createTempRepo("misroute-tier2-hash");
    const fixture = buildMisroutingFixture(repoPath);

    expect(getActiveSessionFiles(repoPath).length).toBe(5); // sanity

    // Make a real commit so the hook's git-log fallback returns a useful
    // message for the entry text.
    writeFileSync(join(repoPath, "feat.ts"), "export const z = 3;\n", "utf-8");
    execFileSync("git", ["add", "."], { cwd: repoPath, stdio: "pipe" });
    execFileSync("git", ["commit", "-m", "feat: dev.30 misroute regression"], {
      cwd: repoPath,
      stdio: "pipe",
    });

    const result = await runLoaf(["log", "--from-hook"], {
      cwd: repoPath,
      input: JSON.stringify({
        session_id: fixture.active.claudeSessionId,
        tool_input: {
          command: "git commit -m 'feat: dev.30 misroute regression'",
        },
      }),
    });

    expect(result.exitCode).toBe(0);
    // Hook-driven path with valid session_id MUST NOT emit the WARN.
    expect(result.stderr).not.toContain("WARN: no session_id signal");

    // Active session got the new entry.
    const activeContent = readFileSync(fixture.active.filePath, "utf-8");
    expect(activeContent).toContain("feat: dev.30 misroute regression");
    expect(activeContent).toContain("commit(");
    // Active hash MUST have changed (entry was appended).
    expect(hashFile(fixture.active.filePath)).not.toBe(fixture.active.initialHash);

    // Each stopped session is byte-for-byte identical to its initial state.
    for (const s of fixture.stopped) {
      expect(hashFile(s.filePath)).toBe(s.initialHash);
      // Sentinel survives, new commit entry never landed here.
      const content = readFileSync(s.filePath, "utf-8");
      expect(content).toContain(`discover(stopped-${fixture.stopped.indexOf(s)}): pre-existing entry that must not move`);
      expect(content).not.toContain("feat: dev.30 misroute regression");
    }

    // Total file count unchanged — no command should create new session files.
    expect(getActiveSessionFiles(repoPath).length).toBe(5);
  });

  it("no signal (Tier 3) writes to active session AND emits exact stderr WARN literal", async () => {
    const repoPath = createTempRepo("misroute-tier3-warn");
    const fixture = buildMisroutingFixture(repoPath);

    // Snapshot stopped hashes (active will mutate; stopped must not).
    const stoppedHashesBefore = fixture.stopped.map((s) => hashFile(s.filePath));

    // TASK-119 acceptance criterion uses the wording "regression(test): no
    // signal" but `regression` isn't an accepted entry type in `parseEntry`
    // (see cli/commands/session.ts validTypes). Use `discover(regression):
    // no signal` so the entry passes validation while still naming the
    // dev.30 incident in the journal.
    const result = await runLoaf(
      ["log", "discover(regression): dev.30 no-signal Tier 3 path"],
      { cwd: repoPath }
    );

    expect(result.exitCode).toBe(0);

    // Exact WARN literal from cli/lib/session/resolve.ts — must match
    // character-for-character so future formatting drift is loud.
    expect(result.stderr).toContain(
      "WARN: no session_id signal — falling back to branch routing for branch 'main'. Pass --session-id <id> to silence."
    );

    // The new entry landed in the active session.
    const activeContent = readFileSync(fixture.active.filePath, "utf-8");
    expect(activeContent).toContain("discover(regression): dev.30 no-signal Tier 3 path");

    // None of the four stopped sessions changed under Tier 3 either.
    for (let i = 0; i < fixture.stopped.length; i++) {
      expect(hashFile(fixture.stopped[i].filePath)).toBe(stoppedHashesBefore[i]);
    }
  });

  it("--session-id targeting active (Tier 1) routes correctly, no WARN, stopped untouched", async () => {
    const repoPath = createTempRepo("misroute-tier1-flag");
    const fixture = buildMisroutingFixture(repoPath);

    const result = await runLoaf(
      [
        "log",
        "decision(test): tier1 explicit override",
        "--session-id",
        fixture.active.claudeSessionId,
      ],
      { cwd: repoPath }
    );

    expect(result.exitCode).toBe(0);
    expect(result.stderr).not.toContain("WARN: no session_id signal");

    const activeContent = readFileSync(fixture.active.filePath, "utf-8");
    expect(activeContent).toContain("decision(test): tier1 explicit override");

    for (const s of fixture.stopped) {
      expect(hashFile(s.filePath)).toBe(s.initialHash);
    }
  });

  it("bogus --session-id with valid hook session_id falls through Tier 1→Tier 2 (no WARN)", async () => {
    // The TASK-117 review finding: action body used to coalesce flag+stdin
    // into a single sessionIdFlag, silently demoting the chain to 2-tier.
    // After SPEC-032's call-site fix, a present-but-invalid `--session-id`
    // must still let a valid stdin id win.
    const repoPath = createTempRepo("misroute-flag-fallthrough");
    const fixture = buildMisroutingFixture(repoPath);

    writeFileSync(join(repoPath, "x.ts"), "export const x = 1;\n", "utf-8");
    execFileSync("git", ["add", "."], { cwd: repoPath, stdio: "pipe" });
    execFileSync("git", ["commit", "-m", "chore: tier1 to tier2 fallthrough"], {
      cwd: repoPath,
      stdio: "pipe",
    });

    const result = await runLoaf(
      ["log", "--from-hook", "--session-id", "no-such-session-id"],
      {
        cwd: repoPath,
        input: JSON.stringify({
          session_id: fixture.active.claudeSessionId,
          tool_input: {
            command: "git commit -m 'chore: tier1 to tier2 fallthrough'",
          },
        }),
      }
    );

    expect(result.exitCode).toBe(0);
    // Critical: even though Tier 1 was a miss, Tier 2 (stdin) succeeded —
    // so NO WARN. If this fires, the chain has collapsed to 2-tier again.
    expect(result.stderr).not.toContain("WARN: no session_id signal");

    const activeContent = readFileSync(fixture.active.filePath, "utf-8");
    expect(activeContent).toContain("commit(");
    expect(activeContent).toContain("chore: tier1 to tier2 fallthrough");

    for (const s of fixture.stopped) {
      expect(hashFile(s.filePath)).toBe(s.initialHash);
    }
  });

  it("--from-hook with empty stdin is a silent no-op (no WARN, no entry, no error)", async () => {
    // The TASK-117 review finding: `--from-hook` with no piped JSON used to
    // call resolveCurrentSession → Tier 3 WARN, then exit 0 having logged
    // nothing. The WARN was misleading (no session was actually misrouted —
    // there was no entry to misroute). After the fix, the action body
    // exits early before any chain runs.
    const repoPath = createTempRepo("misroute-empty-hook-stdin");
    const fixture = buildMisroutingFixture(repoPath);

    const result = await runLoaf(["log", "--from-hook"], {
      cwd: repoPath,
      input: "", // empty stdin
    });

    expect(result.exitCode).toBe(0);
    expect(result.stderr).not.toContain("WARN: no session_id signal");
    // Sanity: nothing logged means active session is byte-for-byte unchanged.
    expect(hashFile(fixture.active.filePath)).toBe(fixture.active.initialHash);
    for (const s of fixture.stopped) {
      expect(hashFile(s.filePath)).toBe(s.initialHash);
    }
  });

  it("--from-hook --session-id <active> with empty stdin still honors Tier 1 (no silent no-op)", async () => {
    // Codex review of commit 763bb393: the empty-stdin guard fired
    // unconditionally when --from-hook was set, silently no-opping even
    // when the caller had supplied an explicit `--session-id` Tier 1
    // override. That breaks the very invariant SPEC-032 was built to
    // protect: a present `--session-id` is the strongest signal and must
    // never be discarded.
    //
    // After the fix, the guard checks `!options.sessionId` too — so a
    // flag-set + empty-stdin call falls through to the chain, Tier 1
    // wins, and the entry lands in the targeted session with no WARN.
    const repoPath = createTempRepo("misroute-flag-empty-stdin");
    const fixture = buildMisroutingFixture(repoPath);

    const result = await runLoaf(
      [
        "log",
        "decision(test): tier1 wins despite empty hook stdin",
        "--from-hook",
        "--session-id",
        fixture.active.claudeSessionId,
      ],
      {
        cwd: repoPath,
        input: "", // empty stdin — the bug condition
      }
    );

    expect(result.exitCode).toBe(0);
    expect(result.stderr).not.toContain("WARN: no session_id signal");

    // Entry lands in the active session targeted by the flag.
    const activeContent = readFileSync(fixture.active.filePath, "utf-8");
    expect(activeContent).toContain(
      "decision(test): tier1 wins despite empty hook stdin"
    );
    // Active hash MUST have changed — if the silent no-op regression
    // returned, the file would be byte-identical to its initial state.
    expect(hashFile(fixture.active.filePath)).not.toBe(fixture.active.initialHash);

    // Stopped sessions remain untouched.
    for (const s of fixture.stopped) {
      expect(hashFile(s.filePath)).toBe(s.initialHash);
    }
  });
});
