/**
 * loaf release command
 *
 * Orchestrates the full release workflow: analyze commits, generate changelog,
 * bump versions, build, tag, and create GitHub release draft.
 */

import { Command } from "commander";
import { execFileSync } from "child_process";
import {
  existsSync,
  readFileSync,
  writeFileSync,
  readdirSync,
  mkdtempSync,
  unlinkSync,
} from "fs";
import { join } from "path";
import { tmpdir } from "os";
import { createInterface } from "readline";

import { askYesNo } from "../lib/prompts.js";
import { getLastTag, getCommitsSince, suggestBump } from "../lib/release/commits.js";
import type { ParsedCommit } from "../lib/release/commits.js";
import {
  detectVersionFiles,
  bumpVersion,
  prepareVersionUpdates,
  parseSemVer,
} from "../lib/release/version.js";
import type { VersionFile, BumpType } from "../lib/release/version.js";
import {
  generateChangelogSection,
  buildChangelogSectionFromEntries,
  extractUnreleasedBody,
  insertIntoChangelog,
  createChangelog,
} from "../lib/release/changelog.js";
import {
  validateBumpType,
  validateBaseRef,
  normalizeSkipFlags,
} from "../lib/release/options.js";
import type { ReleaseOptions } from "../lib/release/options.js";
import { resolveBaseBranch } from "../lib/release/base.js";
import {
  checkPostMergeGuardrails,
  executePostMergeActions,
} from "../lib/release/post-merge.js";
import type { PostMergeLogger } from "../lib/release/post-merge.js";
import { readLoafConfig } from "../lib/config/agents-config.js";

// ANSI color helpers
const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const red = (s: string) => `\x1b[31m${s}\x1b[0m`;
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;
const cyan = (s: string) => `\x1b[36m${s}\x1b[0m`;

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

async function askChoice(
  question: string,
  options: BumpType[],
  defaultChoice: BumpType,
): Promise<BumpType> {
  if (!process.stdin.isTTY) {
    return defaultChoice;
  }

  const rl = createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  return new Promise((resolve) => {
    let resolved = false;
    rl.on("close", () => {
      if (!resolved) {
        resolved = true;
        resolve(defaultChoice);
      }
    });
    rl.question(question, (answer) => {
      resolved = true;
      rl.close();
      const num = parseInt(answer.trim(), 10);
      if (num >= 1 && num <= options.length) {
        resolve(options[num - 1]);
      } else {
        resolve(defaultChoice);
      }
    });
  });
}

function isGitRepo(cwd: string): boolean {
  try {
    execFileSync("git", ["rev-parse", "--is-inside-work-tree"], {
      cwd,
      encoding: "utf-8",
      stdio: ["ignore", "pipe", "ignore"],
    });
    return true;
  } catch {
    return false;
  }
}

function isGhAvailable(): boolean {
  try {
    execFileSync("which", ["gh"], {
      encoding: "utf-8",
      stdio: ["ignore", "pipe", "ignore"],
    });
    return true;
  } catch {
    return false;
  }
}

