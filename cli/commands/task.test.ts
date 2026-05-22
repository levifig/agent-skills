/**
 * Regression tests for `rebuildTaskIndex` (PR #49 Codex HOLD).
 *
 * The defect: the original SPEC-036 refactor replaced `current.tasks` and
 * `current.specs` with the pre-scan snapshot from disk. If a concurrent
 * `task create` committed a task DURING our outside scan, the fresh entry
 * could be in the live index even though the scan did not observe its .md
 * file, and it would be silently dropped at the in-lock swap.
 *
 * The fix: scan-window-aware union merge.
 *
 *   - Scanned (on-disk) entries are authoritative.
 *   - PLUS entries minted DURING the scan window (preserved even if the
 *     outside scan did not observe their .md file).
 *   - DROP pre-scan entries that aren't on disk (the whole point of
 *     `refresh` is to reconcile with disk truth).
 */

import { describe, it, expect, afterEach } from "vitest";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import matter from "gray-matter";

import { filterTaskIndexForList, rebuildTaskIndex } from "./task.js";
import { saveIndex } from "../lib/tasks/migrate.js";
import type { TaskIndex, TaskEntry } from "../lib/tasks/types.js";

// ─────────────────────────────────────────────────────────────────────────────
// Fixture helpers
// ─────────────────────────────────────────────────────────────────────────────

let tempDir: string;

function makeAgentsDir(): string {
  tempDir = mkdtempSync(join(tmpdir(), "loaf-task-rebuild-"));
  const agentsDir = join(tempDir, ".agents");
  mkdirSync(agentsDir, { recursive: true });
  mkdirSync(join(agentsDir, "tasks"), { recursive: true });
  return agentsDir;
}

function taskEntry(id: string, overrides: Partial<TaskEntry> = {}): TaskEntry {
  return {
    title: `Task ${id}`,
    slug: "seeded",
    spec: null,
    status: "todo",
    priority: "P2",
    depends_on: [],
    files: [],
    verify: null,
    done: null,
    session: null,
    created: "2026-05-19T00:00:00Z",
    updated: "2026-05-19T00:00:00Z",
    completed_at: null,
    file: `${id}-seeded.md`,
    ...overrides,
  };
}

function writeTaskMd(agentsDir: string, id: string): void {
  const fm = {
    id,
    title: `Task ${id}`,
    status: "todo",
    priority: "P2",
    created: "2026-05-19T00:00:00Z",
    updated: "2026-05-19T00:00:00Z",
  };
  const body = `\n# ${id}: seeded\n`;
  writeFileSync(
    join(agentsDir, "tasks", `${id}-seeded.md`),
    matter.stringify(body, fm),
    "utf-8",
  );
}

afterEach(() => {
  if (tempDir) {
    rmSync(tempDir, { recursive: true, force: true });
  }
});

// ─────────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────────

