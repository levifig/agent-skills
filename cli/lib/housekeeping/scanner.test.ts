import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdirSync, mkdtempSync, writeFileSync, rmSync, existsSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";

import { scanArtifacts } from "./scanner.js";
import type { ArtifactType, CleanupRecommendation } from "./types.js";

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

let tempDir: string;

function agentsDir(): string {
  return join(tempDir, ".agents");
}

function setupDir(...subpaths: string[]): string {
  const dir = join(agentsDir(), ...subpaths);
  mkdirSync(dir, { recursive: true });
  return dir;
}

function writeArtifact(subdir: string, filename: string, frontmatter: Record<string, unknown>, body = ""): void {
  const dir = setupDir(subdir);
  const lines = Object.entries(frontmatter).map(([k, v]) => `${k}: ${JSON.stringify(v)}`);
  const content = `---\n${lines.join("\n")}\n---\n\n${body}`;
  writeFileSync(join(dir, filename), content, "utf-8");
}

/** Write a file with a nested YAML block (e.g., session:, council:, report:) */
function writeNestedArtifact(subdir: string, filename: string, blockName: string, fields: Record<string, unknown>, body = ""): void {
  const dir = setupDir(subdir);
  let yaml = `${blockName}:\n`;
  for (const [k, v] of Object.entries(fields)) {
    yaml += `  ${k}: ${JSON.stringify(v)}\n`;
  }
  const content = `---\n${yaml}---\n\n${body}`;
  writeFileSync(join(dir, filename), content, "utf-8");
}

/** Write a session file with the nested session: block format used in this repo */
function writeSession(filename: string, sessionFields: Record<string, unknown>, body = ""): void {
  writeNestedArtifact("sessions", filename, "session", sessionFields, body);
}

/** Write a minimal TASKS.json */
function writeIndex(tasks: Record<string, unknown> = {}, specs: Record<string, unknown> = {}): void {
  const index = { version: 1, next_id: 100, tasks, specs };
  writeFileSync(join(agentsDir(), "TASKS.json"), JSON.stringify(index, null, 2), "utf-8");
}

function findRec(recs: CleanupRecommendation[], filename: string): CleanupRecommendation | undefined {
  return recs.find((r) => r.filename === filename);
}

beforeEach(() => {
  tempDir = mkdtempSync(join(tmpdir(), "loaf-housekeeping-test-"));
  setupDir("sessions");
  setupDir("tasks");
  setupDir("specs");
});

afterEach(() => {
  rmSync(tempDir, { recursive: true, force: true });
});

// ─────────────────────────────────────────────────────────────────────────────
// Sessions
// ─────────────────────────────────────────────────────────────────────────────

