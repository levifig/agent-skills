/**
 * SPEC-036 / TASK-170 — `loaf migrate worktree-storage` tests
 *
 * Uses real `git init` + `git worktree add` against tmp directories (no git
 * mocking). Mirrors the fixture pattern from
 * `cli/lib/tasks/resolve.test.ts` and `cli/commands/session.misrouting.e2e.test.ts`.
 *
 * Covers:
 *   - Round-trip: pre-A3 layout → dry-run (no changes) → apply (changes
 *     correct) → re-apply (no-op).
 *   - Conflict policy: newer-mtime wins by default; `--force-from-worktree`
 *     and `--force-from-main` overrides flip the result.
 *   - Back-pointer behavior: written after --apply, idempotent on re-run.
 *   - Main-checkout invocation: clean no-op exit.
 *   - Outside git context: error status.
 *   - Pre-A3 detector: only fires on linked worktrees with unmigrated content.
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  statSync,
  utimesSync,
  writeFileSync,
} from "fs";
import { execFileSync } from "child_process";
import { dirname, join } from "path";
import { tmpdir } from "os";

import {
  BACK_POINTER_FILE,
  buildMainMissingMessage,
  detectMainMissingForRefusal,
  detectPreA3State,
  PARTIAL_SUFFIX,
  PRE_A3_REFUSAL_MESSAGE,
  readBackPointer,
  runMigration,
  worktreeAgentsHasContent,
} from "./worktree-storage.js";

// ─────────────────────────────────────────────────────────────────────────────
// Scaffolding (mirrors cli/lib/tasks/resolve.test.ts)
// ─────────────────────────────────────────────────────────────────────────────

let TEST_ROOT: string;

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

function createMainRepo(name: string, opts: { withAgents?: boolean } = {}): string {
  const withAgents = opts.withAgents !== false;
  const repoPath = realpathSync(mkdtempSync(join(TEST_ROOT, `${name}-`)));

  git(["init", "--initial-branch=main"], repoPath);
  writeFileSync(join(repoPath, "README.md"), "# Test\n", "utf-8");
  git(["add", "."], repoPath);
  git([...TEST_IDENTITY, "commit", "-m", "Initial commit"], repoPath);

  if (withAgents) {
    mkdirSync(join(repoPath, ".agents"), { recursive: true });
    writeFileSync(
      join(repoPath, ".agents", "AGENTS.md"),
      "# Project Instructions\n",
      "utf-8",
    );
  }
  return repoPath;
}

function addWorktree(repoPath: string, branch: string): string {
  const wtPath = `${repoPath}-wt-${branch}`;
  git(["worktree", "add", "-b", branch, wtPath], repoPath);
  return realpathSync(wtPath);
}

/**
 * Populate a linked worktree's `.agents/` with the structure SPEC-036 enumerates
 * (sessions/, kb/, ideas/, drafts/, reports/, councils/, tasks/, specs/, plans/,
 * AGENTS.md, loaf.json, SOUL.md, TASKS.json) so tests can assert that the full
 * surface migrates.
 */
function seedPreA3WorktreeLayout(worktreePath: string): {
  files: string[];
  agentsDir: string;
} {
  const agents = join(worktreePath, ".agents");
  mkdirSync(agents, { recursive: true });

  const files: string[] = [];
  const addFile = (rel: string, content: string) => {
    const abs = join(agents, rel);
    mkdirSync(join(abs, ".."), { recursive: true });
    writeFileSync(abs, content, "utf-8");
    files.push(rel);
  };

  addFile("AGENTS.md", "# Worktree AGENTS\n");
  addFile("loaf.json", '{"foo":"bar"}\n');
  addFile("SOUL.md", "# Soul\n");
  addFile("TASKS.json", '{"version":1,"next_id":100,"tasks":{},"specs":{}}\n');
  addFile("sessions/20260519-120000-session.md", "# Session\n");
  addFile("kb/some-note.md", "# KB Note\n");
  addFile("ideas/idea-001.md", "# Idea\n");
  addFile("drafts/draft-001.md", "# Draft\n");
  addFile("reports/report-001.md", "# Report\n");
  addFile("councils/council-001.md", "# Council\n");
  addFile("tasks/TASK-200-example.md", "# Task 200\n");
  addFile("specs/SPEC-040-example.md", "# Spec 040\n");
  addFile("plans/PLAN-001.md", "# Plan 001\n");

  return { files, agentsDir: agents };
}

