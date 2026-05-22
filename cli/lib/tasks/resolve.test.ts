/**
 * findAgentsDir() Tests (SPEC-036, TASK-166)
 *
 * Validates worktree-aware `.agents/` resolution under the A3 storage model:
 *
 *   - Linked worktree    → main worktree's `.agents/`
 *   - Main checkout      → parent-walk (current behavior, verbatim)
 *   - Outside a git repo → parent-walk (current behavior, verbatim)
 *
 * The tests use real `git init` + `git worktree add` against temp directories
 * because the function shells out to `git rev-parse`; mocking that would
 * defeat the regression value. Pattern mirrors
 * `cli/commands/session.misrouting.e2e.test.ts`.
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "fs";
import { execFileSync } from "child_process";
import { dirname, join } from "path";
import { tmpdir } from "os";

import {
  DEBUG_RESOLVE_ENV,
  findAgentsDir,
  getOrBuildIndex,
  isDebugResolveEnabled,
} from "./resolve.js";
import { saveIndex } from "./migrate.js";
import type { TaskIndex } from "./types.js";

// ─────────────────────────────────────────────────────────────────────────────
// Scaffolding
// ─────────────────────────────────────────────────────────────────────────────

let TEST_ROOT: string;
let TEST_ENV_ROOT: string;
let ORIGINAL_ENV: {
  HOME?: string;
  TMPDIR?: string;
  TMP?: string;
  TEMP?: string;
};

/** Per-command identity flags so tests never mutate any real `.git/config`. */
const TEST_IDENTITY = [
  "-c",
  "user.name=Test User",
  "-c",
  "user.email=test@test.com",
] as const;

function git(args: readonly string[], cwd: string): string {
  return execFileSync("git", args as string[], { cwd, stdio: ["ignore", "pipe", "pipe"] })
    .toString();
}

/**
 * Initialize a git repo at `repoPath` with a single commit and an `.agents/`
 * directory. Returns the absolute repo path (realpath-resolved so it matches
 * what git emits from `--path-format=absolute`).
 */
function createMainRepo(name: string, opts: { withAgents?: boolean } = {}): string {
  const withAgents = opts.withAgents !== false;
  // We keep `realpathSync` here because path-equality assertions later compare
  // against `findAgentsDir`'s output, which canonicalizes internally (see
  // `normalizePathForComparison` in `cli/lib/tasks/resolve.ts`). The
  // "symlinked startDir" case the new normalization fixes is covered
  // explicitly by the `path normalization` describe block below — it builds
  // a symlink that points at the realpath'd repo and asserts the resolver
  // produces the same answer regardless of which path is used as input.
  const repoPath = realpathSync(mkdtempSync(join(TEST_ROOT, `${name}-`)));

  git(["init", "--initial-branch=main"], repoPath);
  writeFileSync(join(repoPath, "README.md"), "# Test\n", "utf-8");
  git(["add", "."], repoPath);
  git([...TEST_IDENTITY, "commit", "-m", "Initial commit"], repoPath);

  if (withAgents) {
    mkdirSync(join(repoPath, ".agents"), { recursive: true });
    // Drop a sentinel so cross-worktree reach-the-same-file tests can
    // confidently assert "same directory" via byte-identity.
    writeFileSync(
      join(repoPath, ".agents", "AGENTS.md"),
      "# Project Instructions\n",
      "utf-8",
    );
  }
  return repoPath;
}

/**
 * Add a linked worktree at `<repoPath>-wt-<branch>`. Returns the realpath of
 * the linked worktree (matches what `git rev-parse --path-format=absolute`
 * emits).
 */
function addWorktree(repoPath: string, branch: string): string {
  const wtPath = `${repoPath}-wt-${branch}`;
  git(["worktree", "add", "-b", branch, wtPath], repoPath);
  return realpathSync(wtPath);
}

