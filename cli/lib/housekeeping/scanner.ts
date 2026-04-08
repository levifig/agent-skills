/**
 * Walks .agents/ and produces cleanup recommendations (no prompts).
 */

import { existsSync, readFileSync, readdirSync, statSync } from "fs";
import { join } from "path";
import matter from "gray-matter";

import { loadIndex, buildIndexFromFiles, findOrphans } from "../tasks/migrate.js";
import type { TaskIndex } from "../tasks/types.js";
import type {
  ArtifactType,
  CleanupRecommendation,
  ScanResult,
  ScanOptions,
  TypeSummary,
  ArtifactDirectory,
} from "./types.js";
import { ARTIFACT_DIRS, ARTIFACT_TYPES } from "./types.js";

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

/** Read .md files from a directory (non-recursive, excludes archive/) */
function readMdFiles(dir: string): Array<{ path: string; filename: string; frontmatter: Record<string, unknown>; raw: string }> {
  if (!existsSync(dir)) return [];

  const results: Array<{ path: string; filename: string; frontmatter: Record<string, unknown>; raw: string }> = [];

  let entries: string[];
  try {
    entries = readdirSync(dir);
  } catch {
    return results;
  }

  for (const name of entries) {
    if (name === "archive" || !name.endsWith(".md")) continue;
    const filePath = join(dir, name);
    try {
      if (!statSync(filePath).isFile()) continue;
      const raw = readFileSync(filePath, "utf-8");
      const { data } = matter(raw);
      results.push({ path: filePath, filename: name, frontmatter: data, raw });
    } catch {
      continue;
    }
  }

  return results;
}

/** Days since a date string (ISO 8601 or similar) */
function daysSince(dateStr: string | undefined): number | null {
  if (!dateStr) return null;
  const date = new Date(dateStr);
  if (isNaN(date.getTime())) return null;
  return Math.floor((Date.now() - date.getTime()) / (1000 * 60 * 60 * 24));
}

/**
 * Normalize a session reference to a bare filename.
 * Templates store paths like ".agents/sessions/FILE.md" — strip the prefix.
 */
function normalizeSessionRef(ref: string): string {
  return ref.replace(/^\.agents\/sessions\/(archive\/)?/, "");
}

/** Most recent activity date from frontmatter (nested `session.*` or top-level). */
function lastActivity(fm: Record<string, unknown>): string | undefined {
  const sessionBlock = fm.session as Record<string, unknown> | undefined;
  if (sessionBlock && typeof sessionBlock === "object") {
    return (sessionBlock.last_updated as string) || (sessionBlock.created as string) || undefined;
  }
  return (fm.updated as string) || (fm.created as string) || undefined;
}

/** Extract status from frontmatter, supporting nested session.status */
function getStatus(fm: Record<string, unknown>): string {
  const sessionBlock = fm.session as Record<string, unknown> | undefined;
  if (sessionBlock && typeof sessionBlock === "object" && sessionBlock.status) {
    return String(sessionBlock.status).toLowerCase();
  }
  return String(fm.status || "").toLowerCase();
}

// ─────────────────────────────────────────────────────────────────────────────
// Per-Artifact Scanners
// ─────────────────────────────────────────────────────────────────────────────