/** Get the current branch name (`git symbolic-ref --short HEAD`) or null if detached. */
function getCurrentBranch(cwd: string): string | null {
  try {
    return execFileSync("git", ["symbolic-ref", "--short", "HEAD"], {
      cwd,
      encoding: "utf-8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
  } catch {
    return null;
  }
}

interface IncompleteTask {
  filename: string;
  status: string;
}

function scanIncompleteTasks(cwd: string): IncompleteTask[] {
  const tasksDir = join(cwd, ".agents", "tasks");
  if (!existsSync(tasksDir)) return [];

  const incomplete: IncompleteTask[] = [];

  try {
    const files = readdirSync(tasksDir).filter((f) => f.endsWith(".md"));

    for (const file of files) {
      try {
        const content = readFileSync(join(tasksDir, file), "utf-8");
        const lines = content.split("\n").slice(0, 20);

        for (const line of lines) {
          const match = line.match(/^status:\s*(.+)/);
          if (match) {
            const status = match[1].trim();
            if (status !== "complete" && status !== "archived") {
              incomplete.push({ filename: file, status });
            }
            break;
          }
        }
      } catch {
        // Can't read file — skip
        continue;
      }
    }
  } catch {
    // Can't read directory — skip
  }

  return incomplete;
}

function getEditor(): string | null {
  return process.env.VISUAL || process.env.EDITOR || null;
}

/** Commander collector for repeatable string options. */
function collect(value: string, previous: string[]): string[] {
  return previous.concat([value]);
}

// ─────────────────────────────────────────────────────────────────────────────
// Command
// ─────────────────────────────────────────────────────────────────────────────

export function registerReleaseCommand(program: Command): void {
  const releaseCmd = program
    .command("release")
    .description("Create a new release with changelog, version bump, and tag")
    .option("--dry-run", "Preview release without making changes")
    .option("--bump <type>", "Skip interactive bump choice (prerelease, release, major, minor, patch)")
    .option("--base <ref>", "Use commits since <ref> instead of last tag (e.g. main)")
    .option("--no-tag", "Skip git tag creation")
    .option("--tag", "Force git tag creation (overrides --pre-merge default)")
    .option("--no-gh", "Skip GitHub release draft")
    .option("--gh", "Force GitHub release draft (overrides --pre-merge default)")
    .option(
      "--version-file <path>",
      "Override version file path (repeatable). Replaces .agents/loaf.json release.versionFiles and root auto-detection.",
      collect,
      [] as string[],
    )
    .option(
      "--pre-merge",
      "Shortcut for --no-tag --no-gh --base <auto-detected>. Auto-detects base via 4-step priority: --base flag > open PR base > git config loaf.release.base > default branch.",
    )
    .option(
      "--post-merge",
      "Finalize release after squash-merge: verify 8-point guardrails, then tag, push tag, create GH release, pull base, best-effort branch cleanup.",
      false,
    )
    .option("-y, --yes", "Skip confirmation prompt (for non-interactive use)");

  releaseCmd
    .action(async (options: ReleaseOptions) => {
      const cwd = process.cwd();

      console.log(`\n${bold("loaf release")}\n`);

      // ── Pre-flight checks ──────────────────────────────────────────────

      if (!isGitRepo(cwd)) {
        console.error(`  ${red("error:")} Not a git repository`);
        process.exit(1);
      }

      // ── Early flag validation ─────────────────────────────────────────
      //
      // Validate purely flag-based inputs up front, before we do any commit
      // analysis. Two reasons:
      //   1. Bad flags should fail fast with a clear error, regardless of
      //      whether the repo has unreleased commits.
      //   2. Tests (and users running `--dry-run` to verify wiring) expect
      //      these errors to surface even when `commits.length === 0`
      //      causes the main flow to short-circuit later.
      if (options.bump) {
        try {
          validateBumpType(options.bump);
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          console.error(`  ${red("error:")} ${message}`);
          process.exit(1);
        }
      }

      // Pre-validate --version-file paths before any commit work. The full
      // detectVersionFiles call still runs later (it returns the parsed
      // VersionFile objects); this guard just turns "missing path" into an
      // immediate, actionable error.
      const cliVersionFileOverrides = options.versionFile ?? [];
      for (const declared of cliVersionFileOverrides) {
        const normalized = declared.replace(/\\/g, "/");
        if (!existsSync(join(cwd, normalized))) {
          console.error(
            `  ${red("error:")} version file ${normalized} not found`,
          );
          process.exit(1);
        }
      }

      // ── --post-merge handling ─────────────────────────────────────────
      //
      // `--post-merge` finalizes a release after the squash-merge has landed
      // on the base branch. It is post-bump: the bump + changelog + commit
      // already happened in the pre-merge run, the PR was reviewed and
      // squash-merged, and now we tag + push + create the GH release +
      // cleanup.
      //
      // It is incompatible with every flag that influences the bump flow
      // (--bump, --dry-run, --no-tag, --tag, --no-gh, --gh, --base,
      // --version-file, --yes). Reject up front before any guardrail.
      if (options.postMerge) {
        const incompatibleFlags: Array<{ source: string; label: string }> = [];
        const checkIncompatible = (
          optionName: string,
          label: string,
        ): void => {
          const source = releaseCmd.getOptionValueSource(optionName);
          if (source === "cli") incompatibleFlags.push({ source, label });
        };
        checkIncompatible("bump", "--bump");
        checkIncompatible("dryRun", "--dry-run");
        checkIncompatible("tag", options.tag === false ? "--no-tag" : "--tag");
        checkIncompatible("gh", options.gh === false ? "--no-gh" : "--gh");
        checkIncompatible("base", "--base");
        checkIncompatible("versionFile", "--version-file");
        checkIncompatible("yes", "--yes");
        checkIncompatible("preMerge", "--pre-merge");
        if (incompatibleFlags.length > 0) {
          for (const { label } of incompatibleFlags) {
            console.error(
              `  ${red("error:")} --post-merge is incompatible with ${label}`,
            );
          }
          process.exit(1);
        }

        const logger: PostMergeLogger = {
          ok: (msg) => console.log(`    ${green("✓")} ${msg}`),
          warn: (msg) => process.stderr.write(`    ${yellow("⚠")} ${msg}\n`),
          err: (msg) => process.stderr.write(`    ${red("✗")} ${msg}\n`),
        };

        console.log(`  ${cyan("Verifying post-merge state")}...`);
        console.log();

        let guardrailResult;
        try {
          guardrailResult = await checkPostMergeGuardrails({ cwd });
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          console.error(`  ${red("error:")} ${message}`);
          process.exit(1);
        }

        if (!guardrailResult.ok) {
          console.error(
            `  ${red("error:")} guardrail ${guardrailResult.guardrail} failed: ${guardrailResult.message}`,
          );
          process.exit(1);
        }

        console.log(
          `  ${green("✓")} All 8 guardrails passed for ${bold(`v${guardrailResult.version}`)} on ${bold(guardrailResult.base)}`,
        );
        if (guardrailResult.featureBranch) {
          console.log(
            `  ${gray(`feature branch:`)} ${guardrailResult.featureBranch}`,
          );
        }
        console.log();
        console.log(`  ${bold("Executing:")}`);

        try {
          await executePostMergeActions({ cwd }, guardrailResult, logger);
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          console.error(`    ${red("✗")} ${message}`);
          process.exit(1);
        }

        console.log();
        console.log(
          `  ${green("✓")} Release v${guardrailResult.version} finalized\n`,
        );
        return;
      }

      // ── --pre-merge handling ──────────────────────────────────────────
      //
      // `--pre-merge` bundles `--no-tag --no-gh --base <auto>`. Explicit
      // overrides win:
      //   - `--tag` with `--pre-merge`     → warn and tag anyway
      //   - `--gh` with `--pre-merge`      → warn and create release anyway
      //   - `--base <ref>` with --pre-merge → use the explicit ref (skip
      //                                       the auto-detection lookup)
      //
      // We use Commander's option-source API to distinguish "user passed
      // the flag explicitly" from "default value". Without the source check
      // we can't tell --no-tag from "default tag=true" since both leave
      // options.tag at a known value.
      if (options.preMerge) {
        const tagSource = releaseCmd.getOptionValueSource("tag");
        const ghSource = releaseCmd.getOptionValueSource("gh");
        const baseSource = releaseCmd.getOptionValueSource("base");

        if (tagSource === "cli" && options.tag === true) {
          process.stderr.write(
            `  ${yellow("warning:")} --tag overrides --pre-merge default (no tag); proceeding with tag enabled\n`,
          );
        } else {
          options.tag = false;
        }

        if (ghSource === "cli" && options.gh === true) {
          process.stderr.write(
            `  ${yellow("warning:")} --gh overrides --pre-merge default (no gh release); proceeding with GitHub release enabled\n`,
          );
        } else {
          options.gh = false;
        }

        // If user did NOT pass --base explicitly, auto-detect it.
        if (baseSource !== "cli") {
          const currentBranch = getCurrentBranch(cwd);
          if (!currentBranch) {
            console.error(
              `  ${red("error:")} --pre-merge requires a named branch ` +
                `(detached HEAD detected). Pass --base <ref> explicitly.`,
            );
            process.exit(1);
          }
          try {
            const detected = await resolveBaseBranch({
              currentBranch,
              cwd,
            });
            options.base = detected.base;
            console.log(
              `  ${cyan("Auto-detected base:")} ${bold(detected.base)} ` +
                `${gray(`(via ${detected.source})`)}`,
            );
          } catch (error) {
            const message = error instanceof Error ? error.message : String(error);
            console.error(`  ${red("error:")} ${message}`);
            process.exit(1);
          }
        }
      }

      // ── Phase 1: Gather ────────────────────────────────────────────────

      process.stdout.write(`  ${cyan("Analyzing")}...\n\n`);

      let baseRef = options.base ?? getLastTag(cwd);

      // Validate --base ref exists before proceeding (may resolve to origin/<ref>)
      if (options.base) {
        try {
          baseRef = validateBaseRef(cwd, options.base);
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          console.error(`  ${red("error:")} ${message}`);
          process.exit(1);
        }
      }

      const commits = getCommitsSince(cwd, baseRef);

      if (options.base) {
        const baseRefLabel = baseRef === options.base
          ? `${bold(options.base)} (via --base flag)`
          : `${bold(options.base)} (resolved to ${bold(baseRef!)} via --base flag)`;
        console.log(`  Base ref: ${baseRefLabel}`);
      } else {
        console.log(`  Last tag: ${baseRef ? bold(baseRef) : gray("(none)")}`);
      }
      console.log(`  Commits since ${options.base ? "base" : "tag"}: ${bold(String(commits.length))}`);
      console.log();

      if (commits.length === 0 && !options.dryRun) {
        console.log(`  ${gray("No unreleased changes found.")}\n`);
        process.exit(0);
      }
      if (commits.length === 0) {
        console.log(`  ${gray("No unreleased changes found.")}\n`);
        // Fall through in --dry-run so users can still preview flag wiring
        // (action list, --no-tag/--no-gh skipped markers, version files, etc.)
        // and the dry-run exit summary.
      }

      // Print each commit
      for (const commit of commits) {
        if (commit.section === null) {
          console.log(`  ${gray(`${commit.raw} (${commit.hash})`)}  ${gray("[filtered]")}`);
        } else {
          console.log(`  ${green(`${commit.raw} (${commit.hash})`)}`);
        }
      }
      console.log();

      // Detect version files. Two-tier resolution:
      //   1. --version-file CLI override (repeatable)
      //   2. .agents/loaf.json `release.versionFiles`
      //   3. Root auto-detect (fallback)
      const cliOverrides = options.versionFile ?? [];
      const loafConfig = readLoafConfig(cwd);
      const configOverrides = Array.isArray(loafConfig.release?.versionFiles)
        ? (loafConfig.release?.versionFiles as string[])
        : [];

      let versionFiles: VersionFile[];
      try {
        versionFiles = detectVersionFiles(cwd, {
          cliOverrides,
          configOverrides,
        });
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        console.error(`  ${red("error:")} ${message}`);
        process.exit(1);
      }

      if (versionFiles.length === 0) {
        console.error(`  ${red("error:")} No version files found`);
        process.exit(1);
      }

      // Scan for incomplete tasks
      const incompleteTasks = scanIncompleteTasks(cwd);

      // ── Phase 2: Generate ──────────────────────────────────────────────

      const commitBump = suggestBump(commits);
      const currentVersion = versionFiles[0].currentVersion;
      const parsed = parseSemVer(currentVersion);
      const isPrerelease = parsed !== null && parsed.prerelease !== undefined;

      let bump: BumpType;
      let newVersion: string | null;

      // Validate --bump flag if provided
      if (options.bump) {
        try {
          bump = validateBumpType(options.bump);
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          console.error(`  ${red("error:")} ${message}`);
          process.exit(1);
        }
        newVersion = bumpVersion(currentVersion, bump);
        console.log(`  Bump type: ${bold(bump)} (via --bump flag)`);
        console.log();
      } else if (isPrerelease) {
        // On a pre-release version — default to bumping the prerelease counter
        // Show available options
        console.log(`  ${bold("Current pre-release:")} ${currentVersion}`);
        console.log();
        console.log(`  ${bold("Bump options:")}`);
        console.log(`    ${cyan("1.")} prerelease → ${bumpVersion(currentVersion, "prerelease")}`);
        console.log(`    ${cyan("2.")} release    → ${bumpVersion(currentVersion, "release")}`);
        console.log(`    ${cyan("3.")} ${commitBump.padEnd(10)} → ${bumpVersion(currentVersion, commitBump)} ${gray("(based on commits)")}`);
        console.log();

        // Ask user to choose (with non-interactive fallback defaulting to "prerelease")
        const choice = await askChoice(
          `  Bump type [1/2/3]: `,
          ["prerelease", "release", commitBump],
          "prerelease",
        );
        bump = choice;
        newVersion = bumpVersion(currentVersion, bump);
      } else {
        // Normal release — use commit-based suggestion
        bump = commitBump;
        newVersion = bumpVersion(currentVersion, bump);
      }

      if (!newVersion) {
        console.error(`  ${red("error:")} Could not compute new version from "${currentVersion}"`);
        process.exit(1);
      }

      const today = new Date().toISOString().slice(0, 10);

      // Curated body vs auto-generation:
      // If [Unreleased] in the existing CHANGELOG already contains user-written
      // content (list items, subsection headers like ### Added / ### Changed /
      // ### Removed / ### Fixed / ### Internal, prose, etc.), preserve it
      // verbatim under the new version header. Auto-generation from commit
      // subjects only runs when [Unreleased] is empty (or contains only the
      // stub). The stub is always re-inserted after the move (see
      // insertIntoChangelog).
      const existingChangelogPath = join(cwd, "CHANGELOG.md");
      const curatedBody = existsSync(existingChangelogPath)
        ? extractUnreleasedBody(readFileSync(existingChangelogPath, "utf-8"))
        : null;

      let changelogSection: string;
      if (curatedBody !== null) {
        changelogSection = buildChangelogSectionFromEntries(
          newVersion,
          today,
          curatedBody,
        );
      } else {
        changelogSection = generateChangelogSection(newVersion, today, commits);
      }

      // Editor workflow
      const editor = getEditor();
      if (editor && process.stdin.isTTY) {
        try {
          const tmpDir = mkdtempSync(join(tmpdir(), "loaf-release-"));
          const tmpFile = join(tmpDir, "CHANGELOG_SECTION.md");
          writeFileSync(tmpFile, changelogSection, "utf-8");

          execFileSync(editor, [tmpFile], { stdio: "inherit" });

          changelogSection = readFileSync(tmpFile, "utf-8");
          unlinkSync(tmpFile);

          console.log(`  ${green("Edited changelog accepted.")}`);
          console.log();
        } catch {
          // Editor failed — fall through to terminal preview
          console.log(`  ${yellow("Editor failed — using generated changelog.")}`);
          console.log();
        }
      } else {
        console.log(`  ${bold("Generated changelog:")}\n`);
        // Indent each line for display
        for (const line of changelogSection.split("\n")) {
          console.log(`  ${line}`);
        }
        console.log();
        console.log(`  ${gray("(Set $EDITOR to edit before confirming)")}`);
        console.log();
      }

      // ── Phase 3: Present ───────────────────────────────────────────────

      const tagName = `v${newVersion}`;
      const ghAvailable = isGhAvailable();

      // Version files summary
      console.log(`  ${bold("Version files:")}`);
      for (const file of versionFiles) {
        console.log(`    \u2022 ${file.relativePath} (${file.currentVersion} \u2192 ${newVersion})`);
      }
      console.log();

      // Incomplete tasks warning
      if (incompleteTasks.length > 0) {
        console.log(`  ${bold("Incomplete tasks:")} ${incompleteTasks.length}`);
        for (const task of incompleteTasks) {
          console.log(`    ${yellow("\u26A0")} ${task.filename} (status: ${task.status})`);
        }
        console.log();
      }

      // Bump summary
      const bumpReasons: Record<string, string> = {
        major: "breaking changes detected",
        minor: "new features detected",
        patch: "bug fixes only",
        prerelease: "development milestone",
        release: "stable release",
      };
      console.log(`  Suggested bump: ${bold(bump)} (${bumpReasons[bump]})`);
      console.log(`  New version: ${bold(newVersion)}`);
      console.log();

      // Determine which steps are enabled
      const { skipTag, skipGh } = normalizeSkipFlags(options);

      // Action list
      console.log(`  ${bold("Actions:")}`);
      let actionNum = 1;
      // Detect build command for action-list display. Mirrors the Phase 4
      // heuristic so the preview matches what executes.
      const buildPkgPathPreview = join(cwd, "package.json");
      let buildCommandLabel = "loaf build";
      if (existsSync(buildPkgPathPreview)) {
        try {
          const pkg = JSON.parse(readFileSync(buildPkgPathPreview, "utf-8")) as {
            scripts?: Record<string, string>;
          };
          if (pkg.scripts && typeof pkg.scripts.build === "string") {
            buildCommandLabel = "npm run build";
          }
        } catch {
          // Malformed package.json — keep default
        }
      }
      console.log(`    ${actionNum++}. Update version in ${versionFiles.length} file(s)`);
      console.log(`    ${actionNum++}. Update CHANGELOG.md`);
      console.log(`    ${actionNum++}. Run ${buildCommandLabel}`);
      console.log(`    ${actionNum++}. Commit release artifacts`);
      if (skipTag) {
        console.log(`    ${gray(`${actionNum++}. Create git tag ${tagName} (--no-tag — skipped)`)}`);
      } else {
        console.log(`    ${actionNum++}. Create git tag ${tagName}`);
      }
      if (skipGh) {
        console.log(`    ${gray(`${actionNum++}. Create GitHub release draft (--no-gh — skipped)`)}`);
      } else if (ghAvailable) {
        console.log(`    ${actionNum++}. Create GitHub release draft (gh available)`);
      } else {
        console.log(`    ${gray(`${actionNum++}. Create GitHub release draft (gh not available — skipped)`)}`);
      }
      console.log();

      // Dry run exit
      if (options.dryRun) {
        console.log(`  ${cyan("--dry-run:")} No changes made.\n`);
        process.exit(0);
      }

      // Confirmation (--yes skips the prompt for non-interactive use)
      if (!options.yes) {
        const confirmed = await askYesNo(`  Proceed with release ${bold(tagName)}? [y/N] `);
        if (!confirmed) {
          console.log(`\n  ${gray("Release cancelled.")}\n`);
          process.exit(0);
        }
      }

      console.log();

      // ── Phase 4: Execute ───────────────────────────────────────────────

      console.log(`  ${bold("Executing:")}`);

      // Step 1: Write version files
      try {
        const updates = prepareVersionUpdates(versionFiles, newVersion);
        for (const [filePath, content] of updates) {
          writeFileSync(filePath, content, "utf-8");
          const relPath = versionFiles.find((f) => f.path === filePath)?.relativePath ?? filePath;
          const oldVer = versionFiles.find((f) => f.path === filePath)?.currentVersion ?? "?";
          console.log(`    ${green("\u2713")} Updated ${relPath} (${oldVer} \u2192 ${newVersion})`);
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        console.error(`    ${red("\u2717")} Failed to update version files: ${message}`);
        process.exit(1);
      }

      // Step 2: Write changelog
      try {
        const changelogPath = join(cwd, "CHANGELOG.md");
        let changelogContent: string;

        if (existsSync(changelogPath)) {
          const existing = readFileSync(changelogPath, "utf-8");
          const inserted = insertIntoChangelog(existing, changelogSection);
          if (inserted) {
            changelogContent = inserted;
          } else {
            // No [Unreleased] marker — append at end of file
            changelogContent = existing + "\n" + changelogSection + "\n";
          }
        } else {
          changelogContent = createChangelog(changelogSection);
        }

        writeFileSync(changelogPath, changelogContent, "utf-8");
        console.log(`    ${green("\u2713")} Updated CHANGELOG.md`);
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        console.error(`    ${red("\u2717")} Failed to update CHANGELOG.md: ${message}`);
        process.exit(1);
      }

      // Step 3: Run build
      //
      // Heuristic: if the project has a `package.json` with a `build` script,
      // invoke `npm run build`. This refreshes the full toolchain (e.g. the
      // bundled CLI in `plugins/loaf/bin/loaf`), not just the content
      // artifacts that `loaf build` produces. Without this, projects that
      // bundle their own CLI (like loaf itself) would commit a stale bundle
      // with the previous version baked in \u2014 the bug that motivated this
      // path. See TASK-149.
      //
      // For non-Node projects, or Node projects without a `build` script,
      // fall back to `loaf build` (content-only) \u2014 the prior behavior.
      try {
        const buildPkgPath = join(cwd, "package.json");
        let useNpmBuild = false;
        if (existsSync(buildPkgPath)) {
          try {
            const pkg = JSON.parse(readFileSync(buildPkgPath, "utf-8")) as {
              scripts?: Record<string, string>;
            };
            if (pkg.scripts && typeof pkg.scripts.build === "string") {
              useNpmBuild = true;
            }
          } catch {
            // Malformed package.json \u2014 fall back to loaf build
          }
        }

        if (useNpmBuild) {
          execFileSync("npm", ["run", "build"], {
            cwd,
            stdio: "inherit",
          });
          console.log(`    ${green("\u2713")} Ran npm run build`);
        } else {
          execFileSync(process.execPath, [process.argv[1], "build"], {
            cwd,
            stdio: "inherit",
          });
          console.log(`    ${green("\u2713")} Built all targets`);
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        console.error(`    ${red("\u2717")} Build failed: ${message}`);
        process.exit(1);
      }

      // Step 4: Commit release artifacts
      try {
        execFileSync("git", ["add", "-A"], {
          cwd,
          stdio: ["ignore", "pipe", "ignore"],
        });
        execFileSync(
          "git",
          ["commit", "-m", `chore: release ${tagName}`],
          { cwd, stdio: ["ignore", "pipe", "ignore"] },
        );
        console.log(`    ${green("\u2713")} Committed release artifacts`);
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        console.error(`    ${red("\u2717")} Failed to commit release: ${message}`);
        process.exit(1);
      }

      // Step 5: Create git tag
      if (skipTag) {
        console.log(`    ${gray("-")} Git tag skipped (--no-tag)`);
      } else {
        try {
          execFileSync("git", ["tag", "-a", tagName, "-m", `Release ${newVersion}`], {
            cwd,
            stdio: ["ignore", "pipe", "ignore"],
          });
          console.log(`    ${green("\u2713")} Created tag ${tagName}`);
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          console.error(`    ${red("\u2717")} Failed to create tag: ${message}`);
          process.exit(1);
        }
      }

      // Step 6: Create GitHub release draft
      if (skipGh) {
        console.log(`    ${gray("-")} GitHub release skipped (--no-gh)`);
      } else if (ghAvailable) {
        try {
          execFileSync(
            "gh",
            [
              "release",
              "create",
              tagName,
              "--draft",
              "--title",
              `v${newVersion}`,
              "--notes",
              changelogSection,
            ],
            { cwd, stdio: "inherit" },
          );
          console.log(`    ${green("\u2713")} Created GitHub release draft`);
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          console.error(`    ${red("\u2717")} Failed to create GitHub release: ${message}`);
          process.exit(1);
        }
      } else {
        console.log(`    ${gray("-")} GitHub release skipped (gh not available)`);
      }

      console.log();
      console.log(`  ${green("\u2713")} Release ${bold(tagName)} complete\n`);
    });
}
