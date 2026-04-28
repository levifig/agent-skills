/**
 * Session Resolve Tests
 *
 * Unit tests for `parseHookSessionId` and `resolveCurrentSession` (SPEC-032).
 *
 * Strategy:
 *   - Real session fixtures on disk for the resolution chain (tiers 1/2/3).
 *   - `fs.readFileSync` spy for stdin parsing (fd 0 reads).
 *   - `process.stderr.write` spy for the WARN assertion.
 */

import {
  describe,
  it,
  expect,
  beforeEach,
  afterEach,
  vi,
} from "vitest";

// Mock fs so we can intercept readFileSync(0, ...) — fd 0 is stdin and the
// real implementation would block waiting for terminal input. We delegate
// every other path/fd to the real fs via `vi.importActual` inside the factory
// and stash the original on the mock itself so the test setup can fall
// through for non-stdin reads.
vi.mock("fs", async () => {
  const actual = await vi.importActual<typeof import("fs")>("fs");
  const realReadFileSync = actual.readFileSync;
  const readFileSync = vi.fn((path: unknown, options?: unknown) => {
    return realReadFileSync(
      path as Parameters<typeof realReadFileSync>[0],
      options as Parameters<typeof realReadFileSync>[1],
    );
  });
  // Stash the real implementation on the mock so test code can recover it
  // without re-importing fs (which would re-enter the mock).
  (readFileSync as unknown as { __real: typeof realReadFileSync }).__real = realReadFileSync;
  return {
    ...actual,
    readFileSync,
    default: { ...actual, readFileSync },
  };
});

