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
 * Why not share code with the session lock today:
 *   - The session-lock helper is intentionally not exported from its module.
 *     Lifting it to a shared utility is a larger refactor; this file uses the
 *     same primitives (openSync wx + retry + atomic rename) at the same
 *     thresholds, keeping the audit trail consistent across both locks.
 */

import {
  closeSync,
  existsSync,
  mkdirSync,
  openSync,
  readFileSync,
  renameSync,
  statSync,
  unlinkSync,
  writeFileSync,
} from "fs";
import { hostname } from "os";
import { dirname, join } from "path";

import { buildIndexFromFiles, loadIndex } from "./migrate.js";
import type { TaskIndex } from "./types.js";

// ─────────────────────────────────────────────────────────────────────────────
// Lock primitives (mirrors session.ts staleness + retry semantics)
// ─────────────────────────────────────────────────────────────────────────────

/** Lock file name relative to the .agents/ directory. */
export const TASKS_JSON_LOCK_FILE = ".tasks-json.lock";

/**
 * Foreign-host / malformed-content age fallback (5 seconds).
 *
 * Lock holders complete in milliseconds. Every callback is either a prepared
 * read-apply-write barrier or one small task-file create plus an index update:
 * no file scans, no network calls, nothing that could legitimately push a
 * critical section into the seconds. A foreign-host lock older than a few
 * seconds is therefore genuinely stale: the only reason it would still be
 * present is a crash that the dead process never cleaned up.
 *
 * The PID-liveness check covers same-host crashes authoritatively; this
 * fallback covers the much rarer foreign-host crash on a shared filesystem,
 * and the (impossible-on-current-Loaf) case where a lock file is missing the
 * `host` field entirely. Long-lived but live local holders are protected by
 * PID liveness, NOT by this age threshold — there is no scenario in which a
 * legitimate same-host holder takes more than a few hundred ms.
 */
const LOCK_AGE_FALLBACK_THRESHOLD_MS = 5 * 1000;

/**
 * Default retry budget. 100 retries × 25ms ≈ 2.5s ceiling — long enough to
 * outlast a normal multi-worktree contention burst, short enough to surface
 * a real deadlock rather than hang the user.
 */
const DEFAULT_MAX_RETRIES = 100;
const DEFAULT_RETRY_DELAY_MS = 25;

interface LockFileContent {
  pid: number;
  /** Hostname of the machine that wrote the lock. PID liveness is only
   *  meaningful on the same machine — we read this back to avoid checking a
   *  local PID against a remote process' PID space. */
  host: string;
  timestamp: number;
}

/**
 * Probe whether `pid` is alive on this host.
 *
 *   - `process.kill(pid, 0)` does not actually signal — it just performs the
 *     permission/existence check. Throws ESRCH when no such process exists,
 *     EPERM when the process exists but we don't have permission to signal.
 *   - We treat EPERM as alive: a different user owns the process, so we can't
 *     poke it, but it is definitely running.
 *   - Any other error (e.g. EINVAL) we conservatively treat as alive too so
 *     we never force-evict on the basis of an inconclusive probe.
 */
function isProcessAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch (err) {
    const code = (err as NodeJS.ErrnoException)?.code;
    if (code === "ESRCH") return false; // Confirmed dead.
    // EPERM or anything else: process likely exists, just unreachable. Treat
    // as alive — we'd rather wait on a live holder than steal from one.
    return true;
  }
}

function readLockContent(lockPath: string): LockFileContent | null {
  try {
    const parsed = JSON.parse(readFileSync(lockPath, "utf-8")) as Partial<LockFileContent>;
    if (typeof parsed?.pid !== "number") return null;
    return {
      pid: parsed.pid,
      host: typeof parsed.host === "string" ? parsed.host : "",
      timestamp: typeof parsed.timestamp === "number" ? parsed.timestamp : 0,
    };
  } catch {
    return null;
  }
}

/**
 * Decide whether an existing lock file is stale.
 *
 * Primary signal — PID liveness on the same host. If the lock file records a
 * PID and a host that matches `os.hostname()`, we trust `process.kill(pid, 0)`
 * to answer authoritatively. A long-running but live local holder will NOT be
 * declared stale, no matter the age — that was the original defect this
 * function fixes.
 *
 * Fallback — conservative age check (`LOCK_AGE_FALLBACK_THRESHOLD_MS`). Used
 * only when we genuinely cannot probe liveness:
 *
 *   - Lock file content is empty/malformed (pre-fix Loaf orphaned the lock).
 *   - Lock was written on a different host (PID namespace is theirs, not
 *     ours; probing a "matching" local PID would be a false signal).
 *
 * On the foreign-host path, age is a last-resort heuristic. We accept extra
 * wait there in exchange for never evicting a live holder elsewhere.
 */
function isLockStale(lockPath: string): boolean {
  let stats;
  try {
    stats = statSync(lockPath);
  } catch {
    // Can't stat — likely the file was removed mid-check. Treat as stale so
    // the caller proceeds to attempt creation; the openSync("wx") will catch
    // any genuine race.
    return true;
  }

  const content = readLockContent(lockPath);
  const age = Date.now() - stats.mtimeMs;

  // Empty/malformed payload (pre-fix Loaf or partial write) — we can't probe
  // liveness. Fall back to the conservative age threshold.
  if (!content || !content.pid) {
    return age > LOCK_AGE_FALLBACK_THRESHOLD_MS;
  }

  // Different host: local PID checks are meaningless against a remote PID
  // namespace, so the only safe answer is age-based fallback. We mark this
  // explicitly so future readers understand why we're not calling kill(0).
  //
  // Clock-skew assumption: this age comparison subtracts `stats.mtimeMs`
  // (recorded by the filesystem) from `Date.now()` (the local machine).
  // Foreign-host detection assumes the local clock and the filesystem's
  // recorded `mtime` come from the same time domain (the local machine).
  // On shared filesystems across hosts with non-trivial NTP clock skew
  // (>5s), a live foreign-host lock could be incorrectly identified as
  // stale. This is acceptable for the supported single-host model; cross-
  // host shared-FS use is not supported.
  if (content.host && content.host !== hostname()) {
    return age > LOCK_AGE_FALLBACK_THRESHOLD_MS;
  }

  // Same host (or unknown host on a legacy lock file written by this fix
  // before host was stamped — treat as local). PID liveness is the truth.
  return !isProcessAlive(content.pid);
}

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
      const payload: LockFileContent = {
        pid: process.pid,
        host: hostname(),
        timestamp: Date.now(),
      };
      writeFileSync(fd, JSON.stringify(payload), "utf-8");
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
  let diagnostic = "";
  try {
    const content = readLockContent(lockPath);
    const stats = statSync(lockPath);
    const ageSeconds = Math.round((Date.now() - stats.mtimeMs) / 1000);
    const sameHost = !content?.host || content.host === hostname();
    const alive = content?.pid && sameHost ? isProcessAlive(content.pid) : "unknown (foreign host)";
    diagnostic =
      `\n  Lock age: ${ageSeconds}s, PID: ${content?.pid ?? "unknown"}` +
      `, host: ${content?.host ?? "unknown"}, process alive: ${alive}` +
      `\n  Remove manually if you're sure no other loaf process is active:` +
      `\n    rm "${lockPath}"`;
  } catch { /* best-effort diagnostic */ }

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
