/**
 * SPEC-036 / TASK-170 — Worktree storage migration
 *
 * Under SPEC-036 (A3), `.agents/` is project-scoped, not branch-scoped: every
 * worktree resolves to the **main worktree's** `.agents/` directory. Existing
 * checkouts that already populated a linked worktree's local `.agents/` need
 * a one-shot migration that moves everything into the main worktree's store.
 *
 * This module implements the migration as a pure function (`runMigration`)
 * plus a tiny commander adapter (`cli/commands/migrate.ts`). The function is
 * deliberately decoupled from process.exit / Commander so tests can call it
 * directly against tmp fixtures.
 *
 * Behavior summary:
 *   - Dry-run by default; `--apply` mutates.
 *   - Conflict policy: identical content is deduplicated; otherwise newest
 *     mtime wins, with `--force-from-worktree` / `--force-from-main` overrides.
 *   - After a successful `--apply`, writes a `.moved-to` back-pointer in the
 *     worktree-local `.agents/` so the refusal nudge can detect post-A3 state.
 *   - Idempotent: re-running on a migrated worktree is a no-op.
 *   - Run from the main checkout: clean no-op exit (nothing to migrate).
 *   - Outside a git context: error (the command is meaningless).
 */

import {
  cpSync,
  existsSync,
  mkdirSync,
  lstatSync,
  readFileSync,
  readdirSync,
  renameSync,
  rmSync,
  statSync,
  writeFileSync,
} from "fs";
import { dirname, join, relative } from "path";

import { DEBUG_RESOLVE_ENV, findMainWorktreeRoot, isDebugResolveEnabled } from "../tasks/resolve.js";

// ─────────────────────────────────────────────────────────────────────────────
// Missing-target error
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Build the error message shown when `findMainWorktreeRoot` returned a path
 * but that path does not exist (or is not a directory). This happens when the
 * main worktree was removed via `git worktree remove`, deleted manually, or
 * is on an unmounted filesystem. Used by both `runMigration` and the
 * top-level refusal nudge in `cli/index.ts`.
 *
 * Exported so the refusal nudge can produce an identical message — the only
 * variable is the actual path, which is interpolated.
 *
 * @param mainPath  Absolute path the resolver returned for the main worktree.
 * @param exists    Whether the path exists on disk. False → "not found".
 *                  True (with `!isDirectory`) → "is not a directory".
 */
export function buildMainMissingMessage(mainPath: string, exists: boolean): string {
  const cause = exists
    ? `Main worktree at ${mainPath} is not a directory.`
    : `Main worktree at ${mainPath} not found.`;
  return [
    cause,
    "",
    "The .agents/ migration target is unreachable. This usually means the main",
    "worktree was removed (`git worktree remove`) or its directory was deleted.",
    "Migration cannot proceed without a valid target.",
    "",
    "To resolve:",
    "- Restore the main worktree, OR",
    "- Check `git worktree list` and re-initialize the project layout you expect",
  ].join("\n");
}

/**
 * Inspect a path; return whether it exists and whether it's a directory.
 * Used by both `runMigration` and the top-level refusal nudge.
 */
export function probeMainWorktreeTarget(path: string): { exists: boolean; isDirectory: boolean } {
  if (!existsSync(path)) return { exists: false, isDirectory: false };
  try {
    return { exists: true, isDirectory: statSync(path).isDirectory() };
  } catch {
    // The path showed up in existsSync but stat failed — treat as
    // not-a-directory for messaging. We surface the same error class.
    return { exists: true, isDirectory: false };
  }
}

/**
 * In a linked git worktree, `.git` is a FILE (not a directory) containing a
 * `gitdir: <path>` line that points at `<main>/.git/worktrees/<name>`. When
 * the main worktree's directory has been deleted, `git rev-parse` no longer
 * works (it has nothing to resolve against) — but the linked worktree's
 * `.git` file is still on disk, and we can read it to recover the recorded
 * main path. This lets us produce a clear error instead of the misleading
 * "not in a worktree" outcome.
 *
 * Returns the absolute path to the main worktree's root as recorded in the
 * gitdir pointer, or null if the worktree is actually a main checkout, the
 * `.git` file is missing, or the pointer can't be parsed.
 */
