#!/usr/bin/env node
/**
 * Build System for Universal Agent Skills
 *
 * Generates tool-specific distributions from canonical structure:
 * - Claude Code: plugins/{name}/ at repo root (for marketplace)
 * - OpenCode: dist/opencode/ with flat skill/, agent/, command/, plugin/
 * - Cursor: dist/cursor/.cursor/rules/*.mdc
 * - Copilot: dist/copilot/.github/copilot-instructions.md
 *
 * Usage:
 *   node build/build.js [target]
 *
 * Targets:
 *   all (default), claude-code, opencode, cursor, copilot
 */

import { readFileSync, existsSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";
import { parse as parseYaml } from "yaml";

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT_DIR = join(__dirname, "..");
const SRC_DIR = join(ROOT_DIR, "src");
const CONFIG_PATH = join(SRC_DIR, "config", "hooks.yaml");
const DIST_DIR = join(ROOT_DIR, "dist");

// Load configuration
function loadConfig() {
  const configContent = readFileSync(CONFIG_PATH, "utf-8");
  return parseYaml(configContent);
}

// Available build targets
const TARGETS = {
  "claude-code": () => import("./targets/claude-code.js"),
  opencode: () => import("./targets/opencode.js"),
  cursor: () => import("./targets/cursor.js"),
  copilot: () => import("./targets/copilot.js"),
};

async function build(targetName) {
  console.log(`\n🔨 Building ${targetName}...`);

  const targetModule = await TARGETS[targetName]();
  const config = loadConfig();

  // Claude Code outputs to root, others to dist/
  const outputDir =
    targetName === "claude-code" ? ROOT_DIR : join(DIST_DIR, targetName);

  try {
    await targetModule.build({
      config,
      rootDir: ROOT_DIR,
      srcDir: SRC_DIR,
      distDir: outputDir,
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

  console.log("🚀 Universal Agent Skills Build System");
  console.log(`   Root: ${ROOT_DIR}`);
  console.log(`   Source: ${SRC_DIR}`);
  console.log(`   Config: ${CONFIG_PATH}`);
  console.log(`   Dist: ${DIST_DIR}`);

  if (!existsSync(CONFIG_PATH)) {
    console.error("❌ Config file not found:", CONFIG_PATH);
    process.exit(1);
  }

  const startTime = Date.now();

  if (target === "all") {
    for (const targetName of Object.keys(TARGETS)) {
      await build(targetName);
    }
  } else if (TARGETS[target]) {
    await build(target);
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
