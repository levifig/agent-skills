/**
 * TASKS.json file lock
 *
 * Provides `withTasksJsonLock(agentsDir, fn)` — a re-entrant-safe (per process)
 * read-modify-write barrier for `TASKS.json` shared across concurrent loaf
 * invocations (cross-worktree, cross-shell, cross-anything).
 *
 * The lock mirrors the session-journal lock pattern in `cli/commands/session.ts`:
 *
 *   - Acquired by `openSync(lockPath, "wx")` — atomic on POSIX and Windows.
 *   - Bounded retry/backoff on `EEXIST`.
 *   - Staleness detection: PID-liveness on the same host (authoritative)
 *     with a conservative age fallback only when the lock content is missing/
 *     malformed or written by a foreign host. Long-lived but live local
 *     critical sections are NEVER evicted on age alone.
 *   - Released via `unlinkSync` in a `finally` block.
 *
 * The lock file is `<agentsDir>/.tasks-json.lock`. We always re-read TASKS.json
 * inside the lock (never trust a stale in-memory copy across the lock barrier),
 * mutate, and atomic-write via temp file + `renameSync`. If TASKS.json doesn't
 * exist yet (first task creation, or fresh project), we build it from .md files
 * inside the lock as part of the same barrier.
 *
 * Why a separate file from the session-lock helper:
 *   - The session lock is async (`acquireLock` returns a Promise); the call
 *     sites here are synchronous (`task create`, `spec archive`, etc.).
 *   - The session lock lives next to a moving target (the session file's `.lock`);
 *     this one always lives at a fixed path under `.agents/`.
 *
 * Shared with the session lock:
 *   - Lock payload shape, PID-liveness staleness checks, foreign-host age
 *     fallback, and diagnostics live in `cli/lib/locks/file-lock.ts`.
 *   - Acquisition loops stay separate because this helper is synchronous
 *     while the session journal helper is async.
 */

import {
  closeSync,
  existsSync,
  mkdirSync,
  openSync,
  renameSync,
  unlinkSync,
  writeFileSync,
} from "fs";
import { dirname, join } from "path";

import {
  createLockFileContent,
  formatLockDiagnostic,
  isLockStale,
} from "../locks/file-lock.js";
import { buildIndexFromFiles, loadIndex } from "./migrate.js";
import type { TaskIndex } from "./types.js";

// ─────────────────────────────────────────────────────────────────────────────
// Lock primitives (mirrors session.ts staleness + retry semantics)
// ─────────────────────────────────────────────────────────────────────────────

/** Lock file name relative to the .agents/ directory. */
export const TASKS_JSON_LOCK_FILE = ".tasks-json.lock";

/**
 * Default retry budget. 100 retries × 25ms ≈ 2.5s ceiling — long enough to
 * outlast a normal multi-worktree contention burst, short enough to surface
 * a real deadlock rather than hang the user.
 */
const DEFAULT_MAX_RETRIES = 100;
const DEFAULT_RETRY_DELAY_MS = 25;

/** Synchronous millisecond sleep — used inside the retry loop. */
function sleepMs(ms: number): void {
  // SharedArrayBuffer + Atomics.wait gives a genuine cross-platform sync
  // sleep without shelling out to `sleep`. The buffer is otherwise unused.
  const sab = new Int32Array(new SharedArrayBuffer(4));
  Atomics.wait(sab, 0, 0, ms);
}

function acquireLockSync(
  lockPath: string,
  maxRetries: number,
  retryDelayMs: number,
): void {
  for (let i = 0; i < maxRetries; i++) {
    try {
      if (existsSync(lockPath) && isLockStale(lockPath)) {
        try { unlinkSync(lockPath); } catch { /* race: another process won the cleanup */ }
      }
      // Atomic create-or-fail. After openSync("wx") succeeds we exclusively
      // own the file descriptor — safe to stamp the holder identity (pid +
      // host) before anyone else can read it.
      const fd = openSync(lockPath, "wx");
      writeFileSync(fd, JSON.stringify(createLockFileContent()), "utf-8");
      closeSync(fd);
      return;
    } catch (err) {
      const code = (err as NodeJS.ErrnoException)?.code;
      if (code === "EEXIST") {
        sleepMs(retryDelayMs);
        continue;
      }
      if (code === "ENOENT") {
        // Parent directory missing — create it and try again on the next loop.
        try { mkdirSync(dirname(lockPath), { recursive: true }); } catch { /* ignore */ }
        continue;
      }
      throw err;
    }
  }

  // Exhausted retries — surface a diagnostic so the user can recover.
  const diagnostic =
    formatLockDiagnostic(lockPath) +
    `\n  Remove manually if you're sure no other loaf process is active:` +
    `\n    rm "${lockPath}"`;

  throw new Error(
    `Could not acquire TASKS.json lock after ${maxRetries} retries: ${lockPath}${diagnostic}`,
  );
}

function releaseLock(lockPath: string): void {
  try { unlinkSync(lockPath); } catch { /* idempotent — already gone is fine */ }
}

