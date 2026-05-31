/**
 * Session finders
 *
 * Two routing helpers used by every session subcommand:
 *
 * - `findSessionByClaudeId` — primary lookup keyed by `claude_session_id`,
 *   scans active + archive, consolidates splits.
 * - `findActiveSessionForBranch` — branch-keyed fallback, with rename-link
 *   detection and most-recent-active adoption.
 *
 * These helpers are called from both `cli/commands/session.ts` (existing call
 * sites) and `cli/lib/session/resolve.ts` (the SPEC-032 3-tier chain).
 *
 * Note on shell usage: `findActiveSessionForBranch` calls `git reflog` via
 * `execSync` with a piped `head -10`. This is moved verbatim from the
 * pre-extraction implementation in `cli/commands/session.ts` and intentionally
 * preserved as-is per SPEC-032 (move, don't change). The branch name comes
 * from `git branch --show-current` upstream — not user input.
 *
 * SPEC-042 Track B: this module no longer mutates session frontmatter when
 * resolving via fallback (rename-link or most-recent-active). The session's
 * `branch:` field represents its ORIGIN and is itself a routing key. The
 * caller still gets the session, plus an `adoption` discriminator so the
 * resolver can emit a richer WARN that names the resolved target.
 */

import { execSync } from "child_process";
import {
  existsSync,
  readdirSync,
  readFileSync,
  renameSync,
} from "fs";
import { basename, join } from "path";
import matter from "gray-matter";

import {
  consolidateSession,
  readSessionFile,
  type SessionFrontmatter,
  type SpecFrontmatterWithBranch,
} from "./store.js";

/** Find session by claude_session_id — scans active AND archived sessions, consolidates splits */
export function findSessionByClaudeId(
  agentsDir: string,
  claudeSessionId: string,
  currentBranch?: string
): { filePath: string; data: SessionFrontmatter; content: string } | null {
  const sessionsDir = join(agentsDir, "sessions");
  if (!existsSync(sessionsDir)) return null;

  type SessionMatch = {
    filePath: string;
    data: SessionFrontmatter;
    content: string;
    isArchived: boolean;
  };
  const matches: SessionMatch[] = [];

  // Scan active sessions
  const activeFiles = readdirSync(sessionsDir).filter(
    (f) => f.endsWith(".md") && !f.startsWith("archive")
  );
  for (const file of activeFiles) {
    const filePath = join(sessionsDir, file);
    const session = readSessionFile(filePath);
    if (session && session.data.claude_session_id === claudeSessionId) {
      matches.push({
        filePath,
        data: session.data,
        content: session.content,
        isArchived: false,
      });
    }
  }

  // Scan archive
  const archiveDir = join(sessionsDir, "archive");
  if (existsSync(archiveDir)) {
    const archiveFiles = readdirSync(archiveDir).filter((f) => f.endsWith(".md"));
    for (const file of archiveFiles) {
      const filePath = join(archiveDir, file);
      const session = readSessionFile(filePath);
      if (session && session.data.claude_session_id === claudeSessionId) {
        matches.push({
          filePath,
          data: session.data,
          content: session.content,
          isArchived: true,
        });
      }
    }
  }

  if (matches.length === 0) return null;

  // Pick primary: prefer current branch match, then most recent
  matches.sort((a, b) => {
    // Current branch wins
    if (currentBranch) {
      const aMatch = a.data.branch === currentBranch ? 0 : 1;
      const bMatch = b.data.branch === currentBranch ? 0 : 1;
      if (aMatch !== bMatch) return aMatch - bMatch;
    }
    // Active over archived
    if (a.isArchived !== b.isArchived) return a.isArchived ? 1 : -1;
    // Most recent wins
    const aTime = a.data.last_updated || a.data.last_entry || "0";
    const bTime = b.data.last_updated || b.data.last_entry || "0";
    return bTime.localeCompare(aTime);
  });

  const primary = matches[0];

  // Restore from archive if needed
  if (primary.isArchived) {
    const restoredPath = join(sessionsDir, basename(primary.filePath));
    try {
      renameSync(primary.filePath, restoredPath);
      primary.filePath = restoredPath;
      primary.isArchived = false;
    } catch {
      // Use in place if move fails
    }
  }

  // Consolidate duplicates into primary
  for (const secondary of matches.slice(1)) {
    consolidateSession(primary.filePath, secondary, agentsDir);
  }

  // Re-read primary after consolidation
  if (matches.length > 1) {
    const refreshed = readSessionFile(primary.filePath);
    if (refreshed) {
      return {
        filePath: primary.filePath,
        data: refreshed.data,
        content: refreshed.content,
      };
    }
  }

  return {
    filePath: primary.filePath,
    data: primary.data,
    content: primary.content,
  };
}

