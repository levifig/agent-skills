#!/usr/bin/env node
/**
 * Build System for Loaf - Levi's Opinionated Agentic Framework
 *
 * Generates tool-specific distributions from canonical structure:
 * - Claude Code: plugins/{name}/ at repo root (for marketplace)
 * - OpenCode: dist/opencode/ with flat skill/, agent/, command/, plugin/
 * - Agent Skills: dist/agentskills/skills/ (shared by Codex, Cursor, Copilot, Gemini)
 *
 * Usage:
 *   node build/build.js [target]
 *
 * Targets:
 *   all (default), claude-code, opencode, agentskills
 */

import { readFileSync, existsSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";
import { parse as parseYaml } from "yaml";

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT_DIR = join(__dirname, "..");
const SRC_DIR = join(ROOT_DIR, "src");
const CONFIG_DIR = join(SRC_DIR, "config");
const HOOKS_CONFIG_PATH = join(CONFIG_DIR, "hooks.yaml");
const TARGETS_CONFIG_PATH = join(CONFIG_DIR, "targets.yaml");
const DIST_DIR = join(ROOT_DIR, "dist");

// Load hooks configuration
function loadHooksConfig() {
  const configContent = readFileSync(HOOKS_CONFIG_PATH, "utf-8");
  return parseYaml(configContent);
}

// Load targets configuration
function loadTargetsConfig() {
  if (!existsSync(TARGETS_CONFIG_PATH)) {
    return { targets: {} };
  }
  const configContent = readFileSync(TARGETS_CONFIG_PATH, "utf-8");
  return parseYaml(configContent);
}

// Available build targets
const TARGETS = {
  "claude-code": () => import("./targets/claude-code.js"),
  opencode: () => import("./targets/opencode.js"),
  agentskills: () => import("./targets/agentskills.js"),
  remote: () => import("./targets/remote.js"),
};

async function build(targetName, hooksConfig, targetsConfig) {
  console.log(`\n🔨 Building ${targetName}...`);

  const targetModule = await TARGETS[targetName]();

  // Claude Code and remote output to root, others to dist/
  const outputDir =
    targetName === "claude-code" || targetName === "remote"
      ? ROOT_DIR
      : join(DIST_DIR, targetName);

  // Get target-specific config
  const targetConfig = targetsConfig.targets?.[targetName] || {};

  try {
    await targetModule.build({
      config: hooksConfig, // hooks.yaml (for backward compat, renamed from 'config')
      targetConfig, // Target-specific config from targets.yaml
      targetsConfig, // Full targets.yaml (for utilities)
      rootDir: ROOT_DIR,
      srcDir: SRC_DIR,
      distDir: outputDir,
      targetName,
    });
    console.log(`✅ ${targetName} build complete`);
  } catch (error) {
    console.error(`❌ ${targetName} build failed:`, error.message);
    throw error;
  }
}

async function main() {
  const args = process.argv.slice(2);
  const target = args[0] || "all";

  console.log("🍞 Loaf Build System");
  console.log(`   Root: ${ROOT_DIR}`);
  console.log(`   Source: ${SRC_DIR}`);
  console.log(`   Config: ${CONFIG_DIR}`);
  console.log(`   Dist: ${DIST_DIR}`);

  if (!existsSync(HOOKS_CONFIG_PATH)) {
    console.error("❌ Hooks config not found:", HOOKS_CONFIG_PATH);
    process.exit(1);
  }

  // Load both config files
  const hooksConfig = loadHooksConfig();
  const targetsConfig = loadTargetsConfig();

  const startTime = Date.now();

  if (target === "all") {
    for (const targetName of Object.keys(TARGETS)) {
      await build(targetName, hooksConfig, targetsConfig);
    }
  } else if (TARGETS[target]) {
    await build(target, hooksConfig, targetsConfig);
  } else {
    console.error(`❌ Unknown target: ${target}`);
    console.log(`   Available: ${Object.keys(TARGETS).join(", ")}, all`);
    process.exit(1);
  }

  const elapsed = ((Date.now() - startTime) / 1000).toFixed(2);
  console.log(`\n✨ Build completed in ${elapsed}s`);
}

main().catch((error) => {
  console.error("Build failed:", error);
  process.exit(1);
});
