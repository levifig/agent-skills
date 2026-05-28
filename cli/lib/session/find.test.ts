/**
 * Session Find Tests
 *
 * Unit tests for `findActiveSessionForBranch` (SPEC-042 Track B).
 *
 * Strategy:
 *   - Real session fixtures on disk for finder behavior.
 *   - SHA-256 of file contents to assert no-mutation invariants.
 *   - Direct calls — no resolve.ts involvement.
 */

import {
  describe,
  it,
  expect,
  beforeEach,
  afterEach,
  vi,
} from "vitest";

import { createHash } from "crypto";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "fs";
import { join } from "path";
import { tmpdir } from "os";
import matter from "gray-matter";

import { findActiveSessionForBranch } from "./find.js";
import type { SessionFrontmatter } from "./store.js";

// ─────────────────────────────────────────────────────────────────────────────
// Fixtures
// ─────────────────────────────────────────────────────────────────────────────

let TEST_ROOT: string;
let AGENTS_DIR: string;
let SESSIONS_DIR: string;

interface SessionSeed {
  fileName: string;
  branch?: string;
  status?: SessionFrontmatter["status"];
  claude_session_id?: string;
  created?: string;
  last_updated?: string;
  last_entry?: string;
}

function writeSessionFile(seed: SessionSeed): string {
  const data: SessionFrontmatter = {
    branch: seed.branch ?? "feat/example",
    status: seed.status ?? "active",
    created: seed.created ?? "2026-04-27T22:00:00.000Z",
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

function fileHash(path: string): string {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}

// ─────────────────────────────────────────────────────────────────────────────
// Setup / Teardown
// ─────────────────────────────────────────────────────────────────────────────

beforeEach(() => {
  TEST_ROOT = realpathSync(mkdtempSync(join(tmpdir(), "loaf-test-find-")));
  AGENTS_DIR = join(TEST_ROOT, ".agents");
  SESSIONS_DIR = join(AGENTS_DIR, "sessions");
  mkdirSync(SESSIONS_DIR, { recursive: true });
});

afterEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
  vi.restoreAllMocks();
});

// ─────────────────────────────────────────────────────────────────────────────
// Direct branch match
// ─────────────────────────────────────────────────────────────────────────────

describe("findActiveSessionForBranch — direct branch match", () => {
  it("returns the session with adoption='branch-match'", () => {
    const filePath = writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "feat/foo",
      claude_session_id: "session-foo",
    });

    const result = findActiveSessionForBranch(AGENTS_DIR, "feat/foo");

    expect(result).not.toBeNull();
    expect(result?.adoption).toBe("branch-match");
    expect(result?.filePath).toBe(filePath);
    expect(result?.data.claude_session_id).toBe("session-foo");
  });

  it("does NOT mutate session frontmatter on branch match", () => {
    const filePath = writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "feat/foo",
      claude_session_id: "session-foo",
    });
    const before = fileHash(filePath);

    findActiveSessionForBranch(AGENTS_DIR, "feat/foo");

    expect(fileHash(filePath)).toBe(before);
  });

  it("prefers active over stopped for the same branch", () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "feat/foo",
      status: "stopped",
      claude_session_id: "stopped-session",
    });
    const activePath = writeSessionFile({
      fileName: "20260427-221000-session.md",
      branch: "feat/foo",
      status: "active",
      claude_session_id: "active-session",
    });

    const result = findActiveSessionForBranch(AGENTS_DIR, "feat/foo");

    expect(result?.filePath).toBe(activePath);
    expect(result?.adoption).toBe("branch-match");
  });

  it("skips archived sessions even with branch match", () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "feat/foo",
      status: "archived",
      claude_session_id: "archived",
    });

    // No other active sessions — should fall through to most-recent-active
    // and find nothing.
    const result = findActiveSessionForBranch(AGENTS_DIR, "feat/foo");
    expect(result).toBeNull();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Most-recent-active adoption (SPEC-042 Track B Bug B2)
// ─────────────────────────────────────────────────────────────────────────────

