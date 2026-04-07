/**
 * Fenced-Section Management Tests
 *
 * Tests for installing and upgrading Loaf framework conventions
 * in project CLAUDE.md/AGENTS.md files.
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import {
  existsSync,
  mkdirSync,
  rmSync,
  readFileSync,
  writeFileSync,
  rmdirSync,
} from "fs";
import { join, dirname } from "path";
import {
  installFencedSection,
  getTargetFile,
  getFencedVersion,
  installFencedSectionsForTargets,
} from "./fenced-section.js";

// ─────────────────────────────────────────────────────────────────────────────
// Test Fixtures
// ─────────────────────────────────────────────────────────────────────────────

const TEST_ROOT = join(process.cwd(), ".test-fenced-section");

// ─────────────────────────────────────────────────────────────────────────────
// Test Setup / Teardown
// ─────────────────────────────────────────────────────────────────────────────

beforeEach(() => {
  // Clean and recreate test directory
  if (existsSync(TEST_ROOT)) {
    rmSync(TEST_ROOT, { recursive: true, force: true });
  }
  mkdirSync(TEST_ROOT, { recursive: true });
});

afterEach(() => {
  // Clean up test directory
  if (existsSync(TEST_ROOT)) {
    rmSync(TEST_ROOT, { recursive: true, force: true });
  }
});

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

function createTestFile(path: string, content: string): void {
  const fullPath = join(TEST_ROOT, path);
  mkdirSync(dirname(fullPath), { recursive: true });
  writeFileSync(fullPath, content);
}

function readTestFile(path: string): string {
  return readFileSync(join(TEST_ROOT, path), "utf-8");
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests: installFencedSection
// ─────────────────────────────────────────────────────────────────────────────

describe("installFencedSection", () => {
  it("creates new file when file doesn't exist", () => {
    const targetFile = join(TEST_ROOT, ".agents", "AGENTS.md");
    const result = installFencedSection(targetFile, false);

    expect(result.action).toBe("created");
    expect(result.version).toBeTruthy();
    expect(existsSync(targetFile)).toBe(true);

    const content = readFileSync(targetFile, "utf-8");
    expect(content).toContain("<!-- loaf:managed:start");
    expect(content).toContain("<!-- loaf:managed:end -->");
    expect(content).toContain("## Loaf Framework");
  });

  it("appends to existing file without fenced section", () => {
    const targetFile = join(TEST_ROOT, ".agents", "AGENTS.md");
    const userContent = "# My Project\n\nUser content here.";
    createTestFile(".agents/AGENTS.md", userContent);

    const result = installFencedSection(targetFile, false);

    expect(result.action).toBe("appended");
    expect(result.version).toBeTruthy();

    const content = readFileSync(targetFile, "utf-8");
    expect(content).toContain("# My Project");
    expect(content).toContain("User content here.");
    expect(content).toContain("<!-- loaf:managed:start");
    expect(content).toContain("<!-- loaf:managed:end -->");
    // User content should be before the fenced section
    expect(content.indexOf("User content here.")).toBeLessThan(
      content.indexOf("<!-- loaf:managed:start")
    );
  });

  it("updates existing fenced section in upgrade mode", () => {
    const targetFile = join(TEST_ROOT, ".agents", "AGENTS.md");
    const oldFencedSection = `<!-- loaf:managed:start v1.0.0 -->
<!-- Maintained by loaf install/upgrade — do not edit manually -->
## Loaf Framework
Old content
<!-- loaf:managed:end -->`;
    const userContent = `# My Project\n\nUser content here.\n\n${oldFencedSection}`;
    createTestFile(".agents/AGENTS.md", userContent);

    const result = installFencedSection(targetFile, true);

    expect(result.action).toBe("updated");
    expect(result.version).toBeTruthy();

    const content = readFileSync(targetFile, "utf-8");
    expect(content).toContain("# My Project");
    expect(content).toContain("User content here.");
    expect(content).toContain("Session Journal Entry Types:");
    expect(content).not.toContain("Old content");
  });

  it("skips update when version matches in upgrade mode", () => {
    const targetFile = join(TEST_ROOT, ".agents", "AGENTS.md");
    // We can't know the exact version, but we can test the logic by checking
    // that when the same version is installed twice in upgrade mode, it skips

    // First install
    const result1 = installFencedSection(targetFile, false);
    expect(result1.action).toBe("created");

    // Second install with upgrade mode should skip (same version)
    const result2 = installFencedSection(targetFile, true);
    expect(result2.action).toBe("skipped");
  });

  it("replaces fenced section even without upgrade mode", () => {
    const targetFile = join(TEST_ROOT, ".agents", "AGENTS.md");
    const oldFencedSection = `<!-- loaf:managed:start v0.0.1 -->
<!-- Maintained by loaf install/upgrade — do not edit manually -->
## Loaf Framework
Old content
<!-- loaf:managed:end -->`;
    const userContent = `# My Project\n\n${oldFencedSection}`;
    createTestFile(".agents/AGENTS.md", userContent);

    const result = installFencedSection(targetFile, false);

    expect(result.action).toBe("updated");

    const content = readFileSync(targetFile, "utf-8");
    expect(content).toContain("# My Project");
    expect(content).not.toContain("Old content");
    expect(content).toContain("Session Journal Entry Types:");
  });

  it("preserves user content when updating fenced section", () => {
    const targetFile = join(TEST_ROOT, ".agents", "AGENTS.md");
    const userContentBefore = "# Header before\n\nContent before.";
    const oldFencedSection = `<!-- loaf:managed:start v0.0.1 -->
<!-- Maintained by loaf install/upgrade — do not edit manually -->
## Loaf Framework
Old content
<!-- loaf:managed:end -->`;
    const userContentAfter = "\n\n# Header after\n\nContent after.";
    const fullContent = `${userContentBefore}\n\n${oldFencedSection}${userContentAfter}`;
    createTestFile(".agents/AGENTS.md", fullContent);

    installFencedSection(targetFile, false);

    const content = readFileSync(targetFile, "utf-8");
    expect(content).toContain("# Header before");
    expect(content).toContain("Content before.");
    expect(content).toContain("# Header after");
    expect(content).toContain("Content after.");
    expect(content).not.toContain("Old content");
    expect(content).toContain("Session Journal Entry Types:");

    // Verify order is preserved
    const beforeIdx = content.indexOf("# Header before");
    const fencedIdx = content.indexOf("<!-- loaf:managed:start");
    const afterIdx = content.indexOf("# Header after");

    expect(beforeIdx).toBeLessThan(fencedIdx);
    expect(fencedIdx).toBeLessThan(afterIdx);
  });

  it("appends new fenced section when old one was deleted", () => {
    const targetFile = join(TEST_ROOT, ".agents", "AGENTS.md");
    const userContent = "# My Project\n\nUser content only, no fences.";
    createTestFile(".agents/AGENTS.md", userContent);

    // First install adds fenced section
    const result1 = installFencedSection(targetFile, false);
    expect(result1.action).toBe("appended");

    // Simulate user deleting the fences but keeping content
    writeFileSync(targetFile, userContent);

    // Second install should append again
    const result2 = installFencedSection(targetFile, false);
    expect(result2.action).toBe("appended");

    const content = readFileSync(targetFile, "utf-8");
    expect(content).toContain("# My Project");
    expect(content).toContain("<!-- loaf:managed:start");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Tests: getTargetFile
// ─────────────────────────────────────────────────────────────────────────────

describe("getTargetFile", () => {
  it("returns correct path for opencode", () => {
    const result = getTargetFile("opencode", TEST_ROOT);
    expect(result).toBe(join(TEST_ROOT, ".agents", "AGENTS.md"));
  });

  it("returns correct path for codex", () => {
    const result = getTargetFile("codex", TEST_ROOT);
    expect(result).toBe(join(TEST_ROOT, ".agents", "AGENTS.md"));
  });

  it("returns correct path for claude-code", () => {
    const result = getTargetFile("claude-code", TEST_ROOT);
    expect(result).toBe(join(TEST_ROOT, ".claude", "CLAUDE.md"));
  });

  it("returns correct path for cursor", () => {
    const result = getTargetFile("cursor", TEST_ROOT);
    expect(result).toBe(join(TEST_ROOT, ".cursor", "rules", "loaf.mdc"));
  });

  it("returns null for unknown target", () => {
    const result = getTargetFile("unknown", TEST_ROOT);
    expect(result).toBeNull();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Tests: getFencedVersion
// ─────────────────────────────────────────────────────────────────────────────

describe("getFencedVersion", () => {
  it("returns version when fenced section exists", () => {
    const targetFile = join(TEST_ROOT, "test.md");
    const content = `<!-- loaf:managed:start v2.1.0 -->
## Loaf Framework
Content
<!-- loaf:managed:end -->`;
    writeFileSync(targetFile, content);

    const version = getFencedVersion(targetFile);
    expect(version).toBe("2.1.0");
  });

  it("returns null when no fenced section", () => {
    const targetFile = join(TEST_ROOT, "test.md");
    writeFileSync(targetFile, "# Just user content");

    const version = getFencedVersion(targetFile);
    expect(version).toBeNull();
  });

  it("returns null when file doesn't exist", () => {
    const targetFile = join(TEST_ROOT, "nonexistent.md");
    const version = getFencedVersion(targetFile);
    expect(version).toBeNull();
  });

  it("handles prerelease versions", () => {
    const targetFile = join(TEST_ROOT, "test.md");
    const content = `<!-- loaf:managed:start v2.1.0-beta.3 -->
## Loaf Framework
Content
<!-- loaf:managed:end -->`;
    writeFileSync(targetFile, content);

    const version = getFencedVersion(targetFile);
    expect(version).toBe("2.1.0-beta.3");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Tests: installFencedSectionsForTargets
// ─────────────────────────────────────────────────────────────────────────────

describe("installFencedSectionsForTargets", () => {
  it("installs to multiple targets", () => {
    const results = installFencedSectionsForTargets(
      ["opencode", "claude-code"],
      TEST_ROOT,
      false
    );

    expect(results["opencode"].action).toBe("created");
    expect(results["claude-code"].action).toBe("created");

    expect(existsSync(join(TEST_ROOT, ".agents", "AGENTS.md"))).toBe(true);
    expect(existsSync(join(TEST_ROOT, ".claude", "CLAUDE.md"))).toBe(true);
  });

  it("handles unknown targets", () => {
    const results = installFencedSectionsForTargets(
      ["unknown"],
      TEST_ROOT,
      false
    );

    expect(results["unknown"].action).toBe("error");
    expect(results["unknown"].error).toContain("Unknown target");
  });

  it("upgrades existing sections in upgrade mode", () => {
    // First install
    installFencedSectionsForTargets(["opencode"], TEST_ROOT, false);

    // Upgrade with same version should skip
    const results = installFencedSectionsForTargets(
      ["opencode"],
      TEST_ROOT,
      true
    );

    expect(results["opencode"].action).toBe("skipped");
  });

  it("handles targets sharing the same file", () => {
    // opencode and codex both use .agents/AGENTS.md
    const results = installFencedSectionsForTargets(
      ["opencode", "codex"],
      TEST_ROOT,
      false
    );

    // First creates the file
    expect(results["opencode"].action).toBe("created");
    // Second updates the existing fence (same file)
    expect(results["codex"].action).toBe("updated");

    // File should exist with one fence
    const content = readTestFile(".agents/AGENTS.md");
    expect(content).toContain("<!-- loaf:managed:start");
    expect(content).toContain("<!-- loaf:managed:end -->");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Tests: Fenced Content Format
// ─────────────────────────────────────────────────────────────────────────────

describe("fenced content format", () => {
  it("contains all required elements", () => {
    const targetFile = join(TEST_ROOT, ".agents", "AGENTS.md");
    installFencedSection(targetFile, false);

    const content = readFileSync(targetFile, "utf-8");

    // Required markers
    expect(content).toContain("<!-- loaf:managed:start");
    expect(content).toContain("<!-- loaf:managed:end -->");
    expect(content).toContain(
      "<!-- Maintained by loaf install/upgrade — do not edit manually -->"
    );

    // Required sections
    expect(content).toContain("## Loaf Framework");
    expect(content).toContain("**Session Journal Entry Types:**");
    expect(content).toContain("`decision(scope)`");
    expect(content).toContain("`discover(scope)`");
    expect(content).toContain("`block(scope)`");
    expect(content).toContain("`unblock(scope)`");
    expect(content).toContain("`spark(scope)`");
    expect(content).toContain("`todo(scope)`");

    // CLI commands
    expect(content).toContain("**CLI Commands:**");
    expect(content).toContain("`loaf session");
    expect(content).toContain("`loaf check`");

    // Link to orchestration skill
    expect(content).toContain("orchestration skill");
    expect(content).toContain("skills/orchestration/SKILL.md");
  });

  it("is compact (under 30 lines)", () => {
    const targetFile = join(TEST_ROOT, ".agents", "AGENTS.md");
    installFencedSection(targetFile, false);

    const content = readFileSync(targetFile, "utf-8");
    const lines = content.trim().split("\n");

    // The fenced section should be approximately 20-30 lines
    expect(lines.length).toBeGreaterThanOrEqual(15);
    expect(lines.length).toBeLessThanOrEqual(30);
  });

  it("adds Cursor frontmatter for new .mdc files", () => {
    const targetFile = join(TEST_ROOT, ".cursor", "rules", "loaf.mdc");
    const result = installFencedSection(targetFile, false);

    expect(result.action).toBe("created");

    const content = readFileSync(targetFile, "utf-8");
    // Should have Cursor frontmatter
    expect(content).toMatch(/^---\n/);
    expect(content).toContain("description: Loaf framework conventions");
    expect(content).toContain("alwaysApply: true");
    expect(content).toContain("<!-- loaf:managed:start");
  });

  it("does not add frontmatter for new .md files", () => {
    const targetFile = join(TEST_ROOT, ".agents", "NEW.md");
    const result = installFencedSection(targetFile, false);

    expect(result.action).toBe("created");

    const content = readFileSync(targetFile, "utf-8");
    // Should NOT have frontmatter for regular markdown
    expect(content).not.toMatch(/^---\n/);
    expect(content).toContain("<!-- loaf:managed:start");
  });
});
