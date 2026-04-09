/**
 * Report Command Tests
 *
 * Tests for the `loaf report` command — durable report management.
 *
 * @vitest-environment node
 * @vitest-run-sequential
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import {
  mkdirSync,
  mkdtempSync,
  realpathSync,
  writeFileSync,
  readFileSync,
  existsSync,
  rmSync,
  readdirSync,
} from "fs";
import { join } from "path";
import { tmpdir } from "os";
import { execFileSync } from "child_process";
import matter from "gray-matter";

// ─────────────────────────────────────────────────────────────────────────────
// Test Fixtures — unique per test to avoid cross-file interference
// ─────────────────────────────────────────────────────────────────────────────

let TEST_ROOT: string;
let AGENTS_DIR: string;
let REPORTS_DIR: string;
const CLI_PATH = join(process.cwd(), "dist-cli", "index.js");

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

function runReport(
  args: string[],
  options: { expectError?: boolean } = {}
): { exitCode: number; stdout: string; stderr: string } {
  try {
    const result = execFileSync("node", [CLI_PATH, "report", ...args], {
      cwd: TEST_ROOT,
      encoding: "utf-8",
      timeout: 30000,
      stdio: ["pipe", "pipe", "pipe"],
    });
    return { exitCode: 0, stdout: result, stderr: "" };
  } catch (error: unknown) {
    const err = error as {
      status?: number;
      stdout?: string;
      stderr?: string;
    };
    if (!options.expectError) {
      // Useful for debugging test failures
      console.error("Unexpected error:", err.stderr || err.stdout);
    }
    return {
      exitCode: err.status || 1,
      stdout: err.stdout || "",
      stderr: err.stderr || "",
    };
  }
}

/** Create a report file directly for test setup */
function createTestReport(
  filename: string,
  overrides: Partial<{
    title: string;
    type: string;
    status: string;
    source: string;
    created: string;
    tags: string[];
    finalized_at: string;
  }> = {}
): string {
  const data = {
    title: overrides.title || "Test Report",
    type: overrides.type || "research",
    created: overrides.created || new Date().toISOString(),
    status: overrides.status || "draft",
    source: overrides.source || "ad-hoc",
    tags: overrides.tags || [],
    ...(overrides.finalized_at ? { finalized_at: overrides.finalized_at } : {}),
  };

  const body = `# ${data.title}\n\n## Summary\n\nTest content.\n`;
  const content = matter.stringify(body, data);
  const filePath = join(REPORTS_DIR, filename);
  writeFileSync(filePath, content, "utf-8");
  return filePath;
}

// ─────────────────────────────────────────────────────────────────────────────
// Setup / Teardown
// ─────────────────────────────────────────────────────────────────────────────

beforeEach(() => {
  TEST_ROOT = realpathSync(mkdtempSync(join(tmpdir(), "loaf-test-report-")));
  AGENTS_DIR = join(TEST_ROOT, ".agents");
  REPORTS_DIR = join(AGENTS_DIR, "reports");
  mkdirSync(REPORTS_DIR, { recursive: true });
});

afterEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
});

// ─────────────────────────────────────────────────────────────────────────────
// Tests: list
// ─────────────────────────────────────────────────────────────────────────────

