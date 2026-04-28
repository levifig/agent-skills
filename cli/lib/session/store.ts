/**
 * Session store
 *
 * Foundational session-file helpers: frontmatter types, file IO, time helpers,
 * and journal-line extraction/consolidation. These are used by `find.ts` and
 * by the rest of the session command surface.
 */

import { existsSync, readFileSync, writeFileSync, renameSync, unlinkSync } from "fs";
import { basename } from "path";
import matter from "gray-matter";

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

export interface SessionFrontmatter {
  branch: string;
  status: "active" | "stopped" | "done" | "blocked" | "archived";
  created: string;
  last_updated?: string;
  last_entry?: string;
  archived_at?: string;
  archived_by?: string;
  linear_issue?: string;
  linear_url?: string;
  task?: string;
  spec?: string;
  title?: string;
  claude_session_id?: string;
  enriched_at?: string;
}

export interface SpecFrontmatterWithBranch {
  id: string;
  title: string;
  branch?: string;
  status?: string;
  session?: string;
  [key: string]: unknown;
}

// ─────────────────────────────────────────────────────────────────────────────
// Time helpers
// ─────────────────────────────────────────────────────────────────────────────

/** Current timestamp in ISO 8601 format */
export function getTimestamp(): string {
  return new Date().toISOString();
}

/** Date-time string for journal entries (YYYY-MM-DD HH:MM) */
export function getDateTimeString(): string {
  const now = new Date();
  const date = now.toISOString().split("T")[0];
  const time = now.toTimeString().split(":")[0] + ":" + now.toTimeString().split(":")[1];
  return `${date} ${time}`;
}

// ─────────────────────────────────────────────────────────────────────────────
// File IO
// ─────────────────────────────────────────────────────────────────────────────

/** Atomic write using temp file + rename */
export function writeFileAtomic(filePath: string, content: string): void {
  const tempPath = `${filePath}.tmp.${Date.now()}.${Math.random().toString(36).slice(2, 11)}`;
  try {
    writeFileSync(tempPath, content, "utf-8");
    renameSync(tempPath, filePath);
  } catch (err) {
    // Cleanup temp file on failure
    try { unlinkSync(tempPath); } catch { /* ignore */ }
    throw err;
  }
}

/** Read session file or return null */
export function readSessionFile(
  filePath: string
): { data: SessionFrontmatter; content: string } | null {
  if (!existsSync(filePath)) return null;

  try {
    const raw = readFileSync(filePath, "utf-8");
    const parsed = matter(raw);
    const rawData = parsed.data as unknown as Record<string, unknown>;

    // Handle legacy nested frontmatter migration (SPEC-020 format change)
    // Old format: { session: { branch, status, created, ... }, otherFields... }
    // New format: { branch, status, created, ... }
    // Migration strategy: nested fields take precedence over top-level fields on collision
    if (rawData.session && typeof rawData.session === "object") {
      const nested = rawData.session as Record<string, unknown>;
      // Start with all existing top-level fields
      const migratedData: Record<string, unknown> = { ...rawData };
      // Remove the nested session object itself
      delete migratedData.session;
      // Add/replace fields from nested session data
      for (const [key, value] of Object.entries(nested)) {
        migratedData[key] = value;
      }
      return {
        data: migratedData as unknown as SessionFrontmatter,
        content: parsed.content,
      };
    }

    return {
      data: rawData as unknown as SessionFrontmatter,
      content: parsed.content,
    };
  } catch {
    return null;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Journal helpers
// ─────────────────────────────────────────────────────────────────────────────

/** Extract journal entries from session content (lines matching [YYYY-MM-DD HH:MM] pattern) */
export function extractJournalLines(content: string): string[] {
  return content
    .split("\n")
    .filter((line) => /^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}\] /.test(line));
}

/** Merge journal entries from a secondary session into a primary session file */
export function consolidateSession(
  primaryPath: string,
  secondary: { filePath: string; data: SessionFrontmatter; content: string },
  _agentsDir: string
): void {
  const primary = readSessionFile(primaryPath);
  if (!primary) return;

  // Extract journal entries from secondary that aren't already in primary
  const primaryEntries = new Set(extractJournalLines(primary.content));
  const secondaryEntries = extractJournalLines(secondary.content);
  const newEntries = secondaryEntries.filter((e) => !primaryEntries.has(e));

  if (newEntries.length > 0) {
    // Append new entries before the last line of the journal
    const timestamp = getDateTimeString();
    const mergeMarker = `[${timestamp}] session(merge): consolidated from ${basename(secondary.filePath)}`;
    const entriesToAdd = [mergeMarker, ...newEntries];

    // Use appendEntry synchronously via direct file manipulation
    const trimmedContent = primary.content.trimEnd();
    const newContent = matter.stringify(
      trimmedContent + "\n" + entriesToAdd.join("\n") + "\n",
      {
        ...primary.data,
        last_updated: getTimestamp(),
        last_entry: getTimestamp(),
      } as unknown as Record<string, unknown>
    );
    writeFileAtomic(primaryPath, newContent);
  }

  // Delete the duplicate — its content is now in the primary
  try {
    unlinkSync(secondary.filePath);
  } catch {
    /* already gone */
  }
}
