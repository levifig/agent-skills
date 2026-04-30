/**
 * Changelog Generator
 *
 * Takes parsed conventional commits and produces Keep-a-Changelog formatted
 * markdown. Can insert a new release section into an existing CHANGELOG.md
 * or create one from scratch.
 */

import type { ParsedCommit, ChangelogSection } from "./commits.js";

// ─────────────────────────────────────────────────────────────────────────────
// Constants
// ─────────────────────────────────────────────────────────────────────────────

/** Section ordering — Breaking Changes first, Other last. */
const SECTION_ORDER: ChangelogSection[] = [
  "Breaking Changes",
  "Added",
  "Changed",
  "Fixed",
  "Other",
];

const UNRELEASED_RE = /^## \[unreleased\]/i;

/**
 * Stub line re-inserted under [Unreleased] after a release. Wrapped as a
 * markdown list item so that workflow-pre-pr's `^[-*]\s` entry-detection
 * does not flag the section as empty between releases.
 */
export const UNRELEASED_STUB = "- _No unreleased changes yet._";

/**
 * Matches the stub regardless of the trailing "yet." vs "since vX.Y.Z." form.
 *
 * Exported so consumers (e.g. workflow-pre-pr's empty-section detector) can
 * recognize the stub as non-entry content and not mistakenly treat it as a
 * curated changelog entry just because it happens to be a markdown list item.
 */
export const UNRELEASED_STUB_RE = /^[-*]\s+_No unreleased changes.*_\.?\s*$/;

// ─────────────────────────────────────────────────────────────────────────────
// Grouping
// ─────────────────────────────────────────────────────────────────────────────

/** Group commits by changelog section, filtering out null-section commits. */
export function groupBySection(
  commits: ParsedCommit[],
): Map<ChangelogSection, ParsedCommit[]> {
  const groups = new Map<ChangelogSection, ParsedCommit[]>();

  for (const commit of commits) {
    if (commit.section === null) continue;

    const existing = groups.get(commit.section);
    if (existing) {
      existing.push(commit);
    } else {
      groups.set(commit.section, [commit]);
    }
  }

  return groups;
}

// ─────────────────────────────────────────────────────────────────────────────
// Section Generation
// ─────────────────────────────────────────────────────────────────────────────

/** Capitalize the first letter of a string. */
function capitalize(str: string): string {
  if (!str) return str;
  return str.charAt(0).toUpperCase() + str.slice(1);
}

/** Format a single commit as a changelog entry. */
function formatEntry(commit: ParsedCommit): string {
  return `- ${capitalize(commit.message)} (${commit.hash})`;
}

/** Generate a markdown changelog section for a version. */
export function generateChangelogSection(
  version: string,
  date: string,
  commits: ParsedCommit[],
): string {
  const groups = groupBySection(commits);
  const lines: string[] = [];

  lines.push(`## [${version}] - ${date}`);

  for (const section of SECTION_ORDER) {
    const sectionCommits = groups.get(section);
    if (!sectionCommits || sectionCommits.length === 0) continue;

    lines.push("");
    lines.push(`### ${section}`);
    for (const commit of sectionCommits) {
      lines.push(formatEntry(commit));
    }
  }

  return lines.join("\n");
}

/**
 * Build a versioned changelog section from a curated `[Unreleased]` body.
 *
 * Used when the user wrote their own content under `[Unreleased]` before
 * running `loaf release`. Auto-generation from commit subjects is skipped
 * and the curated body is preserved verbatim — including any subsection
 * headers (`### Added`, `### Changed`, `### Removed`, `### Fixed`,
 * `### Internal`, …), prose, list items, and blank lines between them —
 * under the new version header.
 *
 * The `body` is emitted as-is; only leading and trailing blank lines are
 * trimmed so the section starts and ends cleanly. Internal whitespace is
 * preserved.
 */
export function buildChangelogSectionFromEntries(
  version: string,
  date: string,
  body: string,
): string {
  const trimmed = body.replace(/^(?:[ \t]*\n)+/, "").replace(/(?:\n[ \t]*)+$/, "");
  return `## [${version}] - ${date}\n\n${trimmed}`;
}

// ─────────────────────────────────────────────────────────────────────────────
// Unreleased section inspection
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Extract the body of the `[Unreleased]` section from a CHANGELOG.md.
 *
 * Returns the lines between `## [Unreleased]` and the next `## [` heading,
 * or `null` when no `[Unreleased]` marker exists.
 */