import * as fs from "fs";
import {
  mkdirSync,
  mkdtempSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "fs";
import { join } from "path";
import { tmpdir } from "os";
import matter from "gray-matter";

import {
  _parseHookSessionId,
  resolveCurrentSession,
} from "./resolve.js";
import type { SessionFrontmatter } from "./store.js";

const mockedReadFileSync = fs.readFileSync as unknown as ReturnType<typeof vi.fn>;
const realReadFileSync = (mockedReadFileSync as unknown as {
  __real: typeof fs.readFileSync;
}).__real;

// ─────────────────────────────────────────────────────────────────────────────
// Fixtures
// ─────────────────────────────────────────────────────────────────────────────

let TEST_ROOT: string;
let AGENTS_DIR: string;
let SESSIONS_DIR: string;

const BRANCH = "feat/example";

const WARN_FOR_BRANCH = (branch: string) =>
  `WARN: no session_id signal — falling back to branch routing for branch '${branch}'. Pass --session-id <id> to silence.\n`;

interface SessionSeed {
  fileName: string;
  branch?: string;
  status?: SessionFrontmatter["status"];
  claude_session_id?: string;
  last_updated?: string;
  last_entry?: string;
}

function writeSessionFile(seed: SessionSeed): string {
  const data: SessionFrontmatter = {
    branch: seed.branch ?? BRANCH,
    status: seed.status ?? "active",
    created: "2026-04-27T22:00:00.000Z",
    last_updated: seed.last_updated ?? "2026-04-27T22:30:00.000Z",
    last_entry: seed.last_entry ?? "2026-04-27T22:30:00.000Z",
  };
  if (seed.claude_session_id) data.claude_session_id = seed.claude_session_id;

  const body = `# Session\n\n## Journal\n\n[2026-04-27 22:00] session(start): === SESSION STARTED ===\n`;
  const content = matter.stringify(body, data as unknown as Record<string, unknown>);

  const filePath = join(SESSIONS_DIR, seed.fileName);
  writeFileSync(filePath, content, "utf-8");
  return filePath;
}

function mockStdin(payload: string | undefined): void {
  // Override the readFileSync mock for fd-0 reads while delegating every
  // other path/fd to the real fs.readFileSync (captured via vi.hoisted so
  // it bypasses the module mock).
  mockedReadFileSync.mockImplementation((path: unknown, options?: unknown) => {
    if (path === 0) {
      if (payload === undefined) {
        throw new Error("EOF");
      }
      return payload;
    }
    return realReadFileSync(
      path as Parameters<typeof realReadFileSync>[0],
      options as Parameters<typeof realReadFileSync>[1],
    );
  });
}

function captureStderr(): { lines: string[]; restore: () => void } {
  const lines: string[] = [];
  const spy = vi
    .spyOn(process.stderr, "write")
    .mockImplementation(((chunk: unknown) => {
      lines.push(String(chunk));
      return true;
    }) as unknown as typeof process.stderr.write);
  return {
    lines,
    restore: () => spy.mockRestore(),
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// Setup / Teardown
// ─────────────────────────────────────────────────────────────────────────────

beforeEach(() => {
  TEST_ROOT = realpathSync(mkdtempSync(join(tmpdir(), "loaf-test-resolve-")));
  AGENTS_DIR = join(TEST_ROOT, ".agents");
  SESSIONS_DIR = join(AGENTS_DIR, "sessions");
  mkdirSync(SESSIONS_DIR, { recursive: true });
});

afterEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
  vi.restoreAllMocks();
});

// ─────────────────────────────────────────────────────────────────────────────
// _parseHookSessionId — internal helper; covered here for edge-case
// completeness. Public callers must go through `resolveCurrentSession`.
// ─────────────────────────────────────────────────────────────────────────────

describe("_parseHookSessionId", () => {
  it("returns session_id from valid hook JSON", () => {
    mockStdin(JSON.stringify({ session_id: "abc-123", other: "field" }));
    expect(_parseHookSessionId()).toBe("abc-123");
  });

  it("returns undefined for empty stdin", () => {
    mockStdin("");
    expect(_parseHookSessionId()).toBeUndefined();
  });

  it("returns undefined for whitespace-only stdin", () => {
    mockStdin("   \n  ");
    expect(_parseHookSessionId()).toBeUndefined();
  });

  it("returns undefined for malformed JSON", () => {
    mockStdin("{not valid json");
    expect(_parseHookSessionId()).toBeUndefined();
  });

  it("returns undefined when session_id field is missing", () => {
    mockStdin(JSON.stringify({ tool_name: "Bash", cwd: "/tmp" }));
    expect(_parseHookSessionId()).toBeUndefined();
  });

  it("returns undefined when session_id is not a string", () => {
    mockStdin(JSON.stringify({ session_id: 12345 }));
    expect(_parseHookSessionId()).toBeUndefined();
  });

  it("returns undefined when session_id is an empty string", () => {
    mockStdin(JSON.stringify({ session_id: "" }));
    expect(_parseHookSessionId()).toBeUndefined();
  });

  it("returns undefined when stdin read throws", () => {
    mockStdin(undefined);
    expect(_parseHookSessionId()).toBeUndefined();
  });

  it("returns undefined for non-object JSON (array)", () => {
    mockStdin(JSON.stringify(["session_id", "abc"]));
    expect(_parseHookSessionId()).toBeUndefined();
  });

  it("returns undefined for non-object JSON (string)", () => {
    mockStdin(JSON.stringify("just-a-string"));
    expect(_parseHookSessionId()).toBeUndefined();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// resolveCurrentSession — Tier 1: --session-id flag
// ─────────────────────────────────────────────────────────────────────────────

describe("resolveCurrentSession — Tier 1 (sessionIdFlag)", () => {
  it("resolves to the session matching the flag, no stderr", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "session-A",
    });
    writeSessionFile({
      fileName: "20260427-221000-session.md",
      claude_session_id: "session-B",
    });

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, BRANCH, {
      sessionIdFlag: "session-B",
    });
    stderr.restore();

    expect(result).not.toBeNull();
    expect(result?.data.claude_session_id).toBe("session-B");
    expect(stderr.lines).toEqual([]);
  });

  it("falls through to Tier 3 (with WARN) when flag matches no session", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "session-A",
    });

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, BRANCH, {
      sessionIdFlag: "missing-id",
    });
    stderr.restore();

    expect(result).not.toBeNull();
    expect(result?.data.claude_session_id).toBe("session-A"); // branch fallback
    expect(stderr.lines).toEqual([WARN_FOR_BRANCH(BRANCH)]);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// resolveCurrentSession — Tier 2: parseStdin
// ─────────────────────────────────────────────────────────────────────────────

describe("resolveCurrentSession — Tier 2 (parseStdin)", () => {
  it("resolves via stdin session_id, no stderr", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "session-from-hook",
    });

    mockStdin(JSON.stringify({ session_id: "session-from-hook" }));
    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, BRANCH, {
      parseStdin: true,
    });
    stderr.restore();

    expect(result).not.toBeNull();
    expect(result?.data.claude_session_id).toBe("session-from-hook");
    expect(stderr.lines).toEqual([]);
  });

  it("falls through to Tier 3 (with WARN) when stdin is empty", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "session-A",
    });

    mockStdin("");
    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, BRANCH, {
      parseStdin: true,
    });
    stderr.restore();

    expect(result).not.toBeNull();
    expect(result?.data.claude_session_id).toBe("session-A");
    expect(stderr.lines).toEqual([WARN_FOR_BRANCH(BRANCH)]);
  });

  it("falls through to Tier 3 (with WARN) when stdin JSON is malformed", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "session-A",
    });

    mockStdin("{not json}");
    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, BRANCH, {
      parseStdin: true,
    });
    stderr.restore();

    expect(result).not.toBeNull();
    expect(result?.data.claude_session_id).toBe("session-A");
    expect(stderr.lines).toEqual([WARN_FOR_BRANCH(BRANCH)]);
  });

  it("falls through to Tier 3 (with WARN) when session_id field is missing from stdin", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "session-A",
    });

    mockStdin(JSON.stringify({ tool_name: "Bash" }));
    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, BRANCH, {
      parseStdin: true,
    });
    stderr.restore();

    expect(result).not.toBeNull();
    expect(result?.data.claude_session_id).toBe("session-A");
    expect(stderr.lines).toEqual([WARN_FOR_BRANCH(BRANCH)]);
  });

  it("falls through to Tier 3 (with WARN) when stdin id matches no session", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "session-A",
    });

    mockStdin(JSON.stringify({ session_id: "no-such-session" }));
    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, BRANCH, {
      parseStdin: true,
    });
    stderr.restore();

    expect(result).not.toBeNull();
    expect(result?.data.claude_session_id).toBe("session-A");
    expect(stderr.lines).toEqual([WARN_FOR_BRANCH(BRANCH)]);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// resolveCurrentSession — Tier 3: branch fallback