function scanSessions(agentsDir: string): CleanupRecommendation[] {
  const dir = join(agentsDir, "sessions");
  const files = readMdFiles(dir);
  const recs: CleanupRecommendation[] = [];

  for (const file of files) {
    const fm = file.frontmatter;
    const status = getStatus(fm);

    const hasLearnings =
      file.raw.includes("## Key Decisions") ||
      file.raw.includes("## Lessons Learned") ||
      file.raw.includes("lessons_learned") ||
      (fm.traceability && typeof fm.traceability === "object" &&
        "decisions" in (fm.traceability as Record<string, unknown>));

    if (status === "completed" || status === "complete") {
      if (hasLearnings) {
        recs.push({
          type: "session",
          path: file.path,
          filename: file.filename,
          action: "archive",
          reason: "Completed session with extractable learnings",
          hint: "Consider running /crystallize before archiving",
          frontmatter: fm,
        });
      } else {
        recs.push({
          type: "session",
          path: file.path,
          filename: file.filename,
          action: "archive",
          reason: "Completed session",
          frontmatter: fm,
        });
      }
    } else if (status === "cancelled" || status === "abandoned") {
      recs.push({
        type: "session",
        path: file.path,
        filename: file.filename,
        action: "archive",
        reason: `Session ${status} — archive with status preserved`,
        frontmatter: fm,
      });
    } else {
      const days = daysSince(lastActivity(fm));
      if (days !== null && days > 7) {
        recs.push({
          type: "session",
          path: file.path,
          filename: file.filename,
          action: "flag",
          reason: `Stale session — inactive for ${days} days`,
          frontmatter: fm,
        });
      } else {
        recs.push({
          type: "session",
          path: file.path,
          filename: file.filename,
          action: "skip",
          reason: "Active session",
          frontmatter: fm,
        });
      }
    }
  }

  return recs;
}

function scanTasks(agentsDir: string, index: TaskIndex): CleanupRecommendation[] {
  const recs: CleanupRecommendation[] = [];

  const knownSpecIds = new Set(Object.keys(index.specs));

  for (const [id, entry] of Object.entries(index.tasks)) {
    if (entry.file.startsWith("archive/")) continue;

    const filePath = join(agentsDir, "tasks", entry.file);

    if (entry.status === "done") {
      recs.push({
        type: "task",
        path: filePath,
        filename: entry.file,
        action: "archive",
        reason: `Task ${id} is done`,
        frontmatter: { id, title: entry.title, status: entry.status, spec: entry.spec },
      });
    } else if (entry.spec && !knownSpecIds.has(entry.spec)) {
      recs.push({
        type: "task",
        path: filePath,
        filename: entry.file,
        action: "flag",
        reason: `Task ${id} references missing spec ${entry.spec}`,
        frontmatter: { id, title: entry.title, status: entry.status, spec: entry.spec },
      });
    } else {
      recs.push({
        type: "task",
        path: filePath,
        filename: entry.file,
        action: "skip",
        reason: `Task ${id} is ${entry.status}`,
        frontmatter: { id, title: entry.title, status: entry.status },
      });
    }
  }

  const orphans = findOrphans(agentsDir, index);
  for (const orphan of orphans.tasks) {
    recs.push({
      type: "task",
      path: join(agentsDir, "tasks", orphan.entry.file),
      filename: orphan.entry.file,
      action: "flag",
      reason: `Task ${orphan.id} exists on disk but not in index — run loaf task sync --import`,
      frontmatter: { id: orphan.id, title: orphan.entry.title },
    });
  }

  return recs;
}

function scanSpecs(agentsDir: string, index: TaskIndex): CleanupRecommendation[] {
  const recs: CleanupRecommendation[] = [];

  for (const [id, entry] of Object.entries(index.specs)) {
    if (entry.file.startsWith("archive/")) continue;

    const filePath = join(agentsDir, "specs", entry.file);

    if (entry.status === "complete") {
      recs.push({
        type: "spec",
        path: filePath,
        filename: entry.file,
        action: "archive",
        reason: `Spec ${id} is complete`,
        frontmatter: { id, title: entry.title, status: entry.status },
      });
    } else if (entry.status === "drafting") {
      const days = daysSince(entry.created);
      if (days !== null && days > 14) {
        recs.push({
          type: "spec",
          path: filePath,
          filename: entry.file,
          action: "flag",
          reason: `Spec ${id} has been drafting for ${days} days`,
          frontmatter: { id, title: entry.title, status: entry.status },
        });
      } else {
        recs.push({
          type: "spec",
          path: filePath,
          filename: entry.file,
          action: "skip",
          reason: `Spec ${id} is ${entry.status}`,
          frontmatter: { id, title: entry.title, status: entry.status },
        });
      }
    } else {
      recs.push({
        type: "spec",
        path: filePath,
        filename: entry.file,
        action: "skip",
        reason: `Spec ${id} is ${entry.status}`,
        frontmatter: { id, title: entry.title, status: entry.status },
      });
    }
  }

  const orphans = findOrphans(agentsDir, index);
  for (const orphan of orphans.specs) {
    recs.push({
      type: "spec",
      path: join(agentsDir, "specs", orphan.entry.file),
      filename: orphan.entry.file,
      action: "flag",
      reason: `Spec ${orphan.id} exists on disk but not in index — run loaf task sync --import`,
      frontmatter: { id: orphan.id, title: orphan.entry.title },
    });
  }

  return recs;
}

