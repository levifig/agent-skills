/**
 * Parser Tests
 *
 * Tests for parseTaskFile() and parseSpecFile() — the public API of parser.ts.
 */

import { describe, it, expect } from "vitest";
import { parseTaskFile, parseSpecFile } from "./parser.js";

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

/** Build a frontmatter YAML string from key-value pairs */
function fm(fields: Record<string, unknown>): string {
  const lines = Object.entries(fields).map(([k, v]) => {
    if (Array.isArray(v)) {
      if (v.length === 0) return `${k}: []`;
      return `${k}:\n${v.map((item) => `  - ${item}`).join("\n")}`;
    }
    if (v === null || v === undefined) return null;
    return `${k}: ${typeof v === "string" && v.includes(":") ? `"${v}"` : v}`;
  });
  const body = lines.filter(Boolean).join("\n");
  return `---\n${body}\n---\n\n# Task body\n`;
}

function expectRecentTimestamp(value: string): void {
  const parsed = new Date(value).getTime();
  const now = Date.now();
  expect(parsed).toBeGreaterThan(now - 5000);
  expect(parsed).toBeLessThanOrEqual(now + 1000);
}

// ─────────────────────────────────────────────────────────────────────────────
// parseTaskFile — Status Normalization
// ─────────────────────────────────────────────────────────────────────────────