function getUnreleasedBody(existingContent: string): string[] | null {
  const lines = existingContent.split("\n");

  let unreleasedIndex = -1;
  for (let i = 0; i < lines.length; i++) {
    if (UNRELEASED_RE.test(lines[i].trim())) {
      unreleasedIndex = i;
      break;
    }
  }
  if (unreleasedIndex === -1) return null;

  let nextReleaseIndex = lines.length;
  for (let i = unreleasedIndex + 1; i < lines.length; i++) {
    if (/^## \[/.test(lines[i].trim())) {
      nextReleaseIndex = i;
      break;
    }
  }

  return lines.slice(unreleasedIndex + 1, nextReleaseIndex);
}

/**
 * Extract the curated body of the `[Unreleased]` section, preserving
 * structure verbatim.
 *
 * Returns the body between `## [Unreleased]` and the next `## [` heading
 * with the stub line (`- _No unreleased changes ..._`) removed. All other
 * content — subsection headers (`### Added`, `### Changed`, `### Removed`,
 * `### Fixed`, `### Internal`, etc.), list items, prose, and the blank
 * lines between them — is preserved exactly as written.
 *
 * Returns `null` (signalling "no curated content; fall through to
 * auto-generation") when:
 *   - There is no `[Unreleased]` section.
 *   - The section is empty or contains only the stub and/or whitespace.
 *
 * Otherwise returns the body as a single string. Leading and trailing
 * blank lines are trimmed; the consuming builder will re-add the single
 * blank line that separates the body from the `## [X.Y.Z]` header.
 */
export function extractUnreleasedBody(existingContent: string): string | null {
  const body = getUnreleasedBody(existingContent);
  if (body === null) return null;

  // Strip stub lines but keep everything else verbatim (including blank
  // lines, sub-headings, prose, list items).
  const filtered = body.filter((line) => !UNRELEASED_STUB_RE.test(line));

  // After stub removal, decide whether this counts as "curated content"
  // worth preserving. Whitespace-only is treated as empty so the caller
  // routes to the auto-generation path.
  const hasContent = filtered.some((line) => line.trim().length > 0);
  if (!hasContent) return null;

  // Trim leading/trailing blank lines but keep internal structure intact.
  let start = 0;
  let end = filtered.length;
  while (start < end && filtered[start].trim().length === 0) start++;
  while (end > start && filtered[end - 1].trim().length === 0) end--;

  return filtered.slice(start, end).join("\n");
}

// ─────────────────────────────────────────────────────────────────────────────
// Changelog File Operations
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Insert a new release section into an existing CHANGELOG.md content.
 *
 * Replaces the `[Unreleased]` body with the stub line so the section is no
 * longer empty (workflow-pre-pr requires at least one list item there) and
 * adds the new versioned section below it. Returns null if no `[Unreleased]`
 * marker is found.
 */
export function insertIntoChangelog(
  existingContent: string,
  newSection: string,
): string | null {
  const lines = existingContent.split("\n");

  // Find the [Unreleased] heading
  let unreleasedIndex = -1;
  for (let i = 0; i < lines.length; i++) {
    if (UNRELEASED_RE.test(lines[i].trim())) {
      unreleasedIndex = i;
      break;
    }
  }

  if (unreleasedIndex === -1) return null;

  // Find the next release heading (## [...]) after [Unreleased]
  let nextReleaseIndex = -1;
  for (let i = unreleasedIndex + 1; i < lines.length; i++) {
    if (/^## \[/.test(lines[i].trim())) {
      nextReleaseIndex = i;
      break;
    }
  }

  // Build the replacement:
  //   ## [Unreleased]
  //   <blank>
  //   {UNRELEASED_STUB}
  //   <blank>
  //   {newSection}
  //   <blank>
  //   {rest of file from next release onward}
  const before = lines.slice(0, unreleasedIndex + 1);
  const after =
    nextReleaseIndex === -1 ? [] : lines.slice(nextReleaseIndex);

  const result = [
    ...before,
    "",
    UNRELEASED_STUB,
    "",
    newSection,
    "",
    ...after,
  ];

  return result.join("\n");
}

/**
 * Create a fresh CHANGELOG.md with the release section.
 * Used when no CHANGELOG.md exists.
 */
export function createChangelog(releaseSection: string): string {
  const lines = [
    "# Changelog",
    "",
    "All notable changes to this project will be documented in this file.",
    "",
    "The format is based on [Keep a Changelog](https://keepachangelog.com/).",
    "",
    "## [Unreleased]",
    "",
    UNRELEASED_STUB,
    "",
    releaseSection,
    "",
  ];

  return lines.join("\n");
}