function scanPlans(agentsDir: string): CleanupRecommendation[] {
  const dir = join(agentsDir, "plans");
  const files = readMdFiles(dir);
  const recs: CleanupRecommendation[] = [];

  const sessionsDir = join(agentsDir, "sessions");
  const activeSessionFiles: string[] = [];
  const archivedSessionFiles: string[] = [];

  if (existsSync(sessionsDir)) {
    for (const name of readdirSync(sessionsDir)) {
      if (name === "archive" || !name.endsWith(".md")) continue;
      try {
        const raw = readFileSync(join(sessionsDir, name), "utf-8");
        const { data } = matter(raw);
        const status = getStatus(data);
        if (status === "completed" || status === "complete" || status === "cancelled" || status === "archived") {
          archivedSessionFiles.push(name);
        } else {
          activeSessionFiles.push(name);
        }
      } catch {
        continue;
      }
    }
  }

  const sessionArchive = join(sessionsDir, "archive");
  if (existsSync(sessionArchive)) {
    try {
      for (const name of readdirSync(sessionArchive)) {
        if (name.endsWith(".md")) archivedSessionFiles.push(name);
      }
    } catch {
      /* skip */
    }
  }

  const matchesSession = (ref: string, files: string[]): boolean => {
    if (files.includes(ref)) return true;
    const stem = ref.replace(/\.md$/, "");
    return files.some((f) => f.startsWith(stem));
  };

  for (const file of files) {
    const fm = file.frontmatter;
    const sessionRef = fm.session as string | undefined;

    if (!sessionRef) {
      recs.push({
        type: "plan",
        path: file.path,
        filename: file.filename,
        action: "delete",
        reason: "Orphaned plan — no linked session",
        frontmatter: fm,
      });
    } else if (matchesSession(sessionRef, archivedSessionFiles)) {
      recs.push({
        type: "plan",
        path: file.path,
        filename: file.filename,
        action: "delete",
        reason: "Linked session is archived/completed",
        frontmatter: fm,
      });
    } else if (!matchesSession(sessionRef, activeSessionFiles)) {
      recs.push({
        type: "plan",
        path: file.path,
        filename: file.filename,
        action: "delete",
        reason: `Linked session "${sessionRef}" not found`,
        frontmatter: fm,
      });
    } else {
      const days = daysSince(lastActivity(fm));
      if (days !== null && days > 14) {
        recs.push({
          type: "plan",
          path: file.path,
          filename: file.filename,
          action: "delete",
          reason: `Stale plan — inactive for ${days} days with active session`,
          frontmatter: fm,
        });
      } else {
        recs.push({
          type: "plan",
          path: file.path,
          filename: file.filename,
          action: "skip",
          reason: "Plan linked to active session",
          frontmatter: fm,
        });
      }
    }
  }

  return recs;
}