describe("sessions", () => {
  it("recommends archive for completed sessions (nested session.status)", () => {
    writeSession("SESSION-001.md", { status: "completed", created: "2026-03-01" });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "SESSION-001.md");
    expect(rec?.action).toBe("archive");
  });

  it("hints at /crystallize for sessions with learnings", () => {
    writeSession("SESSION-002.md", { status: "completed", created: "2026-03-01" }, "## Key Decisions\n- Did a thing");
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "SESSION-002.md");
    expect(rec?.action).toBe("archive");
    expect(rec?.hint).toContain("crystallize");
  });

  it("flags stale sessions (>7 days inactive)", () => {
    const staleDate = new Date(Date.now() - 10 * 24 * 60 * 60 * 1000).toISOString();
    writeSession("SESSION-003.md", { status: "active", last_updated: staleDate });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "SESSION-003.md");
    expect(rec?.action).toBe("flag");
    expect(rec?.reason).toContain("inactive");
  });

  it("skips active sessions", () => {
    writeSession("SESSION-004.md", { status: "active", last_updated: new Date().toISOString() });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "SESSION-004.md");
    expect(rec?.action).toBe("skip");
  });

  it("archives cancelled sessions", () => {
    writeSession("SESSION-005.md", { status: "cancelled", created: "2026-03-01" });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "SESSION-005.md");
    expect(rec?.action).toBe("archive");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Tasks
// ─────────────────────────────────────────────────────────────────────────────

describe("tasks", () => {
  it("recommends archive for done tasks", () => {
    writeIndex(
      {
        "TASK-001": {
          title: "Done task", slug: "done", spec: null, status: "done",
          priority: "P1", depends_on: [], files: [], verify: null, done: null,
          session: null, created: "2026-03-01", updated: "2026-03-01",
          completed_at: "2026-03-10", file: "TASK-001-done.md",
        },
      },
      {},
    );
    writeArtifact("tasks", "TASK-001-done.md", { id: "TASK-001", title: "Done task", status: "done" });
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "TASK-001-done.md");
    expect(rec?.action).toBe("archive");
  });

  it("flags tasks referencing missing specs (orphaned ref)", () => {
    writeIndex(
      {
        "TASK-002": {
          title: "Orphan ref", slug: "orphan", spec: "SPEC-099", status: "todo",
          priority: "P2", depends_on: [], files: [], verify: null, done: null,
          session: null, created: "2026-03-01", updated: "2026-03-01",
          completed_at: null, file: "TASK-002-orphan.md",
        },
      },
      {},
    );
    writeArtifact("tasks", "TASK-002-orphan.md", { id: "TASK-002", title: "Orphan ref", spec: "SPEC-099" });
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "TASK-002-orphan.md");
    expect(rec?.action).toBe("flag");
    expect(rec?.reason).toContain("SPEC-099");
  });

  it("does NOT flag tasks with spec: null (valid ad-hoc tasks)", () => {
    writeIndex(
      {
        "TASK-003": {
          title: "Ad-hoc", slug: "adhoc", spec: null, status: "todo",
          priority: "P2", depends_on: [], files: [], verify: null, done: null,
          session: null, created: "2026-03-01", updated: "2026-03-01",
          completed_at: null, file: "TASK-003-adhoc.md",
        },
      },
      {},
    );
    writeArtifact("tasks", "TASK-003-adhoc.md", { id: "TASK-003", title: "Ad-hoc" });
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "TASK-003-adhoc.md");
    expect(rec?.action).toBe("skip");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Specs
// ─────────────────────────────────────────────────────────────────────────────

describe("specs", () => {
  it("recommends archive for complete specs", () => {
    writeIndex(
      {},
      {
        "SPEC-001": {
          title: "Done spec", status: "complete", appetite: null,
          requirement: null, source: null, created: "2026-03-01",
          file: "SPEC-001-done.md",
        },
      },
    );
    writeArtifact("specs", "SPEC-001-done.md", { id: "SPEC-001", title: "Done spec", status: "complete" });
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "SPEC-001-done.md");
    expect(rec?.action).toBe("archive");
  });

  it("flags stale drafting specs (>14 days)", () => {
    const staleDate = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000).toISOString();
    writeIndex(
      {},
      {
        "SPEC-002": {
          title: "Stale spec", status: "drafting", appetite: null,
          requirement: null, source: null, created: staleDate,
          file: "SPEC-002-stale.md",
        },
      },
    );
    writeArtifact("specs", "SPEC-002-stale.md", { id: "SPEC-002", title: "Stale spec", status: "drafting" });
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "SPEC-002-stale.md");
    expect(rec?.action).toBe("flag");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Plans
// ─────────────────────────────────────────────────────────────────────────────

describe("plans", () => {
  it("recommends delete for orphaned plans (no session link)", () => {
    writeArtifact("plans", "plan-001.md", { title: "Orphan plan", created: "2026-03-01" });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "plan-001.md");
    expect(rec?.action).toBe("delete");
    expect(rec?.reason).toContain("no linked session");
  });

  it("recommends delete for plans linked to completed sessions", () => {
    writeSession("20260327-163059-spec-015-workflow-hooks.md", { status: "complete", created: "2026-03-01" });
    writeArtifact("plans", "plan-002.md", { session: "20260327-163059-spec-015", created: "2026-03-01" });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "plan-002.md");
    expect(rec?.action).toBe("delete");
  });

  it("matches session refs by stem (ID prefix without full filename)", () => {
    writeSession("20260327-181352-task-020.md", { status: "active", last_updated: new Date().toISOString() });
    // Plan uses the ID stem, not the full filename
    writeArtifact("plans", "plan-003.md", { session: "20260327-181352-task-020", updated: new Date().toISOString() });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "plan-003.md");
    expect(rec?.action).toBe("skip");
  });

  it("also matches session refs by full filename", () => {
    writeSession("SESSION-011.md", { status: "active", last_updated: new Date().toISOString() });
    writeArtifact("plans", "plan-004.md", { session: "SESSION-011.md", updated: new Date().toISOString() });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "plan-004.md");
    expect(rec?.action).toBe("skip");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Drafts
// ─────────────────────────────────────────────────────────────────────────────

describe("drafts", () => {
  it("flags stale drafts (>30 days)", () => {
    const staleDate = new Date(Date.now() - 35 * 24 * 60 * 60 * 1000).toISOString();
    writeArtifact("drafts", "draft-001.md", { title: "Old draft", created: staleDate });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "draft-001.md");
    expect(rec?.action).toBe("flag");
    expect(rec?.reason).toContain("days old");
  });

  it("notes sparks section in stale drafts", () => {
    const staleDate = new Date(Date.now() - 35 * 24 * 60 * 60 * 1000).toISOString();
    writeArtifact("drafts", "draft-002.md", { title: "Sparky draft", created: staleDate }, "## Sparks\n- Great idea");
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "draft-002.md");
    expect(rec?.hint).toContain("Sparks");
  });

  it("flags drafts promoted to a spec", () => {
    writeArtifact("drafts", "brainstorm-loaf-cli.md", { title: "Brainstorm", created: new Date().toISOString() });
    writeIndex(
      {},
      {
        "SPEC-009": {
          title: "Knowledge Management", status: "complete", appetite: null,
          requirement: null, source: ".agents/drafts/brainstorm-loaf-cli.md",
          created: "2026-03-01", file: "archive/SPEC-009-knowledge.md",
        },
      },
    );
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "brainstorm-loaf-cli.md");
    expect(rec?.action).toBe("delete");
    expect(rec?.reason).toContain("promoted to spec");
  });

  it("skips recent drafts", () => {
    writeArtifact("drafts", "draft-003.md", { title: "Fresh draft", created: new Date().toISOString() });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "draft-003.md");
    expect(rec?.action).toBe("skip");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Reports
// ─────────────────────────────────────────────────────────────────────────────

describe("councils", () => {
  it("reads dates from nested council.created", () => {
    const staleDate = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000).toISOString();
    writeNestedArtifact("councils", "council-001.md", "council", {
      topic: "Architecture decision",
      created: staleDate,
      status: "in_progress",
    });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "council-001.md");
    expect(rec?.action).toBe("flag");
    expect(rec?.reason).toContain("Stale");
  });

  it("flags orphaned councils with missing session reference", () => {
    writeNestedArtifact("councils", "council-002.md", "council", {
      topic: "Missing session",
      created: new Date().toISOString(),
      session_reference: ".agents/sessions/20260101-000000-nonexistent.md",
    });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "council-002.md");
    expect(rec?.action).toBe("flag");
    expect(rec?.reason).toContain("Orphaned");
  });

  it("skips recent councils with valid session (normalizes .agents/sessions/ prefix)", () => {
    writeSession("20260327-181352-task-020.md", { status: "active", last_updated: new Date().toISOString() });
    writeNestedArtifact("councils", "council-003.md", "council", {
      topic: "Active council",
      created: new Date().toISOString(),
      session_reference: ".agents/sessions/20260327-181352-task-020.md",
    });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "council-003.md");
    expect(rec?.action).toBe("skip");
  });

  it("reads dates from orchestration schema (council.timestamp + council.session)", () => {
    const staleDate = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000).toISOString();
    writeSession("20260301-session.md", { status: "complete", last_updated: new Date().toISOString() });
    writeNestedArtifact("councils", "council-005.md", "council", {
      topic: "Orchestration format",
      timestamp: staleDate,
      session: "20260301-session",
    }, "## Decision\n\nUse option B.\n");
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "council-005.md");
    expect(rec?.action).toBe("archive");
    expect(rec?.reason).toContain("session completed");
  });

  it("recommends archive for old councils with decision and completed session", () => {
    const staleDate = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000).toISOString();
    writeSession("20260301-session.md", { status: "completed", last_updated: new Date().toISOString() });
    writeNestedArtifact("councils", "council-004.md", "council", {
      topic: "Old council",
      created: staleDate,
      session_reference: ".agents/sessions/20260301-session.md",
    }, "## Decision\n\nApproved option A.\n");
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "council-004.md");
    expect(rec?.action).toBe("archive");
  });

  it("flags old councils with decision but active session", () => {
    const staleDate = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000).toISOString();
    writeSession("20260301-session.md", { status: "active", last_updated: new Date().toISOString() });
    writeNestedArtifact("councils", "council-007.md", "council", {
      topic: "Decision made but session open",
      created: staleDate,
      session_reference: ".agents/sessions/20260301-session.md",
    }, "## Decision\n\nApproved option A.\n");
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "council-007.md");
    expect(rec?.action).toBe("flag");
    expect(rec?.reason).toContain("linked session not yet completed");
  });

  it("flags old councils without decision recorded", () => {
    const staleDate = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000).toISOString();
    writeSession("20260301-session.md", { status: "complete", last_updated: new Date().toISOString() });
    writeNestedArtifact("councils", "council-006.md", "council", {
      topic: "Undecided council",
      created: staleDate,
      session_reference: ".agents/sessions/20260301-session.md",
    }, "## Decision\n\n[To be filled after user approval]\n");
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "council-006.md");
    expect(rec?.action).toBe("flag");
    expect(rec?.reason).toContain("decision not yet recorded");
  });
});