beforeEach(() => {
  ORIGINAL_ENV = {
    HOME: process.env.HOME,
    TMPDIR: process.env.TMPDIR,
    TMP: process.env.TMP,
    TEMP: process.env.TEMP,
  };

  TEST_ENV_ROOT = realpathSync(mkdtempSync(join(tmpdir(), "loaf-resolve-env-")));
  process.env.HOME = TEST_ENV_ROOT;
  process.env.TMPDIR = TEST_ENV_ROOT;
  process.env.TMP = TEST_ENV_ROOT;
  process.env.TEMP = TEST_ENV_ROOT;

  TEST_ROOT = realpathSync(mkdtempSync(join(TEST_ENV_ROOT, "case-")));
  assertNoAgentsAncestor(TEST_ROOT);
});

afterEach(() => {
  restoreEnv();
  rmSync(TEST_ENV_ROOT, { recursive: true, force: true });
});

function restoreEnv(): void {
  for (const key of ["HOME", "TMPDIR", "TMP", "TEMP"] as const) {
    const value = ORIGINAL_ENV[key];
    if (value === undefined) {
      delete process.env[key];
    } else {
      process.env[key] = value;
    }
  }
}

function assertNoAgentsAncestor(startDir: string): void {
  let current = dirname(realpathSync(startDir));
  while (true) {
    if (existsSync(join(current, ".agents"))) {
      throw new Error(`resolve.test isolation failed: ancestor has .agents/: ${current}`);
    }
    const parent = dirname(current);
    if (parent === current) return;
    current = parent;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Core findAgentsDir behavior
// ─────────────────────────────────────────────────────────────────────────────

describe("findAgentsDir — main checkout (verbatim parent-walk behavior)", () => {
  it("returns the repo's .agents/ when invoked at the repo root", () => {
    const repo = createMainRepo("main-root");
    const result = findAgentsDir(repo);
    expect(result).toBe(join(repo, ".agents"));
  });

  it("returns the repo's .agents/ when invoked from a subdirectory", () => {
    const repo = createMainRepo("main-subdir");
    const subDir = join(repo, "src", "deep");
    mkdirSync(subDir, { recursive: true });
    const result = findAgentsDir(subDir);
    expect(result).toBe(join(repo, ".agents"));
  });

  it("returns null when no .agents/ exists anywhere up the tree (git context)", () => {
    const repo = createMainRepo("main-no-agents", { withAgents: false });
    // `.agents/` was never created — main-checkout probe sees gitDir ===
    // commonDir and falls through to parent-walk, which finds nothing under
    // the temp root.
    const result = findAgentsDir(repo);
    expect(result).toBeNull();
  });
});

describe("findAgentsDir — outside a git context (verbatim parent-walk behavior)", () => {
  it("parent-walks normally when startDir is not inside any git repo", () => {
    // TEST_ROOT itself is just a tmp dir — no `git init` here.
    const nonGitDir = mkdtempSync(join(TEST_ROOT, "no-git-"));
    mkdirSync(join(nonGitDir, ".agents"), { recursive: true });
    const sub = join(nonGitDir, "a", "b");
    mkdirSync(sub, { recursive: true });
    const result = findAgentsDir(sub);
    expect(result).toBe(join(realpathSync(nonGitDir), ".agents"));
  });

  it("returns null when no .agents/ exists and no git context is available", () => {
    const nonGitDir = mkdtempSync(join(TEST_ROOT, "no-git-empty-"));
    const result = findAgentsDir(nonGitDir);
    expect(result).toBeNull();
  });
});

describe("findAgentsDir — linked worktree (A3 redirect to main)", () => {
  it("returns the MAIN worktree's .agents/ when invoked from a linked worktree root", () => {
    const main = createMainRepo("a3-redirect-root");
    const linked = addWorktree(main, "feat/x");

    expect(findAgentsDir(linked)).toBe(join(main, ".agents"));
  });

  it("returns the MAIN worktree's .agents/ when invoked from a linked-worktree subdirectory", () => {
    const main = createMainRepo("a3-redirect-subdir");
    const linked = addWorktree(main, "feat/y");
    const sub = join(linked, "src", "feature");
    mkdirSync(sub, { recursive: true });

    expect(findAgentsDir(sub)).toBe(join(main, ".agents"));
  });

  it("main checkout and linked worktree resolve to the SAME path", () => {
    const main = createMainRepo("a3-symmetry");
    const linked = addWorktree(main, "feat/z");

    const fromMain = findAgentsDir(main);
    const fromLinked = findAgentsDir(linked);

    expect(fromMain).not.toBeNull();
    expect(fromLinked).not.toBeNull();
    expect(fromMain).toBe(fromLinked);
    expect(fromMain).toBe(join(main, ".agents"));
  });

  it("returns null when the main worktree has no .agents/ directory", () => {
    const main = createMainRepo("a3-no-agents-on-main", { withAgents: false });
    const linked = addWorktree(main, "feat/q");

    // Linked worktrees do NOT parent-walk — they probe main directly. If
    // main has no `.agents/`, the answer is null (matches A3 semantics:
    // there is no project-scoped store yet).
    expect(findAgentsDir(linked)).toBeNull();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Cross-worktree integration: both views reach the same session file
// ─────────────────────────────────────────────────────────────────────────────

describe("cross-worktree integration: shared session file via findAgentsDir", () => {
  it("an entry appended from a linked worktree is visible in the main worktree's session file", () => {
    const main = createMainRepo("xwt-session");
    const linked = addWorktree(main, "feat/journal");

    // Simulate the start of a session in the main worktree.
    const sessionsDir = join(findAgentsDir(main)!, "sessions");
    mkdirSync(sessionsDir, { recursive: true });
    const sessionPath = join(sessionsDir, "20260519-120000-session.md");
    writeFileSync(
      sessionPath,
      [
        "---",
        "branch: main",
        "status: active",
        "claude_session_id: xwt-test",
        "created: '2026-05-19T12:00:00.000Z'",
        "---",
        "# Session",
        "",
        "## Journal",
        "",
        "[2026-05-19 12:00] session(start):  === SESSION STARTED ===",
        "",
      ].join("\n"),
      "utf-8",
    );

    // Resolve `.agents/` from the LINKED worktree, then append an entry to
    // what we computed as the session file. Under A3 this must reach the
    // same byte-identical file the main worktree owns.
    const agentsFromLinked = findAgentsDir(linked);
    expect(agentsFromLinked).toBe(join(main, ".agents"));

    const targetSessionPath = join(
      agentsFromLinked!,
      "sessions",
      "20260519-120000-session.md",
    );
    expect(targetSessionPath).toBe(sessionPath); // path identity
    expect(existsSync(targetSessionPath)).toBe(true);

    // Append from the linked-worktree-resolved path and read back via the
    // main-worktree-resolved path.
    const before = readFileSync(sessionPath, "utf-8");
    writeFileSync(
      targetSessionPath,
      before + "[2026-05-19 12:05] discover(xwt): entry written via linked-worktree resolver\n",
      "utf-8",
    );

    const afterFromMain = readFileSync(
      join(findAgentsDir(main)!, "sessions", "20260519-120000-session.md"),
      "utf-8",
    );
    expect(afterFromMain).toContain(
      "discover(xwt): entry written via linked-worktree resolver",
    );
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Parallel ID allocation: single-view scanning is sufficient under A3
// ─────────────────────────────────────────────────────────────────────────────

describe("parallel ID allocation across worktrees (A3 single-view scan)", () => {
  /**
   * Two simulated worktrees mint a task ID sequentially against the SHARED
   * index. The shared-view contract means both views see the same `next_id`
   * counter, and consecutive allocations produce distinct IDs.
   *
   * Sequential rather than truly parallel: this test locks in the SPEC-036
   * "shared view" property only — that both worktrees address the same
   * TASKS.json. The companion cross-process concurrency test that proves
   * the lock's read-modify-write barrier under contention lives in
   * `cli/lib/tasks/lock.test.ts`.
   */
  it("two worktrees allocating against the shared TASKS.json produce distinct IDs", () => {
    const main = createMainRepo("alloc-shared");
    const linked = addWorktree(main, "feat/alloc");

    // Seed a TASKS.json at the main worktree's `.agents/`.
    const agentsDir = findAgentsDir(main)!;
    const indexPath = join(agentsDir, "TASKS.json");
    const seed: TaskIndex = {
      version: 1,
      next_id: 100,
      tasks: {},
      specs: {},
    };
    saveIndex(indexPath, seed);

    // Worktree A (the main checkout) mints an ID.
    const indexFromMain = getOrBuildIndex(findAgentsDir(main)!);
    const idA = `TASK-${String(indexFromMain.next_id).padStart(3, "0")}`;
    indexFromMain.next_id += 1;
    saveIndex(join(findAgentsDir(main)!, "TASKS.json"), indexFromMain);

    // Worktree B (the linked worktree) mints next. It MUST observe the
    // bumped counter because both worktrees resolve to the same store.
    const indexFromLinked = getOrBuildIndex(findAgentsDir(linked)!);
    const idB = `TASK-${String(indexFromLinked.next_id).padStart(3, "0")}`;
    indexFromLinked.next_id += 1;
    saveIndex(join(findAgentsDir(linked)!, "TASKS.json"), indexFromLinked);

    expect(idA).toBe("TASK-100");
    expect(idB).toBe("TASK-101");
    expect(idA).not.toBe(idB);

    // Final state seen from either side is consistent.
    const finalFromMain = getOrBuildIndex(findAgentsDir(main)!);
    const finalFromLinked = getOrBuildIndex(findAgentsDir(linked)!);
    expect(finalFromMain.next_id).toBe(102);
    expect(finalFromLinked.next_id).toBe(102);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// isDebugResolveEnabled — env-var truthiness allow-list
// ─────────────────────────────────────────────────────────────────────────────

describe("isDebugResolveEnabled — explicit allow-list truthiness", () => {
  let original: string | undefined;

  beforeEach(() => {
    original = process.env[DEBUG_RESOLVE_ENV];
  });

  afterEach(() => {
    if (original === undefined) {
      delete process.env[DEBUG_RESOLVE_ENV];
    } else {
      process.env[DEBUG_RESOLVE_ENV] = original;
    }
  });

  function set(value: string | undefined): void {
    if (value === undefined) {
      delete process.env[DEBUG_RESOLVE_ENV];
    } else {
      process.env[DEBUG_RESOLVE_ENV] = value;
    }
  }

  it("returns false when unset", () => {
    set(undefined);
    expect(isDebugResolveEnabled()).toBe(false);
  });

  it("returns false for empty string", () => {
    set("");
    expect(isDebugResolveEnabled()).toBe(false);
  });

  it("returns false for '0'", () => {
    set("0");
    expect(isDebugResolveEnabled()).toBe(false);
  });

  it("returns false for 'false'", () => {
    set("false");
    expect(isDebugResolveEnabled()).toBe(false);
  });

  it("returns false for 'no'", () => {
    set("no");
    expect(isDebugResolveEnabled()).toBe(false);
  });

  it("returns false for arbitrary non-allow-listed strings", () => {
    set("enabled");
    expect(isDebugResolveEnabled()).toBe(false);
  });

  it.each(["1", "true", "yes", "on", "TRUE", "Yes", "ON"])(
    "returns true for %s",
    (value) => {
      set(value);
      expect(isDebugResolveEnabled()).toBe(true);
    },
  );
});

// ─────────────────────────────────────────────────────────────────────────────
// Path normalization edge cases (Codex finding #3)
//
// The original `findMainWorktreeRoot` compared raw `path.resolve()` strings;
// on case-insensitive filesystems (macOS default, Windows) and through
// symlinks/junctions, two paths pointing at the same FS object could compare
// unequal. The fix wraps both sides in `realpathSync.native` (and lowercases
// on win32) before comparing. These tests exercise the normalization on
// POSIX via symlinks; the Windows-only case-fold path is covered by the
// `process.platform === "win32"` gate in the production code.
// ─────────────────────────────────────────────────────────────────────────────

describe("findAgentsDir — path normalization edge cases", () => {
  it("a linked worktree reached through a symlink still resolves to main's .agents/", () => {
    const main = createMainRepo("norm-wt-symlink");
    const linked = addWorktree(main, "feat/symlink");

    // Symlink to the linked worktree. Invoking the resolver FROM the symlinked
    // path must still produce the main worktree's `.agents/`. Without
    // realpath normalization on `gitDir`/`commonDir`, the comparison in
    // `findMainWorktreeRoot` could disagree depending on whether git
    // canonicalized one side and not the other.
    const wtSymlink = join(TEST_ROOT, "wt-symlink-alias");
    symlinkSync(linked, wtSymlink, "dir");

    const fromRealpath = findAgentsDir(linked);
    const fromSymlink = findAgentsDir(wtSymlink);

    expect(fromRealpath).toBe(join(main, ".agents"));
    expect(fromSymlink).toBe(join(main, ".agents"));
  });

  it("a main checkout reached through a symlink is still classified as a main checkout", () => {
    // The actual bug Codex flagged was that two paths pointing at the same
    // FS object could compare unequal — making a main checkout look like
    // a linked worktree (gitDir !== commonDir after raw-string compare).
    //
    // With realpath normalization, both sides canonicalize to the same
    // path and the comparison returns "main checkout" — caller parent-walks.
    //
    // The parent-walk itself doesn't canonicalize (it's outside the scope
    // of the path-normalization fix), so the returned path may still
    // contain the symlinked prefix. We assert via realpathSync to verify
    // the resolver landed in the right *directory* regardless of which
    // path string it returned.
    const main = createMainRepo("norm-main-via-symlink");
    const symlinkPath = join(TEST_ROOT, "main-alias");
    symlinkSync(main, symlinkPath, "dir");

    const result = findAgentsDir(symlinkPath);
    expect(result).not.toBeNull();
    expect(realpathSync(result!)).toBe(realpathSync(join(main, ".agents")));
  });

  it("worktrees reached through symlinks land on the same store as the main checkout", () => {
    // Strongest property: regardless of which path string is used to address
    // a worktree (main or linked, realpath or symlink), the resolver
    // produces a path whose realpath points at the SAME store. This is
    // the cross-worktree continuity guarantee that the Windows
    // case-fold + symlink normalization is in service of.
    const main = createMainRepo("norm-symmetry");
    const linked = addWorktree(main, "feat/sym");
    const mainAlias = join(TEST_ROOT, "main-alias-sym");
    const linkedAlias = join(TEST_ROOT, "linked-alias-sym");
    symlinkSync(main, mainAlias, "dir");
    symlinkSync(linked, linkedAlias, "dir");

    const candidates = [
      findAgentsDir(main),
      findAgentsDir(linked),
      findAgentsDir(mainAlias),
      findAgentsDir(linkedAlias),
    ];
    expect(candidates.every((c) => c !== null)).toBe(true);

    const canonicals = candidates.map((c) => realpathSync(c!));
    // All four must land on the SAME .agents/ store.
    expect(new Set(canonicals).size).toBe(1);
  });

  // TODO: Windows-only test fixture — verifying that the `process.platform === "win32"`
  // case-fold path correctly compares `C:\...` and `c:\...` as equal. We
  // can't simulate Windows-style case-insensitive paths reliably on POSIX
  // (macOS HFS+ case-insensitivity is opt-in and not the default on APFS).
  // Cross-platform test rigs (Windows runner in CI) should add coverage
  // for this path.
});