/**
 * Test-only start barrier. When `LOAF_LOCK_TEST_STARTER` is set, the worker:
 *   1. Writes a `ready-${pid}` marker next to the starter path so the parent
 *      can count how many workers are queued up.
 *   2. Busy-polls (1ms sleep) for the starter file to exist.
 *   3. Proceeds to acquire the lock.
 *
 * Hard cap of 5s to prevent runaway test hangs. No effect when the env var
 * is unset — production callers never touch this path.
 */
function awaitTestStartBarrier(): void {
  const starter = process.env.LOAF_LOCK_TEST_STARTER;
  const readyDir = process.env.LOAF_LOCK_TEST_READY;
  if (!starter) return;

  if (readyDir) {
    try {
      mkdirSync(readyDir, { recursive: true });
      writeFileSync(join(readyDir, `ready-${process.pid}`), "", "utf-8");
    } catch { /* best-effort */ }
  }

  const deadline = Date.now() + 5000;
  while (!existsSync(starter)) {
    if (Date.now() > deadline) return; // Fail-open: don't hang the test runner.
    sleepMs(1);
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Atomic write
// ─────────────────────────────────────────────────────────────────────────────

/** Atomic write via temp file + rename. Sibling temp keeps the rename intra-FS. */
function writeJsonAtomic(filePath: string, content: string): void {
  const tempPath = `${filePath}.tmp.${process.pid}.${Date.now()}.${Math.random().toString(36).slice(2, 11)}`;
  try {
    writeFileSync(tempPath, content, "utf-8");
    renameSync(tempPath, filePath);
  } catch (err) {
    try { unlinkSync(tempPath); } catch { /* ignore */ }
    throw err;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Public API
// ─────────────────────────────────────────────────────────────────────────────

export interface WithTasksJsonLockOptions {
  /** Override retry budget (default: 100). */
  maxRetries?: number;
  /** Override per-retry delay in ms (default: 25). */
  retryDelayMs?: number;
}

/**
 * Sentinel callbacks can return to skip the post-callback write while keeping
 * the lock semantics (consistent read under the barrier). Useful for callers
 * that detect "nothing to do" inside the lock and want to short-circuit.
 */
export const SKIP_WRITE = Symbol("loaf.tasks.lock.skip-write");

/**
 * Acquire the TASKS.json lock, re-read TASKS.json under the lock, hand the
 * fresh index to `fn`, persist the result atomically, and release the lock.
 *
 * The callback mutates `index` in place. Return value is passed through to
 * the caller. Returning the `SKIP_WRITE` sentinel (or `false`) skips the
 * post-callback persist — useful for read-only critical sections or
 * "nothing changed" short-circuits.
 *
 * If TASKS.json doesn't exist when the lock is acquired, it is built from
 * .md files inside the lock as the first read.
 */
export function withTasksJsonLock<T>(
  agentsDir: string,
  fn: (index: TaskIndex) => T,
  options: WithTasksJsonLockOptions = {},
): T {
  const lockPath = join(agentsDir, TASKS_JSON_LOCK_FILE);
  const indexPath = join(agentsDir, "TASKS.json");
  const maxRetries = options.maxRetries ?? DEFAULT_MAX_RETRIES;
  const retryDelayMs = options.retryDelayMs ?? DEFAULT_RETRY_DELAY_MS;

  // Ensure the agents directory exists before we try to create the lock there.
  mkdirSync(agentsDir, { recursive: true });

  // Test-only: start barrier. Each worker writes a ready marker, then waits
  // for the parent to flip the starter file. Guarantees all workers race for
  // the lock at the same instant, making the concurrency test deterministic.
  // Zero effect on production (no env vars set).
  awaitTestStartBarrier();

  acquireLockSync(lockPath, maxRetries, retryDelayMs);

  // Test-only: emit acquired-at timestamp so the test can assert genuine
  // overlap across workers rather than accidental serialization.
  if (process.env.LOAF_LOCK_TEST_DELAY_MS) {
    process.stdout.write(`LOAF_LOCK_TEST_ACQUIRED_AT=${Date.now()}\n`);
  }

  try {
    // Test-only: artificially extend the critical section to force
    // contention in the cross-process concurrency test. Guarded by an env
    // var so it has zero effect on production callers. See lock.test.ts.
    const testDelay = Number(process.env.LOAF_LOCK_TEST_DELAY_MS ?? 0);
    if (testDelay > 0) sleepMs(testDelay);

    // Read inside the lock: never trust a stale in-memory copy across the
    // barrier. loadIndex returns null on invalid shape — rebuild then.
    let index: TaskIndex;
    if (existsSync(indexPath)) {
      const loaded = loadIndex(indexPath);
      index = loaded ?? buildIndexFromFiles(agentsDir);
    } else {
      index = buildIndexFromFiles(agentsDir);
    }

    const result = fn(index);

    // `false` or `SKIP_WRITE` short-circuits the persist (read-only barrier).
    const skip = (result as unknown) === false || (result as unknown) === SKIP_WRITE;
    if (!skip) {
      writeJsonAtomic(indexPath, JSON.stringify(index, null, 2) + "\n");
    }

    return result;
  } finally {
    releaseLock(lockPath);
  }
}