describe("parseTaskFile", () => {
  describe("status normalization", () => {
    const validStatuses = ["todo", "in_progress", "blocked", "review", "done"] as const;

    for (const status of validStatuses) {
      it(`passes through valid status "${status}"`, () => {
        const content = fm({ id: "TASK-001", title: "Test", status });
        const result = parseTaskFile("TASK-001-test.md", content);
        expect(result).not.toBeNull();
        expect(result!.entry.status).toBe(status);
      });
    }

    const aliases: Array<[string, string]> = [
      ["complete", "done"],
      ["completed", "done"],
      ["archived", "done"],
      ["in-progress", "in_progress"],
      ["in progress", "in_progress"],
      ["wip", "in_progress"],
      ["pending", "todo"],
      ["waiting", "blocked"],
    ];

    for (const [alias, expected] of aliases) {
      it(`normalizes alias "${alias}" to "${expected}"`, () => {
        const content = fm({ id: "TASK-001", title: "Test", status: alias });
        const result = parseTaskFile("TASK-001-test.md", content);
        expect(result).not.toBeNull();
        expect(result!.entry.status).toBe(expected);
      });
    }

    const caseVariants: Array<[string, string]> = [
      ["TODO", "todo"],
      ["In_Progress", "in_progress"],
      ["DONE", "done"],
      ["Blocked", "blocked"],
      ["REVIEW", "review"],
      ["WIP", "in_progress"],
      ["Complete", "done"],
    ];

    for (const [variant, expected] of caseVariants) {
      it(`handles case-insensitive status "${variant}" -> "${expected}"`, () => {
        const content = fm({ id: "TASK-001", title: "Test", status: variant });
        const result = parseTaskFile("TASK-001-test.md", content);
        expect(result).not.toBeNull();
        expect(result!.entry.status).toBe(expected);
      });
    }

    it("defaults to 'todo' when status is missing", () => {
      const content = fm({ id: "TASK-001", title: "Test" });
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.status).toBe("todo");
    });

    it("defaults to 'todo' for unrecognized status values", () => {
      const content = fm({ id: "TASK-001", title: "Test", status: "banana" });
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.status).toBe("todo");
    });
  });

  describe("priority normalization", () => {
    const validPriorities = ["P0", "P1", "P2", "P3"] as const;

    for (const priority of validPriorities) {
      it(`passes through valid priority "${priority}"`, () => {
        const content = fm({ id: "TASK-001", title: "Test", priority });
        const result = parseTaskFile("TASK-001-test.md", content);
        expect(result).not.toBeNull();
        expect(result!.entry.priority).toBe(priority);
      });
    }

    const caseVariants: Array<[string, string]> = [
      ["p0", "P0"],
      ["p1", "P1"],
      ["p2", "P2"],
      ["p3", "P3"],
    ];

    for (const [variant, expected] of caseVariants) {
      it(`handles case-insensitive priority "${variant}" -> "${expected}"`, () => {
        const content = fm({ id: "TASK-001", title: "Test", priority: variant });
        const result = parseTaskFile("TASK-001-test.md", content);
        expect(result).not.toBeNull();
        expect(result!.entry.priority).toBe(expected);
      });
    }

    it("defaults to 'P2' when priority is missing", () => {
      const content = fm({ id: "TASK-001", title: "Test" });
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.priority).toBe("P2");
    });

    it("defaults to 'P2' for invalid priority values", () => {
      const content = fm({ id: "TASK-001", title: "Test", priority: "HIGH" });
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.priority).toBe("P2");
    });

    it("defaults to 'P2' for out-of-range priority", () => {
      const content = fm({ id: "TASK-001", title: "Test", priority: "P9" });
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.priority).toBe("P2");
    });
  });

  describe("date normalization", () => {
    it("preserves ISO timestamp strings (containing T)", () => {
      const iso = "2026-03-15T10:30:00.000Z";
      const content = `---\nid: TASK-001\ntitle: Test\ncreated: "${iso}"\n---\n`;
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.created).toBe(iso);
    });

    it("converts bare date to midnight UTC", () => {
      // Quoted to prevent gray-matter from auto-parsing into a Date object
      const content = `---\nid: TASK-001\ntitle: Test\ncreated: "2026-03-15"\n---\n`;
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.created).toBe("2026-03-15T00:00:00Z");
    });

    it("defaults to a recent timestamp when created is missing", () => {
      const content = fm({ id: "TASK-001", title: "Test" });
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expectRecentTimestamp(result!.entry.created);
    });

    it("handles Date objects from gray-matter auto-parsing", () => {
      // Unquoted YAML date — gray-matter parses this into a Date object
      const content = `---\nid: TASK-001\ntitle: Test\ncreated: 2026-03-15\n---\n`;
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.created).toMatch(/2026-03-1[45]T/);
    });
  });

  describe("full frontmatter parsing", () => {
    it("parses all fields from complete frontmatter", () => {
      const content = `---
id: TASK-019
title: Intelligent resume
spec: SPEC-002
status: in_progress
priority: P1
created: "2026-03-10T08:00:00Z"
updated: "2026-03-14T12:00:00Z"
depends_on:
  - TASK-017
  - TASK-018
files:
  - cli/index.ts
  - cli/lib/tasks/parser.ts
verify: npm run test
done: All tests pass
session: session-2026-03-14.md
---

# TASK-019: Intelligent resume
`;
      const result = parseTaskFile("TASK-019-intelligent-resume.md", content);

      expect(result).not.toBeNull();
      expect(result!.id).toBe("TASK-019");
      expect(result!.entry.title).toBe("Intelligent resume");
      expect(result!.entry.slug).toBe("intelligent-resume");
      expect(result!.entry.spec).toBe("SPEC-002");
      expect(result!.entry.status).toBe("in_progress");
      expect(result!.entry.priority).toBe("P1");
      expect(result!.entry.created).toBe("2026-03-10T08:00:00Z");
      expect(result!.entry.updated).toBe("2026-03-14T12:00:00Z");
      expect(result!.entry.depends_on).toEqual(["TASK-017", "TASK-018"]);
      expect(result!.entry.files).toEqual(["cli/index.ts", "cli/lib/tasks/parser.ts"]);
      expect(result!.entry.verify).toBe("npm run test");
      expect(result!.entry.done).toBe("All tests pass");
      expect(result!.entry.session).toBe("session-2026-03-14.md");
      expect(result!.entry.completed_at).toBeNull(); // not "done" status
      expect(result!.entry.file).toBe("TASK-019-intelligent-resume.md");
    });

    it("applies defaults for minimal frontmatter", () => {
      const content = fm({ id: "TASK-042", title: "Minimal task" });
      const result = parseTaskFile("TASK-042-minimal-task.md", content);

      expect(result).not.toBeNull();
      expect(result!.id).toBe("TASK-042");
      expect(result!.entry.title).toBe("Minimal task");
      expect(result!.entry.slug).toBe("minimal-task");
      expect(result!.entry.spec).toBeNull();
      expect(result!.entry.status).toBe("todo");
      expect(result!.entry.priority).toBe("P2");
      expect(result!.entry.depends_on).toEqual([]);
      expect(result!.entry.files).toEqual([]);
      expect(result!.entry.verify).toBeNull();
      expect(result!.entry.done).toBeNull();
      expect(result!.entry.session).toBeNull();
      expect(result!.entry.completed_at).toBeNull();
      expectRecentTimestamp(result!.entry.created);
      expectRecentTimestamp(result!.entry.updated);
    });
  });

  describe("ID resolution", () => {
    it("falls back to filename ID when frontmatter has no id field", () => {
      const content = `---\ntitle: Some task\n---\n`;
      const result = parseTaskFile("TASK-042-something.md", content);
      expect(result).not.toBeNull();
      expect(result!.id).toBe("TASK-042");
    });

    it("prefers frontmatter id over filename id", () => {
      const content = fm({ id: "TASK-099", title: "Override" });
      const result = parseTaskFile("TASK-001-wrong.md", content);
      expect(result).not.toBeNull();
      expect(result!.id).toBe("TASK-099");
    });

    it("returns null when neither frontmatter nor filename provide an ID", () => {
      const content = `---\ntitle: No ID\n---\n`;
      const result = parseTaskFile("random-file.md", content);
      expect(result).toBeNull();
    });
  });

  describe("slug from filename", () => {
    it("extracts slug from standard filename", () => {
      const content = fm({ id: "TASK-019", title: "Test" });
      const result = parseTaskFile("TASK-019-intelligent-resume.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.slug).toBe("intelligent-resume");
    });

    it("returns empty slug for bare task filename", () => {
      const content = fm({ id: "TASK-019", title: "Test" });
      const result = parseTaskFile("TASK-019.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.slug).toBe("");
    });

    it("uses full basename as slug for non-matching filename", () => {
      const content = fm({ id: "TASK-019", title: "Test" });
      const result = parseTaskFile("random-file.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.slug).toBe("random-file");
    });

    it("handles path with directory components", () => {
      const content = fm({ id: "TASK-019", title: "Test" });
      const result = parseTaskFile("/some/path/TASK-019-my-slug.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.slug).toBe("my-slug");
    });
  });

  describe("completed_at", () => {
    it("auto-sets completed_at when status is 'done'", () => {
      const content = `---\nid: TASK-001\ntitle: Test\nstatus: done\ncreated: "2026-03-10T08:00:00Z"\n---\n`;
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.completed_at).not.toBeNull();
    });

    it("uses explicit completed_at when status is 'done' and field is set", () => {
      const content = `---
id: TASK-001
title: Test
status: done
completed_at: "2026-03-12T15:00:00Z"
---
`;
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.completed_at).toBe("2026-03-12T15:00:00Z");
    });

    it("sets completed_at to null when status is not 'done'", () => {
      const content = fm({ id: "TASK-001", title: "Test", status: "todo" });
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.completed_at).toBeNull();
    });

    it("sets completed_at to null when status is 'in_progress'", () => {
      const content = fm({ id: "TASK-001", title: "Test", status: "in_progress" });
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.completed_at).toBeNull();
    });
  });

  describe("array fields", () => {
    it("passes through depends_on array", () => {
      const content = `---
id: TASK-001
title: Test
depends_on:
  - TASK-002
  - TASK-003
---
`;
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.depends_on).toEqual(["TASK-002", "TASK-003"]);
    });

    it("defaults depends_on to empty array when missing", () => {
      const content = fm({ id: "TASK-001", title: "Test" });
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.depends_on).toEqual([]);
    });

    it("defaults depends_on to empty array when not an array", () => {
      const content = `---\nid: TASK-001\ntitle: Test\ndepends_on: TASK-002\n---\n`;
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.depends_on).toEqual([]);
    });

    it("passes through files array", () => {
      const content = `---
id: TASK-001
title: Test
files:
  - src/main.ts
  - src/utils.ts
---
`;
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.files).toEqual(["src/main.ts", "src/utils.ts"]);
    });

    it("defaults files to empty array when not an array", () => {
      const content = `---\nid: TASK-001\ntitle: Test\nfiles: main.ts\n---\n`;
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.files).toEqual([]);
    });
  });

  describe("edge cases", () => {
    it("returns null for empty string content with non-matching filename", () => {
      const result = parseTaskFile("random-file.md", "");
      expect(result).toBeNull();
    });

    it("returns null for content with no frontmatter delimiters and non-matching filename", () => {
      const result = parseTaskFile("random-file.md", "Just some text\nwithout frontmatter");
      expect(result).toBeNull();
    });

    it("handles empty frontmatter with ID from filename", () => {
      const content = `---\n---\nBody only\n`;
      const result = parseTaskFile("TASK-055-empty-fm.md", content);
      expect(result).not.toBeNull();
      expect(result!.id).toBe("TASK-055");
      expect(result!.entry.title).toBe("TASK-055-empty-fm");
    });

    it("handles extremely long title", () => {
      const longTitle = "A".repeat(500);
      const content = fm({ id: "TASK-001", title: longTitle });
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.title).toBe(longTitle);
    });

    it("handles special characters in title", () => {
      const title = "Fix: <script> injection & 'SQL' \"bugs\" (urgent!)";
      const content = `---\nid: TASK-001\ntitle: "Fix: <script> injection & 'SQL' \\"bugs\\" (urgent!)"\n---\n`;
      const result = parseTaskFile("TASK-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.title).toBe(title);
    });

    it("uses filename as title when frontmatter title is missing", () => {
      const content = `---\nid: TASK-001\n---\n`;
      const result = parseTaskFile("TASK-001-my-task.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.title).toBe("TASK-001-my-task");
    });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// parseSpecFile
// ─────────────────────────────────────────────────────────────────────────────

describe("parseSpecFile", () => {
  describe("spec status normalization", () => {
    const validStatuses = ["drafting", "approved", "implementing", "complete"] as const;

    for (const status of validStatuses) {
      it(`passes through valid status "${status}"`, () => {
        const content = fm({ id: "SPEC-001", title: "Test", status });
        const result = parseSpecFile("SPEC-001-test.md", content);
        expect(result).not.toBeNull();
        expect(result!.entry.status).toBe(status);
      });
    }

    const aliases: Array<[string, string]> = [
      ["draft", "drafting"],
      ["done", "complete"],
      ["completed", "complete"],
      ["archived", "complete"],
      ["implemented", "complete"],
      ["in-progress", "implementing"],
      ["in_progress", "implementing"],
    ];

    for (const [alias, expected] of aliases) {
      it(`normalizes alias "${alias}" to "${expected}"`, () => {
        const content = fm({ id: "SPEC-001", title: "Test", status: alias });
        const result = parseSpecFile("SPEC-001-test.md", content);
        expect(result).not.toBeNull();
        expect(result!.entry.status).toBe(expected);
      });
    }

    it("defaults to 'drafting' when status is missing", () => {
      const content = fm({ id: "SPEC-001", title: "Test" });
      const result = parseSpecFile("SPEC-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.status).toBe("drafting");
    });

    it("defaults to 'drafting' for unrecognized status values", () => {
      const content = fm({ id: "SPEC-001", title: "Test", status: "unknown" });
      const result = parseSpecFile("SPEC-001-test.md", content);
      expect(result).not.toBeNull();
      expect(result!.entry.status).toBe("drafting");
    });
  });

  describe("full spec parsing", () => {
    it("parses all fields from complete frontmatter", () => {
      const content = `---
id: SPEC-010
title: Task management CLI
status: implementing
requirement: Must support CRUD operations
source: direct
created: "2026-03-01T09:00:00Z"
---

# SPEC-010: Task management CLI
`;
      const result = parseSpecFile("SPEC-010-task-management-cli.md", content);

      expect(result).not.toBeNull();
      expect(result!.id).toBe("SPEC-010");
      expect(result!.entry.title).toBe("Task management CLI");
      expect(result!.entry.status).toBe("implementing");
      expect(result!.entry.requirement).toBe("Must support CRUD operations");
      expect(result!.entry.source).toBe("direct");
      expect(result!.entry.created).toBe("2026-03-01T09:00:00Z");
      expect(result!.entry.file).toBe("SPEC-010-task-management-cli.md");
    });

    it("applies defaults for minimal spec frontmatter", () => {
      const content = fm({ id: "SPEC-005" });
      const result = parseSpecFile("SPEC-005-minimal.md", content);

      expect(result).not.toBeNull();
      expect(result!.id).toBe("SPEC-005");
      expect(result!.entry.status).toBe("drafting");
      expect(result!.entry.requirement).toBeNull();
      expect(result!.entry.source).toBeNull();
      expectRecentTimestamp(result!.entry.created);
    });
  });

  describe("ID resolution", () => {
    it("falls back to filename ID when frontmatter has no id field", () => {
      const content = `---\ntitle: Some spec\n---\n`;
      const result = parseSpecFile("SPEC-010-something.md", content);
      expect(result).not.toBeNull();
      expect(result!.id).toBe("SPEC-010");
    });

    it("prefers frontmatter id over filename id", () => {
      const content = fm({ id: "SPEC-099", title: "Override" });
      const result = parseSpecFile("SPEC-001-wrong.md", content);
      expect(result).not.toBeNull();
      expect(result!.id).toBe("SPEC-099");
    });

    it("returns null when neither frontmatter nor filename provide an ID", () => {
      const content = `---\ntitle: No ID\n---\n`;
      const result = parseSpecFile("random-file.md", content);
      expect(result).toBeNull();
    });
  });

  describe("edge cases", () => {
    it("returns null for empty content with non-matching filename", () => {
      const result = parseSpecFile("random.md", "");
      expect(result).toBeNull();
    });

    it("handles empty frontmatter with ID from filename", () => {
      const content = `---\n---\nBody only\n`;
      const result = parseSpecFile("SPEC-020-empty.md", content);
      expect(result).not.toBeNull();
      expect(result!.id).toBe("SPEC-020");
    });

    it("extracts ID from filename with extra suffix", () => {
      const content = fm({ title: "Test" });
      const result = parseSpecFile("SPEC-010-task-management-cli.md", content);
      expect(result).not.toBeNull();
      expect(result!.id).toBe("SPEC-010");
    });
  });
});
