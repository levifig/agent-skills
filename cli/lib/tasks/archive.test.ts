/**
 * Archive Operation Tests
 *
 * Tests for archiveTasks() and archiveSpecs() — moving completed
 * items to archive/ and updating their index entries.
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdtempSync, mkdirSync, writeFileSync, existsSync, rmSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import matter from "gray-matter";

import { archiveTasks, archiveSpecs } from "./migrate.js";
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
  tempDir = mkdtempSync(join(tmpdir(), "loaf-archive-test-"));
});

afterEach(() => {
  rmSync(tempDir, { recursive: true, force: true });
});

// ─────────────────────────────────────────────────────────────────────────────
// archiveTasks
// ─────────────────────────────────────────────────────────────────────────────

describe("archiveTasks", () => {
  it("archives a done task — moves file and updates index entry", () => {
    writeTask(tempDir, "TASK-001-test.md", {
      id: "TASK-001",
      title: "Test task",
      status: "done",
    });

    const index = minimalIndex({
      next_id: 2,
      tasks: {
        "TASK-001": taskEntry({
          status: "done",
          completed_at: "2026-03-10T12:00:00Z",
          file: "TASK-001-test.md",
        }),
      },
    });

    const results = archiveTasks(tempDir, index, ["TASK-001"]);

    expect(results).toHaveLength(1);
    expect(results[0]).toEqual({ id: "TASK-001", status: "archived" });
    expect(existsSync(join(tempDir, "tasks", "TASK-001-test.md"))).toBe(false);
    expect(existsSync(join(tempDir, "tasks", "archive", "TASK-001-test.md"))).toBe(true);
    expect(index.tasks["TASK-001"].file).toBe("archive/TASK-001-test.md");
  });

  it("rejects a task that is not done", () => {
    writeTask(tempDir, "TASK-001-test.md", {
      id: "TASK-001",
      title: "In progress",
      status: "in_progress",
    });

    const index = minimalIndex({
      next_id: 2,
      tasks: {
        "TASK-001": taskEntry({ status: "in_progress", file: "TASK-001-test.md" }),
      },
    });

    const results = archiveTasks(tempDir, index, ["TASK-001"]);

    expect(results).toHaveLength(1);
    expect(results[0].status).toBe("skipped");
    expect(results[0].reason).toContain("must be done");
    expect(existsSync(join(tempDir, "tasks", "TASK-001-test.md"))).toBe(true);
  });

  it("skips a task already in archive", () => {
    const index = minimalIndex({
      next_id: 2,
      tasks: {
        "TASK-001": taskEntry({
          status: "done",
          file: "archive/TASK-001-test.md",
        }),
      },
    });

    const results = archiveTasks(tempDir, index, ["TASK-001"]);

    expect(results).toHaveLength(1);
    expect(results[0].status).toBe("skipped");
    expect(results[0].reason).toBe("already archived");
  });

  it("skips a task not found in index", () => {
    const index = minimalIndex();

    const results = archiveTasks(tempDir, index, ["TASK-999"]);

    expect(results).toHaveLength(1);
    expect(results[0].status).toBe("skipped");
    expect(results[0].reason).toBe("not found in index");
  });

  it("skips when source file is missing on disk", () => {
    const index = minimalIndex({
      next_id: 2,
      tasks: {
        "TASK-001": taskEntry({ status: "done", file: "TASK-001-ghost.md" }),
      },
    });

    const results = archiveTasks(tempDir, index, ["TASK-001"]);

    expect(results).toHaveLength(1);
    expect(results[0].status).toBe("skipped");
    expect(results[0].reason).toContain("file not found");
  });

  it("skips when destination already exists", () => {
    writeTask(tempDir, "TASK-001-test.md", {
      id: "TASK-001",
      title: "Test",
      status: "done",
    });

    const archiveDir = join(tempDir, "tasks", "archive");
    mkdirSync(archiveDir, { recursive: true });
    writeFileSync(join(archiveDir, "TASK-001-test.md"), "existing file", "utf-8");

    const index = minimalIndex({
      next_id: 2,
      tasks: {
        "TASK-001": taskEntry({ status: "done", file: "TASK-001-test.md" }),
      },
    });

    const results = archiveTasks(tempDir, index, ["TASK-001"]);

    expect(results).toHaveLength(1);
    expect(results[0].status).toBe("skipped");
    expect(results[0].reason).toContain("already exists");
    expect(existsSync(join(tempDir, "tasks", "TASK-001-test.md"))).toBe(true);
  });

  it("archives multiple tasks in one call", () => {
    writeTask(tempDir, "TASK-001-first.md", {
      id: "TASK-001",
      title: "First",
      status: "done",
    });
    writeTask(tempDir, "TASK-002-second.md", {
      id: "TASK-002",
      title: "Second",
      status: "done",
    });

    const index = minimalIndex({
      next_id: 3,
      tasks: {
        "TASK-001": taskEntry({ title: "First", status: "done", file: "TASK-001-first.md" }),
        "TASK-002": taskEntry({ title: "Second", status: "done", file: "TASK-002-second.md" }),
      },
    });

    const results = archiveTasks(tempDir, index, ["TASK-001", "TASK-002"]);

    const archived = results.filter((r) => r.status === "archived");
    expect(archived).toHaveLength(2);

    expect(index.tasks["TASK-001"].file).toBe("archive/TASK-001-first.md");
    expect(index.tasks["TASK-002"].file).toBe("archive/TASK-002-second.md");
  });

  it("handles mixed valid and invalid IDs", () => {
    writeTask(tempDir, "TASK-001-good.md", {
      id: "TASK-001",
      title: "Good",
      status: "done",
    });

    const index = minimalIndex({
      next_id: 3,
      tasks: {
        "TASK-001": taskEntry({ status: "done", file: "TASK-001-good.md" }),
        "TASK-002": taskEntry({ status: "in_progress", file: "TASK-002-wip.md" }),
      },
    });

    const results = archiveTasks(tempDir, index, ["TASK-001", "TASK-002", "TASK-999"]);

    expect(results).toHaveLength(3);
    expect(results[0]).toEqual({ id: "TASK-001", status: "archived" });
    expect(results[1].status).toBe("skipped");
    expect(results[2].status).toBe("skipped");
  });

  it("preserves updated timestamp on archive", () => {
    writeTask(tempDir, "TASK-001-test.md", {
      id: "TASK-001",
      title: "Test",
      status: "done",
    });

    const index = minimalIndex({
      next_id: 2,
      tasks: {
        "TASK-001": taskEntry({
          status: "done",
          updated: "2026-03-10T08:00:00Z",
          file: "TASK-001-test.md",
        }),
      },
    });

    archiveTasks(tempDir, index, ["TASK-001"]);

    expect(index.tasks["TASK-001"].updated).toBe("2026-03-10T08:00:00Z");
  });

  it("does not mutate index for skipped tasks", () => {
    const index = minimalIndex({
      next_id: 2,
      tasks: {
        "TASK-001": taskEntry({
          status: "in_progress",
          file: "TASK-001-test.md",
          updated: "2026-01-01T00:00:00Z",
        }),
      },
    });

    archiveTasks(tempDir, index, ["TASK-001"]);

    expect(index.tasks["TASK-001"].file).toBe("TASK-001-test.md");
    expect(index.tasks["TASK-001"].updated).toBe("2026-01-01T00:00:00Z");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// archiveSpecs
// ─────────────────────────────────────────────────────────────────────────────

describe("archiveSpecs", () => {
  it("archives a complete spec — moves file and updates index entry", () => {
    writeSpec(tempDir, "SPEC-001-test.md", {
      id: "SPEC-001",
      title: "Test spec",
      status: "complete",
    });

    const index = minimalIndex({
      specs: {
        "SPEC-001": specEntry({
          status: "complete",
          file: "SPEC-001-test.md",
        }),
      },
    });

    const results = archiveSpecs(tempDir, index, ["SPEC-001"]);

    expect(results).toHaveLength(1);
    expect(results[0]).toEqual({ id: "SPEC-001", status: "archived" });
    expect(existsSync(join(tempDir, "specs", "SPEC-001-test.md"))).toBe(false);
    expect(existsSync(join(tempDir, "specs", "archive", "SPEC-001-test.md"))).toBe(true);
    expect(index.specs["SPEC-001"].file).toBe("archive/SPEC-001-test.md");
  });

  it("rejects a spec that is not complete", () => {
    writeSpec(tempDir, "SPEC-001-test.md", {
      id: "SPEC-001",
      title: "Drafting",
      status: "drafting",
    });

    const index = minimalIndex({
      specs: {
        "SPEC-001": specEntry({ status: "drafting", file: "SPEC-001-test.md" }),
      },
    });

    const results = archiveSpecs(tempDir, index, ["SPEC-001"]);

    expect(results).toHaveLength(1);
    expect(results[0].status).toBe("skipped");
    expect(results[0].reason).toContain("must be complete");
    expect(existsSync(join(tempDir, "specs", "SPEC-001-test.md"))).toBe(true);
  });

  it("skips a spec already in archive", () => {
    const index = minimalIndex({
      specs: {
        "SPEC-001": specEntry({
          status: "complete",
          file: "archive/SPEC-001-test.md",
        }),
      },
    });

    const results = archiveSpecs(tempDir, index, ["SPEC-001"]);

    expect(results).toHaveLength(1);
    expect(results[0].status).toBe("skipped");
    expect(results[0].reason).toBe("already archived");
  });

  it("skips a spec not found in index", () => {
    const index = minimalIndex();

    const results = archiveSpecs(tempDir, index, ["SPEC-999"]);

    expect(results).toHaveLength(1);
    expect(results[0].status).toBe("skipped");
    expect(results[0].reason).toBe("not found in index");
  });

  it("skips when destination already exists", () => {
    writeSpec(tempDir, "SPEC-001-test.md", {
      id: "SPEC-001",
      title: "Test",
      status: "complete",
    });

    const archiveDir = join(tempDir, "specs", "archive");
    mkdirSync(archiveDir, { recursive: true });
    writeFileSync(join(archiveDir, "SPEC-001-test.md"), "existing file", "utf-8");

    const index = minimalIndex({
      specs: {
        "SPEC-001": specEntry({ status: "complete", file: "SPEC-001-test.md" }),
      },
    });

    const results = archiveSpecs(tempDir, index, ["SPEC-001"]);

    expect(results).toHaveLength(1);
    expect(results[0].status).toBe("skipped");
    expect(results[0].reason).toContain("already exists");
    expect(existsSync(join(tempDir, "specs", "SPEC-001-test.md"))).toBe(true);
  });

  it("archives multiple specs in one call", () => {
    writeSpec(tempDir, "SPEC-001-first.md", {
      id: "SPEC-001",
      title: "First",
      status: "complete",
    });
    writeSpec(tempDir, "SPEC-002-second.md", {
      id: "SPEC-002",
      title: "Second",
      status: "complete",
    });

    const index = minimalIndex({
      specs: {
        "SPEC-001": specEntry({ title: "First", status: "complete", file: "SPEC-001-first.md" }),
        "SPEC-002": specEntry({ title: "Second", status: "complete", file: "SPEC-002-second.md" }),
      },
    });

    const results = archiveSpecs(tempDir, index, ["SPEC-001", "SPEC-002"]);

    const archived = results.filter((r) => r.status === "archived");
    expect(archived).toHaveLength(2);

    expect(index.specs["SPEC-001"].file).toBe("archive/SPEC-001-first.md");
    expect(index.specs["SPEC-002"].file).toBe("archive/SPEC-002-second.md");
  });

  it("does not mutate index for skipped specs", () => {
    const index = minimalIndex({
      specs: {
        "SPEC-001": specEntry({
          status: "drafting",
          file: "SPEC-001-test.md",
        }),
      },
    });

    archiveSpecs(tempDir, index, ["SPEC-001"]);

    expect(index.specs["SPEC-001"].file).toBe("SPEC-001-test.md");
  });
});
