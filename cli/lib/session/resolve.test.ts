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
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "fs";
import { execFileSync } from "child_process";
import { createHash } from "crypto";
import { basename, join } from "path";
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

/**
 * Adoption WARN — fires when fallback resolves to a session on a different
 * branch. The wording depends on how the finder resolved:
 *   - `most-recent-active`: heuristic adoption across all active sessions.
 *   - `rename-link`: the current branch is a `git branch -m` rename of the
 *     session's origin branch (detected via reflog).
 */
const WARN_ADOPTION_MOST_RECENT = (
  branch: string,
  filePath: string,
  originBranch: string
) =>
  `WARN: no session for branch '${branch}'; logging to most-recent active session '${basename(filePath)}' (origin branch '${originBranch}'). Pass --session-id <id> to silence.\n`;

const WARN_ADOPTION_RENAME_LINK = (
  branch: string,
  filePath: string,
  originBranch: string
) =>
  `WARN: branch '${branch}' appears to be a rename of '${originBranch}'; logging to its session '${basename(filePath)}'. Pass --session-id <id> to silence.\n`;

/** Zero-active WARN — fires when fallback finds nothing. */
const WARN_NO_ACTIVE = (branch: string) =>
  `WARN: no session for branch '${branch}'; no active sessions to fall back to. Pass --session-id <id> to silence.\n`;

interface SessionSeed {
  fileName: string;
  branch?: string;
  status?: SessionFrontmatter["status"];
  claude_session_id?: string;
  last_updated?: string;
  last_entry?: string;
}

function fileHash(path: string): string {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
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

  it("adopts a lone active session on a different branch and emits adoption WARN (SPEC-042 Track B)", async () => {
    // Seed exactly ONE active session on a different branch. Under SPEC-042
    // Track B the finder no longer re-tags the session — it returns the
    // session as-is and the resolver emits an adoption WARN naming both the
    // requested branch and the session's origin branch.
    const filePath = writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "other-branch",
      claude_session_id: "session-A",
    });
    const before = fileHash(filePath);

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, "feat/missing", {});
    stderr.restore();

    expect(result).not.toBeNull();
    expect(result?.data.claude_session_id).toBe("session-A");
    // Origin branch preserved — no mutation.
    expect(result?.data.branch).toBe("other-branch");
    expect(fileHash(filePath)).toBe(before);
    expect(stderr.lines).toEqual([
      WARN_ADOPTION_MOST_RECENT("feat/missing", filePath, "other-branch"),
    ]);
  });

  it("adopts the most-recent active session when multiple are active on different branches (SPEC-042 Track B)", async () => {
    // Multi-worktree case: more than one active session, none matching the
    // requested branch. The finder must adopt the most-recent-by-last_updated
    // rather than returning null. This pins SPEC-042 Track B Bug B2.
    const olderPath = writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "branch-a",
      claude_session_id: "session-A",
      last_updated: "2026-04-27T22:00:00.000Z",
    });
    const newerPath = writeSessionFile({
      fileName: "20260427-221000-session.md",
      branch: "branch-b",
      claude_session_id: "session-B",
      last_updated: "2026-04-27T22:30:00.000Z",
    });
    const olderBefore = fileHash(olderPath);
    const newerBefore = fileHash(newerPath);

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, "branch-c", {});
    stderr.restore();

    expect(result?.data.claude_session_id).toBe("session-B");
    expect(result?.data.branch).toBe("branch-b"); // unchanged
    // Neither file mutated.
    expect(fileHash(olderPath)).toBe(olderBefore);
    expect(fileHash(newerPath)).toBe(newerBefore);
    expect(stderr.lines).toEqual([
      WARN_ADOPTION_MOST_RECENT("branch-c", newerPath, "branch-b"),
    ]);
  });

  it("emits distinct WARN and returns null when no active sessions exist (SPEC-042 Track B)", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "branch-a",
      status: "stopped",
      claude_session_id: "session-A",
    });
    writeSessionFile({
      fileName: "20260427-221000-session.md",
      branch: "branch-b",
      status: "done",
      claude_session_id: "session-B",
    });

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, "branch-c", {});
    stderr.restore();

    expect(result).toBeNull();
    expect(stderr.lines).toEqual([WARN_NO_ACTIVE("branch-c")]);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// SPEC-042 Track B integration — release-agent scenario
// ─────────────────────────────────────────────────────────────────────────────

