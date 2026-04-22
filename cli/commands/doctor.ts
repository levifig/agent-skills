/**
 * loaf doctor command
 *
 * Run alignment diagnostics on a Loaf project and report misalignments.
 * Covers symlink health, stale files, fenced-section version drift, and
 * canonical-file presence. Designed as an extensible Check registry — new
 * diagnostics drop in by adding a Check object to the registry.
 *
 * Exit codes:
 *   0 = all checks passed (warnings allowed)
 *   1 = one or more checks failed
 *
 * Usage:
 *   loaf doctor                 # Run all checks in the current project
 *   loaf doctor --verbose       # Print each check name even when passing
 *   loaf doctor --fix           # Apply safe auto-fixes (symlinks, stale files)
 */

import { Command } from "commander";
import {
  existsSync,
  lstatSync,
  mkdirSync,
  readFileSync,
  readlinkSync,
  renameSync,
  rmSync,
  symlinkSync,
} from "fs";
import { dirname, join, relative } from "path";
import { fileURLToPath } from "url";

import { getFencedVersion } from "../lib/install/fenced-section.js";
import {
  mergeContentIntoCanonical,
  stripLoafFence,
} from "../lib/install/symlinks.js";

// ─────────────────────────────────────────────────────────────────────────────
// ANSI color helpers
// ─────────────────────────────────────────────────────────────────────────────

const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const red = (s: string) => `\x1b[31m${s}\x1b[0m`;
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;
const cyan = (s: string) => `\x1b[36m${s}\x1b[0m`;

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

/** Overall outcome of a single diagnostic check. */
export type CheckStatus = "pass" | "warn" | "fail" | "skip";

/** Result returned by a check's run() function. */
export interface CheckResult {
  status: CheckStatus;
  /** Short, single-line message shown next to the status glyph. */
  message: string;
  /** Optional multi-line detail, printed indented beneath the message. */
  detail?: string;
  /** If true and --fix is passed, fix() will be attempted. */
  fixable?: boolean;
}

/** Result returned by a check's fix() function. */
export interface FixResult {
  /** True if the fix ran and succeeded. */
  fixed: boolean;
  /** Short description of what the fix did (or why it didn't). */
  message: string;
}

/** Runtime context passed to every check and fix. */
export interface CheckContext {
  /** Absolute path to the project root (usually process.cwd()). */
  projectRoot: string;
}

/**
 * A diagnostic check. Keep run() pure (read-only, no writes). Put any
 * repair logic in fix(). New checks drop in by adding an entry to CHECKS.
 */
