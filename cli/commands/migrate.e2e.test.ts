/**
 * SPEC-036 / TASK-170 — Refusal nudge + LOAF_DEBUG_RESOLVE e2e
 *
 * Spawns the built `dist-cli/index.js` against real git fixtures because the
 * refusal nudge runs *before* `program.parse()` in `cli/index.ts` and is
 * therefore awkward to exercise in-process.
 *
 * Why e2e (option (a) from the task brief): the dispatcher behavior is the
 * actual product surface — users see it via `loaf <cmd>`. An in-process unit
 * test would have to mock `process.argv` and re-import the entrypoint, which
 * is fragile and doesn't catch wiring regressions. The trade is slower runs
 * (a few hundred ms per spawn) for behavior fidelity.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { spawn, execFileSync } from "child_process";
import {
  mkdirSync,
  mkdtempSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "fs";
import { join } from "path";
import { tmpdir } from "os";

import { BACK_POINTER_FILE } from "../lib/migrate/worktree-storage.js";

vi.setConfig({ testTimeout: 30000 });

let TEST_ROOT: string;
const CLI_PATH = join(process.cwd(), "dist-cli/index.js");

const TEST_IDENTITY = [
  "-c",
  "user.name=Test User",
  "-c",
  "user.email=test@test.com",
] as const;

function git(args: readonly string[], cwd: string): void {
  execFileSync("git", args as string[], { cwd, stdio: ["ignore", "pipe", "pipe"] });
}

function createMainRepo(name: string): string {
  const repoPath = realpathSync(mkdtempSync(join(TEST_ROOT, `${name}-`)));
  git(["init", "--initial-branch=main"], repoPath);
  writeFileSync(join(repoPath, "README.md"), "# Test\n", "utf-8");
  git(["add", "."], repoPath);
  git([...TEST_IDENTITY, "commit", "-m", "Initial commit"], repoPath);
  mkdirSync(join(repoPath, ".agents"), { recursive: true });
  writeFileSync(
    join(repoPath, ".agents", "AGENTS.md"),
    "# Project Instructions\n",
    "utf-8",
  );
  return repoPath;
}

function addWorktree(repoPath: string, branch: string): string {
  const wtPath = `${repoPath}-wt-${branch}`;
  git(["worktree", "add", "-b", branch, wtPath], repoPath);
  return realpathSync(wtPath);
}

function seedPreA3WorktreeLayout(worktreePath: string): void {
  const agents = join(worktreePath, ".agents");
  mkdirSync(agents, { recursive: true });
  writeFileSync(join(agents, "AGENTS.md"), "# Worktree AGENTS\n", "utf-8");
  mkdirSync(join(agents, "sessions"), { recursive: true });
  writeFileSync(
    join(agents, "sessions", "20260519-120000-session.md"),
    "# Session\n",
    "utf-8",
  );
  mkdirSync(join(agents, "kb"), { recursive: true });
  writeFileSync(join(agents, "kb", "note.md"), "# Note\n", "utf-8");
}

async function runLoaf(
  args: string[],
  options: { cwd: string; env?: NodeJS.ProcessEnv },
): Promise<{ stdout: string; stderr: string; exitCode: number }> {
  return new Promise((resolve) => {
    const child = spawn("node", [CLI_PATH, ...args], {
      cwd: options.cwd,
      stdio: ["ignore", "pipe", "pipe"],
      env: { ...process.env, ...options.env },
    });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (d) => (stdout += d.toString()));
    child.stderr.on("data", (d) => (stderr += d.toString()));
    child.on("close", (code) => resolve({ stdout, stderr, exitCode: code ?? 0 }));
  });
}

beforeEach(() => {
  TEST_ROOT = realpathSync(mkdtempSync(join(tmpdir(), "loaf-migrate-e2e-")));
});

afterEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
});

// ─────────────────────────────────────────────────────────────────────────────
// Help discoverability
// ─────────────────────────────────────────────────────────────────────────────

describe("loaf migrate worktree-storage --help", () => {
  it("documents the dry-run default and the LOAF_DEBUG_RESOLVE knob", async () => {
    const main = createMainRepo("help-main");
    const r = await runLoaf(["migrate", "worktree-storage", "--help"], { cwd: main });
    expect(r.exitCode).toBe(0);
    expect(r.stdout + r.stderr).toContain("--apply");
    expect(r.stdout + r.stderr).toContain("dry-run");
    expect(r.stdout + r.stderr).toContain("LOAF_DEBUG_RESOLVE");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Refusal nudge
// ─────────────────────────────────────────────────────────────────────────────

describe("pre-A3 refusal nudge", () => {
  it("refuses arbitrary commands in a populated linked worktree", async () => {
    const main = createMainRepo("nudge-refuse");
    const linked = addWorktree(main, "feat/nudge-refuse");
    seedPreA3WorktreeLayout(linked);

    const r = await runLoaf(["session", "list"], { cwd: linked });
    expect(r.exitCode).toBe(2);
    expect(r.stderr).toContain("SPEC-036");
    expect(r.stderr).toContain("loaf migrate worktree-storage");
    expect(r.stderr).toContain("LOAF_DEBUG_RESOLVE");
  });

  it("allows `loaf migrate worktree-storage` to run in pre-A3 state", async () => {
    const main = createMainRepo("nudge-allow-migrate");
    const linked = addWorktree(main, "feat/nudge-allow");
    seedPreA3WorktreeLayout(linked);

    // Dry-run should succeed (exit 0) without triggering the refusal.
    const r = await runLoaf(["migrate", "worktree-storage"], { cwd: linked });
    expect(r.exitCode).toBe(0);
    expect(r.stdout).toContain("Dry run");
    expect(r.stderr).not.toContain("SPEC-036 centralizes");
  });

  it("allows `--help` to surface even in pre-A3 state", async () => {
    const main = createMainRepo("nudge-allow-help");
    const linked = addWorktree(main, "feat/nudge-help");
    seedPreA3WorktreeLayout(linked);

    const r = await runLoaf(["--help"], { cwd: linked });
    expect(r.exitCode).toBe(0);
    expect(r.stderr).not.toContain("SPEC-036 centralizes");
  });

  it("allows `--version` to surface even in pre-A3 state", async () => {
    const main = createMainRepo("nudge-allow-version");
    const linked = addWorktree(main, "feat/nudge-version");
    seedPreA3WorktreeLayout(linked);

    const r = await runLoaf(["--version"], { cwd: linked });
    expect(r.exitCode).toBe(0);
    expect(r.stderr).not.toContain("SPEC-036 centralizes");
  });

  it("does NOT refuse in a main checkout (no linked worktrees, no .agents/ outside main)", async () => {
    const main = createMainRepo("nudge-main-checkout");
    const r = await runLoaf(["version"], { cwd: main });
    // `loaf version` may exit 0 or 1 depending on environment; what we care
    // about is that it isn't blocked by the refusal nudge (exit 2 + refusal text).
    expect(r.exitCode).not.toBe(2);
    expect(r.stderr).not.toContain("SPEC-036 centralizes");
  });

  it("does NOT refuse in a linked worktree once the back-pointer is in place", async () => {
    const main = createMainRepo("nudge-already-migrated");
    const linked = addWorktree(main, "feat/migrated");
    // Mimic post-migration state: worktree-local `.agents/` has only the
    // back-pointer, pointing at main.
    mkdirSync(join(linked, ".agents"), { recursive: true });
    writeFileSync(
      join(linked, ".agents", BACK_POINTER_FILE),
      `${main}\n`,
      "utf-8",
    );

    const r = await runLoaf(["version"], { cwd: linked });
    expect(r.exitCode).not.toBe(2);
    expect(r.stderr).not.toContain("SPEC-036 centralizes");
  });

  it("DOES refuse when the back-pointer points to a stale/nonexistent path", async () => {
    const main = createMainRepo("nudge-stale-ptr");
    const linked = addWorktree(main, "feat/stale-ptr");
    seedPreA3WorktreeLayout(linked);
    writeFileSync(
      join(linked, ".agents", BACK_POINTER_FILE),
      "/this/does/not/exist\n",
      "utf-8",
    );

    const r = await runLoaf(["session", "list"], { cwd: linked });
    expect(r.exitCode).toBe(2);
    expect(r.stderr).toContain("SPEC-036");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Mutually-exclusive conflict-override flags
// ─────────────────────────────────────────────────────────────────────────────

describe("loaf migrate worktree-storage — flag validation", () => {
  it("exits non-zero when both --force-from-worktree and --force-from-main are set", async () => {
    const main = createMainRepo("flag-mutex");
    const linked = addWorktree(main, "feat/flag-mutex");
    seedPreA3WorktreeLayout(linked);

    const r = await runLoaf(
      [
        "migrate",
        "worktree-storage",
        "--force-from-worktree",
        "--force-from-main",
      ],
      { cwd: linked },
    );
    expect(r.exitCode).not.toBe(0);
    // Error message must mention both flags so the user knows what to fix.
    expect(r.stderr).toContain("--force-from-worktree");
    expect(r.stderr).toContain("--force-from-main");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// LOAF_DEBUG_RESOLVE
// ─────────────────────────────────────────────────────────────────────────────

describe("LOAF_DEBUG_RESOLVE observability knob", () => {
  it("surfaces the parent-walk fallback diagnostic when set in a non-git directory", async () => {
    // Use a non-git directory so the git probe definitely errors.
    const nonGit = realpathSync(mkdtempSync(join(TEST_ROOT, "no-git-debug-")));
    mkdirSync(join(nonGit, ".agents"), { recursive: true });
    writeFileSync(join(nonGit, ".agents", "AGENTS.md"), "# x\n", "utf-8");

    // `loaf version` triggers findAgentsDir via init/etc, but more reliably
    // we can use `loaf migrate worktree-storage` which calls
    // findMainWorktreeRoot directly. With LOAF_DEBUG_RESOLVE=1 the catch
    // branch writes a diagnostic line to stderr.
    const r = await runLoaf(["migrate", "worktree-storage"], {
      cwd: nonGit,
      env: { LOAF_DEBUG_RESOLVE: "1" },
    });
    // Diagnostic written via process.stderr.write — we just assert presence.
    expect(r.stderr).toContain("LOAF_DEBUG_RESOLVE");
    expect(r.stderr).toContain("findMainWorktreeRoot fell back to parent-walk");
  });
});
