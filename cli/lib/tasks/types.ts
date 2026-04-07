/**
 * Task Management Types
 *
 * Type definitions for the TASKS.json data model. Loaf uses a "managed .md"
 * approach: TASKS.json is the source of truth for structured metadata, while
 * task/spec .md files hold body content (descriptions, criteria, work logs).
 * Frontmatter in .md files is a read-only mirror synced from JSON.
 * This module defines the shape of both the index and the frontmatter types
 * used during initial migration from existing .md files.
 */

// ─────────────────────────────────────────────────────────────────────────────
// Status & Priority Enums
// ─────────────────────────────────────────────────────────────────────────────

/** Task statuses, ordered for display */
export type TaskStatus = "todo" | "in_progress" | "blocked" | "review" | "done";

/** Spec lifecycle statuses */
export type SpecStatus = "drafting" | "approved" | "implementing" | "complete";

/** Task priorities (P0 = critical, P3 = nice-to-have) */
export type TaskPriority = "P0" | "P1" | "P2" | "P3";

/** Ordered arrays for iteration and validation */
export const TASK_STATUSES: TaskStatus[] = [
  "todo",
  "in_progress",
  "blocked",
  "review",
  "done",
];

export const SPEC_STATUSES: SpecStatus[] = [
  "drafting",
  "approved",
  "implementing",
  "complete",
];

export const TASK_PRIORITIES: TaskPriority[] = ["P0", "P1", "P2", "P3"];

// ─────────────────────────────────────────────────────────────────────────────
// TASKS.json — Index Entries
// ─────────────────────────────────────────────────────────────────────────────

/** Individual task entry in TASKS.json */
export interface TaskEntry {
  title: string;
  /** Derived from filename, e.g. "intelligent-resume" */
  slug: string;
  /** Associated spec ID, e.g. "SPEC-002", or null */
  spec: string | null;
  status: TaskStatus;
  priority: TaskPriority;
  /** Task IDs this task depends on, e.g. ["TASK-017", "TASK-018"] */
  depends_on: string[];
  /** Hint files relevant to this task */
  files: string[];
  /** Shell command to verify completion */
  verify: string | null;
  /** Observable done condition */
  done: string | null;
  /** Session filename when task is picked up */
  session: string | null;
  /** ISO 8601 creation timestamp */
  created: string;
  /** ISO 8601 last-updated timestamp */
  updated: string;
  /** ISO 8601 timestamp, set when status transitions to "done" */
  completed_at: string | null;
  /** Relative path to the task file, e.g. "TASK-019-intelligent-resume.md" */
  file: string;
}

/** Individual spec entry in TASKS.json */
export interface SpecEntry {
  title: string;
  status: SpecStatus;
  /** Requirement or constraint summary */
  requirement: string | null;
  /** Origin: "direct" or reference to an idea file */
  source: string | null;
  /** ISO 8601 creation timestamp */
  created: string;
  /** Relative path to the spec file, e.g. "SPEC-010-task-management-cli.md" */
  file: string;
}

// ─────────────────────────────────────────────────────────────────────────────
// TASKS.json — Root Structure
// ─────────────────────────────────────────────────────────────────────────────

/** Root shape of TASKS.json */
export interface TaskIndex {
  /** Schema version for future migrations */
  version: 1;
  /** Next task ID to assign */
  next_id: number;
  /** Tasks keyed by ID, e.g. "TASK-019" */
  tasks: Record<string, TaskEntry>;
  /** Specs keyed by ID, e.g. "SPEC-010" */
  specs: Record<string, SpecEntry>;
}

// ─────────────────────────────────────────────────────────────────────────────
// Markdown Frontmatter — Parser Input Types
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Frontmatter as found in existing task .md files.
 * Fields may be missing or loosely typed; the parser normalizes these
 * into TaskEntry values.
 */
export interface TaskFrontmatter {
  id: string;
  title: string;
  spec?: string;
  status?: string;
  priority?: string;
  created?: string;
  updated?: string;
  depends_on?: string[];
  files?: string[];
  verify?: string;
  done?: string;
  session?: string;
  completed_at?: string;
}

/**
 * Frontmatter as found in existing spec .md files.
 * Fields may be missing or loosely typed; the parser normalizes these
 * into SpecEntry values.
 */
export interface SpecFrontmatter {
  id: string;
  title: string;
  status?: string;
  requirement?: string;
  source?: string;
  created?: string;
  reshaped?: string;
}
