/**
 * loaf setup command
 *
 * One-step bootstrap: init + build + install.
 * Optionally creates the target directory first.
 */

import { Command } from "commander";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  statSync,
  writeFileSync,
} from "fs";
import { join, dirname, resolve } from "path";
import { fileURLToPath } from "url";
import { parse as parseYaml } from "yaml";
import { detectProject } from "../lib/detect/project.js";
import {
  detectTools,
  detectClaudeCode,
  DEFAULT_CONFIG_DIRS,
  isDevMode,
} from "../lib/detect/tools.js";
import { INSTALLERS } from "../lib/install/installer.js";
import type {
  TargetModule,
  HooksConfig,
  TargetsConfig,
} from "../lib/build/types.js";

// Import target modules directly (bundled by tsup)
import * as claudeCodeTarget from "../lib/build/targets/claude-code.js";
import * as opencodeTarget from "../lib/build/targets/opencode.js";
import * as cursorTarget from "../lib/build/targets/cursor.js";
import * as codexTarget from "../lib/build/targets/codex.js";
import * as geminiTarget from "../lib/build/targets/gemini.js";

const __dirname = dirname(fileURLToPath(import.meta.url));

// ─────────────────────────────────────────────────────────────────────────────
// ANSI color helpers
// ─────────────────────────────────────────────────────────────────────────────

const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const red = (s: string) => `\x1b[31m${s}\x1b[0m`;
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;
const cyan = (s: string) => `\x1b[36m${s}\x1b[0m`;
const white = (s: string) => `\x1b[97m${s}\x1b[0m`;

// ─────────────────────────────────────────────────────────────────────────────
// Loaf root resolution (same pattern as build.ts / install.ts)
// ─────────────────────────────────────────────────────────────────────────────