describe("rebuildTaskIndex — scan-window-aware union merge", () => {
  it("preserves an in-lock entry minted DURING the scan window (scan did not observe its .md)", () => {
    // Seed TASK-001..TASK-010 with matching .md files. We model the race
    // window by placing TASK-011 in the index with `next_id` still at 11
    // — the preservation threshold captured by the pre-scan snapshot
    // (`scanStartNextId = 11`) before a concurrent create commits:
    //
    //   - Real race: pre-scan snapshot reads next_id=11; concurrent
    //     `task create` then commits TASK-011 + sets next_id=12 inside its
    //     lock; our scan does not observe TASK-011.md; our in-lock read sees
    //     next_id=12 with TASK-011 present.
    //   - In-process model: snapshot and in-lock read see the same file
    //     content because there's no concurrent writer. Setting next_id=11
    //     ensures `scanStartNextId = 11`, putting TASK-011 (parsed id=11)
    //     INSIDE the preservation window (id >= 11). This is the load-
    //     bearing condition: a defective implementation that replaces
    //     `current.tasks` with `scannedIndex.tasks` will drop TASK-011
    //     even though it should be preserved.
    const agentsDir = makeAgentsDir();
    const seedIndex: TaskIndex = {
      version: 1,
      next_id: 11,
      tasks: {},
      specs: {},
    };
    for (let i = 1; i <= 10; i++) {
      const id = `TASK-${String(i).padStart(3, "0")}`;
      seedIndex.tasks[id] = taskEntry(id);
      writeTaskMd(agentsDir, id);
    }
    // TASK-011 is in TASKS.json with NO matching .md in this in-process
    // model — the freshly-minted entry the outside scan did not observe.
    seedIndex.tasks["TASK-011"] = taskEntry("TASK-011");
    saveIndex(join(agentsDir, "TASKS.json"), seedIndex);

    const result = rebuildTaskIndex(agentsDir);

    // Defect would have dropped TASK-011 here (the in-lock swap replaces
    // current.tasks with the disk scan, which never saw TASK-011.md).
    expect(result.tasks["TASK-011"]).toBeDefined();
    expect(result.tasks["TASK-011"].title).toBe("Task TASK-011");
    // next_id stays monotonic: at least max(parsed-id)+1 = 12.
    expect(result.next_id).toBeGreaterThanOrEqual(12);
    // All seeded tasks still present.
    for (let i = 1; i <= 10; i++) {
      const id = `TASK-${String(i).padStart(3, "0")}`;
      expect(result.tasks[id]).toBeDefined();
    }
  });

  it("DROPS a pre-scan entry that exists in TASKS.json but has no .md file (the legitimate cleanup case)", () => {
    // Seed TASK-001..TASK-010 with matching .md files. Plus seed a STALE
    // TASK-005-bogus entry into TASKS.json that has no corresponding .md
    // and an ID below scanStartNextId — the cleanup case.
    const agentsDir = makeAgentsDir();
    const seedIndex: TaskIndex = {
      version: 1,
      next_id: 50,
      tasks: {},
      specs: {},
    };
    for (let i = 1; i <= 10; i++) {
      const id = `TASK-${String(i).padStart(3, "0")}`;
      seedIndex.tasks[id] = taskEntry(id);
      writeTaskMd(agentsDir, id);
    }
    // Stale entry: ID < scanStartNextId (50), no .md file.
    seedIndex.tasks["TASK-042"] = taskEntry("TASK-042", { title: "Stale" });
    saveIndex(join(agentsDir, "TASKS.json"), seedIndex);

    const result = rebuildTaskIndex(agentsDir);

    // Window semantics: TASK-042 was in the pre-scan snapshot AND has
    // id-number < scanStartNextId (=50). The scan didn't find its .md, so
    // it should be dropped.
    expect(result.tasks["TASK-042"]).toBeUndefined();
    // The 10 real tasks survive.
    expect(Object.keys(result.tasks).length).toBe(10);
  });

  it("DROPS an index-only task immediately below next_id (committed before the snapshot, no .md)", () => {
    // This is the real stale shape that previously looked indistinguishable
    // from a pending create: TASK-011 is already below scanStartNextId (=12).
    // `task create` now creates the .md before the locked index commit, so an
    // index-only entry in this state is not a legitimate fresh task.
    const agentsDir = makeAgentsDir();
    const seedIndex: TaskIndex = {
      version: 1,
      next_id: 12,
      tasks: {},
      specs: {},
    };
    for (let i = 1; i <= 10; i++) {
      const id = `TASK-${String(i).padStart(3, "0")}`;
      seedIndex.tasks[id] = taskEntry(id);
      writeTaskMd(agentsDir, id);
    }
    seedIndex.tasks["TASK-011"] = taskEntry("TASK-011", { title: "Index only" });
    saveIndex(join(agentsDir, "TASKS.json"), seedIndex);

    const result = rebuildTaskIndex(agentsDir);

    expect(result.tasks["TASK-011"]).toBeUndefined();
    expect(result.next_id).toBeGreaterThanOrEqual(12);
  });

  it("DROPS malformed index-only task IDs instead of treating them as fresh", () => {
    const agentsDir = makeAgentsDir();
    const seedIndex: TaskIndex = {
      version: 1,
      next_id: 1,
      tasks: {
        "TASK-ABC": taskEntry("TASK-ABC", { title: "Malformed" }),
        "TASK-12A": taskEntry("TASK-12A", { title: "Malformed suffix" }),
      },
      specs: {},
    };
    saveIndex(join(agentsDir, "TASKS.json"), seedIndex);

    const result = rebuildTaskIndex(agentsDir);

    expect(result.tasks["TASK-ABC"]).toBeUndefined();
    expect(result.tasks["TASK-12A"]).toBeUndefined();
    expect(result.next_id).toBe(1);
  });

  it("monotonic next_id across scan, snapshot, and merged max id", () => {
    const agentsDir = makeAgentsDir();
    const seedIndex: TaskIndex = {
      version: 1,
      next_id: 200,
      tasks: {},
      specs: {},
    };
    for (let i = 1; i <= 5; i++) {
      const id = `TASK-${String(i).padStart(3, "0")}`;
      seedIndex.tasks[id] = taskEntry(id);
      writeTaskMd(agentsDir, id);
    }
    saveIndex(join(agentsDir, "TASKS.json"), seedIndex);

    const result = rebuildTaskIndex(agentsDir);

    // Even though only TASK-001..TASK-005 exist on disk, next_id must not
    // roll back below the pre-existing 200 (a concurrent allocator already
    // moved past us).
    expect(result.next_id).toBeGreaterThanOrEqual(200);
  });
});

describe("filterTaskIndexForList", () => {
  function indexWithStatuses(): TaskIndex {
    return {
      version: 1,
      next_id: 5,
      specs: {},
      tasks: {
        "TASK-001": taskEntry("TASK-001", { status: "todo", spec: "SPEC-001" }),
        "TASK-002": taskEntry("TASK-002", { status: "in_progress" }),
        "TASK-003": taskEntry("TASK-003", { status: "done" }),
        "TASK-004": taskEntry("TASK-004", { status: "blocked" }),
      },
    };
  }

  it("filters to a single requested status", () => {
    const result = filterTaskIndexForList(indexWithStatuses(), { status: "in_progress" });

    expect(Object.keys(result.tasks)).toEqual(["TASK-002"]);
    expect(result.next_id).toBe(5);
  });

  it("combines --active with --status", () => {
    const activeDone = filterTaskIndexForList(indexWithStatuses(), {
      active: true,
      status: "done",
    });
    const activeTodo = filterTaskIndexForList(indexWithStatuses(), {
      active: true,
      status: "todo",
    });

    expect(Object.keys(activeDone.tasks)).toEqual([]);
    expect(Object.keys(activeTodo.tasks)).toEqual(["TASK-001"]);
  });
});
