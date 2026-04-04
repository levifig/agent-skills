#!/usr/bin/env node
/**
 * Pre-build script: Generate CLI reference documentation
 *
 * This runs before the main build to generate content/skills/cli-reference/SKILL.md
 * from the actual CLI source code. Ensures documentation stays in sync.
 */

import { Command } from "commander";
import { writeFileSync, existsSync, mkdirSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";

// Import all command modules to register them
import { registerBuildCommand } from "../commands/build.js";
import { registerInstallCommand } from "../commands/install.js";
import { registerInitCommand } from "../commands/init.js";
import { registerReleaseCommand } from "../commands/release.js";
import { registerTaskCommand } from "../commands/task.js";
import { registerSpecCommand } from "../commands/spec.js";
import { registerKbCommand } from "../commands/kb.js";
import { registerSetupCommand } from "../commands/setup.js";
import { registerVersionCommand } from "../commands/version.js";
import { registerHousekeepingCommand } from "../commands/housekeeping.js";
import { registerCheckCommand } from "../commands/check.js";
import { registerSessionCommand } from "../commands/session.js";

// Import the generator
import { generateCliReferenceSkill, extractCliStructure } from "../lib/cli-reference-generator.js";

const __dirname = dirname(fileURLToPath(import.meta.url));

// Get project root (3 levels up from cli/scripts/)
const rootDir = join(__dirname, "..", "..", "..");

async function main() {
  // Create a temporary CLI instance to introspect
  const program = new Command();
  program.name("loaf").description("Loaf CLI");

  // Register all commands
  registerBuildCommand(program);
  registerInstallCommand(program);
  registerInitCommand(program);
  registerReleaseCommand(program);
  registerTaskCommand(program);
  registerSpecCommand(program);
  registerKbCommand(program);
  registerSetupCommand(program);
  registerVersionCommand(program);
  registerHousekeepingCommand(program);
  registerCheckCommand(program);
  registerSessionCommand(program);

  // Extract structure and generate
  const commands = extractCliStructure(program);
  const skillContent = generateCliReferenceSkill(commands);

  // Output path
  const outputPath = join(rootDir, "content", "skills", "cli-reference", "SKILL.md");

  // Ensure directory exists
  const dir = dirname(outputPath);
  if (!existsSync(dir)) {
    mkdirSync(dir, { recursive: true });
  }

  // Write file
  writeFileSync(outputPath, skillContent, "utf-8");
  console.log(`✓ Generated CLI reference: ${outputPath}`);
}

main().catch((err) => {
  console.error("Error generating CLI reference:", err);
  process.exit(1);
});
