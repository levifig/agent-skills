/**
 * Release Changelog Tests
 *
 * Tests for changelog generation and insertion into existing CHANGELOG.md files.
 */

import { describe, it, expect } from "vitest";
import type { ParsedCommit } from "./commits.js";
import {
  groupBySection,
  generateChangelogSection,
  buildChangelogSectionFromEntries,
  extractUnreleasedEntries,
  insertIntoChangelog,
  createChangelog,
  UNRELEASED_STUB,
} from "./changelog.js";

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

function makeCommit(
  overrides: Partial<ParsedCommit> & { hash: string; message: string },
): ParsedCommit {
  return {
    type: "feat",
    breaking: false,
    section: "Added",
    raw: `feat: ${overrides.message}`,
    ...overrides,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// groupBySection
// ─────────────────────────────────────────────────────────────────────────────

describe("groupBySection", () => {
  it("groups commits by section", () => {
    const commits = [
      makeCommit({ hash: "abc", message: "add feature", section: "Added" }),
      makeCommit({ hash: "def", message: "fix bug", section: "Fixed" }),
      makeCommit({ hash: "ghi", message: "add another", section: "Added" }),
    ];

    const groups = groupBySection(commits);
    expect(groups.get("Added")).toHaveLength(2);
    expect(groups.get("Fixed")).toHaveLength(1);
  });

  it("filters out null-section commits", () => {
    const commits = [
      makeCommit({ hash: "abc", message: "add feature", section: "Added" }),
      makeCommit({ hash: "def", message: "chore stuff", section: null }),
    ];

    const groups = groupBySection(commits);
    expect(groups.size).toBe(1);
    expect(groups.has("Added")).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// generateChangelogSection
// ─────────────────────────────────────────────────────────────────────────────

describe("generateChangelogSection", () => {
  it("generates a versioned section with grouped entries", () => {
    const commits = [
      makeCommit({ hash: "abc", message: "new feature", section: "Added" }),
      makeCommit({ hash: "def", message: "fix crash", section: "Fixed" }),
    ];

    const section = generateChangelogSection("2.0.0-dev.7", "2026-03-30", commits);

    expect(section).toContain("## [2.0.0-dev.7] - 2026-03-30");
    expect(section).toContain("### Added");
    expect(section).toContain("- New feature (abc)");
    expect(section).toContain("### Fixed");
    expect(section).toContain("- Fix crash (def)");
  });

  it("omits sections with no commits", () => {
    const commits = [
      makeCommit({ hash: "abc", message: "fix only", section: "Fixed" }),
    ];

    const section = generateChangelogSection("1.0.1", "2026-03-30", commits);
    expect(section).not.toContain("### Added");
    expect(section).toContain("### Fixed");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// insertIntoChangelog
// ─────────────────────────────────────────────────────────────────────────────

describe("insertIntoChangelog", () => {
  it("inserts below [Unreleased] and preserves it", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      "## [1.0.0] - 2026-01-01",
      "",
      "### Added",
      "- Initial release",
    ].join("\n");

    const newSection = "## [1.1.0] - 2026-03-30\n\n### Added\n- New feature (abc)";
    const result = insertIntoChangelog(existing, newSection);

    expect(result).not.toBeNull();
    // [Unreleased] is preserved at the top
    expect(result).toContain("## [Unreleased]");
    // New section appears between [Unreleased] and old version
    const lines = result!.split("\n");
    const unreleasedIdx = lines.findIndex((l) => l.includes("[Unreleased]"));
    const newSectionIdx = lines.findIndex((l) => l.includes("[1.1.0]"));
    const oldSectionIdx = lines.findIndex((l) => l.includes("[1.0.0]"));
    expect(unreleasedIdx).toBeLessThan(newSectionIdx);
    expect(newSectionIdx).toBeLessThan(oldSectionIdx);
  });

  it("returns null when no [Unreleased] marker exists", () => {
    const existing = "# Changelog\n\n## [1.0.0] - 2026-01-01\n\n### Added\n- Initial";
    const result = insertIntoChangelog(existing, "## [1.1.0] - 2026-03-30");
    expect(result).toBeNull();
  });

  it("is case-insensitive for [Unreleased]", () => {
    const existing = "# Changelog\n\n## [unreleased]\n\n## [1.0.0] - 2026-01-01";
    const result = insertIntoChangelog(existing, "## [1.1.0] - 2026-03-30");
    expect(result).not.toBeNull();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// createChangelog
// ─────────────────────────────────────────────────────────────────────────────

describe("createChangelog", () => {
  it("creates a new changelog with [Unreleased] section", () => {
    const section = "## [1.0.0] - 2026-03-30\n\n### Added\n- First feature";
    const result = createChangelog(section);

    expect(result).toContain("# Changelog");
    expect(result).toContain("## [Unreleased]");
    expect(result).toContain("## [1.0.0] - 2026-03-30");
  });

  it("includes the unreleased stub line under [Unreleased]", () => {
    const section = "## [1.0.0] - 2026-03-30\n\n### Added\n- First feature";
    const result = createChangelog(section);
    expect(result).toContain(UNRELEASED_STUB);

    // Stub appears between [Unreleased] header and the new version
    const lines = result.split("\n");
    const unreleasedIdx = lines.findIndex((l) => l.includes("[Unreleased]"));
    const stubIdx = lines.findIndex((l) => l === UNRELEASED_STUB);
    const versionIdx = lines.findIndex((l) => l.includes("[1.0.0]"));
    expect(unreleasedIdx).toBeLessThan(stubIdx);
    expect(stubIdx).toBeLessThan(versionIdx);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// SPEC-031 — TASK-138 / TASK-139:
// Stub re-insertion + curated [Unreleased] entry preservation
// ─────────────────────────────────────────────────────────────────────────────

describe("UNRELEASED_STUB", () => {
  it("is the literal stub line wrapped as a list item", () => {
    expect(UNRELEASED_STUB).toBe("- _No unreleased changes yet._");
  });
});

describe("insertIntoChangelog stub re-insertion (TASK-138)", () => {
  it("re-inserts the stub under [Unreleased] when moving auto-generated entries", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      "## [1.0.0] - 2026-01-01",
      "",
      "### Added",
      "- Initial release",
    ].join("\n");

    const newSection = "## [1.1.0] - 2026-03-30\n\n### Added\n- New feature (abc)";
    const result = insertIntoChangelog(existing, newSection);

    expect(result).not.toBeNull();
    // Stub line is present under [Unreleased]
    expect(result).toContain(UNRELEASED_STUB);

    // Stub appears between [Unreleased] header and the new version section
    const lines = result!.split("\n");
    const unreleasedIdx = lines.findIndex((l) => l.includes("[Unreleased]"));
    const stubIdx = lines.findIndex((l) => l === UNRELEASED_STUB);
    const newSectionIdx = lines.findIndex((l) => l.includes("[1.1.0]"));
    expect(unreleasedIdx).toBeLessThan(stubIdx);
    expect(stubIdx).toBeLessThan(newSectionIdx);
  });

  it("re-inserts the stub even when [Unreleased] previously had curated entries", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      "- Added curated entry one",
      "- Fixed curated entry two",
      "",
      "## [1.0.0] - 2026-01-01",
      "",
      "### Added",
      "- Initial release",
    ].join("\n");

    const newSection =
      "## [1.1.0] - 2026-03-30\n\n- Added curated entry one\n- Fixed curated entry two";
    const result = insertIntoChangelog(existing, newSection);

    expect(result).not.toBeNull();
    expect(result).toContain(UNRELEASED_STUB);
    // The curated entries no longer appear under [Unreleased] —
    // they live under the new [1.1.0] section now.
    const lines = result!.split("\n");
    const unreleasedIdx = lines.findIndex((l) => l.includes("[Unreleased]"));
    const newSectionIdx = lines.findIndex((l) => l.includes("[1.1.0]"));
    const stubIdx = lines.findIndex((l) => l === UNRELEASED_STUB);
    // Stub sits between [Unreleased] and [1.1.0]
    expect(unreleasedIdx).toBeLessThan(stubIdx);
    expect(stubIdx).toBeLessThan(newSectionIdx);
    // Curated entries appear in the [1.1.0] block, AFTER the stub
    const firstCuratedIdx = lines.findIndex(
      (l, i) => i > stubIdx && l === "- Added curated entry one",
    );
    expect(firstCuratedIdx).toBeGreaterThan(newSectionIdx);
  });
});

describe("extractUnreleasedEntries (TASK-139)", () => {
  it("returns an empty array when [Unreleased] is empty", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      "## [1.0.0] - 2026-01-01",
    ].join("\n");
    expect(extractUnreleasedEntries(existing)).toEqual([]);
  });

  it("returns an empty array when [Unreleased] contains only the stub", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      UNRELEASED_STUB,
      "",
      "## [1.0.0] - 2026-01-01",
    ].join("\n");
    expect(extractUnreleasedEntries(existing)).toEqual([]);
  });

  it('treats the "since vX.Y.Z" stub variant as a non-entry', () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      "- _No unreleased changes since v2.0.0-dev.32._",
      "",
      "## [1.0.0] - 2026-01-01",
    ].join("\n");
    expect(extractUnreleasedEntries(existing)).toEqual([]);
  });

  it("returns curated list-item entries verbatim", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      "- Added X",
      "- Fixed Y",
      "",
      "## [1.0.0] - 2026-01-01",
    ].join("\n");
    expect(extractUnreleasedEntries(existing)).toEqual([
      "- Added X",
      "- Fixed Y",
    ]);
  });

  it("returns curated entries when both stub and list items are present (entries win)", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      UNRELEASED_STUB,
      "- Added something the user actually wrote",
      "",
      "## [1.0.0] - 2026-01-01",
    ].join("\n");
    expect(extractUnreleasedEntries(existing)).toEqual([
      "- Added something the user actually wrote",
    ]);
  });

  it("ignores prose, sub-headings, and whitespace lines", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      "Some prose talking about the upcoming release.",
      "",
      "### Added",
      "- A real curated entry",
      "",
      "## [1.0.0] - 2026-01-01",
    ].join("\n");
    expect(extractUnreleasedEntries(existing)).toEqual([
      "- A real curated entry",
    ]);
  });

  it("returns empty when there is no [Unreleased] section", () => {
    const existing = "# Changelog\n\n## [1.0.0] - 2026-01-01\n";
    expect(extractUnreleasedEntries(existing)).toEqual([]);
  });
});

