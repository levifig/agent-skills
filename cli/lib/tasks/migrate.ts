/**
 * Migration & Sync Logic
 *
 * Scans task/spec .md files, parses frontmatter, and builds TASKS.json.
 * Also handles syncing frontmatter from JSON back to .md files and
 * detecting orphaned files.
 */

import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync, readdirSync } from "fs";
import { join, basename, relative } from "path";
import matter from "gray-matter";
import { parseTaskFile, parseSpecFile } from "./parser.js";
import type { TaskIndex, TaskEntry, SpecEntry } from "./types.js";

// ANSI color helpers (matching project conventions)
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;

// ─────────────────────────────────────────────────────────────────────────────
// File Discovery
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Collect .md files matching a prefix from a directory (non-recursive).
 * Only returns files directly in the given directory.
 */
function collectFiles(dir: string, prefix: string): string[] {
  if (!existsSync(dir)) return [];

  try {
    return readdirSync(dir)
      .filter((f) => f.startsWith(prefix) && f.endsWith(".md"))
      .map((f) => join(dir, f));
  } catch {
    return [];
  }
}

/**
 * Collect .md files matching a prefix from a directory and one level of
 * subdirectories (e.g., archive/YYYY-MM/). Used for archive dirs that
 * may have date-based subdirectory layouts.
 */
function collectFilesDeep(dir: string, prefix: string): string[] {
  if (!existsSync(dir)) return [];

  const results: string[] = [];

  try {
    const entries = readdirSync(dir, { withFileTypes: true });

    for (const entry of entries) {
      if (entry.isFile() && entry.name.startsWith(prefix) && entry.name.endsWith(".md")) {
        results.push(join(dir, entry.name));
      } else if (entry.isDirectory()) {
        try {
          const subEntries = readdirSync(join(dir, entry.name));
          for (const sub of subEntries) {
            if (sub.startsWith(prefix) && sub.endsWith(".md")) {
              results.push(join(dir, entry.name, sub));
            }
          }
        } catch {
          // Can't read subdirectory — skip
        }
      }
    }
  } catch {
    // Can't read directory — skip
  }

  return results;
}

/**
 * Extract the numeric portion of a task/spec ID.
 * "TASK-019" -> 19, "SPEC-010" -> 10
 */
function extractNumber(id: string): number {
  const match = id.match(/\d+$/);
  return match ? parseInt(match[0], 10) : 0;
}

// ─────────────────────────────────────────────────────────────────────────────
// Index Building
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Scan task and spec directories, parse all .md files, build a TaskIndex.
 * Used for initial migration and `loaf task sync --import`.
 *
 * @param agentsDir - Path to .agents/ directory
 * @returns A complete TaskIndex with all tasks and specs
 */
