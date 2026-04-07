/**
 * Frontmatter Parser
 *
 * Parses task and spec .md files, extracting frontmatter and normalizing
 * it into TaskEntry / SpecEntry shapes. Used during migration (building
 * TASKS.json from existing .md files) and during sync operations.
 */

import { basename } from "path";
import matter from "gray-matter";
import type {
  TaskEntry,
  SpecEntry,
  TaskFrontmatter,
  SpecFrontmatter,
  TaskStatus,
  TaskPriority,
  SpecStatus,
} from "./types.js";
import {
  TASK_STATUSES,
  SPEC_STATUSES,
  TASK_PRIORITIES,
} from "./types.js";

// ANSI color helpers (matching project conventions)
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;

// ─────────────────────────────────────────────────────────────────────────────
// Status & Priority Normalization
// ─────────────────────────────────────────────────────────────────────────────

/** Common status aliases that appear in existing frontmatter */
const TASK_STATUS_ALIASES: Record<string, TaskStatus> = {
  "complete": "done",
  "completed": "done",
  "archived": "done",
  "in-progress": "in_progress",
  "in progress": "in_progress",
  "wip": "in_progress",
  "pending": "todo",
  "waiting": "blocked",
};

const SPEC_STATUS_ALIASES: Record<string, SpecStatus> = {
  "draft": "drafting",
  "done": "complete",
  "completed": "complete",
  "archived": "complete",
  "implemented": "complete",
  "in-progress": "implementing",
  "in_progress": "implementing",
};

/**
 * Normalize a raw frontmatter status string to a valid TaskStatus.
 * Maps common variants: "complete" -> "done", "completed" -> "done",
 * "in-progress" -> "in_progress". Returns "todo" as default if unrecognized.
 */
function normalizeTaskStatus(raw: string | undefined): TaskStatus {
  if (!raw) return "todo";

  const lower = raw.trim().toLowerCase();

  // Direct match against valid statuses
  if (TASK_STATUSES.includes(lower as TaskStatus)) {
    return lower as TaskStatus;
  }

  // Alias lookup
  return TASK_STATUS_ALIASES[lower] ?? "todo";
}

/**
 * Normalize a raw priority string. "P1" stays "P1", unrecognized -> "P2"
 * (default medium).
 */
function normalizeTaskPriority(raw: string | undefined): TaskPriority {
  if (!raw) return "P2";

  const upper = raw.trim().toUpperCase();

  if (TASK_PRIORITIES.includes(upper as TaskPriority)) {
    return upper as TaskPriority;
  }

  return "P2";
}

/**
 * Normalize a raw spec status string. Returns "drafting" as default
 * if unrecognized.
 */
function normalizeSpecStatus(raw: string | undefined): SpecStatus {
  if (!raw) return "drafting";

  const lower = raw.trim().toLowerCase();

  if (SPEC_STATUSES.includes(lower as SpecStatus)) {
    return lower as SpecStatus;
  }

  return SPEC_STATUS_ALIASES[lower] ?? "drafting";
}

// ─────────────────────────────────────────────────────────────────────────────
// Date Normalization
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Normalize a date value to an ISO 8601 string.
 * Handles: Date objects (from gray-matter auto-parsing), ISO strings,
 * bare dates like "2026-03-15".
 */
function normalizeDate(value: unknown): string {
  if (!value) return new Date().toISOString();

  // gray-matter auto-parses YAML dates into Date objects
  if (value instanceof Date) {
    return value.toISOString();
  }

  if (typeof value === "string") {
    const trimmed = value.trim();

    // Already an ISO timestamp
    if (trimmed.includes("T")) {
      return trimmed;
    }

    // Bare date like "2026-03-15" — append midnight UTC
    if (/^\d{4}-\d{2}-\d{2}$/.test(trimmed)) {
      return `${trimmed}T00:00:00Z`;
    }

    // Fallback: try parsing
    const parsed = new Date(trimmed);
    if (!isNaN(parsed.getTime())) {
      return parsed.toISOString();
    }
  }

  return new Date().toISOString();
}

