/**
 * Session resolution chain
 *
 * Implements SPEC-032's 3-tier routing:
 *
 *   1. `--session-id` CLI flag        → findSessionByClaudeId
 *   2. hook stdin (when opt-in)       → parseHookSessionId → findSessionByClaudeId
 *   3. branch lookup                  → findActiveSessionForBranch (emits stderr WARN)
 *
 * The chain is the single source of truth for "which session does this command
 * belong to?" — every subcommand that previously called
 * `findActiveSessionForBranch(...)` directly should migrate to this helper
 * (see TASK-117 / TASK-118).
 */

import { readFileSync } from "fs";
import { basename } from "path";

import {
  findActiveSessionForBranch,
  findSessionByClaudeId,
} from "./find.js";
import type { SessionAdoption } from "./find.js";
import type { SessionFrontmatter } from "./store.js";

/**
 * Read stdin synchronously via fd 0 and parse it as hook JSON, returning the
 * `session_id` field if present and non-empty.
 *
 * Returns `undefined` for: missing field, malformed JSON, empty stdin,
 * non-string session_id, or any IO error.
 *
 * **Internal helper** — not part of the public session API. Callers must use
 * `resolveCurrentSession({ parseStdin: true })` instead. See SPEC-032 A5.
 *
 * Underscore prefix marks this as module-private; the only legitimate caller
 * is `resolveCurrentSession` in this same file. The function remains
 * exported (rather than truly private) so unit tests in `resolve.test.ts`
 * can reach it without surface area leaking to other modules.
 */
export function _parseHookSessionId(): string | undefined {
  // Read stdin synchronously from fd 0. Synchronous reads are fine here
  // because hook contexts always pipe a small JSON payload and exit; no
  // long-running IO or interactive consumers compete for stdin.
  let raw: string;
  try {
    raw = readFileSync(0, "utf-8");
  } catch {
    return undefined;
  }

  if (!raw || !raw.trim()) return undefined;

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return undefined;
  }

  if (!parsed || typeof parsed !== "object") return undefined;
  const value = (parsed as { session_id?: unknown }).session_id;
  if (typeof value !== "string" || value.length === 0) return undefined;
  return value;
}

/** Options for `resolveCurrentSession`. */
export interface ResolveCurrentSessionOptions {
  /** Explicit session id (e.g., from a `--session-id` CLI flag). Tier 1. */
  sessionIdFlag?: string;
  /**
   * When `true`, parse stdin via `parseHookSessionId`. Caller must opt in
   * (e.g., `--from-hook`) — auto-detection is rejected per SPEC-032 A5.
   *
   * Mutually exclusive with `stdinSessionIdHint`: if a caller has already
   * consumed stdin, it should pass the parsed id via `stdinSessionIdHint`
   * instead of asking the helper to re-read fd 0.
   */
  parseStdin?: boolean;
  /**
   * Pre-parsed `session_id` from hook stdin, supplied by callers that have
   * already consumed stdin upstream (e.g., `loaf session log` reads stdin
   * once for both routing and entry-text extraction). When set, the helper
   * uses this as Tier 2 instead of re-reading fd 0.
   *
   * Empty string is treated the same as `undefined` (no signal).
   *
   * If both `parseStdin: true` and `stdinSessionIdHint` are passed,
   * `stdinSessionIdHint` wins (no double-read).
   */
  stdinSessionIdHint?: string;
}

export interface ResolvedSession {
  filePath: string;
  data: SessionFrontmatter;
  content: string;
}

/**
 * Emit WARN for a direct branch match in the Tier 3 fallback.
 *
 * Format pinned by SPEC-032 dev.31 — kept verbatim so existing log scrapers
 * keep working.
 */
function emitBranchFallbackWarning(branch: string): void {
  process.stderr.write(
    `WARN: no session_id signal — falling back to branch routing for branch '${branch}'. Pass --session-id <id> to silence.\n`
  );
}

/**
 * Emit WARN when Tier 3 adopts a session whose origin branch differs from the
 * requested branch. Names both branches and the resolved file so operators can
 * spot misrouting at a glance.
 *
 * The wording is shaped by the `adoption` discriminator:
 *
 *   - `rename-link`: the session was located via git reflog detection of a
 *     `git branch -m` rename. "Most-recent" would be factually wrong here, so
 *     the WARN reads "appears to be a rename of <other-branch>".
 *   - `most-recent-active`: the session was the most-recently-updated active
 *     session across all branches. The WARN names this lookup heuristic
 *     directly so operators can tell why their command landed where it did.
 *
 * SPEC-042 Track B.
 */
