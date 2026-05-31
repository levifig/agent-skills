import { Command } from "commander";
import { existsSync, readFileSync, rmSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";
import { parse as parseYaml } from "yaml";
import type { TargetModule, BuildContext, HooksConfig, TargetsConfig } from "../lib/build/types.js";

// Import target modules directly (bundled by tsup)
import * as claudeCodeTarget from "../lib/build/targets/claude-code.js";
import * as opencodeTarget from "../lib/build/targets/opencode.js";
import * as cursorTarget from "../lib/build/targets/cursor.js";
import * as codexTarget from "../lib/build/targets/codex.js";
import * as geminiTarget from "../lib/build/targets/gemini.js";
import * as ampTarget from "../lib/build/targets/amp.js";

// Import shared build modules for intermediate generation
import { copySkills } from "../lib/build/lib/skills.js";
import { createCommandSubstituter } from "../lib/build/lib/commands.js";

const __dirname = dirname(fileURLToPath(import.meta.url));

// ANSI color helpers
const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const red = (s: string) => `\x1b[31m${s}\x1b[0m`;
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;
const cyan = (s: string) => `\x1b[36m${s}\x1b[0m`;

// Target modules map (statically imported for tsup bundling)
const TARGETS: Record<string, TargetModule> = {
  "claude-code": claudeCodeTarget,
  opencode: opencodeTarget,
  cursor: cursorTarget,
  codex: codexTarget,
  gemini: geminiTarget,
  amp: ampTarget,
};

function findRootDir(): string {
  // Walk up from __dirname to find package.json with name "loaf"
  let dir = __dirname;
  for (let i = 0; i < 10; i++) {
    const pkgPath = join(dir, "package.json");
    try {
      const pkg = JSON.parse(readFileSync(pkgPath, "utf-8"));
      if (pkg.name === "loaf") return dir;
    } catch {
      // not found, go up
    }
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  throw new Error("Could not find loaf root directory (no package.json with name 'loaf')");
}

function loadYamlConfig<T>(path: string): T {
  if (!existsSync(path)) return {} as T;
  return parseYaml(readFileSync(path, "utf-8")) as T;
}

// Available target names — order determines build order
const TARGET_NAMES = Object.keys(TARGETS);

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

  // Claude Code outputs to repo root, others to dist/
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

/**
 * Build the shared skills intermediate in dist/skills/.
 *
 * This pre-target step produces a staging artifact that targets will
 * read from instead of content/skills/ in their refactor tasks.
 *
 * Features:
 * - Base frontmatter only (no sidecar merge, no version injection)
 * - Universal command substitution (unscoped /implement, /resume)
 * - Shared templates distributed per targets.yaml config
 * - References, templates, scripts copied
 */
async function buildSharedIntermediate(
  rootDir: string,
  contentDir: string,
  distDir: string,
  targetsConfig: TargetsConfig,
): Promise<void> {
  const skillsDestDir = join(distDir, "skills");

  // Clean existing directory
  if (existsSync(skillsDestDir)) {
    rmSync(skillsDestDir, { recursive: true, force: true });
  }

  // Create command substituter with 'shared' target (unscoped commands)
  const transformMd = createCommandSubstituter("shared");

  // Copy skills with base frontmatter only (no mergeFrontmatter, no version)
  copySkills({
    srcDir: contentDir,  // copySkills joins "skills" internally
    destDir: skillsDestDir,
    targetName: "shared",
    targetsConfig,
    transformMd,
    // No mergeFrontmatter callback - base frontmatter only
    // No version - targets add version during their copy step
  });
}

export function registerBuildCommand(program: Command): void {
  program
    .command("build")
    .description("Build skill distributions for agent harnesses")
    .option("-t, --target <name>", "Build a specific target only")
    .action(async (options: { target?: string }) => {
      try {
        await runBuild(options);
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        console.error(message);
        process.exit(1);
      }
    });
}

export async function runBuild(options: { target?: string } = {}): Promise<void> {
  const startTime = Date.now();
  const rootDir = findRootDir();
  const contentDir = join(rootDir, "content");
  const configDir = join(rootDir, "config");
  const distDir = join(rootDir, "dist");

  console.log(`\n${bold("loaf build")}\n`);

  // Validate target if specified
  if (options.target && !TARGET_NAMES.includes(options.target)) {
    throw new Error(
      `${red("error:")} Unknown target ${bold(options.target)}\n` +
      `${gray("Valid targets:")} ${TARGET_NAMES.join(", ")}`
    );
  }

  // Load config
  const hooksConfigPath = join(configDir, "hooks.yaml");
  if (!existsSync(hooksConfigPath)) {
    throw new Error(`${red("error:")} Hooks config not found: ${hooksConfigPath}`);
  }

  const hooksConfig = loadYamlConfig<HooksConfig>(hooksConfigPath);
  const targetsConfig = loadYamlConfig<TargetsConfig>(join(configDir, "targets.yaml"));

  const targets = options.target ? [options.target] : TARGET_NAMES;

  // Build shared skills intermediate first (before any target builds)
  process.stdout.write(`  ${cyan("building")} shared skills intermediate...`);
  const intermediateStart = Date.now();
  try {
    await buildSharedIntermediate(rootDir, contentDir, distDir, targetsConfig);
    const elapsed = ((Date.now() - intermediateStart) / 1000).toFixed(2);
    process.stdout.write(`\r  ${green("✓")} shared skills intermediate ${gray(`(${elapsed}s)`)}\n`);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    process.stdout.write(`\r  ${red("✗")} shared skills intermediate\n`);
    throw new Error(`    ${red(message)}`);
  }

  let failed = false;
  for (const targetName of targets) {
    const targetStart = Date.now();
    process.stdout.write(`  ${cyan("building")} ${targetName}...`);

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
      process.stdout.write(`\r  ${green("✓")} ${targetName} ${gray(`(${elapsed}s)`)}\n`);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      process.stdout.write(`\r  ${red("✗")} ${targetName}\n`);
      console.error(`    ${red(message)}`);
      failed = true;
    }
  }

  const totalElapsed = ((Date.now() - startTime) / 1000).toFixed(2);
  console.log();

  if (failed) {
    throw new Error(`${red("Build failed")} ${gray(`(${totalElapsed}s)`)}`);
  }

  console.log(`${green("Build complete")} ${gray(`(${totalElapsed}s)`)}`);
}
