/**
 * Multi-Ecosystem Version Detector/Updater
 *
 * Auto-detects version files across ecosystems (Node, Python, Rust, Loaf)
 * and provides read/write capabilities for the `loaf release` command.
 *
 * Supports: package.json, pyproject.toml, Cargo.toml, .agents/loaf.json
 * TOML files use regex parsing — no external dependencies.
 */

import { existsSync, readFileSync } from "fs";
import { join, relative } from "path";

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

export interface VersionFile {
  path: string;
  relativePath: string;
  format: "json" | "toml-regex";
  currentVersion: string;
}

export interface SemVer {
  major: number;
  minor: number;
  patch: number;
  prerelease?: string;
}

export type BumpType = "major" | "minor" | "patch" | "prerelease" | "release";

// ─────────────────────────────────────────────────────────────────────────────
// SemVer Helpers
// ─────────────────────────────────────────────────────────────────────────────

/** Parse a semver string like "2.0.0" or "2.0.0-dev.0" into components. Returns null if invalid. */
export function parseSemVer(version: string): SemVer | null {
  // Split on first hyphen to separate core version from prerelease
  const hyphenIndex = version.indexOf("-");
  const core = hyphenIndex === -1 ? version : version.slice(0, hyphenIndex);
  const prerelease =
    hyphenIndex === -1 ? undefined : version.slice(hyphenIndex + 1);

  const parts = core.split(".");
  if (parts.length !== 3) return null;

  const [major, minor, patch] = parts.map(Number);
  if ([major, minor, patch].some((n) => !Number.isInteger(n) || n < 0)) {
    return null;
  }

  // Prerelease must be non-empty if present
  if (prerelease !== undefined && prerelease.length === 0) return null;

  return { major, minor, patch, ...(prerelease ? { prerelease } : {}) };
}

/** Format SemVer back to string, including prerelease if present. */
export function formatSemVer(ver: SemVer): string {
  const core = `${ver.major}.${ver.minor}.${ver.patch}`;
  return ver.prerelease ? `${core}-${ver.prerelease}` : core;
}