describe("findActiveSessionForBranch — most-recent-active adoption", () => {
  it("picks the most-recently-updated active session when no branch matches (multi-active)", () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "cwt/older",
      claude_session_id: "older",
      last_updated: "2026-04-27T22:00:00.000Z",
    });
    const newerPath = writeSessionFile({
      fileName: "20260427-223000-session.md",
      branch: "cwt/newer",
      claude_session_id: "newer",
      last_updated: "2026-04-27T22:30:00.000Z",
    });
    writeSessionFile({
      fileName: "20260427-221500-session.md",
      branch: "cwt/middle",
      claude_session_id: "middle",
      last_updated: "2026-04-27T22:15:00.000Z",
    });

    const result = findActiveSessionForBranch(AGENTS_DIR, "release/v0.16.0");

    expect(result).not.toBeNull();
    expect(result?.adoption).toBe("most-recent-active");
    expect(result?.filePath).toBe(newerPath);
    expect(result?.data.claude_session_id).toBe("newer");
    // Critical: returned session's origin branch is preserved.
    expect(result?.data.branch).toBe("cwt/newer");
  });

  it("does NOT mutate the adopted session's frontmatter (multi-active)", () => {
    const olderPath = writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "cwt/older",
      claude_session_id: "older",
      last_updated: "2026-04-27T22:00:00.000Z",
    });
    const newerPath = writeSessionFile({
      fileName: "20260427-223000-session.md",
      branch: "cwt/newer",
      claude_session_id: "newer",
      last_updated: "2026-04-27T22:30:00.000Z",
    });

    const olderBefore = fileHash(olderPath);
    const newerBefore = fileHash(newerPath);

    findActiveSessionForBranch(AGENTS_DIR, "release/v0.16.0");

    // Both files must be byte-identical post-call.
    expect(fileHash(olderPath)).toBe(olderBefore);
    expect(fileHash(newerPath)).toBe(newerBefore);
  });

  it("picks the only active session when count === 1 (single-active path no longer mutates)", () => {
    const filePath = writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "main",
      claude_session_id: "lone",
    });
    const before = fileHash(filePath);

    const result = findActiveSessionForBranch(AGENTS_DIR, "feat/missing");

    expect(result).not.toBeNull();
    expect(result?.adoption).toBe("most-recent-active");
    expect(result?.filePath).toBe(filePath);
    expect(result?.data.branch).toBe("main"); // unchanged
    expect(fileHash(filePath)).toBe(before);
  });

  it("falls back to last_entry when last_updated missing", () => {
    // Write files directly so we can omit `last_updated` entirely (the
    // writeSessionFile helper's `??` defaults can't express absence).
    const olderPath = join(SESSIONS_DIR, "20260427-220000-session.md");
    writeFileSync(
      olderPath,
      matter.stringify(
        "# Session\n\n## Journal\n",
        {
          branch: "cwt/older",
          status: "active",
          created: "2026-04-27T22:00:00.000Z",
          last_entry: "2026-04-27T22:00:00.000Z",
          claude_session_id: "older",
        } as Record<string, unknown>
      ),
      "utf-8"
    );
    const newerPath = join(SESSIONS_DIR, "20260427-223000-session.md");
    writeFileSync(
      newerPath,
      matter.stringify(
        "# Session\n\n## Journal\n",
        {
          branch: "cwt/newer",
          status: "active",
          created: "2026-04-27T22:00:00.000Z",
          last_entry: "2026-04-27T22:30:00.000Z",
          claude_session_id: "newer",
        } as Record<string, unknown>
      ),
      "utf-8"
    );

    const result = findActiveSessionForBranch(AGENTS_DIR, "release/v0.16.0");

    expect(result?.filePath).toBe(newerPath);
  });

  it("excludes stopped/done/blocked sessions from the fallback pool", () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "cwt/stopped",
      status: "stopped",
      claude_session_id: "stopped",
      last_updated: "2026-04-27T23:00:00.000Z",
    });
    writeSessionFile({
      fileName: "20260427-221000-session.md",
      branch: "cwt/done",
      status: "done",
      claude_session_id: "done",
      last_updated: "2026-04-27T23:00:00.000Z",
    });
    writeSessionFile({
      fileName: "20260427-222000-session.md",
      branch: "cwt/blocked",
      status: "blocked",
      claude_session_id: "blocked",
      last_updated: "2026-04-27T23:00:00.000Z",
    });
    const activePath = writeSessionFile({
      fileName: "20260427-223000-session.md",
      branch: "cwt/active",
      status: "active",
      claude_session_id: "active",
      last_updated: "2026-04-27T22:00:00.000Z", // older but the only active
    });

    const result = findActiveSessionForBranch(AGENTS_DIR, "release/v0.16.0");

    expect(result?.filePath).toBe(activePath);
    expect(result?.adoption).toBe("most-recent-active");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Zero-active behavior
// ─────────────────────────────────────────────────────────────────────────────

describe("findActiveSessionForBranch — zero active", () => {
  it("returns null when no sessions exist at all", () => {
    const result = findActiveSessionForBranch(AGENTS_DIR, "feat/missing");
    expect(result).toBeNull();
  });

  it("returns null when all sessions are non-active and no branch matches", () => {
    writeSessionFile({
      fileName: "20260427-220000-session.md",
      branch: "cwt/foo",
      status: "stopped",
      claude_session_id: "stopped",
    });
    writeSessionFile({
      fileName: "20260427-221000-session.md",
      branch: "cwt/bar",
      status: "done",
      claude_session_id: "done",
    });

    const result = findActiveSessionForBranch(AGENTS_DIR, "release/v0.16.0");
    expect(result).toBeNull();
  });

  it("returns null when sessionsDir does not exist", () => {
    rmSync(SESSIONS_DIR, { recursive: true });
    const result = findActiveSessionForBranch(AGENTS_DIR, "feat/anything");
    expect(result).toBeNull();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// No-mutation invariant — broad sweep
// ─────────────────────────────────────────────────────────────────────────────

describe("findActiveSessionForBranch — no-mutation invariant", () => {
  it("does not mutate any session file across mixed-status, multi-branch fixtures", () => {
    const paths = [
      writeSessionFile({
        fileName: "20260427-220000-session.md",
        branch: "cwt/a",
        status: "active",
        claude_session_id: "a",
      }),
      writeSessionFile({
        fileName: "20260427-221000-session.md",
        branch: "cwt/b",
        status: "active",
        claude_session_id: "b",
        last_updated: "2026-04-27T22:45:00.000Z",
      }),
      writeSessionFile({
        fileName: "20260427-222000-session.md",
        branch: "cwt/c",
        status: "stopped",
        claude_session_id: "c",
      }),
      writeSessionFile({
        fileName: "20260427-223000-session.md",
        branch: "cwt/d",
        status: "archived",
        claude_session_id: "d",
      }),
    ];

    const hashesBefore = paths.map(fileHash);

    // Hit a non-existent branch — forces fallback.
    findActiveSessionForBranch(AGENTS_DIR, "release/v0.16.0");
    // Hit a matching branch — direct path.
    findActiveSessionForBranch(AGENTS_DIR, "cwt/a");
    // Hit again to make sure repeat calls are still no-op on disk.
    findActiveSessionForBranch(AGENTS_DIR, "release/v0.16.0");

    paths.forEach((p, i) => {
      expect(fileHash(p)).toBe(hashesBefore[i]);
    });
  });
});
