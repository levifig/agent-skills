/**
 * Migration & Sync Tests
 *
 * Tests for buildIndexFromFiles(), loadIndex(), saveIndex(),
 * syncFrontmatterFromIndex(), and findOrphans().
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, statSync, rmSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import matter from "gray-matter";

import {
  buildIndexFromFiles,
  loadIndex,
  saveIndex,
  syncFrontmatterFromIndex,
  findOrphans,
} from "./migrate.js";
import type { TaskIndex, TaskEntry, SpecEntry } from "./types.js";

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

let tempDir: string;

function writeTask(
  dir: string,
  filename: string,
  fields: Record<string, unknown>,
  body = "# Task body\n",
): void {
  const tasksDir = join(dir, "tasks");
  mkdirSync(tasksDir, { recursive: true });
  const content = matter.stringify(body, fields);
  writeFileSync(join(tasksDir, filename), content, "utf-8");
}

function writeArchivedTask(
  dir: string,
  subpath: string,
  filename: string,
  fields: Record<string, unknown>,
  body = "# Archived task\n",
): void {
  const archiveDir = join(dir, "tasks", "archive", subpath);
  mkdirSync(archiveDir, { recursive: true });
  const content = matter.stringify(body, fields);
  writeFileSync(join(archiveDir, filename), content, "utf-8");
}

function writeSpec(
  dir: string,
  filename: string,
  fields: Record<string, unknown>,
  body = "# Spec body\n",
): void {
  const specsDir = join(dir, "specs");
  mkdirSync(specsDir, { recursive: true });
  const content = matter.stringify(body, fields);
  writeFileSync(join(specsDir, filename), content, "utf-8");
}

function writeArchivedSpec(
  dir: string,
  subpath: string,
  filename: string,
  fields: Record<string, unknown>,
  body = "# Archived spec\n",
): void {
  const archiveDir = join(dir, "specs", "archive", subpath);
  mkdirSync(archiveDir, { recursive: true });
  const content = matter.stringify(body, fields);
  writeFileSync(join(archiveDir, filename), content, "utf-8");
}

function taskEntry(overrides: Partial<TaskEntry> = {}): TaskEntry {
  return {
    title: "Test task",
    slug: "test",
    spec: null,
    status: "todo",
    priority: "P2",
    depends_on: [],
    files: [],
    verify: null,
    done: null,
    session: null,
    created: "2026-03-10T08:00:00Z",
    updated: "2026-03-10T08:00:00Z",
    completed_at: null,
    file: "TASK-001-test.md",
    ...overrides,
  };
}

function specEntry(overrides: Partial<SpecEntry> = {}): SpecEntry {
  return {
    title: "Test spec",
    status: "drafting",
    requirement: null,
    source: null,
    created: "2026-03-01T00:00:00Z",
    file: "SPEC-001-test.md",
    ...overrides,
  };
}

function minimalIndex(overrides: Partial<TaskIndex> = {}): TaskIndex {
  return {
    version: 1,
    next_id: 1,
    tasks: {},
    specs: {},
    ...overrides,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// Setup / Teardown
// ─────────────────────────────────────────────────────────────────────────────

beforeEach(() => {
  tempDir = mkdtempSync(join(tmpdir(), "loaf-test-"));
});

afterEach(() => {
  rmSync(tempDir, { recursive: true, force: true });
});

// ─────────────────────────────────────────────────────────────────────────────
// buildIndexFromFiles
// ─────────────────────────────────────────────────────────────────────────────

describe("buildIndexFromFiles", () => {
  it("returns empty index for empty directory", () => {
    const index = buildIndexFromFiles(tempDir);

    expect(index.version).toBe(1);
    expect(index.next_id).toBe(1);
    expect(Object.keys(index.tasks)).toHaveLength(0);
    expect(Object.keys(index.specs)).toHaveLength(0);
  });

  it("parses a single active task file", () => {
    writeTask(tempDir, "TASK-005-add-tests.md", {
      id: "TASK-005",
      title: "Add tests",
      status: "todo",
      priority: "P1",
      created: "2026-03-10T08:00:00Z",
    });

    const index = buildIndexFromFiles(tempDir);

    expect(Object.keys(index.tasks)).toHaveLength(1);
    expect(index.tasks["TASK-005"]).toBeDefined();
    expect(index.tasks["TASK-005"].title).toBe("Add tests");
    expect(index.tasks["TASK-005"].status).toBe("todo");
    expect(index.tasks["TASK-005"].priority).toBe("P1");
    expect(index.tasks["TASK-005"].file).toBe("TASK-005-add-tests.md");
    expect(index.next_id).toBe(6);
  });

  it("parses multiple active task files and computes next_id from max", () => {
    writeTask(tempDir, "TASK-003-first.md", {
      id: "TASK-003",
      title: "First",
      created: "2026-03-10T08:00:00Z",
    });
    writeTask(tempDir, "TASK-010-second.md", {
      id: "TASK-010",
      title: "Second",
      created: "2026-03-10T08:00:00Z",
    });
    writeTask(tempDir, "TASK-007-third.md", {
      id: "TASK-007",
      title: "Third",
      created: "2026-03-10T08:00:00Z",
    });

    const index = buildIndexFromFiles(tempDir);

    expect(Object.keys(index.tasks)).toHaveLength(3);
    expect(index.next_id).toBe(11); // max is 10, so next is 11
  });

  it("includes active and archived task files", () => {
    writeTask(tempDir, "TASK-010-active.md", {
      id: "TASK-010",
      title: "Active task",
      created: "2026-03-10T08:00:00Z",
    });
    writeArchivedTask(tempDir, "", "TASK-001-old.md", {
      id: "TASK-001",
      title: "Old task",
      status: "done",
      created: "2026-01-01T00:00:00Z",
    });

    const index = buildIndexFromFiles(tempDir);

    expect(Object.keys(index.tasks)).toHaveLength(2);
    expect(index.tasks["TASK-010"].file).toBe("TASK-010-active.md");
    expect(index.tasks["TASK-001"].file).toBe("archive/TASK-001-old.md");
  });

  it("preserves date subdirectory paths for archived tasks", () => {
    writeArchivedTask(tempDir, "2026-03", "TASK-001-archived.md", {
      id: "TASK-001",
      title: "Archived",
      status: "done",
      created: "2026-03-01T00:00:00Z",
    });

    const index = buildIndexFromFiles(tempDir);

    expect(index.tasks["TASK-001"]).toBeDefined();
    expect(index.tasks["TASK-001"].file).toBe("archive/2026-03/TASK-001-archived.md");
  });

  it("computes next_id from all tasks including archived", () => {
    writeTask(tempDir, "TASK-003-active.md", {
      id: "TASK-003",
      title: "Active",
      created: "2026-03-10T08:00:00Z",
    });
    writeArchivedTask(tempDir, "", "TASK-020-old.md", {
      id: "TASK-020",
      title: "Archived",
      status: "done",
      created: "2026-01-01T00:00:00Z",
    });

    const index = buildIndexFromFiles(tempDir);
    expect(index.next_id).toBe(21); // max is 20
  });

  it("parses spec files alongside tasks", () => {
    writeTask(tempDir, "TASK-001-test.md", {
      id: "TASK-001",
      title: "Test task",
      created: "2026-03-10T08:00:00Z",
    });
    writeSpec(tempDir, "SPEC-010-task-management.md", {
      id: "SPEC-010",
      title: "Task management",
      status: "implementing",
      created: "2026-03-01T00:00:00Z",
    });

    const index = buildIndexFromFiles(tempDir);

    expect(Object.keys(index.tasks)).toHaveLength(1);
    expect(Object.keys(index.specs)).toHaveLength(1);
    expect(index.specs["SPEC-010"].title).toBe("Task management");
    expect(index.specs["SPEC-010"].status).toBe("implementing");
    expect(index.specs["SPEC-010"].file).toBe("SPEC-010-task-management.md");
  });

  it("includes archived spec files", () => {
    writeSpec(tempDir, "SPEC-010-active.md", {
      id: "SPEC-010",
      title: "Active spec",
      created: "2026-03-10T00:00:00Z",
    });
    writeArchivedSpec(tempDir, "", "SPEC-001-old.md", {
      id: "SPEC-001",
      title: "Old spec",
      status: "complete",
      created: "2026-01-01T00:00:00Z",
    });

    const index = buildIndexFromFiles(tempDir);

    expect(Object.keys(index.specs)).toHaveLength(2);
    expect(index.specs["SPEC-010"].file).toBe("SPEC-010-active.md");
    expect(index.specs["SPEC-001"].file).toBe("archive/SPEC-001-old.md");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// loadIndex / saveIndex Roundtrip
// ─────────────────────────────────────────────────────────────────────────────

describe("loadIndex / saveIndex", () => {
  it("roundtrips an index through save and load", () => {
    const indexPath = join(tempDir, "TASKS.json");
    const original = minimalIndex({
      next_id: 42,
      tasks: {
        "TASK-041": taskEntry({
          title: "Test task",
          slug: "test-task",
          spec: "SPEC-001",
          status: "in_progress",
          priority: "P1",
          depends_on: ["TASK-040"],
          files: ["src/main.ts"],
          verify: "npm test",
          done: "All green",
          session: "session.md",
          updated: "2026-03-14T12:00:00Z",
          file: "TASK-041-test-task.md",
        }),
      },
      specs: {
        "SPEC-001": specEntry({
          status: "implementing",
          requirement: "Must work",
          source: "direct",
          file: "SPEC-001-test-spec.md",
        }),
      },
    });

    saveIndex(indexPath, original);
    const loaded = loadIndex(indexPath);

    expect(loaded).not.toBeNull();
    expect(loaded!.version).toBe(original.version);
    expect(loaded!.next_id).toBe(original.next_id);
    expect(loaded!.tasks).toEqual(original.tasks);
    expect(loaded!.specs).toEqual(original.specs);
  });

  it("returns null for non-existent file", () => {
    const result = loadIndex(join(tempDir, "nonexistent.json"));
    expect(result).toBeNull();
  });

  it("returns null for invalid JSON", () => {
    const indexPath = join(tempDir, "TASKS.json");
    writeFileSync(indexPath, "not valid json {{{", "utf-8");

    const result = loadIndex(indexPath);
    expect(result).toBeNull();
  });

  it("returns null for JSON with missing required fields", () => {
    const indexPath = join(tempDir, "TASKS.json");
    writeFileSync(indexPath, JSON.stringify({ version: 1 }), "utf-8");

    const result = loadIndex(indexPath);
    expect(result).toBeNull();
  });

  it("returns null for JSON with wrong field types", () => {
    const indexPath = join(tempDir, "TASKS.json");
    writeFileSync(
      indexPath,
      JSON.stringify({
        version: "one",
        next_id: 1,
        tasks: {},
        specs: {},
      }),
      "utf-8",
    );

    const result = loadIndex(indexPath);
    expect(result).toBeNull();
  });

  it("saves with pretty-printed JSON and trailing newline", () => {
    const indexPath = join(tempDir, "TASKS.json");
    saveIndex(indexPath, minimalIndex());

    const content = readFileSync(indexPath, "utf-8");
    expect(content).toContain("  ");
    expect(content.endsWith("\n")).toBe(true);
    expect(JSON.parse(content)).toBeDefined();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// syncFrontmatterFromIndex
// ─────────────────────────────────────────────────────────────────────────────

describe("syncFrontmatterFromIndex", () => {
  it("updates task file frontmatter when index differs", () => {
    writeTask(tempDir, "TASK-001-test.md", {
      id: "TASK-001",
      title: "Old title",
      status: "todo",
      priority: "P2",
    }, "# Task body\n");

    const index = minimalIndex({
      next_id: 2,
      tasks: {
        "TASK-001": taskEntry({
          title: "New title",
          status: "in_progress",
          priority: "P1",
          updated: "2026-03-14T12:00:00Z",
        }),
      },
    });

    syncFrontmatterFromIndex(tempDir, index);

    const raw = readFileSync(join(tempDir, "tasks", "TASK-001-test.md"), "utf-8");
    const { data } = matter(raw);
    expect(data.title).toBe("New title");
    expect(data.status).toBe("in_progress");
    expect(data.priority).toBe("P1");
  });

  it("does not rewrite file when frontmatter is identical", () => {
    const frontmatter = {
      id: "TASK-001",
      title: "Unchanged",
      status: "todo",
      priority: "P2",
      created: "2026-03-10T08:00:00Z",
      updated: "2026-03-10T08:00:00Z",
    };

    writeTask(tempDir, "TASK-001-test.md", frontmatter, "# Body\n");

    const filePath = join(tempDir, "tasks", "TASK-001-test.md");
    const mtimeBefore = statSync(filePath).mtimeMs;

    const index = minimalIndex({
      next_id: 2,
      tasks: {
        "TASK-001": taskEntry({ title: "Unchanged" }),
      },
    });

    syncFrontmatterFromIndex(tempDir, index);

    const mtimeAfter = statSync(filePath).mtimeMs;
    expect(mtimeAfter).toBe(mtimeBefore);
  });

  it("handles missing file on disk gracefully", () => {
    const index = minimalIndex({
      next_id: 2,
      tasks: {
        "TASK-099": taskEntry({ title: "Missing file", file: "TASK-099-missing.md" }),
      },
    });

    expect(() => syncFrontmatterFromIndex(tempDir, index)).not.toThrow();
  });

  it("syncs spec files too", () => {
    writeSpec(tempDir, "SPEC-010-test.md", {
      id: "SPEC-010",
      title: "Old spec title",
      status: "drafting",
    }, "# Spec body\n");

    const index = minimalIndex({
      specs: {
        "SPEC-010": specEntry({ title: "Updated spec title", status: "approved", file: "SPEC-010-test.md" }),
      },
    });

    syncFrontmatterFromIndex(tempDir, index);

    const raw = readFileSync(join(tempDir, "specs", "SPEC-010-test.md"), "utf-8");
    const { data } = matter(raw);
    expect(data.title).toBe("Updated spec title");
    expect(data.status).toBe("approved");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// findOrphans
// ─────────────────────────────────────────────────────────────────────────────

describe("findOrphans", () => {
  it("returns no orphans when all files are in the index", () => {
    writeTask(tempDir, "TASK-001-test.md", {
      id: "TASK-001",
      title: "Test",
      created: "2026-03-10T08:00:00Z",
    });

    const index = minimalIndex({
      next_id: 2,
      tasks: {
        "TASK-001": taskEntry({ title: "Test" }),
      },
    });

    const orphans = findOrphans(tempDir, index);
    expect(orphans.tasks).toHaveLength(0);
    expect(orphans.specs).toHaveLength(0);
  });

  it("finds active task file not in index", () => {
    writeTask(tempDir, "TASK-001-known.md", {
      id: "TASK-001",
      title: "Known",
      created: "2026-03-10T08:00:00Z",
    });
    writeTask(tempDir, "TASK-002-orphan.md", {
      id: "TASK-002",
      title: "Orphan",
      created: "2026-03-10T08:00:00Z",
    });

    const index = minimalIndex({
      next_id: 2,
      tasks: {
        "TASK-001": taskEntry({ title: "Known", slug: "known", file: "TASK-001-known.md" }),
      },
    });

    const orphans = findOrphans(tempDir, index);
    expect(orphans.tasks).toHaveLength(1);
    expect(orphans.tasks[0].id).toBe("TASK-002");
    expect(orphans.tasks[0].entry.title).toBe("Orphan");
  });

  it("finds archived orphan task with correct relative path", () => {
    writeArchivedTask(tempDir, "2026-03", "TASK-005-archived.md", {
      id: "TASK-005",
      title: "Archived orphan",
      status: "done",
      created: "2026-03-01T00:00:00Z",
    });

    const index = minimalIndex();

    const orphans = findOrphans(tempDir, index);
    expect(orphans.tasks).toHaveLength(1);
    expect(orphans.tasks[0].id).toBe("TASK-005");
    expect(orphans.tasks[0].entry.file).toBe("archive/2026-03/TASK-005-archived.md");
  });

  it("finds orphan spec files", () => {
    writeSpec(tempDir, "SPEC-001-orphan.md", {
      id: "SPEC-001",
      title: "Orphan spec",
      created: "2026-03-01T00:00:00Z",
    });

    const index = minimalIndex();

    const orphans = findOrphans(tempDir, index);
    expect(orphans.specs).toHaveLength(1);
    expect(orphans.specs[0].id).toBe("SPEC-001");
  });

  it("does not double-count files between active and archive", () => {
    writeTask(tempDir, "TASK-001-active.md", {
      id: "TASK-001",
      title: "Active",
      created: "2026-03-10T08:00:00Z",
    });

    const index = minimalIndex();

    const orphans = findOrphans(tempDir, index);
    expect(orphans.tasks).toHaveLength(1);
    expect(orphans.tasks[0].id).toBe("TASK-001");
  });

  it("returns no orphans for empty directories", () => {
    mkdirSync(join(tempDir, "tasks"), { recursive: true });
    mkdirSync(join(tempDir, "specs"), { recursive: true });

    const index = minimalIndex();

    const orphans = findOrphans(tempDir, index);
    expect(orphans.tasks).toHaveLength(0);
    expect(orphans.specs).toHaveLength(0);
  });

  it("handles nonexistent directories gracefully", () => {
    const index = minimalIndex();

    const orphans = findOrphans(tempDir, index);
    expect(orphans.tasks).toHaveLength(0);
    expect(orphans.specs).toHaveLength(0);
  });
});