function readGitdirPointerMainRoot(wtRoot: string): string | null {
  const dotGit = join(wtRoot, ".git");
  if (!existsSync(dotGit)) return null;
  let stat;
  try {
    stat = statSync(dotGit);
  } catch {
    return null;
  }
  // Main checkout has .git as a directory; linked worktree has it as a file.
  if (stat.isDirectory()) return null;
  let raw: string;
  try {
    raw = readFileSync(dotGit, "utf-8");
  } catch {
    return null;
  }
  const match = raw.match(/^gitdir:\s*(.+?)\s*$/m);
  if (!match) return null;
  const gitdir = match[1];
  // Pattern: <main>/.git/worktrees/<wt-name>
  // Strip the trailing /.git/worktrees/<name> to recover <main>.
  const m2 = gitdir.match(/^(.*)\/\.git\/worktrees\/[^/]+$/);
  if (!m2) return null;
  return m2[1];
}

// SYMLINK BEHAVIOR:
// Refuse symlinks inside worktree-local `.agents/` and require manual handling.
// This avoids the previous platform/path-dependent split where same-FS rename
// preserved links, EXDEV copy dereferenced them, and conflict mtimes followed
// targets. `.agents/` artifacts are expected to be plain files; a symlink is
// rare enough that a loud stop is safer than silent normalization.

// ─────────────────────────────────────────────────────────────────────────────
// Centralized refusal message — referenced by cli/index.ts and tests.
// ─────────────────────────────────────────────────────────────────────────────

/**
 * The single source of truth for the pre-A3 refusal nudge.
 *
 * Properties this string MUST preserve (tests assert on them):
 *   - Mentions `loaf migrate worktree-storage` exactly.
 *   - References SPEC-036 by name (so users can grep the docs).
 *   - Mentions the LOAF_DEBUG_RESOLVE env knob.
 */
export const PRE_A3_REFUSAL_MESSAGE = [
  "This worktree has unmigrated agentic state under .agents/.",
  "SPEC-036 centralizes .agents/ to the main worktree, so this command is refused",
  "until you run:",
  "",
  "    loaf migrate worktree-storage         # dry-run preview",
  "    loaf migrate worktree-storage --apply # perform the migration",
  "",
  `(Tip: set ${DEBUG_RESOLVE_ENV}=1 to see git probe diagnostics if the refusal seems unexpected.)`,
].join("\n");

// ─────────────────────────────────────────────────────────────────────────────
// Detection: is this worktree in a pre-A3 state?
// ─────────────────────────────────────────────────────────────────────────────

/** Filename of the post-migration back-pointer dropped in worktree-local .agents/. */
export const BACK_POINTER_FILE = ".moved-to";

/**
 * Read the worktree-local `.moved-to` back-pointer, if any.
 * Returns the trimmed absolute path it points to, or null if absent/empty.
 */
export function readBackPointer(worktreeAgentsDir: string): string | null {
  const path = join(worktreeAgentsDir, BACK_POINTER_FILE);
  if (!existsSync(path)) return null;
  try {
    const content = readFileSync(path, "utf-8").trim();
    return content.length > 0 ? content : null;
  } catch {
    return null;
  }
}

/**
 * True if `worktreeAgentsDir` contains any *file* (recursively) other than
 * the back-pointer. Empty leftover subdirectories (the residue of a
 * successful migration — sessions/, kb/, etc.) do NOT count as content.
 *
 * Used to short-circuit refusal-nudge detection for already-cleaned (or
 * never-populated) worktrees and to recognise the already-migrated state
 * inside `runMigration`.
 */