describe("reports", () => {
  it("reads nested report.archived_at and skips already-archived reports", () => {
    writeNestedArtifact("reports", "report-001.md", "report", {
      status: "processed",
      archived_at: "2026-03-01T00:00:00Z",
      archived_by: "orchestrator",
    });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "report-001.md");
    expect(rec?.action).toBe("skip");
    expect(rec?.reason).toContain("archived");
  });

  it("recommends archive for processed reports when session is archived", () => {
    // Create an archived session
    const archiveDir = setupDir("sessions", "archive");
    writeFileSync(join(archiveDir, "20260301-session.md"), "---\nsession:\n  status: archived\n---\n", "utf-8");

    writeNestedArtifact("reports", "report-002.md", "report", {
      status: "processed",
      processed_at: "2026-03-01T00:00:00Z",
      session_reference: ".agents/sessions/20260301-session.md",
    });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "report-002.md");
    expect(rec?.action).toBe("archive");
    expect(rec?.reason).toContain("prerequisites met");
  });

  it("flags processed reports missing session_reference", () => {
    writeNestedArtifact("reports", "report-004.md", "report", {
      status: "processed",
      processed_at: "2026-03-01T00:00:00Z",
    });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "report-004.md");
    expect(rec?.action).toBe("flag");
    expect(rec?.reason).toContain("missing session_reference");
  });

  it("flags old unprocessed reports instead of archiving them", () => {
    const staleDate = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000).toISOString();
    writeNestedArtifact("reports", "report-005.md", "report", {
      status: "pending",
    });
    // Need to write with a stale date — use flat frontmatter for created
    const dir = setupDir("reports");
    writeFileSync(join(dir, "report-005.md"), `---\nreport:\n  status: "pending"\ncreated: ${JSON.stringify(staleDate)}\n---\n`, "utf-8");
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "report-005.md");
    expect(rec?.action).toBe("flag");
    expect(rec?.reason).toContain("not yet processed");
  });

  it("skips processed reports when linked session is not yet archived", () => {
    writeSession("20260327-active-session.md", { status: "active", last_updated: new Date().toISOString() });
    writeNestedArtifact("reports", "report-003.md", "report", {
      status: "processed",
      processed_at: "2026-03-01T00:00:00Z",
      session_reference: ".agents/sessions/20260327-active-session.md",
    });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    const rec = findRec(result.recommendations, "report-003.md");
    expect(rec?.action).toBe("skip");
    expect(rec?.reason).toContain("not yet archived");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Missing Directories
// ─────────────────────────────────────────────────────────────────────────────

describe("directory handling", () => {
  it("warns about missing required directories", () => {
    // Remove the sessions dir we created in beforeEach
    rmSync(join(agentsDir(), "sessions"), { recursive: true });
    writeIndex();
    const result = scanArtifacts({ agentsDir: agentsDir() });
    expect(result.warnings).toContain("Required directory missing: sessions/");
  });

  it("skips missing optional directories silently", () => {
    writeIndex();
    // plans/ doesn't exist — should produce no warnings
    const result = scanArtifacts({ agentsDir: agentsDir() });
    expect(result.warnings.filter((w) => w.includes("plans"))).toHaveLength(0);
  });

  it("does not write TASKS.json when it is missing (read-only scan)", () => {
    // Don't call writeIndex() — scanner should not create TASKS.json
    const indexPath = join(agentsDir(), "TASKS.json");
    expect(existsSync(indexPath)).toBe(false);
    scanArtifacts({ agentsDir: agentsDir() });
    expect(existsSync(indexPath)).toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Filter
// ─────────────────────────────────────────────────────────────────────────────

describe("filter option", () => {
  it("restricts scan to specified artifact types", () => {
    writeSession("SESSION-001.md", { status: "completed", created: "2026-03-01" });
    writeArtifact("drafts", "draft-001.md", { title: "Draft", created: new Date().toISOString() });
    writeIndex();

    const result = scanArtifacts({ agentsDir: agentsDir(), filter: ["session"] });
    const types = new Set(result.recommendations.map((r) => r.type));
    expect(types.has("session")).toBe(true);
    expect(types.has("draft")).toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Summary
// ─────────────────────────────────────────────────────────────────────────────

describe("summary", () => {
  it("produces correct counts per artifact type", () => {
    writeSession("SESSION-001.md", { status: "completed", created: "2026-03-01" });
    writeSession("SESSION-002.md", { status: "active", last_updated: new Date().toISOString() });
    writeIndex();

    const result = scanArtifacts({ agentsDir: agentsDir() });
    const sessionSummary = result.summary.find((s) => s.type === "session");
    expect(sessionSummary?.total).toBe(2);
    expect(sessionSummary?.archive).toBe(1);
    expect(sessionSummary?.skip).toBe(1);
  });
});