/** Compute next version given current version and bump type. */
export function bumpVersion(
  current: string,
  bump: BumpType,
): string | null {
  const ver = parseSemVer(current);
  if (!ver) return null;

  switch (bump) {
    case "major":
      return formatSemVer({ major: ver.major + 1, minor: 0, patch: 0 });
    case "minor":
      return formatSemVer({
        major: ver.major,
        minor: ver.minor + 1,
        patch: 0,
      });
    case "patch":
      return formatSemVer({
        major: ver.major,
        minor: ver.minor,
        patch: ver.patch + 1,
      });
    case "prerelease": {
      // Can only bump prerelease on a pre-release version
      if (!ver.prerelease) return null;

      const dotIndex = ver.prerelease.lastIndexOf(".");
      if (dotIndex === -1) {
        // No numeric suffix (e.g. "dev") — append .1
        return formatSemVer({ ...ver, prerelease: `${ver.prerelease}.1` });
      }

      const label = ver.prerelease.slice(0, dotIndex);
      const numStr = ver.prerelease.slice(dotIndex + 1);
      const num = Number(numStr);

      if (!Number.isInteger(num) || num < 0) {
        // Suffix isn't numeric — append .1
        return formatSemVer({ ...ver, prerelease: `${ver.prerelease}.1` });
      }

      return formatSemVer({ ...ver, prerelease: `${label}.${num + 1}` });
    }
    case "release": {
      // Can only "release" a pre-release version
      if (!ver.prerelease) return null;

      return formatSemVer({
        major: ver.major,
        minor: ver.minor,
        patch: ver.patch,
      });
    }
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// TOML Version Extraction (regex-based, no parsing library)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Extract a version string from a TOML section using regex.
 *
 * Finds the target `[sectionName]` header, then scans lines until the next
 * section header or EOF, looking for `version = "X.Y.Z"`.
 */
function readTomlVersion(
  content: string,
  sectionName: string,
): string | null {
  const lines = content.split("\n");
  const sectionPattern = new RegExp(`^\\[${escapeRegex(sectionName)}\\]`);
  let inSection = false;

  for (const line of lines) {
    if (sectionPattern.test(line.trim())) {
      inSection = true;
      continue;
    }

    if (inSection) {
      // Reached another section — stop scanning
      if (/^\[/.test(line.trim())) break;

      const match = line.match(/^version\s*=\s*"([^"]+)"/);
      if (match) return match[1];
    }
  }

  return null;
}

/**
 * Replace the version string within a specific TOML section.
 *
 * Returns the updated file content with only the target section's version
 * line modified.
 */
function replaceTomlVersion(
  content: string,
  sectionName: string,
  newVersion: string,
): string {
  const lines = content.split("\n");
  const sectionPattern = new RegExp(`^\\[${escapeRegex(sectionName)}\\]`);
  let inSection = false;
  let replaced = false;

  const result = lines.map((line) => {
    if (sectionPattern.test(line.trim())) {
      inSection = true;
      return line;
    }

    if (inSection && !replaced) {
      if (/^\[/.test(line.trim())) {
        inSection = false;
        return line;
      }

      if (/^version\s*=\s*"[^"]+"/.test(line)) {
        replaced = true;
        return line.replace(
          /^(version\s*=\s*)"[^"]+"/,
          `$1"${newVersion}"`,
        );
      }
    }

    return line;
  });

  return result.join("\n");
}

function escapeRegex(str: string): string {
  return str.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// ─────────────────────────────────────────────────────────────────────────────
// Detection
// ─────────────────────────────────────────────────────────────────────────────

interface VersionCandidate {
  relativePath: string;
  format: "json" | "toml-regex";
  source: { type: "json"; versionPath?: string } | { type: "toml"; section: string };
}

/** Ordered by priority — ecosystem files first, loaf.json as fallback. */
const CANDIDATES: VersionCandidate[] = [
  {
    relativePath: "package.json",
    format: "json",
    source: { type: "json" },
  },
  {
    relativePath: "pyproject.toml",
    format: "toml-regex",
    source: { type: "toml", section: "project" },
  },
  {
    relativePath: "Cargo.toml",
    format: "toml-regex",
    source: { type: "toml", section: "package" },
  },
  {
    relativePath: ".agents/loaf.json",
    format: "json",
    source: { type: "json" },
  },
  {
    relativePath: ".claude-plugin/marketplace.json",
    format: "json",
    source: { type: "json", versionPath: "metadata.version" },
  },
];

/** Auto-detect version files in the project root. Returns all found, ordered by priority. */
export function detectVersionFiles(cwd: string): VersionFile[] {
  const ecosystemFiles: VersionFile[] = [];
  let loafFile: VersionFile | null = null;

  for (const candidate of CANDIDATES) {
    const absolutePath = join(cwd, candidate.relativePath);
    if (!existsSync(absolutePath)) continue;

    try {
      const content = readFileSync(absolutePath, "utf-8");
      let version: string | undefined;

      if (candidate.source.type === "json") {
        const parsed = JSON.parse(content);
        if (candidate.source.versionPath) {
          version = candidate.source.versionPath.split(".").reduce((obj: Record<string, unknown>, key) => obj?.[key] as Record<string, unknown>, parsed) as unknown as string;
        } else {
          version = parsed.version;
        }
      } else {
        const result = readTomlVersion(content, candidate.source.section);
        if (result) version = result;
      }

      if (!version) continue;

      const file: VersionFile = {
        path: absolutePath,
        relativePath: relative(cwd, absolutePath),
        format: candidate.format,
        currentVersion: version,
      };

      if (candidate.relativePath === ".agents/loaf.json") {
        loafFile = file;
      } else {
        ecosystemFiles.push(file);
      }
    } catch {
      // File exists but can't be read/parsed — skip silently
      continue;
    }
  }

  // loaf.json is fallback — only include when no ecosystem file has a version
  if (ecosystemFiles.length === 0 && loafFile) {
    return [loafFile];
  }

  return ecosystemFiles;
}

// ─────────────────────────────────────────────────────────────────────────────
// Version Updates (non-destructive)
// ─────────────────────────────────────────────────────────────────────────────

/** Update version in all detected files. Returns [absolutePath, newContent] pairs without writing to disk. */
export function prepareVersionUpdates(
  files: VersionFile[],
  newVersion: string,
): Array<[string, string]> {
  const updates: Array<[string, string]> = [];

  for (const file of files) {
    try {
      const content = readFileSync(file.path, "utf-8");
      let updated: string;

      if (file.format === "json") {
        // Regex replacement handles both top-level and nested version fields
        updated = content.replace(
          new RegExp(`"version"(\\s*:\\s*)"${escapeRegex(file.currentVersion)}"`, "g"),
          `"version"$1"${newVersion}"`
        );
      } else {
        // Determine the TOML section from the candidate list
        const section = tomlSectionForPath(file.relativePath);
        if (!section) continue;
        updated = replaceTomlVersion(content, section, newVersion);
      }

      updates.push([file.path, updated]);
    } catch {
      // Can't read/parse — skip this file
      continue;
    }
  }

  return updates;
}

/** Map a relative file path back to its TOML section name. */
function tomlSectionForPath(relativePath: string): string | null {
  const normalized = relativePath.replace(/\\/g, "/");
  if (normalized === "pyproject.toml") return "project";
  if (normalized === "Cargo.toml") return "package";
  return null;
}