export function worktreeAgentsHasContent(worktreeAgentsDir: string): boolean {
  if (!existsSync(worktreeAgentsDir)) return false;
  return enumerateFiles(worktreeAgentsDir).length > 0;
}

function debugResolve(message: string): void {
  if (isDebugResolveEnabled()) {
    process.stderr.write(`${DEBUG_RESOLVE_ENV}: ${message}\n`);
  }
}

/**
 * Diagnostic for the top-level refusal nudge when the main worktree target
 * is unreachable. The CLI dispatcher checks this BEFORE telling the user to
 * run `loaf migrate worktree-storage` — because that command can't complete
 * if the target is gone, telling the user to run it would be cheerful
 * misdirection.
 *
 * Returns the formatted error message if the main worktree path resolved but
 * does not exist (or is not a directory). Returns null when the target is
 * fine or when there's no resolvable main worktree at all (the caller's
 * normal refusal path applies in that case).
 */
export function detectMainMissingForRefusal(startDir: string = process.cwd()): string | null {
  const wtRoot = findWorktreeRootForDetection(startDir);
  if (!wtRoot) return null;

  const mainRoot = findMainWorktreeRoot(wtRoot);
  if (mainRoot) {
    const probe = probeMainWorktreeTarget(mainRoot);
    if (!probe.exists || !probe.isDirectory) {
      return buildMainMissingMessage(mainRoot, probe.exists);
    }
    return null;
  }

  // `git rev-parse` couldn't resolve — could be a main checkout (no
  // diagnostic needed) or a linked worktree whose main was deleted. The
  // `.git` file in a linked worktree still encodes the main path even when
  // git's own probe fails; read it directly to distinguish.
  const pointerMainRoot = readGitdirPointerMainRoot(wtRoot);
  if (!pointerMainRoot) return null;
  const probe = probeMainWorktreeTarget(pointerMainRoot);
  if (!probe.exists || !probe.isDirectory) {
    return buildMainMissingMessage(pointerMainRoot, probe.exists);
  }
  return null;
}

/**
 * Pre-A3 detection signal used by the top-level CLI dispatcher.
 *
 * Returns true iff ALL of the following hold:
 *   1. `startDir` is inside a linked git worktree (findMainWorktreeRoot is non-null).
 *   2. The worktree-local `.agents/` exists AND contains content other than
 *      the back-pointer.
 *   3. EITHER the back-pointer is absent, OR it points somewhere other than
 *      the current main worktree root.
 *
 * Main checkouts and single-worktree repos always return false, so the
 * dispatcher pays near-zero cost in the common case (existsSync short-circuits
 * before the git probe even fires for repos without a local `.agents/`).
 */
export function detectPreA3State(startDir: string = process.cwd()): boolean {
  // Cheapest possible short-circuit: walk up looking for a worktree-local
  // `.agents/`. If we never find one before we hit a `.git` (worktree root)
  // OR the filesystem root, there's nothing to migrate — and the git probe
  // never fires. This keeps the dispatcher near-zero-cost in the common case
  // (running loaf from any tree without a linked-worktree's local .agents/).
  const wtRoot = findWorktreeRootForDetection(startDir);
  if (!wtRoot) return false;

  const localAgents = join(wtRoot, ".agents");
  if (!existsSync(localAgents)) return false;

  // Run the cheap git probe BEFORE the recursive content scan. On a main
  // checkout with a populated `.agents/`, this collapses the common-case
  // cost from O(.agents/ size) to a single git probe — every loaf invocation
  // would otherwise pay the walk just to be told "not in a linked worktree".
  const mainRoot = findMainWorktreeRoot(wtRoot);
  if (!mainRoot) return false; // main checkout or non-git context

  if (!worktreeAgentsHasContent(localAgents)) return false;

  const pointer = readBackPointer(localAgents);
  if (!pointer) return true; // populated + no pointer = pre-A3

  // Back-pointer present: ensure it actually points at the current main
  // worktree root AND that root still exists. If either is false, treat as
  // pre-A3 (re-migration required).
  if (!existsSync(pointer)) return true;
  return pointer !== mainRoot;
}

