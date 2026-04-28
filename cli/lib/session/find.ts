/**
 * Session finders
 *
 * Two routing helpers used by every session subcommand:
 *
 * - `findSessionByClaudeId` — primary lookup keyed by `claude_session_id`,
 *   scans active + archive, consolidates splits.
 * - `findActiveSessionForBranch` — branch-keyed fallback, with rename and
 *   single-active-session adoption heuristics.
 *
 * These helpers are called from both `cli/commands/session.ts` (existing call
 * sites) and `cli/lib/session/resolve.ts` (the new 3-tier chain).
 *
 * Note on shell usage: `findActiveSessionForBranch` calls `git reflog` via
 * `execSync` with a piped `head -10`. This is moved verbatim from the
 * pre-extraction implementation in `cli/commands/session.ts` and intentionally
 * preserved as-is per SPEC-032 (move, don't change). The branch name comes
 * from `git branch --show-current` upstream — not user input.
 */

import { execSync } from "child_process";
import {
  existsSync,
  readdirSync,
  readFileSync,
  writeFileSync,
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

/** Find active session for a branch by scanning files */
export function findActiveSessionForBranch(
  agentsDir: string,
  branch: string
): { filePath: string; data: SessionFrontmatter; content: string } | null {
  const sessionsDir = join(agentsDir, "sessions");
  if (!existsSync(sessionsDir)) return null;

  const files = readdirSync(sessionsDir).filter(
    (f) => f.endsWith(".md") && !f.startsWith("archive")
  );

  // Collect all candidate sessions for this branch
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

  // If no session found by branch name, check for renamed branch via spec linkage
  // This handles: git branch -m old-name new-name (explicit rename only)
  // We verify the rename by checking git reflog for the explicit "Branch: renamed" pattern
  if (candidates.length === 0) {
    // Get current branch's creation info from reflog
    let parentBranch: string | null = null;
    try {
      // Check reflog for explicit rename pattern from "git branch -m old new"
      const reflogOutput = execSync(
        `git reflog show --format='%H %gs' ${branch} 2>/dev/null | head -10`,
        { encoding: "utf-8" }
      );

      // ONLY match explicit rename: "Branch: renamed refs/heads/old-branch to refs/heads/new-branch"
      // Do NOT match "branch: Created from ..." (that's normal branch creation)
      // Do NOT match "checkout: moving from ..." (that's branch switching)
      const renamedMatch = reflogOutput.match(
        /Branch: renamed refs\/heads\/([^\s]+) to/
      );

      if (renamedMatch) {
        parentBranch = renamedMatch[1];
      }
    } catch {
      // Git command failed, skip rename detection
    }

    // If we found a parent branch, look for sessions that were on that branch
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

            // Only consider specs linked to the PARENT branch (the one we came from)
            if (specFm.branch === parentBranch && specFm.session) {
              const sessionPath = join(agentsDir, "sessions", specFm.session);
              if (existsSync(sessionPath)) {
                const session = readSessionFile(sessionPath);
                // Verify this session was indeed on the parent branch and is non-archived
                if (
                  session &&
                  session.data.branch === parentBranch &&
                  session.data.status !== "archived"
                ) {
                  // RENAME CONFIRMED: Update both session and spec to new branch name
                  session.data.branch = branch;
                  const newSessionContent = matter.stringify(
                    session.content,
                    session.data as unknown as Record<string, unknown>
                  );
                  writeFileSync(sessionPath, newSessionContent, "utf-8");

                  specFm.branch = branch;
                  const newSpecContent = matter.stringify(
                    specParsed.content,
                    specFm as Record<string, unknown>
                  );
                  writeFileSync(specPath, newSpecContent, "utf-8");

                  candidates.push({
                    filePath: sessionPath,
                    data: session.data,
                    content: session.content,
                  });
                  break; // Found it
                }
              }
            }
          } catch {
            // Continue to next spec
            continue;
          }
        }
      }
    }
  }

  // Branch switch detection: if no session matches the current branch,
  // but exactly one active session exists, adopt it (common flow: start on main, then branch)
  if (candidates.length === 0) {
    const allSessions: Array<{
      filePath: string;
      data: SessionFrontmatter;
      content: string;
    }> = [];
    for (const file of files) {
      const filePath = join(sessionsDir, file);
      const session = readSessionFile(filePath);
      if (session && session.data.status === "active") {
        allSessions.push({ filePath, data: session.data, content: session.content });
      }
    }

    if (allSessions.length === 1) {
      const session = allSessions[0];
      session.data.branch = branch;
      const newContent = matter.stringify(
        session.content,
        session.data as unknown as Record<string, unknown>
      );
      writeFileSync(session.filePath, newContent, "utf-8");
      candidates.push(session);
    }
  }

  if (candidates.length === 0) return null;

  // Prioritize: active > stopped/blocked/done > others
  // Sort by status priority (lower number = higher priority)
  const statusPriority: Record<string, number> = {
    active: 1,
    stopped: 2,
    blocked: 2,
    done: 2,
  };

  candidates.sort((a, b) => {
    // First: sort by status priority
    const priorityA = statusPriority[a.data.status] ?? 3;
    const priorityB = statusPriority[b.data.status] ?? 3;
    if (priorityA !== priorityB) {
      return priorityA - priorityB;
    }

    // Second: tie-break by recency (newer wins)
    // Use last_updated from frontmatter, or fall back to last_entry, or 0
    const timeA = a.data.last_updated || a.data.last_entry || "0";
    const timeB = b.data.last_updated || b.data.last_entry || "0";
    return timeB.localeCompare(timeA); // descending (newer first)
  });

  return candidates[0];
}