/**
 * How `findActiveSessionForBranch` resolved the returned session.
 *
 *  - `branch-match` — a session whose `branch:` equals the requested branch.
 *  - `rename-link`  — no direct match, but git reflog shows the branch was
 *    renamed (`git branch -m`) and a spec is linked to the parent branch
 *    pointing at an active session.
 *  - `most-recent-active` — no direct match and no rename link; the
 *    most-recently-updated active session (any branch) was adopted.
 *
 * The discriminator is informational only — `findActiveSessionForBranch` no
 * longer mutates the returned session's frontmatter in any case. Callers
 * (notably `resolveCurrentSession`) use it to shape user-facing WARN text.
 */
export type SessionAdoption = "branch-match" | "rename-link" | "most-recent-active";

/** Result of `findActiveSessionForBranch` — session plus adoption discriminator. */
export interface FindActiveSessionResult {
  filePath: string;
  data: SessionFrontmatter;
  content: string;
  /** How this session was resolved for the requested branch. */
  adoption: SessionAdoption;
}

/**
 * Find an active session for `branch`.
 *
 * Resolution order:
 *
 *   1. Sessions whose `branch:` frontmatter equals `branch` (direct match).
 *   2. If none, look for a `git branch -m` rename via reflog and resolve the
 *      session through the spec linked to the parent branch.
 *   3. If still none, fall back to the most-recently-updated active session
 *      across all branches (`last_updated` ➜ `last_entry` ➜ `created`).
 *
 * Never mutates session or spec frontmatter — see SPEC-042 Track B. Returns
 * `null` only when no active sessions exist at all.
 */
