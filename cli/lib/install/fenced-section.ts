/**
 * Fenced-Section Management for CLAUDE.md/AGENTS.md
 *
 * Installs and upgrades Loaf framework conventions into user project instruction files.
 * Uses HTML comment markers to delimit managed content.
 */

import { readFileSync, writeFileSync, existsSync, mkdirSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";

const FENCED_START = "<!-- loaf:managed:start";
const FENCED_END = "<!-- loaf:managed:end -->";
const FENCED_WARNING = "<!-- Maintained by loaf install/upgrade — do not edit manually -->";

/** Per-target file paths relative to project root */
const TARGET_FILES: Record<string, string> = {
  "claude-code": ".claude/CLAUDE.md",
  cursor: ".cursor/rules/loaf.mdc",  // Always use .mdc for Cursor rules
  codex: ".agents/AGENTS.md",
  opencode: ".agents/AGENTS.md",
  amp: ".agents/AGENTS.md",
  // NOTE: gemini excluded - per SPEC-020, Gemini has no prompt overlay/project-file layer
};

interface FencedSection {
  startIndex: number;
  endIndex: number;
  version: string | null;
  content: string;
}

function getVersion(): string {
  const __dirname = dirname(fileURLToPath(import.meta.url));
  for (const candidate of [
    join(__dirname, "..", "..", "..", "package.json"),
    join(__dirname, "..", "..", "package.json"),
    join(__dirname, "..", "package.json"),
  ]) {
    try {
      const pkg = JSON.parse(readFileSync(candidate, "utf-8"));
      if (pkg.name === "loaf") return pkg.version;
    } catch {
      continue;
    }
  }
  return "0.0.0";
}

function findFencedSection(content: string): FencedSection | null {
  const startIdx = content.indexOf(FENCED_START);
  if (startIdx === -1) return null;

  const endIdx = content.indexOf(FENCED_END, startIdx);
  if (endIdx === -1) return null;

  // Extract version from start marker
  // Format: <!-- loaf:managed:start v2.1.0 -->
  const startLine = content.substring(
    startIdx,
    content.indexOf("-->", startIdx) + 3
  );
  const versionMatch = startLine.match(/v([\d.]+(?:-[-\w.]+)?)/);
  const version = versionMatch ? versionMatch[1] : null;

  return {
    startIndex: startIdx,
    endIndex: endIdx + FENCED_END.length,
    version,
    content: content.substring(startIdx, endIdx + FENCED_END.length),
  };
}

function generateFencedContent(version: string): string {
  return [
    `<!-- loaf:managed:start v${version} -->`,
    FENCED_WARNING,
    "## Loaf Framework",
    "",
    "**Session Journal Entry Types:**",
    "- `decision(scope)`: Key decisions with rationale",
    "- `discover(scope)`: Something learned",
    "- `block(scope)` / `unblock(scope)`: Blockers and resolutions",
    "- `spark(scope)`: Ideas to promote via `/idea`",
    "- `todo(scope)`: Action items to promote to tasks",
    "",
    "**CLI Commands:**",
    "- `loaf session start/end/log/archive` — Session management",
    "- `loaf check` — Run enforcement hooks",
    "- `loaf task/spec/kb` — Task and knowledge management",
    "",
    "**Journal Discipline:**",
    "Before completing any response that includes edits, commits, or significant decisions, log journal entries using `loaf session log \"type(scope): description\"`. Entry types: `decision`, `discover`, `conclude`. Do not defer journaling — log before responding.",
    "",
    "See [orchestration skill](skills/orchestration/SKILL.md) for full details.",
    "<!-- loaf:managed:end -->",
  ].join("\n");
}

function resolveTargetFile(
  target: string,
  projectRoot: string
): string | null {
  const targetPath = TARGET_FILES[target];
  if (!targetPath) return null;
  return join(projectRoot, targetPath);
}

/**
 * Install or upgrade the fenced section in the target file.
 * @param targetFile - Path to the target file (CLAUDE.md/AGENTS.md)
 * @param upgrade - If true, only upgrade if version differs; if false, always install
 * @returns Object indicating what action was taken
 */
export function installFencedSection(
  targetFile: string,
  upgrade: boolean = false
): { action: "created" | "updated" | "skipped" | "appended"; version: string } {
  const currentVersion = getVersion();
  let fileContent = "";
  let fileExisted = false;

  try {
    fileContent = readFileSync(targetFile, "utf-8");
    fileExisted = true;
  } catch {
    // File doesn't exist, will create
  }

  const fencedSection = findFencedSection(fileContent);

  if (fencedSection) {
    // Existing fenced section found
    if (upgrade && fencedSection.version === currentVersion) {
      // Version matches, skip
      return { action: "skipped", version: currentVersion };
    }

    // Replace existing section
    const before = fileContent.substring(0, fencedSection.startIndex);
    const after = fileContent.substring(fencedSection.endIndex);
    const newContent = generateFencedContent(currentVersion);
    const trimmedBefore = before.trimEnd();
    const trimmedAfter = after.trimStart();

    fileContent =
      trimmedBefore +
      (trimmedBefore ? "\n\n" : "") +
      newContent +
      (trimmedAfter ? "\n\n" : "\n") +
      trimmedAfter;

    writeFileSync(targetFile, fileContent);
    return { action: "updated", version: currentVersion };
    } else {
      // No fenced section found
      const newContent = generateFencedContent(currentVersion);

      if (fileExisted) {
        // Append to existing file
        const trimmedContent = fileContent.trimEnd();
        fileContent =
          trimmedContent + (trimmedContent ? "\n\n" : "") + newContent + "\n";
        writeFileSync(targetFile, fileContent);
        return { action: "appended", version: currentVersion };
      } else {
        // Create new file with fenced section
        // Ensure directory exists
        mkdirSync(dirname(targetFile), { recursive: true });
        
        // Add frontmatter for .mdc files (Cursor rules format)
        const isMdc = targetFile.endsWith(".mdc");
        const prefix = isMdc
          ? "---\ndescription: Loaf framework conventions\nalwaysApply: true\n---\n\n"
          : "";
        
        writeFileSync(targetFile, prefix + newContent + "\n");
        return { action: "created", version: currentVersion };
      }
    }
}

/**
 * Get the appropriate target file for a tool in a project.
 * @param target - Tool key (e.g., 'opencode', 'cursor', 'codex')
 * @param projectRoot - Project root directory (default: process.cwd())
 * @returns Full path to the target file, or null if target unknown
 */
export function getTargetFile(
  target: string,
  projectRoot: string = process.cwd()
): string | null {
  return resolveTargetFile(target, projectRoot);
}

/**
 * Check if a fenced section exists and what version it has.
 * @param targetFile - Path to the target file
 * @returns Version string if fenced section exists, null otherwise
 */
export function getFencedVersion(targetFile: string): string | null {
  try {
    const content = readFileSync(targetFile, "utf-8");
    const section = findFencedSection(content);
    return section?.version ?? null;
  } catch {
    return null;
  }
}

/**
 * Install fenced sections for all applicable targets in a project.
 * @param targets - List of target keys to install
 * @param projectRoot - Project root directory (default: process.cwd())
 * @param upgrade - Whether to run in upgrade mode
 * @returns Map of target -> result for each processed target
 */
export function installFencedSectionsForTargets(
  targets: string[],
  projectRoot: string = process.cwd(),
  upgrade: boolean = false
): Record<
  string,
  { action: "created" | "updated" | "skipped" | "appended" | "error"; version?: string; error?: string }
> {
  const results: Record<
    string,
    { action: "created" | "updated" | "skipped" | "appended" | "error"; version?: string; error?: string }
  > = {};

  for (const target of targets) {
    const targetFile = resolveTargetFile(target, projectRoot);

    if (!targetFile) {
      results[target] = { action: "error", error: `Unknown target: ${target}` };
      continue;
    }

    try {
      const result = installFencedSection(targetFile, upgrade);
      results[target] = result;
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      results[target] = { action: "error", error: msg };
    }
  }

  return results;
}
