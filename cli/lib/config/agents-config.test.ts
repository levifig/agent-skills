/**
 * SPEC-042 Track A — `.agents/loaf.json` routing across linked worktrees.
 *
 * Verifies that `readLoafConfig`, `loafConfigPath`, and `mergeLoafConfigIntegrations`
 * resolve to the main worktree's `.agents/loaf.json` when invoked from a linked
 * git worktree that has been migrated under SPEC-036. Without this, a linked
 * worktree's `loaf.json` is invisible to release/kb/install code paths that
 * resolve `.agents/` via `findAgentsDir`, and writes silently create a stray
 * config file next to the `.moved-to` back-pointer.
 *
 * Fixture pattern mirrors `cli/lib/migrate/worktree-storage.test.ts` — real
 * `git init` + `git worktree add`, no git mocking.
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "fs";
import { execFileSync } from "child_process";
import { join } from "path";
import { tmpdir } from "os";

import {
  loafConfigPath,
  mergeLoafConfigIntegrations,
  readLoafConfig,
} from "./agents-config.js";

// ─────────────────────────────────────────────────────────────────────────────
// Scaffolding (mirrors cli/lib/migrate/worktree-storage.test.ts)
// ─────────────────────────────────────────────────────────────────────────────

let TEST_ROOT: string;

const TEST_IDENTITY = [
  "-c",
  "user.name=Test User",
  "-c",
  "user.email=test@test.com",
] as const;

function git(args: readonly string[], cwd: string): string {
  return execFileSync("git", args as string[], {
    cwd,
    stdio: ["ignore", "pipe", "pipe"],
  }).toString();
}

function createMainRepo(name: string): string {
  const repoPath = realpathSync(mkdtempSync(join(TEST_ROOT, `${name}-`)));
  git(["init", "--initial-branch=main"], repoPath);
  writeFileSync(join(repoPath, "README.md"), "# Test\n", "utf-8");
  git(["add", "."], repoPath);
  git([...TEST_IDENTITY, "commit", "-m", "Initial commit"], repoPath);
  mkdirSync(join(repoPath, ".agents"), { recursive: true });
  return repoPath;
}

function addWorktree(repoPath: string, branch: string): string {
  const wtPath = `${repoPath}-wt-${branch.replace(/\//g, "-")}`;
  git(["worktree", "add", "-b", branch, wtPath], repoPath);
  return realpathSync(wtPath);
}

/**
 * Convert a linked worktree into the post-migration shape: empty `.agents/`
 * containing only a `.moved-to` back-pointer to the main worktree.
 */
function migrateLinkedWorktree(linked: string, mainRoot: string): void {
  const agents = join(linked, ".agents");
  mkdirSync(agents, { recursive: true });
  writeFileSync(join(agents, ".moved-to"), `${mainRoot}\n`, "utf-8");
}

beforeEach(() => {
  TEST_ROOT = realpathSync(mkdtempSync(join(tmpdir(), "loaf-agents-config-")));
});

afterEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
});

// ─────────────────────────────────────────────────────────────────────────────
// Single-checkout (regression)
// ─────────────────────────────────────────────────────────────────────────────