export interface Check {
  /** Machine-readable identifier (kebab-case). */
  name: string;
  /** One-line human description for --verbose output. */
  description: string;
  /** Execute the diagnostic; return pass/warn/fail/skip. */
  run(ctx: CheckContext): CheckResult;
  /** Optional repair. Only invoked when --fix is set and run() returned fail. */
  fix?(ctx: CheckContext, last: CheckResult): FixResult;
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

/** Resolve the loaf CLI's own version from its bundled package.json. */
function getCliVersion(): string {
  const here = dirname(fileURLToPath(import.meta.url));
  for (const candidate of [
    join(here, "..", "package.json"),
    join(here, "..", "..", "package.json"),
    join(here, "..", "..", "..", "package.json"),
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

/** True if `path` is a symlink. Returns false for missing paths. */
function isSymlink(path: string): boolean {
  try {
    return lstatSync(path).isSymbolicLink();
  } catch {
    return false;
  }
}

/** True if `path` exists on disk (as file, dir, or symlink target). */
function pathExists(path: string): boolean {
  try {
    lstatSync(path);
    return true;
  } catch {
    return false;
  }
}

/**
 * Read a symlink target and normalise it to an absolute path.
 * Returns null if the symlink can't be read.
 */
function resolveSymlink(linkPath: string): string | null {
  try {
    const target = readlinkSync(linkPath);
    if (target.startsWith("/")) return target;
    return join(dirname(linkPath), target);
  } catch {
    return null;
  }
}

/**
 * Check whether a symlink at `linkPath` resolves to `expectedAbs`.
 * Comparison is done on normalised absolute paths.
 */
function symlinkPointsTo(linkPath: string, expectedAbs: string): boolean {
  const resolved = resolveSymlink(linkPath);
  if (!resolved) return false;
  // Normalise both sides via join to collapse any "./" or trailing slashes.
  return join(resolved) === join(expectedAbs);
}

/** Read file contents, returning null on any error. */
function safeRead(path: string): string | null {
  try {
    return readFileSync(path, "utf-8");
  } catch {
    return null;
  }
}

/** True if the given file contains a loaf:managed fenced section. */
function hasFencedSection(path: string): boolean {
  const content = safeRead(path);
  if (content === null) return false;
  return (
    content.includes("<!-- loaf:managed:start") &&
    content.includes("<!-- loaf:managed:end -->")
  );
}

/**
 * Ensure the parent directory for `filePath` exists.
 * Returns true if the directory was created or already exists.
 */
function ensureParentDir(filePath: string): boolean {
  try {
    mkdirSync(dirname(filePath), { recursive: true });
    return true;
  } catch {
    return false;
  }
}

/**
 * Migrate a real file at `linkPath` into the canonical file and replace
 * the source with a symlink. Mirrors the safe path in `ensureSymlink`:
 *   1. Strip any Loaf-managed fence from the source.
 *   2. Append stripped content to canonical under `## Migrated from <rel>`
 *      (or create canonical with that content if it's absent).
 *   3. Rename the source to `<source>.bak` — never destructive.
 *   4. Create the symlink in place of the source.
 *
 * Used by doctor's --fix path to turn real-file duplication into a safe
 * symlinked layout without losing user content.
 */
function migrateRealFileToSymlink(params: {
  linkPath: string;
  canonicalPath: string;
  relativeTarget: string;
  projectRoot: string;
}): FixResult {
  const { linkPath, canonicalPath, relativeTarget, projectRoot } = params;

  try {
    const sourceContent = readFileSync(linkPath, "utf-8");
    const stripped = stripLoafFence(sourceContent);

    // Heading uses a path relative to the project root for readability.
    const relSource = relative(projectRoot, linkPath) || linkPath;
    let merged = false;
    if (stripped.length > 0) {
      merged = mergeContentIntoCanonical(canonicalPath, stripped, relSource);
    }

    const backupPath = `${linkPath}.bak`;
    if (pathExists(backupPath)) {
      rmSync(backupPath, { force: true, recursive: true });
    }
    renameSync(linkPath, backupPath);

    if (!ensureParentDir(linkPath)) {
      return {
        fixed: false,
        message: "Could not prepare parent directory",
      };
    }
    symlinkSync(relativeTarget, linkPath);

    const suffix = merged ? " (merged content into canonical)" : "";
    return {
      fixed: true,
      message:
        `Migrated ${relSource} -> ${relativeTarget}, backup at ` +
        `${relSource}.bak${suffix}`,
    };
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    return { fixed: false, message: `Migration failed: ${msg}` };
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Check: agents-symlink
// Canonical: /AGENTS.md is a symlink to .agents/AGENTS.md
// ─────────────────────────────────────────────────────────────────────────────

const checkAgentsSymlink: Check = {
  name: "agents-symlink",
  description: "./AGENTS.md is a symlink to .agents/AGENTS.md",

  run(ctx): CheckResult {
    const linkPath = join(ctx.projectRoot, "AGENTS.md");
    const canonical = join(ctx.projectRoot, ".agents", "AGENTS.md");

    if (!existsSync(canonical)) {
      // Nothing to link to — skip (a separate check flags missing canonical).
      return {
        status: "skip",
        message: "No .agents/AGENTS.md to link to",
      };
    }

    if (!pathExists(linkPath)) {
      return {
        status: "fail",
        message: "./AGENTS.md is missing",
        detail: `Expected symlink at ${linkPath} → .agents/AGENTS.md`,
        fixable: true,
      };
    }

    if (!isSymlink(linkPath)) {
      return {
        status: "fail",
        message: "./AGENTS.md exists but is not a symlink",
        detail:
          "Duplicate content risks drift from .agents/AGENTS.md. " +
          "Run `loaf doctor --fix` to merge its content into canonical, " +
          "back up as ./AGENTS.md.bak, and replace with a symlink.",
        // Fixable: we merge content + .bak + symlink (never destructive).
        fixable: true,
      };
    }

    if (!symlinkPointsTo(linkPath, canonical)) {
      const actual = resolveSymlink(linkPath) ?? "<unreadable>";
      return {
        status: "fail",
        message: "./AGENTS.md points to the wrong target",
        detail: `Got: ${actual}\nWant: ${canonical}`,
        fixable: true,
      };
    }

    return {
      status: "pass",
      message: "./AGENTS.md → .agents/AGENTS.md",
    };
  },

  fix(ctx): FixResult {
    const linkPath = join(ctx.projectRoot, "AGENTS.md");
    const canonical = join(ctx.projectRoot, ".agents", "AGENTS.md");

    if (!existsSync(canonical)) {
      return { fixed: false, message: "Cannot fix — canonical .agents/AGENTS.md missing" };
    }

    // Real-file path: safe migration (strip fence → merge → .bak → symlink).
    if (pathExists(linkPath) && !isSymlink(linkPath)) {
      const relTarget = relative(dirname(linkPath), canonical);
      return migrateRealFileToSymlink({
        linkPath,
        canonicalPath: canonical,
        relativeTarget: relTarget,
        projectRoot: ctx.projectRoot,
      });
    }

    try {
      if (isSymlink(linkPath)) {
        rmSync(linkPath, { force: true });
      }
      if (!ensureParentDir(linkPath)) {
        return { fixed: false, message: "Could not prepare parent directory" };
      }
      // Use a project-relative target for readability.
      const relTarget = relative(dirname(linkPath), canonical);
      symlinkSync(relTarget, linkPath);
      return { fixed: true, message: `Created ./AGENTS.md → ${relTarget}` };
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      return { fixed: false, message: `Symlink failed: ${msg}` };
    }
  },
};

// ─────────────────────────────────────────────────────────────────────────────
// Check: claude-symlink
// Canonical: /.claude/CLAUDE.md is a symlink to .agents/AGENTS.md
// ─────────────────────────────────────────────────────────────────────────────

const checkClaudeSymlink: Check = {
  name: "claude-symlink",
  description: ".claude/CLAUDE.md is a symlink to .agents/AGENTS.md",

  run(ctx): CheckResult {
    const linkPath = join(ctx.projectRoot, ".claude", "CLAUDE.md");
    const canonical = join(ctx.projectRoot, ".agents", "AGENTS.md");

    if (!existsSync(canonical)) {
      return {
        status: "skip",
        message: "No .agents/AGENTS.md to link to",
      };
    }

    if (!pathExists(linkPath)) {
      return {
        status: "fail",
        message: ".claude/CLAUDE.md is missing",
        detail: `Expected symlink at ${linkPath} → .agents/AGENTS.md`,
        fixable: true,
      };
    }

    if (!isSymlink(linkPath)) {
      return {
        status: "fail",
        message: ".claude/CLAUDE.md exists but is not a symlink",
        detail:
          "Duplicate content risks drift from .agents/AGENTS.md. " +
          "Run `loaf doctor --fix` to merge its content into canonical, " +
          "back up as .claude/CLAUDE.md.bak, and replace with a symlink.",
        fixable: true,
      };
    }

    if (!symlinkPointsTo(linkPath, canonical)) {
      const actual = resolveSymlink(linkPath) ?? "<unreadable>";
      return {
        status: "fail",
        message: ".claude/CLAUDE.md points to the wrong target",
        detail: `Got: ${actual}\nWant: ${canonical}`,
        fixable: true,
      };
    }

    return {
      status: "pass",
      message: ".claude/CLAUDE.md → .agents/AGENTS.md",
    };
  },

  fix(ctx): FixResult {
    const linkPath = join(ctx.projectRoot, ".claude", "CLAUDE.md");
    const canonical = join(ctx.projectRoot, ".agents", "AGENTS.md");

    if (!existsSync(canonical)) {
      return { fixed: false, message: "Cannot fix — canonical .agents/AGENTS.md missing" };
    }

    // Real-file path: safe migration (strip fence → merge → .bak → symlink).
    if (pathExists(linkPath) && !isSymlink(linkPath)) {
      const relTarget = relative(dirname(linkPath), canonical);
      return migrateRealFileToSymlink({
        linkPath,
        canonicalPath: canonical,
        relativeTarget: relTarget,
        projectRoot: ctx.projectRoot,
      });
    }

    try {
      if (isSymlink(linkPath)) {
        rmSync(linkPath, { force: true });
      }
      if (!ensureParentDir(linkPath)) {
        return { fixed: false, message: "Could not prepare parent directory" };
      }
      const relTarget = relative(dirname(linkPath), canonical);
      symlinkSync(relTarget, linkPath);
      return { fixed: true, message: `Created .claude/CLAUDE.md → ${relTarget}` };
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      return { fixed: false, message: `Symlink failed: ${msg}` };
    }
  },
};

// ─────────────────────────────────────────────────────────────────────────────
// Check: canonical-agents-file
// The canonical .agents/AGENTS.md must exist when symlinks reference it.
// ─────────────────────────────────────────────────────────────────────────────

const checkCanonicalAgentsFile: Check = {
  name: "canonical-agents-file",
  description: ".agents/AGENTS.md exists when referenced by symlinks",

  run(ctx): CheckResult {
    const canonical = join(ctx.projectRoot, ".agents", "AGENTS.md");
    const agentsLink = join(ctx.projectRoot, "AGENTS.md");
    const claudeLink = join(ctx.projectRoot, ".claude", "CLAUDE.md");

    const canonicalExists = existsSync(canonical);

    // We only care when something is pointing at .agents/AGENTS.md.
    const referenced =
      (isSymlink(agentsLink) &&
        symlinkPointsTo(agentsLink, canonical)) ||
      (isSymlink(claudeLink) &&
        symlinkPointsTo(claudeLink, canonical));

    if (!referenced) {
      return {
        status: "skip",
        message: "No symlinks reference .agents/AGENTS.md",
      };
    }

    if (!canonicalExists) {
      return {
        status: "fail",
        message: ".agents/AGENTS.md is missing but referenced by symlinks",
        detail: `Dangling symlinks point at ${canonical}`,
      };
    }

    return {
      status: "pass",
      message: ".agents/AGENTS.md is present",
    };
  },
};

// ─────────────────────────────────────────────────────────────────────────────
// Check: stale-cursor-mdc
// .cursor/rules/loaf.mdc is a migration byproduct; remove it.
// ─────────────────────────────────────────────────────────────────────────────

const checkStaleCursorMdc: Check = {
  name: "stale-cursor-mdc",
  description: "No stale .cursor/rules/loaf.mdc left over from legacy installs",

  run(ctx): CheckResult {
    const stale = join(ctx.projectRoot, ".cursor", "rules", "loaf.mdc");
    if (!pathExists(stale)) {
      return {
        status: "pass",
        message: "No stale .cursor/rules/loaf.mdc",
      };
    }
    return {
      status: "fail",
      message: "Stale .cursor/rules/loaf.mdc should be removed",
      detail:
        "Cursor now uses .agents/AGENTS.md via the consolidated prompt overlay.",
      fixable: true,
    };
  },

  fix(ctx): FixResult {
    const stale = join(ctx.projectRoot, ".cursor", "rules", "loaf.mdc");
    if (!pathExists(stale)) {
      return { fixed: false, message: "Nothing to remove" };
    }
    try {
      rmSync(stale, { force: true });
      return { fixed: true, message: "Removed .cursor/rules/loaf.mdc" };
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      return { fixed: false, message: `Delete failed: ${msg}` };
    }
  },
};

// ─────────────────────────────────────────────────────────────────────────────
// Check: fenced-version
// The fenced section version matches the installed loaf CLI version.
// ─────────────────────────────────────────────────────────────────────────────

const checkFencedVersion: Check = {
  name: "fenced-version",
  description: "Fenced section version matches installed loaf version",

  run(ctx): CheckResult {
    const canonical = join(ctx.projectRoot, ".agents", "AGENTS.md");
    if (!existsSync(canonical)) {
      return {
        status: "skip",
        message: "No .agents/AGENTS.md to inspect",
      };
    }

    const fencedVersion = getFencedVersion(canonical);
    if (fencedVersion === null) {
      // No fenced section at all — treat as a warning so doctor still exits 0.
      return {
        status: "warn",
        message: "No loaf:managed fenced section found in .agents/AGENTS.md",
        detail: "Run `loaf install` to add the framework section.",
      };
    }

    const cliVersion = getCliVersion();
    if (fencedVersion !== cliVersion) {
      return {
        status: "warn",
        message: `Fenced section version drift: ${fencedVersion} (installed: ${cliVersion})`,
        detail: "Run `loaf install --upgrade` to refresh the fenced section.",
      };
    }

    return {
      status: "pass",
      message: `Fenced section is v${fencedVersion} (matches installed loaf)`,
    };
  },
};

// ─────────────────────────────────────────────────────────────────────────────
// Check: duplicate-fenced-sections
// Warn when both .claude/CLAUDE.md and .agents/AGENTS.md are real files
// carrying a fenced section — that means drift will occur on upgrade.
// ─────────────────────────────────────────────────────────────────────────────

const checkDuplicateFencedSections: Check = {
  name: "duplicate-fenced-sections",
  description: "No duplicate fenced sections across real files",

  run(ctx): CheckResult {
    const claudePath = join(ctx.projectRoot, ".claude", "CLAUDE.md");
    const agentsPath = join(ctx.projectRoot, ".agents", "AGENTS.md");

    // Only flag when BOTH are real files (not symlinks). If .claude/CLAUDE.md
    // is a symlink, there is no duplication risk.
    const claudeIsRealFile =
      pathExists(claudePath) && !isSymlink(claudePath);
    const agentsIsRealFile =
      pathExists(agentsPath) && !isSymlink(agentsPath);

    if (!claudeIsRealFile || !agentsIsRealFile) {
      return {
        status: "skip",
        message: "No duplication risk — at least one side is absent or a symlink",
      };
    }

    const claudeHasFenced = hasFencedSection(claudePath);
    const agentsHasFenced = hasFencedSection(agentsPath);

    if (claudeHasFenced && agentsHasFenced) {
      return {
        status: "fail",
        message:
          "Duplicate fenced sections in both .claude/CLAUDE.md and .agents/AGENTS.md",
        detail:
          "These will drift on upgrade. Run `loaf doctor --fix` to merge " +
          ".claude/CLAUDE.md content into .agents/AGENTS.md, back it up, " +
          "and replace it with a symlink. No content is deleted.",
        fixable: true,
      };
    }

    return {
      status: "pass",
      message: "No duplicate fenced sections across real files",
    };
  },

  fix(ctx): FixResult {
    const claudePath = join(ctx.projectRoot, ".claude", "CLAUDE.md");
    const canonical = join(ctx.projectRoot, ".agents", "AGENTS.md");

    if (!existsSync(canonical) || !pathExists(claudePath) || isSymlink(claudePath)) {
      return {
        fixed: false,
        message: "State no longer matches — re-run doctor",
      };
    }

    const relTarget = relative(dirname(claudePath), canonical);
    return migrateRealFileToSymlink({
      linkPath: claudePath,
      canonicalPath: canonical,
      relativeTarget: relTarget,
      projectRoot: ctx.projectRoot,
    });
  },
};

// ─────────────────────────────────────────────────────────────────────────────
// Registry — drop new checks in here.
// Order matters only for report readability.
// ─────────────────────────────────────────────────────────────────────────────

export const CHECKS: Check[] = [
  checkCanonicalAgentsFile,
  checkAgentsSymlink,
  checkClaudeSymlink,
  checkStaleCursorMdc,
  checkFencedVersion,
  checkDuplicateFencedSections,
];

// ─────────────────────────────────────────────────────────────────────────────
// Reporting
// ─────────────────────────────────────────────────────────────────────────────

const STATUS_GLYPH: Record<CheckStatus, string> = {
  pass: green("✓"),
  warn: yellow("⚠"),
  fail: red("✗"),
  skip: gray("-"),
};

interface RunReport {
  passes: number;
  warnings: number;
  failures: number;
  skips: number;
  fixesApplied: number;
  fixesFailed: number;
}

function printCheckLine(
  check: Check,
  result: CheckResult,
  options: { verbose: boolean },
): void {
  const glyph = STATUS_GLYPH[result.status];
  const showName = result.status !== "pass" || options.verbose;
  const label = showName ? `${bold(check.name)} — ${result.message}` : result.message;
  console.log(`  ${glyph} ${label}`);

  if (result.detail && (result.status === "fail" || result.status === "warn")) {
    for (const line of result.detail.split("\n")) {
      console.log(`    ${gray(line)}`);
    }
  }
}

function printFixLine(fix: FixResult): void {
  const glyph = fix.fixed ? green("→") : yellow("→");
  console.log(`    ${glyph} ${fix.message}`);
}

function printSummary(report: RunReport): void {
  const parts: string[] = [];
  parts.push(`${green(`${report.passes} passed`)}`);
  if (report.warnings > 0) parts.push(yellow(`${report.warnings} warning`));
  if (report.failures > 0) parts.push(red(`${report.failures} failed`));
  if (report.skips > 0) parts.push(gray(`${report.skips} skipped`));

  const prefix = report.failures > 0 ? red("✗") : report.warnings > 0 ? yellow("⚠") : green("✓");
  console.log();
  console.log(`  ${prefix} ${parts.join(gray(" · "))}`);

  if (report.fixesApplied > 0 || report.fixesFailed > 0) {
    const fixParts: string[] = [];
    if (report.fixesApplied > 0) fixParts.push(green(`${report.fixesApplied} fixed`));
    if (report.fixesFailed > 0) fixParts.push(red(`${report.fixesFailed} could not be fixed`));
    console.log(`  ${cyan("→")} ${fixParts.join(gray(" · "))}`);
  }
  console.log();
}

// ─────────────────────────────────────────────────────────────────────────────
// Runner
// ─────────────────────────────────────────────────────────────────────────────

export interface DoctorRunOptions {
  fix: boolean;
  verbose: boolean;
  projectRoot: string;
}

export interface DoctorRunResult {
  report: RunReport;
  exitCode: number;
}

/**
 * Execute every registered check and, if requested, apply fixes.
 * Returns the summarised report and a recommended exit code.
 */
export function runDoctor(options: DoctorRunOptions): DoctorRunResult {
  const ctx: CheckContext = { projectRoot: options.projectRoot };

  const report: RunReport = {
    passes: 0,
    warnings: 0,
    failures: 0,
    skips: 0,
    fixesApplied: 0,
    fixesFailed: 0,
  };

  console.log(`\n${bold("loaf doctor")}\n`);

  for (const check of CHECKS) {
    let result: CheckResult;
    try {
      result = check.run(ctx);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      result = {
        status: "fail",
        message: `Check threw an error: ${msg}`,
      };
    }

    printCheckLine(check, result, { verbose: options.verbose });

    // Apply fix if requested, the check failed, and it's fixable.
    if (
      options.fix &&
      result.status === "fail" &&
      result.fixable &&
      typeof check.fix === "function"
    ) {
      let fix: FixResult;
      try {
        fix = check.fix(ctx, result);
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        fix = { fixed: false, message: `Fix threw an error: ${msg}` };
      }
      printFixLine(fix);
      if (fix.fixed) {
        report.fixesApplied += 1;
        // Re-run the check to confirm the fix stuck, and reflect the new
        // status in the tally so the summary isn't misleading.
        let recheck: CheckResult;
        try {
          recheck = check.run(ctx);
        } catch (err) {
          const msg = err instanceof Error ? err.message : String(err);
          recheck = {
            status: "fail",
            message: `Re-check threw an error: ${msg}`,
          };
        }
        // Tally the post-fix status instead of the original failure.
        tally(recheck.status, report);
        continue;
      } else {
        report.fixesFailed += 1;
      }
    }

    tally(result.status, report);
  }

  printSummary(report);

  // Warnings do not fail; only "fail" statuses do.
  const exitCode = report.failures > 0 ? 1 : 0;
  return { report, exitCode };
}

function tally(status: CheckStatus, report: RunReport): void {
  switch (status) {
    case "pass":
      report.passes += 1;
      break;
    case "warn":
      report.warnings += 1;
      break;
    case "fail":
      report.failures += 1;
      break;
    case "skip":
      report.skips += 1;
      break;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Command Registration
// ─────────────────────────────────────────────────────────────────────────────

export function registerDoctorCommand(program: Command): void {
  program
    .command("doctor")
    .description("Diagnose Loaf project alignment (symlinks, stale files, version drift)")
    .option("--fix", "Apply safe auto-fixes for failing checks")
    .option("--verbose", "Print each check name even when passing")
    .action((options: { fix?: boolean; verbose?: boolean }) => {
      try {
        const { exitCode } = runDoctor({
          fix: options.fix ?? false,
          verbose: options.verbose ?? false,
          projectRoot: process.cwd(),
        });
        process.exit(exitCode);
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        console.error(`${red("error:")} doctor failed: ${msg}`);
        process.exit(1);
      }
    });
}