describe("buildChangelogSectionFromEntries (TASK-139)", () => {
  it("preserves curated entries verbatim under the new version header", () => {
    const entries = ["- Added X", "- Fixed Y"];
    const result = buildChangelogSectionFromEntries(
      "1.2.0",
      "2026-04-29",
      entries,
    );

    expect(result).toBe(
      ["## [1.2.0] - 2026-04-29", "", "- Added X", "- Fixed Y"].join("\n"),
    );
  });

  it("emits no auto-generated jargon (no '###' subsections, no commit hashes)", () => {
    const entries = ["- Added something specific"];
    const result = buildChangelogSectionFromEntries(
      "1.2.0",
      "2026-04-29",
      entries,
    );
    expect(result).not.toContain("### Added");
    expect(result).not.toContain("### Fixed");
    expect(result).not.toMatch(/\([a-f0-9]{7,}\)/);
  });
});

describe("end-to-end: curated [Unreleased] preservation across release (TASK-138 + TASK-139)", () => {
  it("preserves curated entries verbatim AND re-inserts the stub", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      "- Added authentication flow",
      "- Fixed race condition in worker",
      "",
      "## [1.0.0] - 2026-01-01",
      "",
      "### Added",
      "- Initial release",
    ].join("\n");

    // Simulate what release.ts does: read curated, build section, insert.
    const curated = extractUnreleasedEntries(existing);
    expect(curated).toEqual([
      "- Added authentication flow",
      "- Fixed race condition in worker",
    ]);

    const section = buildChangelogSectionFromEntries(
      "1.1.0",
      "2026-04-29",
      curated,
    );
    const result = insertIntoChangelog(existing, section);
    expect(result).not.toBeNull();

    // The new [1.1.0] block contains the curated entries verbatim.
    expect(result).toContain("## [1.1.0] - 2026-04-29");
    expect(result).toContain("- Added authentication flow");
    expect(result).toContain("- Fixed race condition in worker");

    // The fresh [Unreleased] block contains the stub.
    expect(result).toContain(UNRELEASED_STUB);

    // After the rewrite, [Unreleased] no longer contains the curated entries.
    // (They moved into [1.1.0].)
    const lines = result!.split("\n");
    const unreleasedIdx = lines.findIndex((l) => l.includes("[Unreleased]"));
    const newVersionIdx = lines.findIndex((l) => l.includes("[1.1.0]"));
    const between = lines.slice(unreleasedIdx + 1, newVersionIdx);
    expect(between.join("\n")).not.toContain("Added authentication flow");
    expect(between.join("\n")).toContain(UNRELEASED_STUB);
  });

  it("falls through to auto-generation when [Unreleased] is empty", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      "## [1.0.0] - 2026-01-01",
    ].join("\n");

    const curated = extractUnreleasedEntries(existing);
    expect(curated).toEqual([]);

    // Caller would call generateChangelogSection here. Verify the empty
    // detection routes to that path.
    const generated = generateChangelogSection(
      "1.1.0",
      "2026-04-29",
      [
        {
          type: "feat",
          breaking: false,
          section: "Added",
          raw: "feat: auto-generated entry",
          hash: "abc1234",
          message: "auto-generated entry",
        },
      ],
    );
    const result = insertIntoChangelog(existing, generated);
    expect(result).not.toBeNull();
    expect(result).toContain("### Added");
    expect(result).toContain("- Auto-generated entry (abc1234)");
    // Stub re-inserted under [Unreleased]
    expect(result).toContain(UNRELEASED_STUB);
  });

  it("falls through to auto-generation when [Unreleased] contains only the stub", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      UNRELEASED_STUB,
      "",
      "## [1.0.0] - 2026-01-01",
    ].join("\n");

    const curated = extractUnreleasedEntries(existing);
    expect(curated).toEqual([]);
  });
});