// ─────────────────────────────────────────────────────────────────────────────
// Filename Parsing
// ─────────────────────────────────────────────────────────────────────────────

/** Extract task ID and slug from a filename like "TASK-019-intelligent-resume.md" */
function parseTaskFilename(filePath: string): { id: string | null; slug: string } {
  const name = basename(filePath, ".md");
  const match = name.match(/^(TASK-\d+)(?:-(.+))?$/);

  if (!match) {
    return { id: null, slug: name };
  }

  return {
    id: match[1],
    slug: match[2] ?? "",
  };
}

/** Extract spec ID from a filename like "SPEC-010-task-management-cli.md" */
function parseSpecFilename(filePath: string): { id: string | null } {
  const name = basename(filePath, ".md");
  const match = name.match(/^(SPEC-\d+)/);

  return { id: match ? match[1] : null };
}

// ─────────────────────────────────────────────────────────────────────────────
// Public API
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Parse a task .md file and return a normalized TaskEntry.
 * Extracts ID and slug from filename (e.g., "TASK-019-intelligent-resume.md"
 * -> id: "TASK-019", slug: "intelligent-resume").
 *
 * @param filePath - Path to the task file (used for filename parsing)
 * @param content - File content string
 * @returns Parsed task with ID and entry, or null if unparseable
 */
export function parseTaskFile(
  filePath: string,
  content: string,
): { id: string; entry: TaskEntry } | null {
  try {
    const { data } = matter(content);
    const fm = data as unknown as TaskFrontmatter;
    const { id: filenameId, slug } = parseTaskFilename(filePath);

    // Resolve task ID: prefer frontmatter, fall back to filename
    const id = fm.id || filenameId;
    if (!id) {
      console.error(`  ${yellow("warn:")} Could not determine task ID for ${basename(filePath)}`);
      return null;
    }

    const now = new Date().toISOString();
    const status = normalizeTaskStatus(fm.status);

    const entry: TaskEntry = {
      title: fm.title || basename(filePath, ".md"),
      slug,
      spec: fm.spec || null,
      status,
      priority: normalizeTaskPriority(fm.priority),
      depends_on: Array.isArray(fm.depends_on) ? fm.depends_on : [],
      files: Array.isArray(fm.files) ? fm.files : [],
      verify: fm.verify || null,
      done: fm.done || null,
      session: fm.session || null,
      created: normalizeDate(fm.created),
      updated: normalizeDate(fm.updated ?? fm.created),
      completed_at: status === "done"
        ? (fm.completed_at ? normalizeDate(fm.completed_at) : normalizeDate(fm.updated ?? fm.created ?? now))
        : null,
      file: basename(filePath),
    };

    return { id, entry };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    console.error(`  ${yellow("warn:")} Failed to parse ${basename(filePath)}: ${message}`);
    return null;
  }
}

/**
 * Parse a spec .md file and return a normalized SpecEntry.
 * Extracts ID from filename (e.g., "SPEC-010-task-management-cli.md"
 * -> id: "SPEC-010").
 *
 * @param filePath - Path to the spec file (used for filename parsing)
 * @param content - File content string
 * @returns Parsed spec with ID and entry, or null if unparseable
 */
export function parseSpecFile(
  filePath: string,
  content: string,
): { id: string; entry: SpecEntry } | null {
  try {
    const { data } = matter(content);
    const fm = data as unknown as SpecFrontmatter;
    const { id: filenameId } = parseSpecFilename(filePath);

    // Resolve spec ID: prefer frontmatter, fall back to filename
    const id = fm.id || filenameId;
    if (!id) {
      console.error(`  ${yellow("warn:")} Could not determine spec ID for ${basename(filePath)}`);
      return null;
    }

    const entry: SpecEntry = {
      title: fm.title || basename(filePath, ".md"),
      status: normalizeSpecStatus(fm.status),
      requirement: fm.requirement || null,
      source: fm.source || null,
      created: normalizeDate(fm.created),
      file: basename(filePath),
    };

    return { id, entry };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    console.error(`  ${yellow("warn:")} Failed to parse ${basename(filePath)}: ${message}`);
    return null;
  }
}