// ─────────────────────────────────────────────────────────────────────────────

describe("resolveCurrentSession — Tier 3 (branch fallback)", () => {
  it("resolves via branch when no flag and parseStdin=false, emits WARN", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "session-A",
    });

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, BRANCH, {});
    stderr.restore();

    expect(result).not.toBeNull();
    expect(result?.data.claude_session_id).toBe("session-A");
    expect(stderr.lines).toEqual([WARN_FOR_BRANCH(BRANCH)]);
  });

  it("adopts a single active session on the wrong branch (heuristic) and emits WARN", async () => {
    // Seed exactly ONE active session on a different branch — the
    // findActiveSessionForBranch adoption heuristic re-tags this session to
    // the requested branch. This test pins that behavior.
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "other-branch",
      claude_session_id: "session-A",
    });

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, "feat/missing", {});
    stderr.restore();

    expect(stderr.lines).toEqual([WARN_FOR_BRANCH("feat/missing")]);
    expect(result).not.toBeNull();
    expect(result?.data.claude_session_id).toBe("session-A");
  });

  it("emits WARN even when all tiers fail to resolve (multiple branches, no match)", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "branch-a",
      claude_session_id: "session-A",
    });
    writeSessionFile({
      fileName: "20260427-221000-session.md",
      branch: "branch-b",
      claude_session_id: "session-B",
    });

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, "branch-c", {});
    stderr.restore();

    // With multiple active sessions and no branch match, the adoption heuristic
    // doesn't fire (it requires exactly one active session) — so result is null.
    expect(result).toBeNull();
    expect(stderr.lines).toEqual([WARN_FOR_BRANCH("branch-c")]);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// resolveCurrentSession — Tier 2 (stdinSessionIdHint)
//
// The hint variant is for callers that have already consumed stdin upstream
// (e.g., `loaf session log --from-hook` reads stdin once for both routing and
// entry-text extraction). The chain order — flag → stdin → branch — must be
// preserved when the hint is supplied; collapsing flag+hint into a single
// signal silently demoted the chain to 2-tier in TASK-117.
// ─────────────────────────────────────────────────────────────────────────────