beforeEach(() => {
  TEST_ROOT = realpathSync(mkdtempSync(join(tmpdir(), "loaf-migrate-")));
});

afterEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
});

// ─────────────────────────────────────────────────────────────────────────────
// Constants & detection
// ─────────────────────────────────────────────────────────────────────────────

describe("PRE_A3_REFUSAL_MESSAGE", () => {
  it("mentions the migrate command, SPEC-036, and the debug env knob", () => {
    expect(PRE_A3_REFUSAL_MESSAGE).toContain("loaf migrate worktree-storage");
    expect(PRE_A3_REFUSAL_MESSAGE).toContain("SPEC-036");
    expect(PRE_A3_REFUSAL_MESSAGE).toContain("LOAF_DEBUG_RESOLVE");
  });
});

describe("detectPreA3State", () => {
  it("returns false for a main checkout with no linked worktrees", () => {
    const main = createMainRepo("det-main");
    expect(detectPreA3State(main)).toBe(false);
  });

  it("returns false for a linked worktree with no local .agents/", () => {
    const main = createMainRepo("det-empty");
    const linked = addWorktree(main, "feat/empty");
    // No .agents/ created in the linked worktree.
    expect(detectPreA3State(linked)).toBe(false);
  });

  it("returns true for a linked worktree with populated .agents/ and no back-pointer", () => {
    const main = createMainRepo("det-populated");
    const linked = addWorktree(main, "feat/populated");
    seedPreA3WorktreeLayout(linked);
    expect(detectPreA3State(linked)).toBe(true);
  });

  it("returns false when back-pointer correctly references the current main root", () => {
    const main = createMainRepo("det-migrated");
    const linked = addWorktree(main, "feat/migrated");
    mkdirSync(join(linked, ".agents"), { recursive: true });
    writeFileSync(
      join(linked, ".agents", BACK_POINTER_FILE),
      `${main}\n`,
      "utf-8",
    );
    expect(detectPreA3State(linked)).toBe(false);
  });

  it("returns true when back-pointer references a stale (nonexistent) path", () => {
    const main = createMainRepo("det-stale");
    const linked = addWorktree(main, "feat/stale");
    seedPreA3WorktreeLayout(linked);
    writeFileSync(
      join(linked, ".agents", BACK_POINTER_FILE),
      "/nonexistent/path\n",
      "utf-8",
    );
    expect(detectPreA3State(linked)).toBe(true);
  });

  it("returns false for non-git directories", () => {
    const nonGit = mkdtempSync(join(TEST_ROOT, "no-git-"));
    mkdirSync(join(nonGit, ".agents"), { recursive: true });
    writeFileSync(join(nonGit, ".agents", "foo.md"), "x\n", "utf-8");
    expect(detectPreA3State(nonGit)).toBe(false);
  });

  it("detects pre-A3 state from a subdirectory of the linked worktree", () => {
    const main = createMainRepo("det-subdir");
    const linked = addWorktree(main, "feat/subdir");
    seedPreA3WorktreeLayout(linked);
    const sub = join(linked, "src", "deep");
    mkdirSync(sub, { recursive: true });
    expect(detectPreA3State(sub)).toBe(true);
  });
});

