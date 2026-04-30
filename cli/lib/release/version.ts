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

/** Options for {@link detectVersionFiles}. */
export interface DetectVersionFilesOptions {
  /** CLI override (`--version-file <path>`, repeatable). Highest precedence. */
  cliOverrides?: string[];
  /** Declared paths from `.agents/loaf.json` `release.versionFiles`. */
  configOverrides?: string[];
}

/** Infer format + TOML section (if applicable) for a declared path based on its filename. */
function classifyDeclaredPath(
  relativePath: string,
): { format: "json" | "toml-regex"; tomlSection?: string; jsonVersionPath?: string } | null {
  const normalized = relativePath.replace(/\\/g, "/");
  const basename = normalized.split("/").pop() ?? normalized;

  if (basename === "package.json") {
    return { format: "json" };
  }
  if (basename === "loaf.json") {
    return { format: "json" };
  }
  if (basename === "marketplace.json") {
    return { format: "json", jsonVersionPath: "metadata.version" };
  }
  if (basename === "pyproject.toml") {
    return { format: "toml-regex", tomlSection: "project" };
  }
  if (basename === "Cargo.toml") {
    return { format: "toml-regex", tomlSection: "package" };
  }
  // Generic JSON / TOML fallback by extension
  if (basename.endsWith(".json")) {
    return { format: "json" };
  }
  if (basename.endsWith(".toml")) {
    return { format: "toml-regex", tomlSection: "project" };
  }
  return null;
}

/**
 * Load a single declared/overridden version file. Throws with a precise
 * error if the path is missing or its version cannot be parsed.
 */
export function loadDeclaredVersionFile(
  cwd: string,
  relativePath: string,
): VersionFile {
  const normalized = relativePath.replace(/\\/g, "/");
  const absolutePath = join(cwd, normalized);

  if (!existsSync(absolutePath)) {
    throw new Error(`version file ${normalized} not found`);
  }

  const classification = classifyDeclaredPath(normalized);
  if (!classification) {
    throw new Error(
      `version file ${normalized}: unsupported file type (expected .json or .toml)`,
    );
  }

  let content: string;
  try {
    content = readFileSync(absolutePath, "utf-8");
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(`version file ${normalized}: could not read (${message})`);
  }

  let version: string | undefined;
  if (classification.format === "json") {
    let parsed: unknown;
    try {
      parsed = JSON.parse(content);
    } catch {
      throw new Error(`version file ${normalized}: could not parse version`);
    }

    if (classification.jsonVersionPath) {
      const segments = classification.jsonVersionPath.split(".");
      let cursor: unknown = parsed;
      for (const segment of segments) {
        if (cursor && typeof cursor === "object" && segment in (cursor as Record<string, unknown>)) {
          cursor = (cursor as Record<string, unknown>)[segment];
        } else {
          cursor = undefined;
          break;
        }
      }
      if (typeof cursor === "string") version = cursor;
    } else if (parsed && typeof parsed === "object" && "version" in (parsed as Record<string, unknown>)) {
      const candidate = (parsed as Record<string, unknown>).version;
      if (typeof candidate === "string") version = candidate;
    }
  } else {
    const section = classification.tomlSection ?? "project";
    const result = readTomlVersion(content, section);
    if (result) version = result;
  }

  if (!version) {
    throw new Error(`version file ${normalized}: could not parse version`);
  }

  return {
    path: absolutePath,
    relativePath: normalized,
    format: classification.format,
    currentVersion: version,
  };
}

/**
 * Resolve the set of version files to operate on.
 *
 * Two-tier resolution, declarative-first:
 *   1. `cliOverrides` (from `--version-file`) — replaces both declared and
 *      auto-detected paths for the invocation.
 *   2. `configOverrides` (from `.agents/loaf.json` `release.versionFiles`) —
 *      replaces root auto-detection.
 *   3. Fallback — root auto-detect: `package.json`, `pyproject.toml`,
 *      `Cargo.toml`, `.agents/loaf.json`, `.claude-plugin/marketplace.json`.
 *
 * For (1) and (2): every declared path must exist and contain a parseable
 * version. Any missing/malformed path throws — partial monorepo bumps are
 * worse than no bump at all.
 */
export function detectVersionFiles(
  cwd: string,
  options: DetectVersionFilesOptions = {},
): VersionFile[] {
  const cliOverrides = options.cliOverrides ?? [];
  const configOverrides = options.configOverrides ?? [];

  if (cliOverrides.length > 0) {
    return cliOverrides.map((path) => loadDeclaredVersionFile(cwd, path));
  }

  if (configOverrides.length > 0) {
    return configOverrides.map((path) => loadDeclaredVersionFile(cwd, path));
  }

  return detectVersionFilesFromRoot(cwd);
}

/** Root-only auto-detect (the original behavior). */
function detectVersionFilesFromRoot(cwd: string): VersionFile[] {
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
  const basename = normalized.split("/").pop() ?? normalized;
  if (basename === "pyproject.toml") return "project";
  if (basename === "Cargo.toml") return "package";
  return null;
}
