/**
 * Project Root Resolution
 *
 * Locates the .agents/ directory by walking up from the current working
 * directory. Shared by task and spec CLI commands.
 *
 * Worktree behavior (SPEC-036, A3):
 *   When invoked from a linked git worktree, `.agents/` is resolved to the
 *   **main worktree's** directory — agentic state is project-scoped, not
 *   branch-scoped, so sessions, IDs, and knowledge converge regardless of
 *   which worktree the user (or hook) is currently in. In a main checkout
 *   (single-worktree repo, or the main worktree of a multi-worktree repo)
 *   and outside any git context, the original parent-walk behavior is
 *   preserved verbatim.
 */

import { execFileSync } from "child_process";
import { existsSync, realpathSync } from "fs";
import { join, dirname, resolve } from "path";

import { loadIndex, buildIndexFromFiles } from "./migrate.js";
import { withTasksJsonLock } from "./lock.js";
import type { TaskIndex } from "./types.js";

/**
 * Environment variable name for the resolver-diagnostics knob.
 *
 * When set to any non-empty value, `findMainWorktreeRoot` will:
 *   - Inherit git's stderr to the user (instead of swallowing it).
 *   - Emit a one-line note to `process.stderr` whenever the git probe fails
 *     and we fall back to parent-walk.
 *
 * Referenced by `cli/lib/migrate/worktree-storage.ts` so the migrate command's
 * help text and the resolver stay in sync.
 */
export const DEBUG_RESOLVE_ENV = "LOAF_DEBUG_RESOLVE";

export function isDebugResolveEnabled(): boolean {
  const v = process.env[DEBUG_RESOLVE_ENV];
  return v !== undefined && /^(1|true|yes|on)$/i.test(v);
}

/**
 * Normalize a path for cross-FS-quirk comparison:
 *
 *   - Resolve symlinks / junctions via `realpathSync.native` (falls back to
 *     `realpathSync` on older Node versions or platforms that lack the
 *     native variant) so two paths that point at the same inode/file compare
 *     equal even if one was reached through a symlink and the other wasn't.
 *   - On Windows only, lowercase the result so case-insensitive paths
 *     compare equal. On POSIX, case matters — we leave the path alone.
 *
 * If `realpathSync` itself throws (the path was removed mid-call, the
 * directory is in a transient state, etc.) we fail open and return the
 * input unchanged. The downstream comparison then uses raw strings, which
 * is the pre-fix behavior — strictly never worse than what we had.
 */
function normalizePathForComparison(p: string): string {
  const canonical = realpathOrSelf(p);
  return process.platform === "win32" ? canonical.toLowerCase() : canonical;
}

/**
 * Best-effort `realpathSync.native` with a fall-open to the input on any
 * filesystem error. Used by `normalizePathForComparison` and by the
 * worktree-root return value (where we want the canonical path, but we
 * never want to crash the resolver because realpath failed in a transient
 * state).
 */
function realpathOrSelf(p: string): string {
  try {
    const fn = (realpathSync as typeof realpathSync & { native?: typeof realpathSync }).native
      ?? realpathSync;
    return fn(p);
  } catch {
    return p;
  }
}

/**
 * Probe whether `startDir` is inside a linked git worktree and, if so, return
 * the absolute path to the main worktree's root. Returns null when not in a
 * linked worktree (including: main checkout, outside any git repo, or any
 * failure invoking git — we fail open and let the caller fall back to the
 * parent-walk path).
 *
 * Exported so the `loaf migrate worktree-storage` command and the top-level
 * refusal-nudge dispatcher can share the exact same probe semantics.
 */