describe("resolveCurrentSession — Tier 2 (stdinSessionIdHint)", () => {
  it("uses the hint as Tier 2 when present, no stderr", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "session-from-hint",
    });

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, BRANCH, {
      stdinSessionIdHint: "session-from-hint",
    });
    stderr.restore();

    expect(result).not.toBeNull();
    expect(result?.data.claude_session_id).toBe("session-from-hint");
    expect(stderr.lines).toEqual([]);
  });

  it("flag wins over hint (Tier 1 beats Tier 2), no stderr", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "session-flag",
    });
    writeSessionFile({
      fileName: "20260427-221000-session.md",
      claude_session_id: "session-hint",
    });

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, BRANCH, {
      sessionIdFlag: "session-flag",
      stdinSessionIdHint: "session-hint",
    });
    stderr.restore();

    expect(result?.data.claude_session_id).toBe("session-flag");
    expect(stderr.lines).toEqual([]);
  });

  it("falls through from bad flag to valid hint (Tier 1 → Tier 2), no WARN", async () => {
    // The TASK-117 review finding: a present-but-bogus `--session-id` plus a
    // valid hook stdin id must reach the hint, not collapse to branch routing.
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "session-real",
    });

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, BRANCH, {
      sessionIdFlag: "no-such-id",
      stdinSessionIdHint: "session-real",
    });
    stderr.restore();

    expect(result?.data.claude_session_id).toBe("session-real");
    expect(stderr.lines).toEqual([]);
  });

  it("falls through bad flag AND bad hint to branch fallback with WARN", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "branch-only",
    });

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, BRANCH, {
      sessionIdFlag: "no-such-id",
      stdinSessionIdHint: "also-no-such-id",
    });
    stderr.restore();

    expect(result?.data.claude_session_id).toBe("branch-only");
    expect(stderr.lines).toEqual([WARN_FOR_BRANCH(BRANCH)]);
  });

  it("treats empty-string hint as no signal (falls through to branch)", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "branch-only",
    });

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, BRANCH, {
      stdinSessionIdHint: "",
    });
    stderr.restore();

    expect(result?.data.claude_session_id).toBe("branch-only");
    expect(stderr.lines).toEqual([WARN_FOR_BRANCH(BRANCH)]);
  });

  it("hint wins over parseStdin (no double-read of fd 0)", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "from-hint",
    });

    // If parseStdin were honored, fd 0 would be read here. mockStdin throws
    // on read so the test would fail loudly. Hint must short-circuit.
    mockStdin(undefined);

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, BRANCH, {
      parseStdin: true,
      stdinSessionIdHint: "from-hint",
    });
    stderr.restore();

    expect(result?.data.claude_session_id).toBe("from-hint");
    expect(stderr.lines).toEqual([]);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// resolveCurrentSession — WARN routing
// ─────────────────────────────────────────────────────────────────────────────

describe("resolveCurrentSession — WARN routing", () => {
  it("WARN goes to stderr (not stdout)", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "session-A",
    });

    const stderrSpy = vi
      .spyOn(process.stderr, "write")
      .mockImplementation((() => true) as unknown as typeof process.stderr.write);
    const stdoutSpy = vi
      .spyOn(process.stdout, "write")
      .mockImplementation((() => true) as unknown as typeof process.stdout.write);

    await resolveCurrentSession(AGENTS_DIR, BRANCH, {});

    expect(stderrSpy).toHaveBeenCalledWith(WARN_FOR_BRANCH(BRANCH));
    // Confirm WARN did not leak to stdout
    const stdoutCalls = stdoutSpy.mock.calls.map((c) => String(c[0]));
    expect(stdoutCalls.some((line) => line.includes("WARN"))).toBe(false);

    stderrSpy.mockRestore();
    stdoutSpy.mockRestore();
  });

  it("WARN message contains exact literal text", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "session-A",
    });

    const stderr = captureStderr();
    await resolveCurrentSession(AGENTS_DIR, "feat/exact-test", {});
    stderr.restore();

    expect(stderr.lines).toHaveLength(1);
    const line = stderr.lines[0];
    expect(line).toContain("WARN: no session_id signal");
    expect(line).toContain("falling back to branch routing for branch 'feat/exact-test'");
    expect(line).toContain("Pass --session-id <id> to silence.");
  });
});