function scanDrafts(agentsDir: string, index: TaskIndex): CleanupRecommendation[] {
  const dir = join(agentsDir, "drafts");
  const files = readMdFiles(dir);
  const recs: CleanupRecommendation[] = [];

  // Build set of archived session filenames for session-linked cleanup
  const archivedSessionFiles: string[] = [];
  const sessionArchiveDir = join(agentsDir, "sessions", "archive");
  if (existsSync(sessionArchiveDir)) {
    try {
      for (const name of readdirSync(sessionArchiveDir)) {
        if (name.endsWith(".md")) archivedSessionFiles.push(name);
      }
    } catch { /* skip */ }
  }
  // Also check sessions with archived/complete status in main dir
  const sessionsDir = join(agentsDir, "sessions");
  if (existsSync(sessionsDir)) {
    for (const name of readdirSync(sessionsDir)) {
      if (name === "archive" || !name.endsWith(".md")) continue;
      try {
        const raw = readFileSync(join(sessionsDir, name), "utf-8");
        const { data } = matter(raw);
        const status = getStatus(data);
        if (["completed", "complete", "archived", "stopped"].includes(status)) {
          archivedSessionFiles.push(name);
        }
      } catch { continue; }
    }
  }

  const sessionIsArchived = (ref: string): boolean => {
    const normalized = normalizeSessionRef(ref);
    if (archivedSessionFiles.includes(normalized)) return true;
    const stem = normalized.replace(/\.md$/, "");
    return archivedSessionFiles.some((f) => f.startsWith(stem));
  };

  const promotedDrafts = new Set<string>();
  for (const entry of Object.values(index.specs)) {
    if (entry.source && entry.source.includes("/drafts/")) {
      const filename = entry.source.replace(/^.*\/drafts\//, "");
      promotedDrafts.add(filename);
    }
  }

  for (const file of files) {
    const fm = file.frontmatter;
    const days = daysSince(lastActivity(fm) || fm.created as string);
    const draftType = (fm.type as string || "").toLowerCase();
    const sessionRef = fm.session as string | undefined;

    if (promotedDrafts.has(file.filename)) {
      const hasSparks = file.raw.includes("## Sparks");
      recs.push({
        type: "draft",
        path: file.path,
        filename: file.filename,
        action: "delete",
        reason: "Draft promoted to spec — served its purpose",
        hint: hasSparks ? "Contains ## Sparks section — review before deletion" : undefined,
        frontmatter: fm,
      });
    } else if (draftType === "state-assessment" && sessionRef && sessionIsArchived(sessionRef)) {
      // Session-linked cleanup: state assessments whose linked session is archived
      recs.push({
        type: "draft",
        path: file.path,
        filename: file.filename,
        action: "delete",
        reason: "State assessment — linked session is archived",
        frontmatter: fm,
      });
    } else if (days !== null && days > 30) {
      const hasSparks = file.raw.includes("## Sparks");
      recs.push({
        type: "draft",
        path: file.path,
        filename: file.filename,
        action: "flag",
        reason: `Stale draft — ${days} days old`,
        hint: hasSparks ? "Contains ## Sparks section — review before deletion" : undefined,
        frontmatter: fm,
      });
    } else {
      recs.push({
        type: "draft",
        path: file.path,
        filename: file.filename,
        action: "skip",
        reason: "Recent draft",
        frontmatter: fm,
      });
    }
  }

  return recs;
}

function scanCouncils(agentsDir: string): CleanupRecommendation[] {
  const dir = join(agentsDir, "councils");
  const files = readMdFiles(dir);
  const recs: CleanupRecommendation[] = [];

  const sessionsDir = join(agentsDir, "sessions");
  const allSessionFiles: string[] = [];
  const completedSessionFiles: string[] = [];

  for (const subdir of [sessionsDir, join(sessionsDir, "archive")]) {
    if (existsSync(subdir)) {
      try {
        for (const name of readdirSync(subdir)) {
          if (!name.endsWith(".md")) continue;
          allSessionFiles.push(name);
          try {
            const raw = readFileSync(join(subdir, name), "utf-8");
            const { data } = matter(raw);
            const status = getStatus(data);
            if (["completed", "complete", "archived", "cancelled"].includes(status)) {
              completedSessionFiles.push(name);
            }
          } catch { /* skip */ }
        }
      } catch { /* skip */ }
    }
  }

  const matchesAnySession = (ref: string): boolean => {
    if (allSessionFiles.includes(ref)) return true;
    const stem = ref.replace(/\.md$/, "");
    return allSessionFiles.some((f) => f.startsWith(stem));
  };

  const sessionIsCompleted = (ref: string): boolean => {
    if (completedSessionFiles.includes(ref)) return true;
    const stem = ref.replace(/\.md$/, "");
    return completedSessionFiles.some((f) => f.startsWith(stem));
  };

  for (const file of files) {
    const fm = file.frontmatter;

    const councilBlock = fm.council as Record<string, unknown> | undefined;
    const councilDate = (councilBlock?.created || councilBlock?.timestamp) as string | undefined;
    const sessionRef = (councilBlock?.session_reference || councilBlock?.session) as string | undefined;
    const days = daysSince(councilDate || lastActivity(fm));

    if (sessionRef && !matchesAnySession(normalizeSessionRef(sessionRef))) {
      recs.push({
        type: "council",
        path: file.path,
        filename: file.filename,
        action: "flag",
        reason: `Orphaned council — linked session "${sessionRef}" not found`,
        frontmatter: fm,
      });
    } else if (days !== null && days > 14) {
      const hasDecision = file.raw.includes("## Decision") &&
        !file.raw.match(/## Decision\s*\n\s*\n\s*(\[To be filled|$)/);

      const normalizedRef = sessionRef ? normalizeSessionRef(sessionRef) : null;
      const sessionDone = normalizedRef && sessionIsCompleted(normalizedRef);

      if (hasDecision && sessionDone) {
        recs.push({
          type: "council",
          path: file.path,
          filename: file.filename,
          action: "archive",
          reason: `Council is ${days} days old — decision recorded, session completed`,
          hint: "Verify the linked session summary captures the council outcome before archiving",
          frontmatter: fm,
        });
      } else {
        const reasons: string[] = [];
        if (!hasDecision) reasons.push("decision not yet recorded");
        if (!sessionRef) reasons.push("no linked session");
        else if (!sessionDone) reasons.push("linked session not yet completed");
        recs.push({
          type: "council",
          path: file.path,
          filename: file.filename,
          action: "flag",
          reason: `Stale council — ${days} days old, ${reasons.join(", ")}`,
          frontmatter: fm,
        });
      }
    } else {
      recs.push({
        type: "council",
        path: file.path,
        filename: file.filename,
        action: "skip",
        reason: "Recent council",
        frontmatter: fm,
      });
    }
  }

  return recs;
}

function scanReports(agentsDir: string): CleanupRecommendation[] {
  const dir = join(agentsDir, "reports");
  const files = readMdFiles(dir);
  const recs: CleanupRecommendation[] = [];

  const sessionArchiveDir = join(agentsDir, "sessions", "archive");
  const archivedSessionFiles: string[] = [];
  if (existsSync(sessionArchiveDir)) {
    try {
      for (const name of readdirSync(sessionArchiveDir)) {
        if (name.endsWith(".md")) archivedSessionFiles.push(name);
      }
    } catch { /* skip */ }
  }

  const sessionIsArchived = (ref: string): boolean => {
    if (archivedSessionFiles.includes(ref)) return true;
    const stem = ref.replace(/\.md$/, "");
    return archivedSessionFiles.some((f) => f.startsWith(stem));
  };

  for (const file of files) {
    const fm = file.frontmatter;

    // Support both flat frontmatter (new: status, archived_at) and
    // nested report: block (legacy: report.status, report.processed_at)
    const reportBlock = fm.report as Record<string, unknown> | undefined;
    const archivedAt = fm.archived_at || reportBlock?.archived_at;
    const status = (fm.status as string || reportBlock?.status as string || "").toLowerCase();
    const sessionRef = (reportBlock?.session_reference as string | undefined);

    if (archivedAt || status === "archived") {
      recs.push({
        type: "report",
        path: file.path,
        filename: file.filename,
        action: "skip",
        reason: "Already archived",
        frontmatter: fm,
      });
    } else if (status === "final" || status === "processed" || reportBlock?.processed_at) {
      // "final" (new schema) and "processed" (legacy) are both archive-ready
      if (sessionRef && !sessionIsArchived(normalizeSessionRef(sessionRef))) {
        recs.push({
          type: "report",
          path: file.path,
          filename: file.filename,
          action: "skip",
          reason: "Report is finalized but linked session is not yet archived",
          frontmatter: fm,
        });
      } else {
        recs.push({
          type: "report",
          path: file.path,
          filename: file.filename,
          action: "archive",
          reason: "Report is finalized and prerequisites met — ready for archive",
          frontmatter: fm,
        });
      }
    } else if (status === "draft") {
      const days = daysSince(lastActivity(fm));
      if (days !== null && days > 14) {
        recs.push({
          type: "report",
          path: file.path,
          filename: file.filename,
          action: "flag",
          reason: `Draft report is ${days} days old — consider finalizing or deleting`,
          frontmatter: fm,
        });
      } else {
        recs.push({
          type: "report",
          path: file.path,
          filename: file.filename,
          action: "skip",
          reason: "Recent draft report",
          frontmatter: fm,
        });
      }
    } else {
      const days = daysSince(lastActivity(fm));
      if (days !== null && days > 14) {
        recs.push({
          type: "report",
          path: file.path,
          filename: file.filename,
          action: "flag",
          reason: `Report is ${days} days old with unknown status "${status || "(none)"}"`,
          frontmatter: fm,
        });
      } else {
        recs.push({
          type: "report",
          path: file.path,
          filename: file.filename,
          action: "skip",
          reason: "Recent report",
          frontmatter: fm,
        });
      }
    }
  }

  return recs;
}

// ─────────────────────────────────────────────────────────────────────────────
// Main Scanner
// ─────────────────────────────────────────────────────────────────────────────

export function scanArtifacts(options: ScanOptions): ScanResult {
  const { agentsDir, filter } = options;
  const recommendations: CleanupRecommendation[] = [];
  const warnings: string[] = [];

  for (const dir of ARTIFACT_DIRS) {
    if (filter && !filter.includes(dir.type)) continue;

    const fullPath = join(agentsDir, dir.dirname);
    if (!existsSync(fullPath)) {
      if (dir.required) {
        warnings.push(`Required directory missing: ${dir.dirname}/`);
      }
    }
  }

  const indexPath = join(agentsDir, "TASKS.json");
  const index = loadIndex(indexPath) ?? buildIndexFromFiles(agentsDir);

  const shouldScan = (type: ArtifactType) => !filter || filter.includes(type);

  if (shouldScan("session")) recommendations.push(...scanSessions(agentsDir));
  if (shouldScan("task")) recommendations.push(...scanTasks(agentsDir, index));
  if (shouldScan("spec")) recommendations.push(...scanSpecs(agentsDir, index));
  if (shouldScan("plan")) recommendations.push(...scanPlans(agentsDir));
  if (shouldScan("draft")) recommendations.push(...scanDrafts(agentsDir, index));
  if (shouldScan("council")) recommendations.push(...scanCouncils(agentsDir));
  if (shouldScan("report")) recommendations.push(...scanReports(agentsDir));

  const summary: TypeSummary[] = ARTIFACT_TYPES
    .filter((t) => shouldScan(t))
    .map((type) => {
      const typeRecs = recommendations.filter((r) => r.type === type);
      return {
        type,
        total: typeRecs.length,
        archive: typeRecs.filter((r) => r.action === "archive").length,
        delete: typeRecs.filter((r) => r.action === "delete").length,
        flag: typeRecs.filter((r) => r.action === "flag").length,
        skip: typeRecs.filter((r) => r.action === "skip").length,
      };
    })
    .filter((s) => s.total > 0);

  return { recommendations, summary, warnings };
}
