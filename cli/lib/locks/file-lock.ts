/**
 * Shared file-lock identity and staleness helpers.
 *
 * Callers still own their sync/async acquisition loops, but they share the
 * same lock payload, PID-liveness policy, foreign-host fallback, and
 * diagnostic formatting.
 */

import { readFileSync, statSync } from "fs";
import { hostname } from "os";

/**
 * Foreign-host / malformed-content age fallback (5 seconds).
 *
 * Same-host locks use PID liveness as the authoritative signal. This fallback
 * is only used when liveness is unprobeable: malformed/empty payloads or locks
 * written by another host.
 *
 * Clock-skew assumption: foreign-host fallback compares local `Date.now()` to
 * the filesystem-recorded `mtime`. Cross-host shared filesystems with >5s
 * clock skew are outside Loaf's supported single-host model.
 */
export const LOCK_AGE_FALLBACK_THRESHOLD_MS = 5 * 1000;

export interface LockFileContent {
  pid: number;
  host: string;
  timestamp: number;
}

export function createLockFileContent(timestamp = Date.now()): LockFileContent {
  return {
    pid: process.pid,
    host: hostname(),
    timestamp,
  };
}

/**
 * Probe whether `pid` is alive on this host.
 *
 * `process.kill(pid, 0)` does not signal the process; it only checks
 * permission/existence. ESRCH means dead. EPERM or any other error is treated
 * as alive so inconclusive probes never force-evict a valid holder.
 */
export function isProcessAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch (err) {
    const code = (err as NodeJS.ErrnoException)?.code;
    if (code === "ESRCH") return false;
    return true;
  }
}

export function readLockContent(lockPath: string): LockFileContent | null {
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

export function isLockStale(lockPath: string): boolean {
  let stats;
  try {
    stats = statSync(lockPath);
  } catch {
    // Caller should proceed to create; atomic open("wx") catches real races.
    return true;
  }

  const content = readLockContent(lockPath);
  const age = Date.now() - stats.mtimeMs;

  if (!content || !content.pid) {
    return age > LOCK_AGE_FALLBACK_THRESHOLD_MS;
  }

  if (content.host && content.host !== hostname()) {
    return age > LOCK_AGE_FALLBACK_THRESHOLD_MS;
  }

  return !isProcessAlive(content.pid);
}

export function formatLockDiagnostic(lockPath: string): string {
  try {
    const content = readLockContent(lockPath);
    const stats = statSync(lockPath);
    const ageSeconds = Math.round((Date.now() - stats.mtimeMs) / 1000);
    const sameHost = !content?.host || content.host === hostname();
    const alive = content?.pid && sameHost ? isProcessAlive(content.pid) : "unknown (foreign host)";
    return (
      `\n  Lock age: ${ageSeconds}s, PID: ${content?.pid ?? "unknown"}` +
      `, host: ${content?.host ?? "unknown"}, process alive: ${alive}`
    );
  } catch {
    return "";
  }
}