describe("report list", () => {
  it("shows empty message when no reports exist", () => {
    const result = runReport(["list"]);
    expect(result.exitCode).toBe(0);
    const clean = result.stdout.replace(/\x1b\[\d+m/g, "");
    expect(clean).toContain("No reports found");
  });

  it("shows reports grouped by status", () => {
    createTestReport("20260407-100000-research-alpha.md", {
      title: "Alpha Research",
      status: "draft",
    });
    createTestReport("20260407-100001-research-beta.md", {
      title: "Beta Research",
      status: "final",
    });

    const result = runReport(["list"]);
    expect(result.exitCode).toBe(0);
    const clean = result.stdout.replace(/\x1b\[\d+m/g, "");
    expect(clean).toContain("Drafts");
    expect(clean).toContain("Alpha Research");
    expect(clean).toContain("Final");
    expect(clean).toContain("Beta Research");
  });

  it("filters by --type", () => {
    createTestReport("20260407-100000-research-alpha.md", {
      title: "Alpha Research",
      type: "research",
    });
    createTestReport("20260407-100001-audit-beta.md", {
      title: "Beta Audit",
      type: "audit",
    });

    const result = runReport(["list", "--type", "audit"]);
    expect(result.exitCode).toBe(0);
    const clean = result.stdout.replace(/\x1b\[\d+m/g, "");
    expect(clean).toContain("Beta Audit");
    expect(clean).not.toContain("Alpha Research");
  });

  it("outputs JSON with --json flag", () => {
    createTestReport("20260407-100000-research-alpha.md", {
      title: "Alpha Research",
    });

    const result = runReport(["list", "--json"]);
    expect(result.exitCode).toBe(0);
    const parsed = JSON.parse(result.stdout);
    expect(Array.isArray(parsed)).toBe(true);
    expect(parsed.length).toBe(1);
    expect(parsed[0].data.title).toBe("Alpha Research");
  });

  it("returns empty JSON array when no reports exist with --json", () => {
    // Remove reports dir so it truly has nothing
    rmSync(REPORTS_DIR, { recursive: true, force: true });

    const result = runReport(["list", "--json"]);
    expect(result.exitCode).toBe(0);
    const parsed = JSON.parse(result.stdout);
    expect(parsed).toEqual([]);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Tests: create
// ─────────────────────────────────────────────────────────────────────────────

describe("report create", () => {
  it("scaffolds a report with correct frontmatter and sections", () => {
    const result = runReport(["create", "my-investigation"]);
    expect(result.exitCode).toBe(0);

    // Find the created file
    const files = readdirSync(REPORTS_DIR).filter((f) => f.endsWith(".md"));
    expect(files.length).toBe(1);
    expect(files[0]).toContain("research-my-investigation.md");

    const raw = readFileSync(join(REPORTS_DIR, files[0]), "utf-8");
    const parsed = matter(raw);

    expect(parsed.data.title).toBe("My Investigation");
    expect(parsed.data.type).toBe("research");
    expect(parsed.data.status).toBe("draft");
    expect(parsed.data.source).toBe("ad-hoc");
    expect(parsed.data.tags).toEqual([]);
    expect(parsed.data.created).toBeDefined();

    // Verify body sections
    expect(parsed.content).toContain("# My Investigation");
    expect(parsed.content).toContain("## Question");
    expect(parsed.content).toContain("## Summary");
    expect(parsed.content).toContain("## Key Findings");
    expect(parsed.content).toContain("## Methodology");
    expect(parsed.content).toContain("## Detailed Analysis");
    expect(parsed.content).toContain("## Recommendations");
    expect(parsed.content).toContain("## Sources");
    expect(parsed.content).toContain("## Open Questions");
  });

  it("respects --type option", () => {
    const result = runReport(["create", "security-review", "--type", "audit"]);
    expect(result.exitCode).toBe(0);

    const files = readdirSync(REPORTS_DIR).filter((f) => f.endsWith(".md"));
    expect(files[0]).toContain("audit-security-review.md");

    const raw = readFileSync(join(REPORTS_DIR, files[0]), "utf-8");
    const parsed = matter(raw);
    expect(parsed.data.type).toBe("audit");
  });

  it("respects --source option", () => {
    const result = runReport([
      "create",
      "api-comparison",
      "--source",
      "SPEC-042",
    ]);
    expect(result.exitCode).toBe(0);

    const files = readdirSync(REPORTS_DIR).filter((f) => f.endsWith(".md"));
    const raw = readFileSync(join(REPORTS_DIR, files[0]), "utf-8");
    const parsed = matter(raw);
    expect(parsed.data.source).toBe("SPEC-042");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Tests: finalize
// ─────────────────────────────────────────────────────────────────────────────

describe("report finalize", () => {
  it("transitions draft to final", () => {
    const filename = "20260407-100000-research-alpha.md";
    createTestReport(filename, {
      title: "Alpha Research",
      status: "draft",
    });

    const result = runReport(["finalize", filename]);
    expect(result.exitCode).toBe(0);

    const raw = readFileSync(join(REPORTS_DIR, filename), "utf-8");
    const parsed = matter(raw);
    expect(parsed.data.status).toBe("final");
    expect(parsed.data.finalized_at).toBeDefined();
  });

  it("errors when report is already final", () => {
    const filename = "20260407-100000-research-alpha.md";
    createTestReport(filename, {
      title: "Alpha Research",
      status: "final",
    });

    const result = runReport(["finalize", filename], { expectError: true });
    expect(result.exitCode).not.toBe(0);
    const clean = result.stderr.replace(/\x1b\[\d+m/g, "");
    expect(clean).toContain("not a draft");
  });

  it("resolves file by substring match", () => {
    const filename = "20260407-100000-research-alpha.md";
    createTestReport(filename, {
      title: "Alpha Research",
      status: "draft",
    });

    const result = runReport(["finalize", "alpha"]);
    expect(result.exitCode).toBe(0);

    const raw = readFileSync(join(REPORTS_DIR, filename), "utf-8");
    const parsed = matter(raw);
    expect(parsed.data.status).toBe("final");
  });

  it("errors when report is not found", () => {
    const result = runReport(["finalize", "nonexistent"], {
      expectError: true,
    });
    expect(result.exitCode).not.toBe(0);
    const clean = result.stderr.replace(/\x1b\[\d+m/g, "");
    expect(clean).toContain("not found");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Tests: archive
// ─────────────────────────────────────────────────────────────────────────────

describe("report archive", () => {
  it("moves final report to archive/", () => {
    const filename = "20260407-100000-research-alpha.md";
    createTestReport(filename, {
      title: "Alpha Research",
      status: "final",
      finalized_at: new Date().toISOString(),
    });

    const result = runReport(["archive", filename]);
    expect(result.exitCode).toBe(0);

    // Original should be gone
    expect(existsSync(join(REPORTS_DIR, filename))).toBe(false);

    // Should be in archive/
    const archivePath = join(REPORTS_DIR, "archive", filename);
    expect(existsSync(archivePath)).toBe(true);

    const raw = readFileSync(archivePath, "utf-8");
    const parsed = matter(raw);
    expect(parsed.data.status).toBe("archived");
    expect(parsed.data.archived_at).toBeDefined();
    expect(parsed.data.archived_by).toBe("cli");
  });

  it("errors when report is a draft", () => {
    const filename = "20260407-100000-research-alpha.md";
    createTestReport(filename, {
      title: "Alpha Research",
      status: "draft",
    });

    const result = runReport(["archive", filename], { expectError: true });
    expect(result.exitCode).not.toBe(0);
    const clean = result.stderr.replace(/\x1b\[\d+m/g, "");
    expect(clean).toContain("not finalized");
  });

  it("creates archive/ directory if it does not exist", () => {
    const filename = "20260407-100000-research-alpha.md";
    createTestReport(filename, {
      title: "Alpha Research",
      status: "final",
    });

    // Ensure archive/ doesn't exist
    expect(existsSync(join(REPORTS_DIR, "archive"))).toBe(false);

    const result = runReport(["archive", filename]);
    expect(result.exitCode).toBe(0);
    expect(existsSync(join(REPORTS_DIR, "archive"))).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Tests: security and edge cases (from Codex review)
// ─────────────────────────────────────────────────────────────────────────────

describe("report create sanitization", () => {
  it("sanitizes path traversal in slug", () => {
    const result = runReport(["create", "../notes"]);
    expect(result.exitCode).toBe(0);

    // File should be in reports/, not escaped out
    const files = readdirSync(REPORTS_DIR).filter((f) => f.endsWith(".md"));
    expect(files.length).toBe(1);
    expect(files[0]).not.toContain("..");
    expect(existsSync(join(REPORTS_DIR, "..", "notes"))).toBe(false);
  });

  it("sanitizes path separators in --type", () => {
    const result = runReport(["create", "test", "--type", "audit/foo"]);
    expect(result.exitCode).toBe(0);

    const files = readdirSync(REPORTS_DIR).filter((f) => f.endsWith(".md"));
    expect(files.length).toBe(1);
    expect(files[0]).not.toContain("/");
  });
});

describe("report list includes archived", () => {
  it("shows archived reports from archive/ directory", () => {
    // Create an archived report in archive/
    const archiveDir = join(REPORTS_DIR, "archive");
    mkdirSync(archiveDir, { recursive: true });

    const data = {
      title: "Old Research",
      type: "research",
      created: new Date().toISOString(),
      status: "archived",
      source: "ad-hoc",
      tags: [],
      archived_at: new Date().toISOString(),
      archived_by: "cli",
    };
    const body = "# Old Research\n\nArchived content.\n";
    writeFileSync(
      join(archiveDir, "20260101-100000-research-old.md"),
      matter.stringify(body, data),
      "utf-8"
    );

    const result = runReport(["list"]);
    expect(result.exitCode).toBe(0);
    const clean = result.stdout.replace(/\x1b\[\d+m/g, "");
    expect(clean).toContain("Old Research");
    expect(clean).toContain("Archived");
  });

  it("filters archived reports with --status archived", () => {
    // Create a draft and an archived report
    createTestReport("20260407-100000-research-active.md", {
      title: "Active Report",
      status: "draft",
    });

    const archiveDir = join(REPORTS_DIR, "archive");
    mkdirSync(archiveDir, { recursive: true });
    const data = {
      title: "Archived Report",
      type: "research",
      created: new Date().toISOString(),
      status: "archived",
      source: "ad-hoc",
      tags: [],
    };
    writeFileSync(
      join(archiveDir, "20260101-100000-research-old.md"),
      matter.stringify("# Archived\n", data),
      "utf-8"
    );

    const result = runReport(["list", "--status", "archived"]);
    expect(result.exitCode).toBe(0);
    const clean = result.stdout.replace(/\x1b\[\d+m/g, "");
    expect(clean).toContain("Archived Report");
    expect(clean).not.toContain("Active Report");
  });
});

describe("report ambiguous match", () => {
  it("errors on ambiguous substring match", () => {
    createTestReport("20260407-100000-research-alpha-one.md", {
      title: "Alpha One",
      status: "draft",
    });
    createTestReport("20260407-100001-research-alpha-two.md", {
      title: "Alpha Two",
      status: "draft",
    });

    const result = runReport(["finalize", "alpha"], { expectError: true });
    expect(result.exitCode).not.toBe(0);
    const clean = result.stderr.replace(/\x1b\[\d+m/g, "");
    expect(clean).toContain("Ambiguous");
    expect(clean).toContain("alpha-one");
    expect(clean).toContain("alpha-two");
  });
});
