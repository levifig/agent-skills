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
  extractUnreleasedBody,
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

describe("extractUnreleasedBody (TASK-139, TASK-148)", () => {
  it("returns null when [Unreleased] is empty", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      "## [1.0.0] - 2026-01-01",
    ].join("\n");
    expect(extractUnreleasedBody(existing)).toBeNull();
  });

  it("returns null when [Unreleased] contains only the stub", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      UNRELEASED_STUB,
      "",
      "## [1.0.0] - 2026-01-01",
    ].join("\n");
    expect(extractUnreleasedBody(existing)).toBeNull();
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
    expect(extractUnreleasedBody(existing)).toBeNull();
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
    expect(extractUnreleasedBody(existing)).toBe(["- Added X", "- Fixed Y"].join("\n"));
  });

  it("returns curated entries when both stub and list items are present (entries win, stub stripped)", () => {
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
    expect(extractUnreleasedBody(existing)).toBe(
      "- Added something the user actually wrote",
    );
  });

  it("preserves prose and sub-headings alongside list items", () => {
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
    expect(extractUnreleasedBody(existing)).toBe(
      [
        "Some prose talking about the upcoming release.",
        "",
        "### Added",
        "- A real curated entry",
      ].join("\n"),
    );
  });

  it("returns null when there is no [Unreleased] section", () => {
    const existing = "# Changelog\n\n## [1.0.0] - 2026-01-01\n";
    expect(extractUnreleasedBody(existing)).toBeNull();
  });

  it("preserves ### Added / ### Changed / ### Removed / ### Fixed / ### Internal subsection headers (TASK-148)", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      "### Added",
      "- foo",
      "- bar",
      "",
      "### Changed",
      "- baz",
      "",
      "### Removed",
      "- qux",
      "",
      "### Fixed",
      "- a fix",
      "",
      "### Internal",
      "- a chore",
      "",
      "## [1.0.0] - 2026-01-01",
    ].join("\n");

    const body = extractUnreleasedBody(existing);
    expect(body).not.toBeNull();
    expect(body).toBe(
      [
        "### Added",
        "- foo",
        "- bar",
        "",
        "### Changed",
        "- baz",
        "",
        "### Removed",
        "- qux",
        "",
        "### Fixed",
        "- a fix",
        "",
        "### Internal",
        "- a chore",
      ].join("\n"),
    );
    // Each subsection header is present, in order.
    expect(body).toContain("### Added");
    expect(body).toContain("### Changed");
    expect(body).toContain("### Removed");
    expect(body).toContain("### Fixed");
    expect(body).toContain("### Internal");
  });

  it("strips the stub line but preserves subsection headers around it (TASK-148)", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      UNRELEASED_STUB,
      "",
      "### Added",
      "- new auth flow",
      "",
      "### Fixed",
      "- crash on startup",
      "",
      "## [1.0.0] - 2026-01-01",
    ].join("\n");

    expect(extractUnreleasedBody(existing)).toBe(
      [
        "### Added",
        "- new auth flow",
        "",
        "### Fixed",
        "- crash on startup",
      ].join("\n"),
    );
  });

  it("preserves arbitrary subsection headers (e.g. ### Security, ### Deprecated)", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      "### Security",
      "- patched CVE-1234",
      "",
      "### Deprecated",
      "- legacy endpoint",
      "",
      "## [1.0.0] - 2026-01-01",
    ].join("\n");

    expect(extractUnreleasedBody(existing)).toBe(
      [
        "### Security",
        "- patched CVE-1234",
        "",
        "### Deprecated",
        "- legacy endpoint",
      ].join("\n"),
    );
  });
});

describe("buildChangelogSectionFromEntries (TASK-139, TASK-148)", () => {
  it("preserves curated body verbatim under the new version header", () => {
    const body = ["- Added X", "- Fixed Y"].join("\n");
    const result = buildChangelogSectionFromEntries(
      "1.2.0",
      "2026-04-29",
      body,
    );

    expect(result).toBe(
      ["## [1.2.0] - 2026-04-29", "", "- Added X", "- Fixed Y"].join("\n"),
    );
  });

  it("does not invent its own '###' subsections or commit hashes (only emits what is in body)", () => {
    const body = "- Added something specific";
    const result = buildChangelogSectionFromEntries(
      "1.2.0",
      "2026-04-29",
      body,
    );
    expect(result).not.toContain("### Added");
    expect(result).not.toContain("### Fixed");
    expect(result).not.toMatch(/\([a-f0-9]{7,}\)/);
  });

  it("preserves subsection headers in the curated body verbatim (TASK-148)", () => {
    const body = [
      "### Added",
      "- foo",
      "- bar",
      "",
      "### Changed",
      "- baz",
    ].join("\n");

    const result = buildChangelogSectionFromEntries(
      "2.0.0-dev.33",
      "2026-04-29",
      body,
    );

    expect(result).toBe(
      [
        "## [2.0.0-dev.33] - 2026-04-29",
        "",
        "### Added",
        "- foo",
        "- bar",
        "",
        "### Changed",
        "- baz",
      ].join("\n"),
    );
  });

  it("trims leading/trailing blank lines but preserves internal whitespace", () => {
    const body = ["", "", "### Added", "- foo", "", "### Fixed", "- bar", "", ""].join("\n");

    const result = buildChangelogSectionFromEntries(
      "1.0.0",
      "2026-04-29",
      body,
    );

    expect(result).toBe(
      [
        "## [1.0.0] - 2026-04-29",
        "",
        "### Added",
        "- foo",
        "",
        "### Fixed",
        "- bar",
      ].join("\n"),
    );
  });

  it("keeps a leading subsection header as the first line under the version heading", () => {
    const body = "### Added\n- only thing";
    const result = buildChangelogSectionFromEntries(
      "1.0.0",
      "2026-04-29",
      body,
    );

    const lines = result.split("\n");
    expect(lines[0]).toBe("## [1.0.0] - 2026-04-29");
    expect(lines[1]).toBe("");
    expect(lines[2]).toBe("### Added");
    expect(lines[3]).toBe("- only thing");
  });
});

