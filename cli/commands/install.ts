/**
 * loaf install command
 *
 * Detects AI tools and installs Loaf distributions to their config directories.
 * Replaces install.sh for post-build installation.
 */

import { Command } from "commander";
import { existsSync, readFileSync, mkdirSync, copyFileSync, chmodSync, unlinkSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";
import { createInterface } from "readline";
import { execFileSync, execSync } from "child_process";
import {
  detectTools,
  detectClaudeCode,
  DEFAULT_CONFIG_DIRS,
  isDevMode,
} from "../lib/detect/tools.js";
import { INSTALLERS } from "../lib/install/installer.js";
import { installFencedSectionsForTargets } from "../lib/install/fenced-section.js";
import { runMcpRecommendations } from "../lib/install/mcp-recommendations.js";

function findProjectRoot(): string {
  try {
    return execFileSync("git", ["rev-parse", "--show-toplevel"], {
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
    }).trim();
  } catch {
    return process.cwd();
  }
}

const __dirname = dirname(fileURLToPath(import.meta.url));

// ANSI colors
const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const red = (s: string) => `\x1b[31m${s}\x1b[0m`;
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;
const white = (s: string) => `\x1b[97m${s}\x1b[0m`;

function findRootDir(): string {
  let dir = __dirname;
  for (let i = 0; i < 10; i++) {
    const pkgPath = join(dir, "package.json");
    try {
      const pkg = JSON.parse(readFileSync(pkgPath, "utf-8"));
      if (pkg.name === "loaf") return dir;
    } catch {
      // not found
    }
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  throw new Error("Could not find loaf root directory");
}

function askYesNo(question: string, defaultYes = false): Promise<boolean> {
  const rl = createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  return new Promise((resolve) => {
    rl.question(question, (answer) => {
      rl.close();
      const a = answer.trim().toLowerCase();
      if (!a) resolve(defaultYes);
      else resolve(a.startsWith("y"));
    });
  });
}

const DISPLAY_NAMES: Record<string, string> = {
  "claude-code": "Claude Code",
  opencode: "OpenCode",
  cursor: "Cursor",
  codex: "Codex",
  gemini: "Gemini",
  amp: "Amp",
};

function displayName(target: string): string {
  return DISPLAY_NAMES[target] ?? target;
}

/** Check if loaf is available on PATH */
function isLoafOnPath(): boolean {
  try {
    execSync("which loaf", { stdio: "pipe" });
    return true;
  } catch {
    return false;
  }
}

/** Install loaf binary to ~/.local/bin/ */
async function installLoafBinary(rootDir: string): Promise<boolean> {
  const localBinDir = join(process.env.HOME || "~", ".local", "bin");
  const sourceBinary = join(rootDir, "dist-cli", "index.js");
  const targetBinary = join(localBinDir, "loaf");
  
  if (!existsSync(sourceBinary)) {
    console.log(`  ${red("✗")} CLI binary not found at ${sourceBinary}`);
    console.log(`  ${gray("Run 'npm run build:cli' first.")}`);
    return false;
  }
  
  // Create ~/.local/bin if needed
  if (!existsSync(localBinDir)) {
    try {
      mkdirSync(localBinDir, { recursive: true });
      console.log(`  ${green("✓")} Created ${localBinDir}`);
    } catch (err) {
      console.log(`  ${red("✗")} Could not create ${localBinDir}: ${err}`);
      return false;
    }
  }
  
  // Remove existing binary if present
  if (existsSync(targetBinary)) {
    try {
      unlinkSync(targetBinary);
    } catch {
      // Ignore unlink errors
    }
  }
  
  // Copy and make executable
  try {
    copyFileSync(sourceBinary, targetBinary);
    chmodSync(targetBinary, 0o755);
    console.log(`  ${green("✓")} Installed loaf binary to ${targetBinary}`);
    
    // Check if ~/.local/bin is on PATH
    const pathEnv = process.env.PATH || "";
    if (!pathEnv.includes(localBinDir)) {
      console.log(`  ${yellow("⚠")} ${localBinDir} is not on your PATH`);
      console.log(`  ${gray("Add this to your shell profile:")}`);
      console.log(`  ${gray(`  export PATH=\"${localBinDir}:\$PATH\"`)}`);
    }
    
    return true;
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    console.log(`  ${red("✗")} Failed to install binary: ${msg}`);
    return false;
  }
}

const VALID_TARGETS = Object.keys(DEFAULT_CONFIG_DIRS);

export function registerInstallCommand(program: Command): void {
  program
    .command("install")
    .description("Install Loaf to detected AI tool configurations")
    .option("--to <target>", 'Target to install to (or "all")')
    .option("--upgrade", "Update only already-installed targets")
    .action(async (options: { to?: string; upgrade?: boolean }) => {
      const rootDir = findRootDir();
      const distDir = join(rootDir, "dist");
      const devMode = isDevMode(rootDir);
      const upgrade = options.upgrade ?? false;
      const projectRoot = findProjectRoot();

      console.log(`\n${bold("loaf install")}\n`);

      // Detect tools
      const hasClaudeCode = detectClaudeCode();
      const tools = detectTools();

      // Show detection results
      if (hasClaudeCode) {
        console.log(`  ${green("✓")} Claude Code detected`);
        if (devMode) {
          console.log(
            `    ${gray("Test locally:")} ${white(`/plugin marketplace add ${rootDir}`)}`,
          );
        } else {
          console.log(
            `    ${gray("Install via:")} ${white("/plugin marketplace add levifig/loaf")}`,
          );
        }
        console.log();
      }

      for (const tool of tools) {
        const status = tool.installed
          ? ` ${yellow("(installed)")}`
          : "";
        console.log(`  ${green("✓")} ${tool.name} detected${status}`);
      }

      if (tools.length === 0 && !hasClaudeCode) {
        console.log(`  ${gray("No AI tools detected")}`);
        console.log();
        return;
      }
      console.log();

      // Determine targets to install
      let selectedTargets: string[];

      if (options.to === "all") {
        selectedTargets = tools.map((t) => t.key);
      } else if (options.to) {
        if (!VALID_TARGETS.includes(options.to) && options.to !== "all") {
          console.error(
            `${red("error:")} Unknown target ${bold(options.to)}\n` +
              `${gray("Valid targets:")} ${VALID_TARGETS.join(", ")}, all`,
          );
          process.exit(1);
        }

        // Ensure config dir exists for the target
        const tool = tools.find((t) => t.key === options.to);
        selectedTargets = [options.to];

        if (!tool) {
          console.log(
            `  ${yellow("⚡")} ${options.to} was not auto-detected; installing to ${DEFAULT_CONFIG_DIRS[options.to]}`,
          );
        }
      } else if (upgrade) {
        selectedTargets = tools
          .filter((t) => t.installed)
          .map((t) => t.key);

        if (selectedTargets.length === 0) {
          // Still install fenced sections for Claude Code even if no other targets to upgrade
          if (hasClaudeCode) {
    
            const fencedResults = installFencedSectionsForTargets(
              ["claude-code"],
              projectRoot,
              upgrade
            );
            if (fencedResults["claude-code"]?.action === "updated") {
              console.log(`  ${green("✓")} Claude Code updated Loaf framework section in project file`);
            } else if (fencedResults["claude-code"]?.action === "skipped") {
              console.log(`  ${gray("○")} Claude Code Loaf framework section already current`);
            }
            console.log();
          }
          console.log(`  ${gray("No installed targets to upgrade")}`);
          console.log();
          return;
        }
        console.log(`  ${gray("Upgrading:")} ${selectedTargets.join(", ")}`);
      } else {
        // Interactive selection
        selectedTargets = [];
        for (const tool of tools) {
          const status = tool.installed ? ` ${yellow("(installed)")}` : "";
          const yes = await askYesNo(
            `  Install to ${bold(tool.name)}${status}? [Y/n] `,
            true,
          );
          if (yes) {
            selectedTargets.push(tool.key);
          }
        }
      }

      if (selectedTargets.length === 0) {

        // Still install fenced sections for Claude Code even if no targets selected
        if (hasClaudeCode) {
          const fencedResults = installFencedSectionsForTargets(
            ["claude-code"],
            projectRoot,
            upgrade
          );
          if (fencedResults["claude-code"]?.action === "created") {
            console.log(`  ${green("✓")} Claude Code created project file with Loaf framework section`);
          } else if (fencedResults["claude-code"]?.action === "appended") {
            console.log(`  ${green("✓")} Claude Code added Loaf framework section to project file`);
          } else if (fencedResults["claude-code"]?.action === "updated") {
            console.log(`  ${green("✓")} Claude Code updated Loaf framework section in project file`);
          } else if (fencedResults["claude-code"]?.action === "skipped") {
            console.log(`  ${gray("○")} Claude Code Loaf framework section already current`);
          }
          console.log();
        }
        // UX: skip MCP prompts when the user declined every target interactively.
        // Still offer them for Claude Code or `loaf install --to all` (explicit intent).
        if (hasClaudeCode || options.to === "all") {
          const mcpTargets = hasClaudeCode ? ["claude-code"] : [];
          await runMcpRecommendations({
            projectRoot,
            upgrade,
            availableTargets: mcpTargets,
          });
        }
        console.log(`  ${gray("No targets selected")}`);
        console.log();
        return;
      }

      // Check/install loaf binary for targets that need PATH access
      // Per SPEC-020: only opencode, cursor, codex, amp need PATH-based binary
      // Claude Code uses plugin-bundled binary via ${CLAUDE_PLUGIN_ROOT}
      const pathDependentTargets = ["opencode", "cursor", "codex", "amp"];
      const needsPathBinary = selectedTargets.some((t) =>
        pathDependentTargets.includes(t),
      );

      if (needsPathBinary && !isLoafOnPath()) {
        console.log(
          `  ${yellow("⚠")} Selected targets need 'loaf' on PATH but it's not available`,
        );
        console.log(
          `  ${gray("Targets:")} ${selectedTargets
            .filter((t) => pathDependentTargets.includes(t))
            .join(", ")}`,
        );

        const shouldInstallBinary = await askYesNo(
          `  Install 'loaf' binary to ~/.local/bin? [y/N] `,
        );

        if (shouldInstallBinary) {
          await installLoafBinary(rootDir);
          console.log();
        } else {
          console.log(
            `  ${gray("Skipping binary installation. Some hooks may not work for PATH-dependent targets.")}`,
          );
          console.log();
        }
      } else if (needsPathBinary) {
        console.log(`  ${green("✓")} loaf binary available on PATH for target hooks`);
        console.log();
      }

      console.log();

      // Install each target
      const installedTargets: string[] = [];

      for (const target of selectedTargets) {
        const tool = tools.find((t) => t.key === target);
        const configDir = tool?.configDir || DEFAULT_CONFIG_DIRS[target];
        const targetDistDir = join(distDir, target);

        if (!existsSync(targetDistDir)) {
          console.log(
            `  ${red("✗")} ${displayName(target)} — no build output found. Run ${bold("loaf build")} first.`,
          );
          continue;
        }

        const installer = INSTALLERS[target];
        if (!installer) {
          console.log(`  ${red("✗")} ${displayName(target)} — no installer available`);
          continue;
        }

        try {
          installer(targetDistDir, configDir, upgrade);
          console.log(`  ${green("✓")} ${displayName(target)} installed to ${gray(configDir)}`);
          installedTargets.push(target);
        } catch (error) {
          const msg = error instanceof Error ? error.message : String(error);
          console.log(`  ${red("✗")} ${displayName(target)} — ${msg}`);
        }
      }

      console.log();

      // Install fenced sections to project files
      // Include: (1) successfully installed targets (excluding gemini which has no project file layer),
      // (2) Claude Code if detected
      // Claude Code uses plugin-bundled binary but still needs fenced sections in project
      const targetsWithFencedSections = installedTargets.filter(t => t !== "gemini");
      const fencedTargets = hasClaudeCode
        ? ["claude-code", ...targetsWithFencedSections]
        : [...targetsWithFencedSections];
      const fencedResults = installFencedSectionsForTargets(
        fencedTargets,
        projectRoot,
        upgrade
      );

      // Report fenced section results
      let hasFencedOutput = false;
      for (const [target, result] of Object.entries(fencedResults)) {
        const name = displayName(target);
        if (result.action === "error") {
          console.log(
            `  ${red("✗")} ${name} project file — ${result.error}`
          );
          hasFencedOutput = true;
        } else if (result.action === "created") {
          console.log(
            `  ${green("✓")} ${name} created project file with Loaf framework section`
          );
          hasFencedOutput = true;
        } else if (result.action === "appended") {
          console.log(
            `  ${green("✓")} ${name} added Loaf framework section to project file`
          );
          hasFencedOutput = true;
        } else if (result.action === "updated") {
          console.log(
            `  ${green("✓")} ${name} updated Loaf framework section in project file (v${result.version})`
          );
          hasFencedOutput = true;
        } else if (result.action === "skipped") {
          console.log(
            `  ${gray("○")} ${name} Loaf framework section already current (v${result.version})`
          );
          hasFencedOutput = true;
        }
      }

      if (hasFencedOutput) {
        console.log();
      }

      // Collect all available targets for MCP recommendations
      const mcpTargets = [...new Set([
        ...(hasClaudeCode ? ["claude-code"] : []),
        ...installedTargets,
        ...selectedTargets,
      ])];
      await runMcpRecommendations({
        projectRoot,
        upgrade,
        availableTargets: mcpTargets,
      });
    });
}