describe("resolveCurrentSession — SPEC-042 release agent scenario", () => {
  it("release-agent branch with orchestrator session active: routes to orchestrator, no mutation, names both branches in WARN", async () => {
    // Simulates a release agent running in a worktree on `release/v0.16.0`
    // while an orchestrator session is active on `cwt/foo`. Before SPEC-042
    // Track B this either silently dropped logs (multi-active) or rewrote
    // the orchestrator's `branch:` to `release/v0.16.0` (single-active).
    // Now: orchestrator session is returned, frontmatter untouched, and WARN
    // names both branches.
    const orchestratorPath = writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "cwt/foo",
      claude_session_id: "orchestrator",
      last_updated: "2026-04-27T22:30:00.000Z",
    });
    const before = fileHash(orchestratorPath);

    const stderr = captureStderr();
    const result = await resolveCurrentSession(
      AGENTS_DIR,
      "release/v0.16.0",
      {}
    );
    stderr.restore();

    expect(result).not.toBeNull();
    expect(result?.data.claude_session_id).toBe("orchestrator");
    expect(result?.data.branch).toBe("cwt/foo"); // origin unchanged
    expect(fileHash(orchestratorPath)).toBe(before);
    expect(stderr.lines).toEqual([
      WARN_ADOPTION_MOST_RECENT(
        "release/v0.16.0",
        orchestratorPath,
        "cwt/foo"
      ),
    ]);
    // WARN names both branches.
    const line = stderr.lines[0];
    expect(line).toContain("release/v0.16.0");
    expect(line).toContain("cwt/foo");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// resolveCurrentSession — rename-link WARN (SPEC-042 Track B)
//
// The finder detects `git branch -m old new` via the current branch's reflog
// and resolves through the spec linked to the old branch name. The resulting
// WARN must say "appears to be a rename of <old>" — "most-recent active" is
// factually wrong here because the lookup never consulted recency.
//
// Fixture: real on-disk git repo created in beforeEach so `git reflog` returns
// a real "Branch: renamed" line. `process.chdir` because find.ts execs git
// without an explicit cwd.
// ─────────────────────────────────────────────────────────────────────────────

describe("resolveCurrentSession — rename-link WARN", () => {
  let originalCwd: string;

  beforeEach(() => {
    originalCwd = process.cwd();
    // Init a fresh repo inside TEST_ROOT so reflog is unpolluted by the
    // host worktree's history. Configure user.* locally to avoid global
    // git config noise on CI.
    execFileSync("git", ["init", "-q", "--initial-branch=feat/old"], {
      cwd: TEST_ROOT,
    });
    execFileSync("git", ["config", "user.email", "test@example.com"], {
      cwd: TEST_ROOT,
    });
    execFileSync("git", ["config", "user.name", "Test"], { cwd: TEST_ROOT });
    execFileSync("git", ["commit", "--allow-empty", "-q", "-m", "init"], {
      cwd: TEST_ROOT,
    });
    // The rename `feat/old -> feat/new` shows up in the *new* branch's reflog,
    // which is what find.ts greps for.
    execFileSync("git", ["branch", "-m", "feat/old", "feat/new"], {
      cwd: TEST_ROOT,
    });
    process.chdir(TEST_ROOT);
  });

  afterEach(() => {
    process.chdir(originalCwd);
  });

  it("rename-link adoption emits 'appears to be a rename of' WARN, not 'most-recent active'", async () => {
    // Seed a session on the OLD branch and a spec that links it. The finder
    // walks: branch lookup fails → reflog match (feat/new from feat/old) →
    // find spec where branch=feat/old → follow spec.session pointer.
    const sessionFile = "20260427-220000-session.md";
    const sessionPath = writeSessionFile({
      fileName: sessionFile,
      branch: "feat/old",
      claude_session_id: "session-pre-rename",
    });

    const specsDir = join(AGENTS_DIR, "specs");
    mkdirSync(specsDir, { recursive: true });
    const specContent = matter.stringify(
      "# Spec body\n",
      {
        id: "SPEC-TEST",
        title: "Rename-link fixture",
        branch: "feat/old",
        session: sessionFile,
      } as unknown as Record<string, unknown>
    );
    writeFileSync(join(specsDir, "SPEC-TEST.md"), specContent, "utf-8");

    const beforeHash = fileHash(sessionPath);

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, "feat/new", {});
    stderr.restore();

    // Routed to the pre-rename session, frontmatter untouched.
    expect(result).not.toBeNull();
    expect(result?.data.claude_session_id).toBe("session-pre-rename");
    expect(result?.data.branch).toBe("feat/old");
    expect(fileHash(sessionPath)).toBe(beforeHash);

    // The WARN matches the rename-link wording exactly — not the
    // most-recent-active wording.
    expect(stderr.lines).toEqual([
      WARN_ADOPTION_RENAME_LINK("feat/new", sessionPath, "feat/old"),
    ]);
    const line = stderr.lines[0];
    expect(line).toContain("appears to be a rename of 'feat/old'");
    expect(line).not.toContain("most-recent active session");
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

  it("branch-match WARN contains exact literal text", async () => {
    // Session is on BRANCH ('feat/example') and the request matches it →
    // direct branch-match path → original WARN text.
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      claude_session_id: "session-A",
    });

    const stderr = captureStderr();
    await resolveCurrentSession(AGENTS_DIR, BRANCH, {});
    stderr.restore();

    expect(stderr.lines).toHaveLength(1);
    const line = stderr.lines[0];
    expect(line).toContain("WARN: no session_id signal");
    expect(line).toContain(`falling back to branch routing for branch '${BRANCH}'`);
    expect(line).toContain("Pass --session-id <id> to silence.");
  });

  it("adoption WARN contains target file and origin branch (SPEC-042 Track B)", async () => {
    // Session is on `cwt/foo`; request is on `release/v0.16.0` → adoption
    // path → richer WARN naming both branches and the resolved file.
    const filePath = writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "cwt/foo",
      claude_session_id: "session-A",
    });

    const stderr = captureStderr();
    await resolveCurrentSession(AGENTS_DIR, "release/v0.16.0", {});
    stderr.restore();

    expect(stderr.lines).toHaveLength(1);
    const line = stderr.lines[0];
    expect(line).toContain("WARN: no session for branch 'release/v0.16.0'");
    expect(line).toContain(`'${basename(filePath)}'`);
    expect(line).toContain("origin branch 'cwt/foo'");
    expect(line).toContain("Pass --session-id <id> to silence.");
  });

  it("zero-active WARN is distinct from adoption WARN (SPEC-042 Track B)", async () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      status: "stopped",
      claude_session_id: "session-A",
    });

    const stderr = captureStderr();
    const result = await resolveCurrentSession(AGENTS_DIR, "release/v0.16.0", {});
    stderr.restore();

    expect(result).toBeNull();
    expect(stderr.lines).toHaveLength(1);
    const line = stderr.lines[0];
    expect(line).toContain("WARN: no session for branch 'release/v0.16.0'");
    expect(line).toContain("no active sessions to fall back to");
    expect(line).toContain("Pass --session-id <id> to silence.");
  });
});