describe("end-to-end: curated [Unreleased] preservation across release (TASK-138, TASK-139, TASK-148)", () => {
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
    const curated = extractUnreleasedBody(existing);
    expect(curated).toBe(
      ["- Added authentication flow", "- Fixed race condition in worker"].join("\n"),
    );

    const section = buildChangelogSectionFromEntries(
      "1.1.0",
      "2026-04-29",
      curated!,
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

    const curated = extractUnreleasedBody(existing);
    expect(curated).toBeNull();

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

    const curated = extractUnreleasedBody(existing);
    expect(curated).toBeNull();
  });

  // SPEC-031 v2.0.0-dev.33 ship: the user wrote a comprehensive [Unreleased]
  // section grouped by ### Added / ### Changed / ### Removed / ### Fixed /
  // ### Internal. Pre-fix, the release flow flattened it to a bare list of
  // bullets. Post-fix, the structure is preserved verbatim under the new
  // ## [X.Y.Z] header.
  it("preserves the full SPEC-031 v2.0.0-dev.33 6-section CHANGELOG structure (TASK-148)", () => {
    const existing = [
      "# Changelog",
      "",
      "## [Unreleased]",
      "",
      "### Added",
      "- New release flow hardening",
      "- Stub re-insertion after release",
      "- Curated [Unreleased] preservation",
      "",
      "### Changed",
      "- `loaf release` now runs `npm run build` for Node projects",
      "- Pre-merge guardrails refined",
      "",
      "### Removed",
      "- Legacy release script",
      "",
      "### Fixed",
      "- Race condition in version-file detection",
      "- CLI bundle drift on release",
      "",
      "### Internal",
      "- Refactored release pipeline into discrete steps",
      "- Documented release flow in SPEC-031",
      "",
      "## [2.0.0-dev.32] - 2026-04-25",
      "",
      "### Added",
      "- Earlier work",
    ].join("\n");

    const curated = extractUnreleasedBody(existing);
    expect(curated).not.toBeNull();

    const section = buildChangelogSectionFromEntries(
      "2.0.0-dev.33",
      "2026-04-29",
      curated!,
    );
    const result = insertIntoChangelog(existing, section);
    expect(result).not.toBeNull();

    // All 5 subsection headers appear under the new version block.
    expect(result).toContain("## [2.0.0-dev.33] - 2026-04-29");

    const lines = result!.split("\n");
    const newVersionIdx = lines.findIndex((l) =>
      l.includes("[2.0.0-dev.33]"),
    );
    const oldVersionIdx = lines.findIndex((l) =>
      l.includes("[2.0.0-dev.32]"),
    );
    const newBlock = lines.slice(newVersionIdx, oldVersionIdx).join("\n");

    expect(newBlock).toContain("### Added");
    expect(newBlock).toContain("### Changed");
    expect(newBlock).toContain("### Removed");
    expect(newBlock).toContain("### Fixed");
    expect(newBlock).toContain("### Internal");

    // All curated entries appear under the new version block.
    expect(newBlock).toContain("- New release flow hardening");
    expect(newBlock).toContain("- Stub re-insertion after release");
    expect(newBlock).toContain("- Curated [Unreleased] preservation");
    expect(newBlock).toContain(
      "- `loaf release` now runs `npm run build` for Node projects",
    );
    expect(newBlock).toContain("- Pre-merge guardrails refined");
    expect(newBlock).toContain("- Legacy release script");
    expect(newBlock).toContain(
      "- Race condition in version-file detection",
    );
    expect(newBlock).toContain("- CLI bundle drift on release");
    expect(newBlock).toContain(
      "- Refactored release pipeline into discrete steps",
    );
    expect(newBlock).toContain("- Documented release flow in SPEC-031");

    // The order of subsection headers is preserved (Added → Changed →
    // Removed → Fixed → Internal).
    const blockLines = newBlock.split("\n");
    const addedIdx = blockLines.findIndex((l) => l === "### Added");
    const changedIdx = blockLines.findIndex((l) => l === "### Changed");
    const removedIdx = blockLines.findIndex((l) => l === "### Removed");
    const fixedIdx = blockLines.findIndex((l) => l === "### Fixed");
    const internalIdx = blockLines.findIndex((l) => l === "### Internal");
    expect(addedIdx).toBeGreaterThan(-1);
    expect(addedIdx).toBeLessThan(changedIdx);
    expect(changedIdx).toBeLessThan(removedIdx);
    expect(removedIdx).toBeLessThan(fixedIdx);
    expect(fixedIdx).toBeLessThan(internalIdx);

    // The fresh [Unreleased] block contains the stub.
    expect(result).toContain(UNRELEASED_STUB);
  });
});