export function findActiveSessionForBranch(
  agentsDir: string,
  branch: string
): FindActiveSessionResult | null {
  const sessionsDir = join(agentsDir, "sessions");
  if (!existsSync(sessionsDir)) return null;

  const files = readdirSync(sessionsDir).filter(
    (f) => f.endsWith(".md") && !f.startsWith("archive")
  );

  // ─── Direct branch match ───────────────────────────────────────────────
  const candidates: Array<{
    filePath: string;
    data: SessionFrontmatter;
    content: string;
  }> = [];

  for (const file of files) {
    const filePath = join(sessionsDir, file);
    const session = readSessionFile(filePath);
    if (
      session &&
      session.data.branch === branch &&
      session.data.status !== "archived"
    ) {
      candidates.push({ filePath, data: session.data, content: session.content });
    }
  }

  if (candidates.length > 0) {
    const winner = pickCandidate(candidates);
    return { ...winner, adoption: "branch-match" };
  }

  // ─── Rename-link detection ─────────────────────────────────────────────
  //
  // If no session matched by branch name, check whether the current branch
  // was renamed from another branch (`git branch -m old new`). When we find
  // a spec linked to the parent branch pointing at an active session, return
  // it — but DO NOT rewrite the session's `branch:` field. The session's
  // origin branch is part of its identity; rewriting it on every adoption
  // breaks subsequent Tier 3 lookups.
  let parentBranch: string | null = null;
  try {
    const reflogOutput = execSync(
      `git reflog show --format='%H %gs' ${branch} 2>/dev/null | head -10`,
      { encoding: "utf-8" }
    );
    const renamedMatch = reflogOutput.match(
      /Branch: renamed refs\/heads\/([^\s]+) to/
    );
    if (renamedMatch) {
      parentBranch = renamedMatch[1];
    }
  } catch {
    // Git command failed, skip rename detection.
  }

  if (parentBranch) {
    const specsDir = join(agentsDir, "specs");
    if (existsSync(specsDir)) {
      const specFiles = readdirSync(specsDir).filter((f) => f.endsWith(".md"));
      for (const specFile of specFiles) {
        try {
          const specPath = join(specsDir, specFile);
          const specContent = readFileSync(specPath, "utf-8");
          const specParsed = matter(specContent);
          const specFm = specParsed.data as SpecFrontmatterWithBranch;

          if (specFm.branch === parentBranch && specFm.session) {
            const sessionPath = join(agentsDir, "sessions", specFm.session);
            if (existsSync(sessionPath)) {
              const session = readSessionFile(sessionPath);
              if (
                session &&
                session.data.branch === parentBranch &&
                session.data.status !== "archived"
              ) {
                return {
                  filePath: sessionPath,
                  data: session.data,
                  content: session.content,
                  adoption: "rename-link",
                };
              }
            }
          }
        } catch {
          continue;
        }
      }
    }
  }

  // ─── Most-recent active adoption (any branch) ──────────────────────────
  //
  // Replaces the prior `allSessions.length === 1` gating per SPEC-042 Track
  // B Bug B2: multi-worktree setups routinely have multiple active sessions
  // (e.g., orchestrator on `cwt/foo` plus a release agent on
  // `release/v0.16.0`). Pick the most-recently-updated one; recency is the
  // only tiebreaker. Returns null when no active sessions exist.
  const activeSessions: Array<{
    filePath: string;
    data: SessionFrontmatter;
    content: string;
  }> = [];
  for (const file of files) {
    const filePath = join(sessionsDir, file);
    const session = readSessionFile(filePath);
    if (session && session.data.status === "active") {
      activeSessions.push({ filePath, data: session.data, content: session.content });
    }
  }

  if (activeSessions.length === 0) return null;

  activeSessions.sort((a, b) => {
    const aTime =
      a.data.last_updated || a.data.last_entry || a.data.created || "0";
    const bTime =
      b.data.last_updated || b.data.last_entry || b.data.created || "0";
    const cmp = bTime.localeCompare(aTime); // descending — newest first
    if (cmp !== 0) return cmp;
    // Deterministic tiebreaker: alphabetical filePath. Session filenames are
    // `YYYYMMDD-HHMMSS-…md`, so this still favors the more recent file when
    // the effective timestamp (`last_updated`, falling back to `last_entry`,
    // falling back to `created`) is identical across candidates, and removes
    // the dependency on `readdirSync` collection order (filesystem-/platform-
    // dependent).
    return a.filePath.localeCompare(b.filePath);
  });

  const winner = activeSessions[0];
  return { ...winner, adoption: "most-recent-active" };
}

/**
 * Status-prioritized + recency tiebreaker selection across direct branch-match
 * candidates. `active` wins over terminal states; ties broken by most-recent
 * `last_updated` / `last_entry`.
 */
function pickCandidate(
  candidates: Array<{
    filePath: string;
    data: SessionFrontmatter;
    content: string;
  }>
): { filePath: string; data: SessionFrontmatter; content: string } {
  const statusPriority: Record<string, number> = {
    active: 1,
    stopped: 2,
    blocked: 2,
    done: 2,
  };

  const sorted = [...candidates].sort((a, b) => {
    const priorityA = statusPriority[a.data.status] ?? 3;
    const priorityB = statusPriority[b.data.status] ?? 3;
    if (priorityA !== priorityB) return priorityA - priorityB;

    const timeA = a.data.last_updated || a.data.last_entry || "0";
    const timeB = b.data.last_updated || b.data.last_entry || "0";
    const cmp = timeB.localeCompare(timeA); // descending — newest first
    if (cmp !== 0) return cmp;
    // Deterministic tiebreaker: alphabetical filePath. Tie only fires when the
    // effective timestamp (`last_updated`, falling back to `last_entry`) is
    // identical across candidates, and removes the dependency on
    // `readdirSync` collection order (filesystem-/platform-dependent).
    return a.filePath.localeCompare(b.filePath);
  });

  return sorted[0];
}