export function buildIndexFromFiles(agentsDir: string): TaskIndex {
  const tasksDir = join(agentsDir, "tasks");
  const specsDir = join(agentsDir, "specs");
  const tasksArchiveDir = join(tasksDir, "archive");
  const specsArchiveDir = join(specsDir, "archive");

  const tasks: Record<string, TaskEntry> = {};
  const specs: Record<string, SpecEntry> = {};
  let maxTaskNum = 0;

  // ── Collect task files ──────────────────────────────────────────────────

  const activeTaskFiles = collectFiles(tasksDir, "TASK-");
  const archivedTaskFiles = collectFilesDeep(tasksArchiveDir, "TASK-");

  for (const filePath of activeTaskFiles) {
    const content = readFileSync(filePath, "utf-8");
    const result = parseTaskFile(filePath, content);

    if (result) {
      result.entry.file = basename(filePath);
      tasks[result.id] = result.entry;
      maxTaskNum = Math.max(maxTaskNum, extractNumber(result.id));
    }
  }

  for (const filePath of archivedTaskFiles) {
    const content = readFileSync(filePath, "utf-8");
    const result = parseTaskFile(filePath, content);

    if (result) {
      // Relative to tasksDir so archive/2026-03/TASK-001.md is preserved
      result.entry.file = relative(tasksDir, filePath);
      tasks[result.id] = result.entry;
      maxTaskNum = Math.max(maxTaskNum, extractNumber(result.id));
    }
  }

  // ── Collect spec files ──────────────────────────────────────────────────

  const activeSpecFiles = collectFiles(specsDir, "SPEC-");
  const archivedSpecFiles = collectFilesDeep(specsArchiveDir, "SPEC-");

  for (const filePath of activeSpecFiles) {
    const content = readFileSync(filePath, "utf-8");
    const result = parseSpecFile(filePath, content);

    if (result) {
      result.entry.file = basename(filePath);
      specs[result.id] = result.entry;
    }
  }

  for (const filePath of archivedSpecFiles) {
    const content = readFileSync(filePath, "utf-8");
    const result = parseSpecFile(filePath, content);

    if (result) {
      result.entry.file = relative(specsDir, filePath);
      specs[result.id] = result.entry;
    }
  }

  return {
    version: 1,
    next_id: maxTaskNum + 1,
    tasks,
    specs,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// Index I/O
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Load an existing TASKS.json, return null if not found or invalid.
 */
export function loadIndex(indexPath: string): TaskIndex | null {
  if (!existsSync(indexPath)) return null;

  try {
    const content = readFileSync(indexPath, "utf-8");
    const parsed = JSON.parse(content) as TaskIndex;

    // Basic shape validation
    if (
      typeof parsed.version !== "number" ||
      typeof parsed.next_id !== "number" ||
      typeof parsed.tasks !== "object" ||
      typeof parsed.specs !== "object"
    ) {
      console.error(`  ${yellow("warn:")} TASKS.json has invalid shape, ignoring`);
      return null;
    }

    return parsed;
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    console.error(`  ${yellow("warn:")} Failed to read TASKS.json: ${message}`);
    return null;
  }
}

/**
 * Save a TaskIndex to TASKS.json (pretty-printed, 2-space indent).
 */
export function saveIndex(indexPath: string, index: TaskIndex): void {
  const content = JSON.stringify(index, null, 2) + "\n";
  writeFileSync(indexPath, content, "utf-8");
}

// ─────────────────────────────────────────────────────────────────────────────
// Frontmatter Sync (JSON -> .md)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Build the frontmatter object for a task entry.
 * Mirrors the TaskFrontmatter shape expected in .md files.
 */
function taskEntryToFrontmatter(
  id: string,
  entry: TaskEntry,
): Record<string, unknown> {
  const fm: Record<string, unknown> = {
    id,
    title: entry.title,
  };

  if (entry.spec) fm.spec = entry.spec;
  fm.status = entry.status;
  fm.priority = entry.priority;

  if (entry.created) fm.created = entry.created;
  if (entry.updated) fm.updated = entry.updated;

  if (entry.depends_on.length > 0) fm.depends_on = entry.depends_on;
  if (entry.files.length > 0) fm.files = entry.files;
  if (entry.verify) fm.verify = entry.verify;
  if (entry.done) fm.done = entry.done;
  if (entry.session) fm.session = entry.session;
  if (entry.completed_at) fm.completed_at = entry.completed_at;

  return fm;
}

/**
 * Build the frontmatter object for a spec entry.
 */
function specEntryToFrontmatter(
  id: string,
  entry: SpecEntry,
): Record<string, unknown> {
  const fm: Record<string, unknown> = {
    id,
    title: entry.title,
  };

  if (entry.source) fm.source = entry.source;
  if (entry.created) fm.created = entry.created;
  fm.status = entry.status;
  if (entry.requirement) fm.requirement = entry.requirement;

  return fm;
}

/**
 * Check if two frontmatter objects are equivalent (shallow comparison).
 * Compares JSON serialization to handle array/object equality.
 */
function frontmatterEquals(
  a: Record<string, unknown>,
  b: Record<string, unknown>,
): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

/**
 * Resolve a task/spec file path from its `file` field relative to the
 * appropriate base directory.
 */
function resolveFilePath(
  agentsDir: string,
  subdir: string,
  relFile: string,
): string {
  return join(agentsDir, subdir, relFile);
}

/**
 * Sync .md frontmatter from the index (JSON -> .md direction).
 * For each task/spec in the index, update the corresponding .md file's
 * frontmatter to match the JSON entry. Preserves body content exactly.
 *
 * @param agentsDir - Path to .agents/ directory
 * @param index - The current TaskIndex
 */
export function syncFrontmatterFromIndex(
  agentsDir: string,
  index: TaskIndex,
): void {
  // ── Sync tasks ──────────────────────────────────────────────────────────

  for (const [id, entry] of Object.entries(index.tasks)) {
    const filePath = resolveFilePath(agentsDir, "tasks", entry.file);

    if (!existsSync(filePath)) {
      console.error(`  ${gray("skip:")} ${entry.file} not found on disk`);
      continue;
    }

    try {
      const raw = readFileSync(filePath, "utf-8");
      const { data: existingFm, content: body } = matter(raw);
      const newFm = taskEntryToFrontmatter(id, entry);

      if (frontmatterEquals(existingFm, newFm)) continue;

      const updated = matter.stringify(body, newFm);
      writeFileSync(filePath, updated, "utf-8");
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      console.error(`  ${yellow("warn:")} Failed to sync ${entry.file}: ${message}`);
    }
  }

  // ── Sync specs ──────────────────────────────────────────────────────────

  for (const [id, entry] of Object.entries(index.specs)) {
    const filePath = resolveFilePath(agentsDir, "specs", entry.file);

    if (!existsSync(filePath)) {
      console.error(`  ${gray("skip:")} ${entry.file} not found on disk`);
      continue;
    }

    try {
      const raw = readFileSync(filePath, "utf-8");
      const { data: existingFm, content: body } = matter(raw);
      const newFm = specEntryToFrontmatter(id, entry);

      if (frontmatterEquals(existingFm, newFm)) continue;

      const updated = matter.stringify(body, newFm);
      writeFileSync(filePath, updated, "utf-8");
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      console.error(`  ${yellow("warn:")} Failed to sync ${entry.file}: ${message}`);
    }
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Orphan Detection
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Find orphan .md files that exist on disk but aren't in the index.
 * Returns parsed entries ready to be merged into the index.
 */
export function findOrphans(
  agentsDir: string,
  index: TaskIndex,
): {
  tasks: Array<{ id: string; entry: TaskEntry }>;
  specs: Array<{ id: string; entry: SpecEntry }>;
} {
  const tasksDir = join(agentsDir, "tasks");
  const specsDir = join(agentsDir, "specs");
  const tasksArchiveDir = join(tasksDir, "archive");
  const specsArchiveDir = join(specsDir, "archive");

  const orphanTasks: Array<{ id: string; entry: TaskEntry }> = [];
  const orphanSpecs: Array<{ id: string; entry: SpecEntry }> = [];

  // Known task/spec IDs from the index
  const knownTaskIds = new Set(Object.keys(index.tasks));
  const knownSpecIds = new Set(Object.keys(index.specs));

  // ── Check task files ────────────────────────────────────────────────────

  const checkTaskFiles = (dir: string, baseDir: string, deep: boolean) => {
    const files = deep ? collectFilesDeep(dir, "TASK-") : collectFiles(dir, "TASK-");
    for (const filePath of files) {
      const content = readFileSync(filePath, "utf-8");
      const result = parseTaskFile(filePath, content);

      if (result && !knownTaskIds.has(result.id)) {
        result.entry.file = relative(baseDir, filePath);
        orphanTasks.push(result);
      }
    }
  };

  checkTaskFiles(tasksDir, tasksDir, false);
  checkTaskFiles(tasksArchiveDir, tasksDir, true);

  // ── Check spec files ────────────────────────────────────────────────────

  const checkSpecFiles = (dir: string, baseDir: string, deep: boolean) => {
    const files = deep ? collectFilesDeep(dir, "SPEC-") : collectFiles(dir, "SPEC-");
    for (const filePath of files) {
      const content = readFileSync(filePath, "utf-8");
      const result = parseSpecFile(filePath, content);

      if (result && !knownSpecIds.has(result.id)) {
        result.entry.file = relative(baseDir, filePath);
        orphanSpecs.push(result);
      }
    }
  };

  checkSpecFiles(specsDir, specsDir, false);
  checkSpecFiles(specsArchiveDir, specsDir, true);

  return { tasks: orphanTasks, specs: orphanSpecs };
}

// ─────────────────────────────────────────────────────────────────────────────
// Archive Operations
// ─────────────────────────────────────────────────────────────────────────────

export interface ArchiveResult {
  id: string;
  status: "archived" | "skipped";
  reason?: string;
}

/**
 * Move completed items to archive/ and update their index entries.
 * Does NOT call saveIndex — caller must persist after reviewing results.
 */
function archiveItems(
  baseDir: string,
  entries: Record<string, { file: string; status: string }>,
  ids: string[],
  requiredStatus: string,
): ArchiveResult[] {
  const archiveDir = join(baseDir, "archive");
  const results: ArchiveResult[] = [];

  for (const id of ids) {
    const entry = entries[id];

    if (!entry) {
      results.push({ id, status: "skipped", reason: "not found in index" });
      continue;
    }

    if (entry.status !== requiredStatus) {
      results.push({ id, status: "skipped", reason: `status is ${entry.status}, must be ${requiredStatus}` });
      continue;
    }

    if (entry.file.startsWith("archive/")) {
      results.push({ id, status: "skipped", reason: "already archived" });
      continue;
    }

    const srcPath = join(baseDir, entry.file);
    if (!existsSync(srcPath)) {
      results.push({ id, status: "skipped", reason: `file not found at ${entry.file}` });
      continue;
    }

    mkdirSync(archiveDir, { recursive: true });

    const destPath = join(archiveDir, entry.file);
    if (existsSync(destPath)) {
      results.push({ id, status: "skipped", reason: `archive/${entry.file} already exists` });
      continue;
    }

    renameSync(srcPath, destPath);
    entry.file = `archive/${entry.file}`;

    results.push({ id, status: "archived" });
  }

  return results;
}

export function archiveTasks(
  agentsDir: string,
  index: TaskIndex,
  taskIds: string[],
): ArchiveResult[] {
  return archiveItems(join(agentsDir, "tasks"), index.tasks, taskIds, "done");
}

export function archiveSpecs(
  agentsDir: string,
  index: TaskIndex,
  specIds: string[],
): ArchiveResult[] {
  return archiveItems(join(agentsDir, "specs"), index.specs, specIds, "complete");
}