export function findMainWorktreeRoot(startDir: string): string | null {
  // Requires git ≥ 2.31 for the `--path-format=absolute` flag. Older git
  // versions will hit the catch block and fall through to parent-walk,
  // which is the safe degradation.
  const debug = isDebugResolveEnabled();
  const stderrMode: "ignore" | "inherit" = debug ? "inherit" : "ignore";
  try {
    // `--git-dir` is the worktree's own git directory (e.g. `.git` for the
    // main checkout; `<main>/.git/worktrees/<name>` for a linked worktree).
    // `--git-common-dir` is the shared `.git/` directory. They differ iff we
    // are inside a linked worktree. `--path-format=absolute` makes both paths
    // canonical so we can compare them safely.
    const rawGitDir = execFileSync(
      "git",
      ["rev-parse", "--path-format=absolute", "--git-dir"],
      { cwd: startDir, stdio: ["ignore", "pipe", stderrMode] },
    ).toString().trim();
    const rawCommonDir = execFileSync(
      "git",
      ["rev-parse", "--path-format=absolute", "--git-common-dir"],
      { cwd: startDir, stdio: ["ignore", "pipe", stderrMode] },
    ).toString().trim();

    if (!rawGitDir || !rawCommonDir) return null;

    // Belt-and-suspenders: resolve against `startDir` in case an older git
    // ignores `--path-format=absolute` and returns a relative path.
    const rawGitAbs = resolve(startDir, rawGitDir);
    const rawCommonAbs = resolve(startDir, rawCommonDir);

    // Normalize both paths through `realpathSync` (and lowercase on win32)
    // before comparing — otherwise symlinked CWDs, junctions, and case
    // differences can make two paths that point at the same FS object
    // compare unequal. Without this, the comparison disagreed with tests
    // that pre-`realpathSync()`d their fixture paths.
    const gitDir = normalizePathForComparison(rawGitAbs);
    const commonDir = normalizePathForComparison(rawCommonAbs);

    if (gitDir === commonDir) return null; // main checkout — caller parent-walks
    // Also covers git submodules: `--git-dir` and `--git-common-dir` resolve
    // to the same `<superproject>/.git/modules/<sub>`, so we return null and
    // the caller parent-walks within the submodule (intentional — submodules
    // are conceptually independent projects).

    // Linked worktree: the common dir is `<main-root>/.git` (or, rarely, a
    // bare-style path that doesn't end in `.git`). Walking up by one segment
    // when it ends in `.git` lands us at the main worktree root. We use the
    // pre-lowercase canonical path (`rawCommonAbs` re-resolved through
    // realpath) for the return value so callers get a usable path on POSIX —
    // lowercasing was for comparison, not for the on-disk path.
    const commonCanonical = realpathOrSelf(rawCommonAbs);
    if (
      commonCanonical.endsWith("/.git") ||
      commonCanonical.endsWith("\\.git")
    ) {
      return dirname(commonCanonical);
    }
    return null;
  } catch (err) {
    // Not a git repo, git not installed, or any other failure — let the
    // caller fall through to the parent-walk path.
    if (debug) {
      const msg = err instanceof Error ? err.message : String(err);
      process.stderr.write(
        `${DEBUG_RESOLVE_ENV}: findMainWorktreeRoot fell back to parent-walk (error: ${msg})\n`,
      );
    }
    return null;
  }
}

/**
 * Walk up from `startDir` looking for a `.agents/` directory.
 * Returns the absolute path to `.agents/` or null if not found.
 *
 * In a linked git worktree, returns the main worktree's `.agents/` directly
 * (without parent-walking the linked tree). See SPEC-036 for rationale.
 */
export function findAgentsDir(startDir: string = process.cwd()): string | null {
  const mainRoot = findMainWorktreeRoot(startDir);
  if (mainRoot) {
    const candidate = join(mainRoot, ".agents");
    return existsSync(candidate) ? candidate : null;
  }

  let current = startDir;

  while (true) {
    const candidate = join(current, ".agents");
    if (existsSync(candidate)) {
      return candidate;
    }

    const parent = dirname(current);
    if (parent === current) {
      // Reached filesystem root
      return null;
    }
    current = parent;
  }
}

/**
 * Load TASKS.json from the agents directory. If it doesn't exist, build
 * the index from .md files and persist it. Returns null only if the
 * index file exists but has an invalid shape.
 */
export function getOrBuildIndex(agentsDir: string): TaskIndex {
  const indexPath = join(agentsDir, "TASKS.json");

  if (existsSync(indexPath)) {
    const index = loadIndex(indexPath);
    if (index) return index;
    // Invalid shape — fall through to rebuild
  }

  // Cold-start build: do the build-and-persist inside the TASKS.json lock so
  // concurrent first-touches from multiple worktrees don't clobber each other
  // (and the result is the same content either way).
  return withTasksJsonLock(agentsDir, (current) => {
    const rebuilt = buildIndexFromFiles(agentsDir);
    current.version = rebuilt.version;
    current.next_id = rebuilt.next_id;
    current.tasks = rebuilt.tasks;
    current.specs = rebuilt.specs;
    return current;
  });
}