describe("agents-config — single-checkout regression", () => {
  it("loafConfigPath resolves to <root>/.agents/loaf.json for a plain repo", () => {
    const main = createMainRepo("sc-path");
    expect(loafConfigPath(main)).toBe(join(main, ".agents", "loaf.json"));
  });

  it("readLoafConfig reads from <root>/.agents/loaf.json", () => {
    const main = createMainRepo("sc-read");
    writeFileSync(
      join(main, ".agents", "loaf.json"),
      JSON.stringify({ release: { versionFiles: ["pyproject.toml"] } }, null, 2) + "\n",
      "utf-8",
    );
    const cfg = readLoafConfig(main);
    expect(cfg.release?.versionFiles).toEqual(["pyproject.toml"]);
  });

  it("mergeLoafConfigIntegrations writes to <root>/.agents/loaf.json", () => {
    const main = createMainRepo("sc-write");
    mergeLoafConfigIntegrations(main, { linear: { enabled: true } });

    const written = JSON.parse(
      readFileSync(join(main, ".agents", "loaf.json"), "utf-8"),
    );
    expect(written.integrations?.linear).toEqual({ enabled: true });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Migrated linked worktree (SPEC-036)
// ─────────────────────────────────────────────────────────────────────────────

describe("agents-config — migrated linked worktree", () => {
  it("readLoafConfig follows the worktree resolution to the main worktree's loaf.json", () => {
    const main = createMainRepo("wt-read");
    const linked = addWorktree(main, "feat/read");
    migrateLinkedWorktree(linked, main);

    writeFileSync(
      join(main, ".agents", "loaf.json"),
      JSON.stringify(
        { release: { versionFiles: ["pyproject.toml"] } },
        null,
        2,
      ) + "\n",
      "utf-8",
    );

    const cfg = readLoafConfig(linked);
    expect(cfg.release?.versionFiles).toEqual(["pyproject.toml"]);
  });

  it("loafConfigPath returns the main worktree's path when called from a linked worktree", () => {
    const main = createMainRepo("wt-path");
    const linked = addWorktree(main, "feat/path");
    migrateLinkedWorktree(linked, main);

    expect(loafConfigPath(linked)).toBe(join(main, ".agents", "loaf.json"));
  });

  it("mergeLoafConfigIntegrations writes to the main worktree's loaf.json (not the linked worktree's)", () => {
    const main = createMainRepo("wt-write");
    const linked = addWorktree(main, "feat/write");
    migrateLinkedWorktree(linked, main);

    // Sanity: linked has only the back-pointer.
    expect(existsSync(join(linked, ".agents", "loaf.json"))).toBe(false);
    expect(existsSync(join(linked, ".agents", ".moved-to"))).toBe(true);

    mergeLoafConfigIntegrations(linked, { linear: { enabled: true } });

    // Write must land in main, not in the linked worktree.
    const mainConfigPath = join(main, ".agents", "loaf.json");
    expect(existsSync(mainConfigPath)).toBe(true);
    const written = JSON.parse(readFileSync(mainConfigPath, "utf-8"));
    expect(written.integrations?.linear).toEqual({ enabled: true });

    // No stray loaf.json next to the back-pointer.
    expect(existsSync(join(linked, ".agents", "loaf.json"))).toBe(false);
  });

  it("read preserves existing main config and merges integrations without clobbering siblings", () => {
    const main = createMainRepo("wt-merge");
    const linked = addWorktree(main, "feat/merge");
    migrateLinkedWorktree(linked, main);

    writeFileSync(
      join(main, ".agents", "loaf.json"),
      JSON.stringify(
        {
          release: { versionFiles: ["pyproject.toml"] },
          integrations: { serena: { enabled: false } },
        },
        null,
        2,
      ) + "\n",
      "utf-8",
    );

    mergeLoafConfigIntegrations(linked, { linear: { enabled: true } });

    const written = JSON.parse(
      readFileSync(join(main, ".agents", "loaf.json"), "utf-8"),
    );
    expect(written.release?.versionFiles).toEqual(["pyproject.toml"]);
    expect(written.integrations?.linear).toEqual({ enabled: true });
    expect(written.integrations?.serena).toEqual({ enabled: false });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Defensive fallback
// ─────────────────────────────────────────────────────────────────────────────

describe("agents-config — defensive fallback", () => {
  it("falls through to projectRoot when not in a git worktree at all", () => {
    // Plain tmp dir, no git init — `findMainWorktreeRoot` returns null and the
    // helper must keep the legacy behavior verbatim.
    const nonGit = realpathSync(mkdtempSync(join(TEST_ROOT, "no-git-")));
    expect(loafConfigPath(nonGit)).toBe(join(nonGit, ".agents", "loaf.json"));

    mkdirSync(join(nonGit, ".agents"), { recursive: true });
    writeFileSync(
      join(nonGit, ".agents", "loaf.json"),
      JSON.stringify({ release: { versionFiles: ["pyproject.toml"] } }, null, 2) + "\n",
      "utf-8",
    );
    const cfg = readLoafConfig(nonGit);
    expect(cfg.release?.versionFiles).toEqual(["pyproject.toml"]);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Linked worktree with unreachable main
// ─────────────────────────────────────────────────────────────────────────────
//
// When the main worktree directory has been removed, `git rev-parse
// --git-common-dir` fails and `findMainWorktreeRoot` returns null — the same
// signal a single-checkout repo produces. The previous fallback wrote a
// stale `loaf.json` into the linked worktree's `.agents/`, invisible to
// every other tool that resolves via `findAgentsDir`. The helper now detects
// the linked-worktree state from the `.git` pointer file and refuses to
// write a shadow config, throwing the same actionable message the migrate
// command surfaces.

describe("agents-config — linked worktree with main removed", () => {
  it("readLoafConfig throws when the main worktree directory has been deleted", () => {
    const main = createMainRepo("mm-read");
    const linked = addWorktree(main, "feat/mm-read");
    rmSync(main, { recursive: true, force: true });

    expect(() => readLoafConfig(linked)).toThrow(/Main worktree at .+ not found/);
  });

  it("loafConfigPath throws when the main worktree directory has been deleted", () => {
    const main = createMainRepo("mm-path");
    const linked = addWorktree(main, "feat/mm-path");
    rmSync(main, { recursive: true, force: true });

    expect(() => loafConfigPath(linked)).toThrow(/Main worktree at .+ not found/);
  });

  it("mergeLoafConfigIntegrations throws AND does not create loaf.json under the linked worktree", () => {
    const main = createMainRepo("mm-write");
    const linked = addWorktree(main, "feat/mm-write");
    rmSync(main, { recursive: true, force: true });

    expect(() =>
      mergeLoafConfigIntegrations(linked, { linear: { enabled: true } }),
    ).toThrow(/Main worktree at .+ not found/);

    // Critical guarantee of the fix: no stray loaf.json was created next to
    // the back-pointer (or anywhere else in the linked worktree's .agents/).
    expect(existsSync(join(linked, ".agents", "loaf.json"))).toBe(false);
  });

  it("error message points at the recorded main path and suggests recovery", () => {
    const main = createMainRepo("mm-msg");
    const linked = addWorktree(main, "feat/mm-msg");
    rmSync(main, { recursive: true, force: true });

    let captured: Error | null = null;
    try {
      readLoafConfig(linked);
    } catch (err) {
      captured = err as Error;
    }
    expect(captured).not.toBeNull();
    expect(captured!.message).toContain(main);
    expect(captured!.message).toContain("git worktree list");
  });

  it("throws when the recorded main path is a file (not a directory)", () => {
    const main = createMainRepo("mm-isfile");
    const linked = addWorktree(main, "feat/mm-isfile");
    rmSync(main, { recursive: true, force: true });
    writeFileSync(main, "not a directory\n", "utf-8");

    expect(() => readLoafConfig(linked)).toThrow(/is not a directory/);
    expect(existsSync(join(linked, ".agents", "loaf.json"))).toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Linked worktree with malformed `.git` pointer file
// ─────────────────────────────────────────────────────────────────────────────
//
// When the `.git` file inside a linked worktree is corrupt or unparseable —
// missing the `gitdir:` line, or pointing somewhere that doesn't match the
// `<main>/.git/worktrees/<name>` shape — `readGitdirPointerMainRoot` returns
// null. The resolver deliberately falls back to `projectRoot` instead of
// throwing (see Case 4 in the resolveEffectiveRoot docblock). These tests
// lock that behavior in so it's not changed accidentally.

describe("agents-config — linked worktree with malformed .git pointer", () => {
  it("falls back to projectRoot when the .git pointer file has no gitdir: line", () => {
    const main = createMainRepo("mp-nogitdir");
    const linked = addWorktree(main, "feat/mp-nogitdir");
    writeFileSync(join(linked, ".git"), "totally bogus contents\n", "utf-8");

    expect(loafConfigPath(linked)).toBe(join(linked, ".agents", "loaf.json"));
  });

  it("falls back to projectRoot when the gitdir: line doesn't match <main>/.git/worktrees/<name>", () => {
    const main = createMainRepo("mp-badshape");
    const linked = addWorktree(main, "feat/mp-badshape");
    writeFileSync(join(linked, ".git"), "gitdir: /tmp/some-arbitrary-path\n", "utf-8");

    expect(loafConfigPath(linked)).toBe(join(linked, ".agents", "loaf.json"));
  });

  it("readLoafConfig returns {} for a malformed-pointer worktree with no .agents/loaf.json", () => {
    const main = createMainRepo("mp-read");
    const linked = addWorktree(main, "feat/mp-read");
    writeFileSync(join(linked, ".git"), "junk\n", "utf-8");

    expect(readLoafConfig(linked)).toEqual({});
  });

  it("mergeLoafConfigIntegrations writes into the linked worktree's .agents/ when the pointer is malformed", () => {
    const main = createMainRepo("mp-write");
    const linked = addWorktree(main, "feat/mp-write");
    writeFileSync(join(linked, ".git"), "junk\n", "utf-8");

    mergeLoafConfigIntegrations(linked, { linear: { enabled: true } });

    // With no parseable pointer, the resolver can't identify a main worktree,
    // so the write lands locally. This is the documented Case 4 fallback —
    // not ideal, but preferable to crashing every loaf.json consumer on a
    // corrupt pointer. See resolveEffectiveRoot docblock.
    const linkedConfigPath = join(linked, ".agents", "loaf.json");
    expect(existsSync(linkedConfigPath)).toBe(true);
    const written = JSON.parse(readFileSync(linkedConfigPath, "utf-8"));
    expect(written.integrations?.linear).toEqual({ enabled: true });
  });
});