describe("worktreeAgentsHasContent", () => {
  it("returns false for a missing directory", () => {
    expect(worktreeAgentsHasContent(join(TEST_ROOT, "nope"))).toBe(false);
  });

  it("returns false for an empty directory", () => {
    const dir = join(TEST_ROOT, "empty");
    mkdirSync(dir, { recursive: true });
    expect(worktreeAgentsHasContent(dir)).toBe(false);
  });

  it("returns false when only the back-pointer is present", () => {
    const dir = join(TEST_ROOT, "ptr-only");
    mkdirSync(dir, { recursive: true });
    writeFileSync(join(dir, BACK_POINTER_FILE), "/x\n", "utf-8");
    expect(worktreeAgentsHasContent(dir)).toBe(false);
  });

  it("returns true when other content is present", () => {
    const dir = join(TEST_ROOT, "with-content");
    mkdirSync(dir, { recursive: true });
    writeFileSync(join(dir, "foo.md"), "x\n", "utf-8");
    expect(worktreeAgentsHasContent(dir)).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// runMigration — main behaviors
// ─────────────────────────────────────────────────────────────────────────────

describe("runMigration — main checkout & non-git", () => {
  it("returns 'not-in-worktree' when invoked from the main checkout", () => {
    const main = createMainRepo("rm-main");
    const result = runMigration({ cwd: main, apply: false, conflictPolicy: "newer" });
    expect(result.status).toBe("not-in-worktree");
    expect(result.message).toContain("main worktree");
  });

  it("returns 'not-in-git' outside any git context", () => {
    const nonGit = mkdtempSync(join(TEST_ROOT, "no-git-rm-"));
    const result = runMigration({
      cwd: nonGit,
      apply: false,
      conflictPolicy: "newer",
    });
    expect(result.status).toBe("not-in-git");
  });
});

describe("runMigration — round-trip on a populated worktree", () => {
  it("dry-run reports moves without mutating; --apply moves files and writes back-pointer; re-apply is a no-op", () => {
    const main = createMainRepo("rt-round");
    const linked = addWorktree(main, "feat/round");
    const { files, agentsDir } = seedPreA3WorktreeLayout(linked);

    // ─── Dry-run ────────────────────────────────────────────────────────
    const dryRun = runMigration({
      cwd: linked,
      apply: false,
      conflictPolicy: "newer",
    });
    expect(dryRun.status).toBe("planned");
    expect(dryRun.plan).toBeDefined();
    // Every seeded file is in the plan.
    const planned = dryRun.plan!.moves.map((m) => m.rel).sort();
    expect(planned).toEqual([...files].sort());

    // Files still in their original location post dry-run.
    for (const rel of files) {
      expect(existsSync(join(agentsDir, rel))).toBe(true);
    }
    // Main has only AGENTS.md (seeded by createMainRepo); the others are absent.
    expect(existsSync(join(main, ".agents", "sessions"))).toBe(false);
    expect(existsSync(join(main, ".agents", "kb"))).toBe(false);
    // No back-pointer in dry-run.
    expect(existsSync(join(agentsDir, BACK_POINTER_FILE))).toBe(false);

    // ─── Apply ──────────────────────────────────────────────────────────
    const applied = runMigration({
      cwd: linked,
      apply: true,
      conflictPolicy: "newer",
    });
    expect(applied.status).toBe("applied");

    // Every file is now under main/.agents/ at its expected relative path.
    for (const rel of files) {
      const fromMain = join(main, ".agents", rel);
      expect(existsSync(fromMain)).toBe(true);
    }
    // AGENTS.md is a conflict — both sides existed. Default policy is "newer".
    // The original seeded AGENTS.md in main was written first; the worktree
    // seeded version is newer (later in the same test). Confirm one of them
    // resolved — content should be one of the two seed strings.
    const finalAgents = readFileSync(join(main, ".agents", "AGENTS.md"), "utf-8");
    expect(["# Project Instructions\n", "# Worktree AGENTS\n"]).toContain(finalAgents);

    // Worktree-local files are gone (renamed away).
    for (const rel of files) {
      expect(existsSync(join(agentsDir, rel))).toBe(false);
    }

    // Back-pointer present and points at main.
    expect(readBackPointer(agentsDir)).toBe(main);

    // ─── Re-apply ───────────────────────────────────────────────────────
    const noOp = runMigration({
      cwd: linked,
      apply: true,
      conflictPolicy: "newer",
    });
    expect(noOp.status).toBe("already-migrated");
  });

  it("dry-run twice in a row yields identical plans (no hidden side effects)", () => {
    const main = createMainRepo("rt-dryrun-twice");
    const linked = addWorktree(main, "feat/twice");
    seedPreA3WorktreeLayout(linked);

    const a = runMigration({ cwd: linked, apply: false, conflictPolicy: "newer" });
    const b = runMigration({ cwd: linked, apply: false, conflictPolicy: "newer" });
    expect(a.plan?.moves.map((m) => m.rel).sort()).toEqual(
      b.plan?.moves.map((m) => m.rel).sort(),
    );
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Conflict policy
// ─────────────────────────────────────────────────────────────────────────────

describe("runMigration — conflict policy", () => {
  /**
   * Set mtimes explicitly with `utimesSync` so the newer-wins logic is
   * deterministic regardless of filesystem timestamp resolution.
   */
  function setMtime(path: string, epochSeconds: number): void {
    const d = new Date(epochSeconds * 1000);
    utimesSync(path, d, d);
  }

  it("default (newer): worktree wins when its mtime is newer than main's", () => {
    const main = createMainRepo("cp-newer-wt");
    const linked = addWorktree(main, "feat/newer-wt");

    const mainFile = join(main, ".agents", "AGENTS.md"); // already exists
    setMtime(mainFile, 1_000_000);

    mkdirSync(join(linked, ".agents"), { recursive: true });
    const wtFile = join(linked, ".agents", "AGENTS.md");
    writeFileSync(wtFile, "# from worktree (newer)\n", "utf-8");
    setMtime(wtFile, 2_000_000);

    const r = runMigration({ cwd: linked, apply: true, conflictPolicy: "newer" });
    expect(r.status).toBe("applied");
    expect(readFileSync(mainFile, "utf-8")).toBe("# from worktree (newer)\n");
  });

  it("default (newer): main wins when its mtime is newer than worktree's", () => {
    const main = createMainRepo("cp-newer-main");
    const linked = addWorktree(main, "feat/newer-main");

    const mainFile = join(main, ".agents", "AGENTS.md");
    writeFileSync(mainFile, "# from main (newer)\n", "utf-8");
    setMtime(mainFile, 3_000_000);

    mkdirSync(join(linked, ".agents"), { recursive: true });
    const wtFile = join(linked, ".agents", "AGENTS.md");
    writeFileSync(wtFile, "# from worktree (older)\n", "utf-8");
    setMtime(wtFile, 1_000_000);

    const r = runMigration({ cwd: linked, apply: true, conflictPolicy: "newer" });
    expect(r.status).toBe("applied");
    expect(readFileSync(mainFile, "utf-8")).toBe("# from main (newer)\n");
    // worktree-local copy must be gone (lost the conflict).
    expect(existsSync(wtFile)).toBe(false);
  });

  it("--force-from-worktree overrides regardless of mtime", () => {
    const main = createMainRepo("cp-force-wt");
    const linked = addWorktree(main, "feat/force-wt");

    const mainFile = join(main, ".agents", "AGENTS.md");
    writeFileSync(mainFile, "# from main (newer)\n", "utf-8");
    setMtime(mainFile, 9_000_000); // main is newer

    mkdirSync(join(linked, ".agents"), { recursive: true });
    const wtFile = join(linked, ".agents", "AGENTS.md");
    writeFileSync(wtFile, "# from worktree (older)\n", "utf-8");
    setMtime(wtFile, 1_000_000);

    const r = runMigration({ cwd: linked, apply: true, conflictPolicy: "worktree" });
    expect(r.status).toBe("applied");
    // Worktree wins despite older mtime.
    expect(readFileSync(mainFile, "utf-8")).toBe("# from worktree (older)\n");
  });

  it("--force-from-main overrides regardless of mtime", () => {
    const main = createMainRepo("cp-force-main");
    const linked = addWorktree(main, "feat/force-main");

    const mainFile = join(main, ".agents", "AGENTS.md");
    writeFileSync(mainFile, "# from main (older)\n", "utf-8");
    setMtime(mainFile, 1_000_000); // main is older

    mkdirSync(join(linked, ".agents"), { recursive: true });
    const wtFile = join(linked, ".agents", "AGENTS.md");
    writeFileSync(wtFile, "# from worktree (newer)\n", "utf-8");
    setMtime(wtFile, 9_000_000);

    const r = runMigration({ cwd: linked, apply: true, conflictPolicy: "main" });
    expect(r.status).toBe("applied");
    expect(readFileSync(mainFile, "utf-8")).toBe("# from main (older)\n");
    // worktree-local copy must be removed.
    expect(existsSync(wtFile)).toBe(false);
  });

  it("conflict plans include a resolution string in dry-run", () => {
    const main = createMainRepo("cp-plan");
    const linked = addWorktree(main, "feat/plan");

    const mainFile = join(main, ".agents", "AGENTS.md");
    setMtime(mainFile, 1_000_000);
    mkdirSync(join(linked, ".agents"), { recursive: true });
    const wtFile = join(linked, ".agents", "AGENTS.md");
    writeFileSync(wtFile, "x\n", "utf-8");
    setMtime(wtFile, 2_000_000);

    const r = runMigration({ cwd: linked, apply: false, conflictPolicy: "newer" });
    const move = r.plan!.moves.find((m) => m.rel === "AGENTS.md");
    expect(move?.conflict).toBe(true);
    expect(move?.resolution).toBe("keep-worktree");
    expect(move?.resolutionReason).toMatch(/worktree mtime .* > main mtime/);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Back-pointer & idempotency edge cases
// ─────────────────────────────────────────────────────────────────────────────

describe("runMigration — back-pointer & idempotency", () => {
  it("writes the back-pointer with a single trailing newline", () => {
    const main = createMainRepo("bp-format");
    const linked = addWorktree(main, "feat/bp");
    seedPreA3WorktreeLayout(linked);

    runMigration({ cwd: linked, apply: true, conflictPolicy: "newer" });
    const raw = readFileSync(join(linked, ".agents", BACK_POINTER_FILE), "utf-8");
    expect(raw).toBe(`${main}\n`);
  });

  it("dry-run on an already-migrated worktree reports already-migrated, not planned", () => {
    const main = createMainRepo("bp-dry-after-apply");
    const linked = addWorktree(main, "feat/dry-after");
    seedPreA3WorktreeLayout(linked);

    runMigration({ cwd: linked, apply: true, conflictPolicy: "newer" });
    const dry = runMigration({ cwd: linked, apply: false, conflictPolicy: "newer" });
    expect(dry.status).toBe("already-migrated");
  });

  it("re-apply doesn't change the back-pointer mtime when nothing moves", () => {
    const main = createMainRepo("bp-stable");
    const linked = addWorktree(main, "feat/stable");
    seedPreA3WorktreeLayout(linked);
    runMigration({ cwd: linked, apply: true, conflictPolicy: "newer" });
    const t1 = statSync(join(linked, ".agents", BACK_POINTER_FILE)).mtimeMs;
    runMigration({ cwd: linked, apply: true, conflictPolicy: "newer" });
    const t2 = statSync(join(linked, ".agents", BACK_POINTER_FILE)).mtimeMs;
    // Already-migrated short-circuits before any writes, so the file is
    // byte-and-mtime stable across re-runs.
    expect(t2).toBe(t1);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Partial-leftover detection (EXDEV recovery)
// ─────────────────────────────────────────────────────────────────────────────
//
// The EXDEV path stages `<dst>.partial.loaf-migrate` and atomically renames it
// onto the final destination. A run that gets interrupted before the rename
// leaves the partial behind. `runMigration` must refuse on the next run and
// surface the partial paths so the user can recover deterministically (rather
// than risk silently picking the staged copy as a conflict winner).

describe("runMigration — partial-leftover detection", () => {
  it("refuses to run when the destination tree contains a *.partial.loaf-migrate path", () => {
    const main = createMainRepo("partial-refuse");
    const linked = addWorktree(main, "feat/partial");
    seedPreA3WorktreeLayout(linked);

    // Simulate an interrupted EXDEV stage: a half-written partial under the
    // main worktree's `.agents/`.
    const partialPath = join(
      main,
      ".agents",
      `kb${PARTIAL_SUFFIX}`,
    );
    mkdirSync(dirname(partialPath), { recursive: true });
    writeFileSync(partialPath, "half-staged content\n", "utf-8");

    const result = runMigration({ cwd: linked, apply: false, conflictPolicy: "newer" });
    expect(result.status).toBe("partial-leftover");
    expect(result.partials).toBeDefined();
    expect(result.partials).toContain(partialPath);
    // Message must list the partial path so users can act on it.
    expect(result.message).toContain(partialPath);
    // Message must mention the suffix so the user can grep / understand.
    expect(result.message).toContain(PARTIAL_SUFFIX);
  });

  it("lists multiple partials in the refusal message", () => {
    const main = createMainRepo("partial-multi");
    const linked = addWorktree(main, "feat/multi");
    seedPreA3WorktreeLayout(linked);

    const paths = [
      join(main, ".agents", `kb${PARTIAL_SUFFIX}`),
      join(main, ".agents", `tasks${PARTIAL_SUFFIX}`),
      join(main, ".agents", "nested", `dir${PARTIAL_SUFFIX}`),
    ];
    for (const p of paths) {
      mkdirSync(dirname(p), { recursive: true });
      writeFileSync(p, "partial\n", "utf-8");
    }

    const result = runMigration({ cwd: linked, apply: false, conflictPolicy: "newer" });
    expect(result.status).toBe("partial-leftover");
    for (const p of paths) {
      expect(result.partials).toContain(p);
      expect(result.message).toContain(p);
    }
  });

  it("after manual cleanup of the partial, re-running migrate completes normally", () => {
    const main = createMainRepo("partial-cleanup-recovery");
    const linked = addWorktree(main, "feat/recover");
    seedPreA3WorktreeLayout(linked);

    const partialPath = join(main, ".agents", `kb${PARTIAL_SUFFIX}`);
    mkdirSync(dirname(partialPath), { recursive: true });
    writeFileSync(partialPath, "partial\n", "utf-8");

    // First run: refused.
    const refused = runMigration({ cwd: linked, apply: true, conflictPolicy: "newer" });
    expect(refused.status).toBe("partial-leftover");

    // Manual cleanup (the user deletes the partial).
    rmSync(partialPath, { force: true });

    // Re-run: now completes normally.
    const applied = runMigration({ cwd: linked, apply: true, conflictPolicy: "newer" });
    expect(applied.status).toBe("applied");
    expect(readBackPointer(join(linked, ".agents"))).toBe(main);
  });

  it("apply refuses just like dry-run when partials are present", () => {
    const main = createMainRepo("partial-apply-refuse");
    const linked = addWorktree(main, "feat/apply-refuse");
    seedPreA3WorktreeLayout(linked);

    const partialPath = join(main, ".agents", `kb${PARTIAL_SUFFIX}`);
    mkdirSync(dirname(partialPath), { recursive: true });
    writeFileSync(partialPath, "partial\n", "utf-8");

    const result = runMigration({ cwd: linked, apply: true, conflictPolicy: "newer" });
    expect(result.status).toBe("partial-leftover");
    // Worktree-local files MUST still be in place — refusal must not have
    // mutated anything.
    expect(existsSync(join(linked, ".agents", "AGENTS.md"))).toBe(true);
  });

  it("ignores partial-looking paths that don't end in the exact suffix", () => {
    // Defense against an over-eager prefix match — only the explicit suffix
    // qualifies as a "partial".
    const main = createMainRepo("partial-suffix-strict");
    const linked = addWorktree(main, "feat/strict");
    seedPreA3WorktreeLayout(linked);

    // A user file that happens to include the prefix but doesn't end with it.
    writeFileSync(
      join(main, ".agents", "ideas-2026.partial-thoughts.md"),
      "not a partial\n",
      "utf-8",
    );

    const result = runMigration({ cwd: linked, apply: false, conflictPolicy: "newer" });
    expect(result.status).toBe("planned");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Main-worktree-missing detection (final pre-merge polish)
// ─────────────────────────────────────────────────────────────────────────────
//
// `findMainWorktreeRoot` resolves via git rev-parse — it cheerfully returns
// the path recorded in `.git/worktrees/<name>/commondir`, even if that path
// no longer exists. Migrating into a non-existent target is worse than useless:
// it would silently mkdir a stale tree, or fail late with a confusing
// filesystem error. The fix surfaces this case before any mutation.

describe("runMigration — main worktree target missing", () => {
  it("errors with main-missing status when the main worktree directory has been deleted", () => {
    const main = createMainRepo("mm-deleted");
    const linked = addWorktree(main, "feat/deleted");
    seedPreA3WorktreeLayout(linked);

    // Delete the main worktree root after the linked worktree was created.
    // git still reports the main path via rev-parse, but the directory is gone.
    rmSync(main, { recursive: true, force: true });

    const result = runMigration({ cwd: linked, apply: false, conflictPolicy: "newer" });
    expect(result.status).toBe("main-missing");
    expect(result.message).toContain(main);
    expect(result.message).toContain("not found");
    expect(result.message).toContain("git worktree list");
  });

  it("errors with main-missing status when the main worktree path is a file (not a directory)", () => {
    const main = createMainRepo("mm-file");
    const linked = addWorktree(main, "feat/file");
    seedPreA3WorktreeLayout(linked);

    // Replace the main worktree with a regular file.
    rmSync(main, { recursive: true, force: true });
    writeFileSync(main, "not a directory\n", "utf-8");

    const result = runMigration({ cwd: linked, apply: false, conflictPolicy: "newer" });
    expect(result.status).toBe("main-missing");
    expect(result.message).toContain("is not a directory");
  });

  it("apply does not mutate anything when the main worktree is missing", () => {
    const main = createMainRepo("mm-no-mutate");
    const linked = addWorktree(main, "feat/no-mutate");
    seedPreA3WorktreeLayout(linked);
    rmSync(main, { recursive: true, force: true });

    const before = readFileSync(join(linked, ".agents", "AGENTS.md"), "utf-8");
    const result = runMigration({ cwd: linked, apply: true, conflictPolicy: "newer" });
    expect(result.status).toBe("main-missing");
    // Worktree-local files must still be there, unmodified.
    expect(readFileSync(join(linked, ".agents", "AGENTS.md"), "utf-8")).toBe(before);
  });
});

describe("buildMainMissingMessage", () => {
  it("uses 'not found' wording when the path does not exist", () => {
    const msg = buildMainMissingMessage("/some/path", false);
    expect(msg).toContain("/some/path");
    expect(msg).toContain("not found");
    expect(msg).not.toContain("is not a directory");
  });

  it("uses 'is not a directory' wording when the path exists but isn't a dir", () => {
    const msg = buildMainMissingMessage("/some/path", true);
    expect(msg).toContain("/some/path");
    expect(msg).toContain("is not a directory");
    expect(msg).not.toContain("not found");
  });
});

describe("detectMainMissingForRefusal — refusal-nudge integration", () => {
  it("returns null in a healthy linked worktree", () => {
    const main = createMainRepo("dm-healthy");
    const linked = addWorktree(main, "feat/healthy");
    seedPreA3WorktreeLayout(linked);
    expect(detectMainMissingForRefusal(linked)).toBeNull();
  });

  it("returns the missing-target message when the main worktree directory is gone", () => {
    const main = createMainRepo("dm-gone");
    const linked = addWorktree(main, "feat/gone");
    seedPreA3WorktreeLayout(linked);
    rmSync(main, { recursive: true, force: true });

    const msg = detectMainMissingForRefusal(linked);
    expect(msg).not.toBeNull();
    expect(msg).toContain(main);
    expect(msg).toContain("not found");
  });

  it("returns null in a main checkout (no linked worktree to be missing)", () => {
    const main = createMainRepo("dm-main-only");
    expect(detectMainMissingForRefusal(main)).toBeNull();
  });

  it("returns null outside a git context", () => {
    const nonGit = mkdtempSync(join(TEST_ROOT, "dm-no-git-"));
    expect(detectMainMissingForRefusal(nonGit)).toBeNull();
  });
});