function findRootDir(): string {
  let dir = __dirname;
  for (let i = 0; i < 10; i++) {
    const pkgPath = join(dir, "package.json");
    try {
      const pkg = JSON.parse(readFileSync(pkgPath, "utf-8"));
      if (pkg.name === "loaf") return dir;
    } catch {
      // not found, keep walking
    }
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  throw new Error(
    "Could not find loaf root directory (no package.json with name 'loaf')",
  );
}

/** Resolve a path and verify it stays within the project root. */
function withinProject(cwd: string, fullPath: string): boolean {
  // Resolve symlinks in the longest existing ancestor.
  let check = fullPath;
  while (!existsSync(check) && check !== cwd) {
    const parent = dirname(check);
    if (parent === check) break;
    check = parent;
  }
  try {
    const realCheck = realpathSync(check);
    const realCwd = realpathSync(cwd);
    return realCheck === realCwd || realCheck.startsWith(realCwd + "/");
  } catch {
    return false;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Init scaffolding (mirrors init.ts logic without interactive prompts)
// ─────────────────────────────────────────────────────────────────────────────

const SCAFFOLD_DIRS = [
  ".agents",
  ".agents/sessions",
  ".agents/ideas",
  ".agents/handoffs",
  ".agents/specs",
  ".agents/tasks",
  "docs",
  "docs/knowledge",
  "docs/decisions",
];

const SCAFFOLD_FILES: Array<[string, () => string]> = [
  [
    ".agents/AGENTS.md",
    () => `# Project Instructions

> Agent instructions for this project. Customize per your needs.

## Quick Start

<!-- Add build/run commands here -->

## Project Structure

<!-- Describe your project layout -->

## Development Practices

<!-- Add coding conventions, testing approach, etc. -->

## Key Decisions

<!-- Link to ADRs in docs/decisions/ -->
`,
  ],
  [
    ".agents/loaf.json",
    () =>
      JSON.stringify(
        {
          version: "1.0.0",
          initialized: new Date().toISOString(),
        },
        null,
        2,
      ) + "\n",
  ],
  [
    "docs/VISION.md",
    () => `# Vision

## Purpose
<!-- Why does this project exist? What problem does it solve? -->

## Target Users
<!-- Who is this for? -->

## Success Criteria
<!-- How do you know when you've succeeded? -->

## Non-Goals
<!-- What is explicitly out of scope? -->
`,
  ],
  [
    "docs/STRATEGY.md",
    () => `# Strategy

## Current Focus
<!-- What are you working on right now and why? -->

## Priorities
<!-- Ordered list of what matters most -->

## Constraints
<!-- Budget, timeline, team size, technical limitations -->

## Open Questions
<!-- Unresolved strategic decisions -->
`,
  ],
  [
    "docs/ARCHITECTURE.md",
    () => `# Architecture

## Overview
<!-- High-level system description -->

## Components
<!-- Key components and their responsibilities -->

## Data Flow
<!-- How data moves through the system -->

## Technology Choices
<!-- Key technology decisions and rationale -->

## Deployment
<!-- How the system is deployed -->
`,
  ],
  [
    "CHANGELOG.md",
    () => `# Changelog

This project follows [Common Changelog](https://common-changelog.org/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). \`## [Unreleased]\`
is a workflow staging section for curated entries before release.

## [Unreleased]
`,
  ],
];

interface ScaffoldResult {
  created: string[];
  skipped: string[];
}

function scaffoldDirs(cwd: string): ScaffoldResult {
  const created: string[] = [];
  const skipped: string[] = [];

  for (const dir of SCAFFOLD_DIRS) {
    const fullPath = join(cwd, dir);
    if (!existsSync(fullPath)) {
      if (!withinProject(cwd, fullPath)) {
        skipped.push(dir + "/");
        continue;
      }
      mkdirSync(fullPath, { recursive: true });
      created.push(dir + "/");
    }
  }

  return { created, skipped };
}

function scaffoldFiles(cwd: string): ScaffoldResult {
  const created: string[] = [];
  const skipped: string[] = [];

  for (const [relPath, contentFn] of SCAFFOLD_FILES) {
    const fullPath = join(cwd, relPath);
    if (!existsSync(fullPath)) {
      if (!withinProject(cwd, fullPath)) {
        skipped.push(relPath);
        continue;
      }
      const parentDir = dirname(fullPath);
      if (!existsSync(parentDir)) {
        mkdirSync(parentDir, { recursive: true });
      }
      writeFileSync(fullPath, contentFn(), "utf-8");
      created.push(relPath);
    }
  }

  return { created, skipped };
}

// ─────────────────────────────────────────────────────────────────────────────
// Build logic (reuses target modules directly, same as build.ts)
// ─────────────────────────────────────────────────────────────────────────────

const TARGETS: Record<string, TargetModule> = {
  "claude-code": claudeCodeTarget,
  opencode: opencodeTarget,
  cursor: cursorTarget,
  codex: codexTarget,
  gemini: geminiTarget,
};

const TARGET_NAMES = Object.keys(TARGETS);

function loadYamlConfig<T>(path: string): T {
  if (!existsSync(path)) return {} as T;
  return parseYaml(readFileSync(path, "utf-8")) as T;
}

async function buildTarget(
  targetName: string,
  rootDir: string,
  contentDir: string,
  distDir: string,
  hooksConfig: HooksConfig,
  targetsConfig: TargetsConfig,
): Promise<void> {
  const targetModule = TARGETS[targetName];
  if (!targetModule) {
    throw new Error(`Unknown target: ${targetName}`);
  }

  const outputDir =
    targetName === "claude-code" ? rootDir : join(distDir, targetName);
  const targetConfig = targetsConfig.targets?.[targetName] || {};

  await targetModule.build({
    config: hooksConfig,
    targetConfig,
    targetsConfig,
    rootDir,
    srcDir: contentDir,
    distDir: outputDir,
    targetName,
  });
}

// ─────────────────────────────────────────────────────────────────────────────
// Install logic (reuses installer + detect modules, same as install.ts)
// ─────────────────────────────────────────────────────────────────────────────

interface InstallResult {
  installed: string[];
  failed: string[];
  claudeCodeDetected: boolean;
}

function runInstall(rootDir: string): InstallResult {
  const distDir = join(rootDir, "dist");
  const devMode = isDevMode(rootDir);

  const hasClaudeCode = detectClaudeCode();
  const tools = detectTools();

  const installed: string[] = [];
  const failed: string[] = [];

  // Claude Code uses plugin marketplace, not file installation
  if (hasClaudeCode) {
    if (devMode) {
      console.log(
        `  ${green("✓")} Claude Code detected ${gray("(test via")} ${white(`/plugin marketplace add ${rootDir}`)}${gray(")")}`,
      );
    } else {
      console.log(
        `  ${green("✓")} Claude Code detected ${gray("(install via")} ${white("/plugin marketplace add levifig/loaf")}${gray(")")}`,
      );
    }
  }

  // Install to all detected tools
  const selectedTargets = tools.map((t) => t.key);

  for (const target of selectedTargets) {
    const tool = tools.find((t) => t.key === target);
    const configDir = tool?.configDir || DEFAULT_CONFIG_DIRS[target];
    const targetDistDir = join(distDir, target);

    if (!existsSync(targetDistDir)) {
      console.log(
        `  ${red("✗")} ${target} — no build output found`,
      );
      failed.push(target);
      continue;
    }

    const installer = INSTALLERS[target];
    if (!installer) {
      console.log(`  ${red("✗")} ${target} — no installer available`);
      failed.push(target);
      continue;
    }

    try {
      installer(targetDistDir, configDir);
      console.log(
        `  ${green("✓")} ${target} installed to ${gray(configDir)}`,
      );
      installed.push(target);
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      console.log(`  ${red("✗")} ${target} — ${msg}`);
      failed.push(target);
    }
  }

  return { installed, failed, claudeCodeDetected: hasClaudeCode };
}

// ─────────────────────────────────────────────────────────────────────────────
// Setup command
// ─────────────────────────────────────────────────────────────────────────────

export function registerSetupCommand(program: Command): void {
  program
    .command("setup")
    .description(
      "One-step bootstrap: init + build + install",
    )
    .argument("[path]", "Directory to set up (created if it does not exist)")
    .action(async (pathArg?: string) => {
      console.log(`\n${bold("loaf setup")}\n`);

      // Step 1: Create directory if path argument provided
      if (pathArg) {
        const targetDir = resolve(pathArg);
        if (existsSync(targetDir)) {
          const stat = statSync(targetDir);
          if (!stat.isDirectory()) {
            console.error(
              `  ${red("error:")} Path exists but is not a directory: ${targetDir}`,
            );
            process.exit(1);
          }
          console.log(
            `  ${gray("·")} ${gray(targetDir)} already exists`,
          );
        } else {
          mkdirSync(targetDir, { recursive: true });
          console.log(
            `  ${green("✓")} Created ${gray(targetDir)}`,
          );
        }

        process.chdir(targetDir);
        console.log();
      }

      const cwd = process.cwd();

      // Step 2: Init (scaffold directories and files)
      console.log(`  ${cyan("init")} Scaffolding project structure...`);

      const dirs = scaffoldDirs(cwd);
      const files = scaffoldFiles(cwd);
      const allSkipped = [...dirs.skipped, ...files.skipped];

      if (dirs.created.length > 0 || files.created.length > 0) {
        for (const dir of dirs.created) {
          console.log(`    ${green("+")} ${dir}`);
        }
        for (const file of files.created) {
          console.log(`    ${green("+")} ${file}`);
        }
      } else if (allSkipped.length === 0) {
        console.log(`    ${gray("Nothing to create — all files exist")}`);
      }
      if (allSkipped.length > 0) {
        console.log(`    ${yellow("Skipped")} (paths resolve outside project root):`);
        for (const path of allSkipped) {
          console.log(`      ${yellow("!")} ${path}`);
        }
      }
      console.log();

      // Step 3: Build all targets
      console.log(`  ${cyan("build")} Compiling skills and agents...`);

      let rootDir: string;
      try {
        rootDir = findRootDir();
      } catch (error) {
        const msg = error instanceof Error ? error.message : String(error);
        console.error(`\n  ${red("error:")} ${msg}`);
        process.exit(1);
      }

      const contentDir = join(rootDir, "content");
      const configDir = join(rootDir, "config");
      const distDir = join(rootDir, "dist");

      const hooksConfigPath = join(configDir, "hooks.yaml");
      if (!existsSync(hooksConfigPath)) {
        console.error(
          `\n  ${red("error:")} Hooks config not found: ${hooksConfigPath}`,
        );
        process.exit(1);
      }

      const hooksConfig = loadYamlConfig<HooksConfig>(hooksConfigPath);
      const targetsConfig = loadYamlConfig<TargetsConfig>(
        join(configDir, "targets.yaml"),
      );

      let buildFailed = false;
      for (const targetName of TARGET_NAMES) {
        const targetStart = Date.now();
        try {
          await buildTarget(
            targetName,
            rootDir,
            contentDir,
            distDir,
            hooksConfig,
            targetsConfig,
          );
          const elapsed = ((Date.now() - targetStart) / 1000).toFixed(2);
          console.log(
            `    ${green("✓")} ${targetName} ${gray(`(${elapsed}s)`)}`,
          );
        } catch (error) {
          const message =
            error instanceof Error ? error.message : String(error);
          console.log(`    ${red("✗")} ${targetName}`);
          console.error(`      ${red(message)}`);
          buildFailed = true;
        }
      }

      if (buildFailed) {
        console.error(`\n  ${red("Build failed. Stopping setup.")}`);
        process.exit(1);
      }
      console.log();

      // Step 4: Install to all detected tools
      console.log(`  ${cyan("install")} Distributing to detected tools...`);

      const installResult = runInstall(rootDir);

      if (installResult.failed.length > 0) {
        console.error(
          `\n  ${red("Install failed for:")} ${installResult.failed.join(", ")}`,
        );
        process.exit(1);
      }

      if (
        installResult.installed.length === 0 &&
        !installResult.claudeCodeDetected
      ) {
        console.log(`    ${gray("No AI tools detected")}`);
      }
      console.log();

      // Step 5: Handoff message
      console.log(
        `  ${green("✓")} Setup complete. Run ${bold("/bootstrap")} in Claude Code to set up your project.`,
      );
      console.log();
    });
}