/**
 * Walk up from `startDir` to the nearest directory containing a `.git`
 * entry. Mirrors `findWorktreeRoot` below but exported through
 * `detectPreA3State` only — kept private to avoid encouraging callers to
 * reinvent it.
 */
function findWorktreeRootForDetection(startDir: string): string | null {
  let current = startDir;
  while (true) {
    if (existsSync(join(current, ".git"))) return current;
    const parent = dirname(current);
    if (parent === current) return null;
    current = parent;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Migration plan
// ─────────────────────────────────────────────────────────────────────────────

export type ConflictResolution = "worktree" | "main" | "newer";

export interface MigrationOptions {
  /** Working directory the user invoked `loaf migrate worktree-storage` from. */
  cwd: string;
  /** When true, mutate the filesystem; otherwise emit a dry-run plan. */
  apply: boolean;
  /** Conflict policy. Defaults to "newer". */
  conflictPolicy: ConflictResolution;
}

export interface PlannedMove {
  /** Absolute source path inside the worktree-local `.agents/`. */
  from: string;
  /** Absolute destination path inside the main worktree's `.agents/`. */
  to: string;
  /** Relative path from `.agents/` (for logging). */
  rel: string;
  /** True if a file already exists at `to`. */
  conflict: boolean;
  /** When `conflict` is true, the resolution this run will apply. */
  resolution?: "keep-worktree" | "keep-main";
  /** When `conflict` is true, a one-line reason for the resolution. */
  resolutionReason?: string;
}

export interface MigrationPlan {
  kind: "plan";
  worktreeAgents: string;
  mainAgents: string;
  mainRoot: string;
  moves: PlannedMove[];
  backPointerPath: string;
}

export type MigrationStatus =
  | "not-in-worktree"
  | "not-in-git"
  | "no-local-agents"
  | "already-migrated"
  | "planned"
  | "applied"
  | "partial-leftover"
  | "main-missing"
  | "symlink-unsupported";

export interface MigrationResult {
  status: MigrationStatus;
  message: string;
  plan?: MigrationPlan;
  /** Set when `status === "partial-leftover"`: the leftover staging paths. */
  partials?: string[];
}

// ─────────────────────────────────────────────────────────────────────────────
// Plan computation
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Recursively enumerate every file under `dir`, returning paths relative to
 * `dir`. Empty directories are not emitted (move-by-file semantics mean
 * empty dirs vanish naturally). The back-pointer file is excluded.
 */
function enumerateFiles(dir: string): string[] {
  const out: string[] = [];
  const walk = (current: string, rel: string) => {
    let entries: import("fs").Dirent[];
    try {
      entries = readdirSync(current, { withFileTypes: true });
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      debugResolve(`failed to read ${current}: ${message}`);
      return;
    }
    for (const entry of entries) {
      const absPath = join(current, entry.name);
      const relPath = rel ? join(rel, entry.name) : entry.name;
      // Skip the back-pointer at the root — it's a control file, not content.
      if (rel === "" && entry.name === BACK_POINTER_FILE) continue;
      if (entry.isDirectory()) {
        walk(absPath, relPath);
      } else if (entry.isFile() || entry.isSymbolicLink()) {
        out.push(relPath);
      }
    }
  };
  walk(dir, "");
  return out;
}

function findSymlinkPaths(dir: string): string[] {
  if (!existsSync(dir)) return [];
  const out: string[] = [];
  const walk = (current: string, rel: string) => {
    let entries: import("fs").Dirent[];
    try {
      entries = readdirSync(current, { withFileTypes: true });
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      debugResolve(`failed to read ${current}: ${message}`);
      return;
    }
    for (const entry of entries) {
      const absPath = join(current, entry.name);
      const relPath = rel ? join(rel, entry.name) : entry.name;
      if (rel === "" && entry.name === BACK_POINTER_FILE) continue;
      if (entry.isSymbolicLink()) {
        out.push(relPath);
        continue;
      }
      if (entry.isDirectory()) {
        try {
          if (lstatSync(absPath).isSymbolicLink()) {
            out.push(relPath);
            continue;
          }
        } catch {
          // If lstat fails, fall back to the Dirent classification above.
        }
        walk(absPath, relPath);
      }
    }
  };
  walk(dir, "");
  return out;
}

function mtime(path: string): number {
  try {
    return statSync(path).mtimeMs;
  } catch {
    return 0;
  }
}

function hasSameContent(a: string, b: string): boolean {
  try {
    return readFileSync(a).equals(readFileSync(b));
  } catch {
    return false;
  }
}

function planMoves(
  worktreeAgents: string,
  mainAgents: string,
  policy: ConflictResolution,
): PlannedMove[] {
  const relFiles = enumerateFiles(worktreeAgents);
  const moves: PlannedMove[] = [];
  for (const rel of relFiles) {
    const from = join(worktreeAgents, rel);
    const to = join(mainAgents, rel);
    const conflict = existsSync(to);

    let resolution: "keep-worktree" | "keep-main" | undefined;
    let resolutionReason: string | undefined;

    if (conflict) {
      if (hasSameContent(from, to)) {
        resolution = "keep-main";
        resolutionReason = "identical content";
      } else {
        switch (policy) {
          case "worktree":
            resolution = "keep-worktree";
            resolutionReason = "forced by --force-from-worktree";
            break;
          case "main":
            resolution = "keep-main";
            resolutionReason = "forced by --force-from-main";
            break;
          case "newer": {
            // Ties resolve to "keep-main" (intentional: prefer the canonical store on no-signal).
            const fromMtime = mtime(from);
            const toMtime = mtime(to);
            if (fromMtime > toMtime) {
              resolution = "keep-worktree";
              resolutionReason = `worktree mtime ${new Date(fromMtime).toISOString()} > main mtime ${new Date(toMtime).toISOString()}`;
            } else {
              resolution = "keep-main";
              resolutionReason = `main mtime ${new Date(toMtime).toISOString()} >= worktree mtime ${new Date(fromMtime).toISOString()}`;
            }
            break;
          }
        }
      }
    }

    moves.push({ from, to, rel, conflict, resolution, resolutionReason });
  }
  return moves;
}

// ─────────────────────────────────────────────────────────────────────────────
// Apply
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Suffix used to stage cross-filesystem moves before the atomic rename swaps
 * the staged copy into place. Exposed so the startup partial-detection scan
 * can identify lingering partials from a previous interrupted run.
 */
export const PARTIAL_SUFFIX = ".partial.loaf-migrate";

function applyMove(move: PlannedMove): void {
  if (move.conflict) {
    if (move.resolution === "keep-main") {
      // Source loses — delete the worktree-local copy.
      try {
        rmSync(move.from, { force: true });
      } catch {
        // best-effort
      }
      return;
    }
    // keep-worktree: overwrite main with source. We need to remove the
    // destination first because renameSync semantics over an existing file
    // differ across platforms.
    try {
      rmSync(move.to, { force: true });
    } catch {
      // ignore
    }
  }

  mkdirSync(dirname(move.to), { recursive: true });
  try {
    renameSync(move.from, move.to);
  } catch (err) {
    // EXDEV: crossing filesystem boundaries. We can't rename across FS
    // boundaries, so we stage a copy on the destination FS first and then
    // atomically rename the staged copy into place. The staging buys us:
    //
    //   - Atomicity at the rename boundary. A disk-full or Ctrl-C during the
    //     cpSync leaves only `<dst>.partial.loaf-migrate` — the destination
    //     itself is never half-written, and the source is still intact.
    //   - A detectable signal for partial-move recovery. `runMigration`
    //     refuses to start when it finds any `*.partial.loaf-migrate` paths
    //     under the destination tree (see `findPartialPaths`).
    //
    // We deliberately do NOT auto-recover on next run. The user fixes it
    // (delete the partial, or rename it into place) and re-runs. This is
    // a "fail loudly" stance consistent with the spec's hard-cut philosophy.
    const nodeErr = err as NodeJS.ErrnoException;
    if (nodeErr.code === "EXDEV") {
      const partial = move.to + PARTIAL_SUFFIX;
      // Clean up any leftover partial from a prior run BEFORE we stage —
      // we wouldn't be here on a sane second run because runMigration's
      // startup check refuses, but be defensive in case the caller bypassed
      // that check (tests, scripts, etc.).
      try { rmSync(partial, { recursive: true, force: true }); } catch { /* ignore */ }
      // Stage onto the destination FS so the subsequent rename is intra-FS.
      cpSync(move.from, partial, { recursive: true, preserveTimestamps: true });
      // Atomic swap — at this point either the destination exists in full
      // or it doesn't exist at all.
      renameSync(partial, move.to);
      // Source can go now that the destination is committed.
      rmSync(move.from, { recursive: true, force: true });
    } else {
      throw err;
    }
  }
}

/**
 * Walk `dir` recursively and collect all file/directory paths ending in
 * `PARTIAL_SUFFIX`. Used at the start of `runMigration` to refuse running
 * when a previous attempt was interrupted mid-EXDEV-stage.
 */
function findPartialPaths(dir: string): string[] {
  if (!existsSync(dir)) return [];
  const out: string[] = [];
  const walk = (current: string) => {
    let entries: import("fs").Dirent[];
    try {
      entries = readdirSync(current, { withFileTypes: true });
    } catch {
      return;
    }
    for (const entry of entries) {
      const abs = join(current, entry.name);
      if (entry.name.endsWith(PARTIAL_SUFFIX)) {
        out.push(abs);
        continue; // don't descend into partials themselves
      }
      if (entry.isDirectory()) walk(abs);
    }
  };
  walk(dir);
  return out;
}

// ─────────────────────────────────────────────────────────────────────────────
// Top-level entry
// ─────────────────────────────────────────────────────────────────────────────

export function runMigration(options: MigrationOptions): MigrationResult {
  const { cwd, apply, conflictPolicy } = options;

  // 1. Must be inside a git repository.
  //    findMainWorktreeRoot returns null both for main-checkouts and for
  //    non-git contexts; we must distinguish them. We do that by attempting
  //    a cheap git probe — if even that fails, we surface a clear error.
  const mainRoot = findMainWorktreeRoot(cwd);

  if (!mainRoot) {
    // Either: (a) main checkout (clean no-op), (b) outside git (error), or
    // (c) we ARE in a linked worktree but the main worktree directory has
    // been deleted — `git rev-parse` can't resolve in that case, so the
    // resolver came back null. Distinguish (c) by reading the linked
    // worktree's `.git` file directly: it's a `gitdir: <path>` pointer that
    // still encodes the recorded main path on disk.
    const wtRootForPointer = findWorktreeRoot(cwd) ?? cwd;
    const pointerMainRoot = readGitdirPointerMainRoot(wtRootForPointer);
    if (pointerMainRoot) {
      const pointerProbe = probeMainWorktreeTarget(pointerMainRoot);
      if (!pointerProbe.exists || !pointerProbe.isDirectory) {
        return {
          status: "main-missing",
          message: buildMainMissingMessage(pointerMainRoot, pointerProbe.exists),
        };
      }
      // Pointer resolved AND the main path exists — but `git rev-parse` still
      // failed for some other reason. Fall through to the same diagnostic
      // path below; the user can re-run with LOAF_DEBUG_RESOLVE=1 to inspect.
    }
    if (!isInGitContext(cwd)) {
      return {
        status: "not-in-git",
        message:
          "loaf migrate worktree-storage: not in a git repository — this command is only meaningful inside a linked git worktree.",
      };
    }
    return {
      status: "not-in-worktree",
      message: "Nothing to migrate — already in the main worktree.",
    };
  }

  // 1a. Main worktree target must exist and be a directory.
  //     `findMainWorktreeRoot` resolves via `git rev-parse --git-common-dir`,
  //     which reads `.git/worktrees/<name>/commondir` for linked worktrees and
  //     will happily return a path that no longer exists on disk in some git
  //     versions or repo states. Migrating into a non-existent target would
  //     either fail late with a confusing filesystem error or silently mkdir
  //     its way into a half-migrated state under a stale path. Surface the
  //     problem before we touch anything.
  const mainProbe = probeMainWorktreeTarget(mainRoot);
  if (!mainProbe.exists || !mainProbe.isDirectory) {
    return {
      status: "main-missing",
      message: buildMainMissingMessage(mainRoot, mainProbe.exists),
    };
  }

  // We need the worktree root, not just `cwd`. The user may have invoked the
  // command from a subdirectory of the worktree (e.g., `src/foo/`); the
  // worktree-local `.agents/` lives at the worktree root.
  const wtRoot = findWorktreeRoot(cwd) ?? cwd;
  const localAgents = join(wtRoot, ".agents");
  const mainAgents = join(mainRoot, ".agents");

  // 1b. Partial-leftover detection. A previous EXDEV-staged run that was
  //     interrupted (Ctrl-C, disk-full, power loss) leaves `*.partial.loaf-migrate`
  //     paths under the destination tree. We refuse to proceed until the user
  //     resolves them — auto-recovery would risk silent corruption (the
  //     conflict policy might prefer the partial main copy over the intact
  //     worktree copy).
  const partials = findPartialPaths(mainAgents);
  if (partials.length > 0) {
    const lines = [
      "Refusing to migrate: found leftover staging paths from a previous interrupted run.",
      "Resolve each path manually (delete it, or rename it into place if you trust the staged copy),",
      "then re-run the migrate command:",
      "",
      ...partials.map((p) => `  ${p}`),
      "",
      `These paths end in '${PARTIAL_SUFFIX}' and were created by an EXDEV cross-filesystem`,
      "stage that did not complete the atomic rename to the final destination.",
    ];
    return {
      status: "partial-leftover",
      message: lines.join("\n"),
      partials,
    };
  }

  // 2. Already migrated?
  const pointer = readBackPointer(localAgents);
  if (pointer === mainRoot && !worktreeAgentsHasContent(localAgents)) {
    return {
      status: "already-migrated",
      message: "Nothing to do — already migrated.",
    };
  }

  // 3. No local `.agents/` at all? Edge case — treat as nothing to migrate.
  if (!existsSync(localAgents) || !worktreeAgentsHasContent(localAgents)) {
    return {
      status: "no-local-agents",
      message: "Nothing to migrate — worktree has no local .agents/ content.",
    };
  }

  const symlinks = findSymlinkPaths(localAgents);
  if (symlinks.length > 0) {
    return {
      status: "symlink-unsupported",
      message: [
        "Refusing to migrate: found symlinks under worktree-local .agents/.",
        "Handle these paths manually, then re-run the migrate command:",
        "",
        ...symlinks.map((p) => `  ${p}`),
      ].join("\n"),
    };
  }

  // 4. Build the plan.
  const moves = planMoves(localAgents, mainAgents, conflictPolicy);
  const plan: MigrationPlan = {
    kind: "plan",
    worktreeAgents: localAgents,
    mainAgents,
    mainRoot,
    moves,
    backPointerPath: join(localAgents, BACK_POINTER_FILE),
  };

  // 5. Dry run? Emit the plan and stop.
  if (!apply) {
    return {
      status: "planned",
      message: "Dry run — re-run with --apply to perform the migration.",
      plan,
    };
  }

  // 6. Apply.
  mkdirSync(mainAgents, { recursive: true });
  for (const move of plan.moves) {
    applyMove(move);
  }
  // Write the back-pointer (overwrites if it existed and was stale).
  writeFileSync(plan.backPointerPath, `${mainRoot}\n`, "utf-8");

  return {
    status: "applied",
    message: `Migrated ${plan.moves.length} file(s) to ${mainAgents}.`,
    plan,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers — git context probes (light, no exec)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Walk up from `startDir` looking for a `.git` entry (file or directory).
 * Used to distinguish "main checkout" (return null) from "outside git
 * context" (return null) without re-running `git rev-parse`.
 */
function isInGitContext(startDir: string): boolean {
  let current = startDir;
  while (true) {
    if (existsSync(join(current, ".git"))) return true;
    const parent = dirname(current);
    if (parent === current) return false;
    current = parent;
  }
}

/**
 * Walk up from `startDir` looking for a directory that contains a `.git`
 * entry — i.e., the working-tree root of the current worktree. We do this
 * client-side (rather than asking git) so the function is safe to call
 * even when `git` is unavailable.
 */
function findWorktreeRoot(startDir: string): string | null {
  let current = startDir;
  while (true) {
    if (existsSync(join(current, ".git"))) return current;
    const parent = dirname(current);
    if (parent === current) return null;
    current = parent;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Pretty printer for CLI output
// ─────────────────────────────────────────────────────────────────────────────

function shouldUseColor(): boolean {
  return process.env.NO_COLOR === undefined && process.stdout.isTTY === true;
}

function color(code: string, enabled: boolean): (s: string) => string {
  return enabled
    ? (s: string) => `\x1b[${code}m${s}\x1b[0m`
    : (s: string) => s;
}

export function formatResult(
  result: MigrationResult,
  opts: { apply: boolean; color?: boolean },
): string {
  const out: string[] = [];
  const colorEnabled = opts.color ?? shouldUseColor();
  const bold = color("1", colorEnabled);
  const green = color("32", colorEnabled);
  const yellow = color("33", colorEnabled);
  const gray = color("90", colorEnabled);
  const cyan = color("36", colorEnabled);

  if (result.plan) {
    const plan = result.plan;
    out.push(`${bold("loaf migrate worktree-storage")} ${opts.apply ? cyan("--apply") : gray("(dry-run)")}\n`);
    out.push(`  ${gray("from")} ${plan.worktreeAgents}`);
    out.push(`  ${gray("to  ")} ${plan.mainAgents}`);
    out.push("");

    if (plan.moves.length === 0) {
      out.push(`  ${green("✓")} Nothing to move.`);
    } else {
      for (const move of plan.moves) {
        const arrow = move.conflict
          ? move.resolution === "keep-main"
            ? yellow("[conflict→main]  ")
            : yellow("[conflict→worktree]")
          : cyan("→");
        const path = relative(plan.worktreeAgents, move.from);
        if (move.conflict) {
          if (move.resolution === "keep-main") {
            out.push(`  ${arrow} ${path}`);
            out.push(`      ${gray(`keep main; discard worktree (${move.resolutionReason})`)}`);
          } else {
            out.push(`  ${arrow} ${path}`);
            out.push(`      ${gray(`keep worktree; overwrite main (${move.resolutionReason})`)}`);
          }
        } else {
          out.push(`  ${arrow} ${path}`);
        }
      }
    }

    out.push("");
    out.push(`  ${gray("back-pointer:")} ${plan.backPointerPath}`);
    out.push("");
  }

  out.push(`  ${result.status === "applied" ? green("✓") : cyan("→")} ${result.message}`);
  return out.join("\n");
}