function emitAdoptionWarning(
  branch: string,
  originBranch: string,
  filePath: string,
  adoption: Extract<SessionAdoption, "rename-link" | "most-recent-active">
): void {
  const file = basename(filePath);
  if (adoption === "rename-link") {
    process.stderr.write(
      `WARN: branch '${branch}' appears to be a rename of '${originBranch}'; logging to its session '${file}'. Pass --session-id <id> to silence.\n`
    );
    return;
  }
  process.stderr.write(
    `WARN: no session for branch '${branch}'; logging to most-recent active session '${file}' (origin branch '${originBranch}'). Pass --session-id <id> to silence.\n`
  );
}

/**
 * Emit WARN when Tier 3 finds zero active sessions to fall back to. Distinct
 * from the adoption WARN so operators can tell "nothing exists" from "fell
 * back to something unexpected".
 *
 * SPEC-042 Track B.
 */
function emitNoActiveSessionsWarning(branch: string): void {
  process.stderr.write(
    `WARN: no session for branch '${branch}'; no active sessions to fall back to. Pass --session-id <id> to silence.\n`
  );
}

/**
 * Resolve the current session via SPEC-032's 3-tier chain.
 *
 * - Tier 1 (`opts.sessionIdFlag`) → `findSessionByClaudeId`. On null, fall through.
 * - Tier 2 (`opts.parseStdin === true`) → `parseHookSessionId` → `findSessionByClaudeId`.
 *   On null, fall through.
 * - Tier 3 → `findActiveSessionForBranch`. Always emits stderr WARN; the
 *   shape of the WARN depends on how the finder resolved:
 *     - branch match: generic "falling back to branch routing"
 *     - rename-link: "branch appears to be a rename of <other-branch>"
 *     - most-recent-active: "logging to most-recent active session ..."
 *     - null: "no active sessions to fall back to"
 *
 * Returns the resolved session (active or stopped — caller decides what to do
 * with `data.status`) or `null` if all three tiers fail.
 */
export async function resolveCurrentSession(
  agentsDir: string,
  branch: string,
  opts: ResolveCurrentSessionOptions = {}
): Promise<ResolvedSession | null> {
  // Tier 1: explicit flag wins
  if (opts.sessionIdFlag) {
    const hit = findSessionByClaudeId(agentsDir, opts.sessionIdFlag, branch);
    if (hit) return hit;
    // Fall through on null — do NOT exit early.
  }

  // Tier 2: hook stdin
  //
  // Two opt-in paths, mutually exclusive (hint wins if both are set):
  //
  //  - `stdinSessionIdHint` — caller has already consumed stdin and passes
  //    the parsed id directly. Use this when the action body needs the rest
  //    of the hook payload too (e.g., `loaf session log --from-hook` reads
  //    stdin once for both routing and entry-text extraction).
  //
  //  - `parseStdin: true` — helper reads fd 0 itself. Use this from action
  //    bodies that don't otherwise touch stdin.
  //
  // Auto-detection (no opt-in) is rejected per SPEC-032 A5.
  const stdinId = opts.stdinSessionIdHint
    ? opts.stdinSessionIdHint
    : opts.parseStdin === true
      ? _parseHookSessionId()
      : undefined;
  if (stdinId) {
    const hit = findSessionByClaudeId(agentsDir, stdinId, branch);
    if (hit) return hit;
    // Fall through on null.
  }

  // Tier 3: branch routing (degraded path — always WARN).
  //
  // The WARN is shaped by how the finder resolved:
  //   - branch-match        → generic "falling back to branch routing"
  //   - rename-link         → "appears to be a rename of <other-branch>" (SPEC-042)
  //   - most-recent-active  → name target file + origin branch (SPEC-042)
  //   - null                → "no active sessions to fall back to" (SPEC-042)
  const found = findActiveSessionForBranch(agentsDir, branch);
  if (!found) {
    emitNoActiveSessionsWarning(branch);
    return null;
  }
  if (found.adoption === "branch-match") {
    emitBranchFallbackWarning(branch);
  } else {
    emitAdoptionWarning(
      branch,
      found.data.branch,
      found.filePath,
      found.adoption
    );
  }
  // Strip the adoption discriminator before returning — callers of
  // `resolveCurrentSession` see the original `ResolvedSession` shape.
  return {
    filePath: found.filePath,
    data: found.data,
    content: found.content,
  };
}
