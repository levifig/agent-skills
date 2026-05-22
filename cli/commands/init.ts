/**
 * loaf init command
 *
 * Bootstraps a project with .agents/ structure, docs, and skill recommendations.
 * Operates on the current working directory, not the Loaf repo root.
 */

import { Command } from "commander";
import {
  existsSync,
  mkdirSync,
  writeFileSync,
  symlinkSync,
  lstatSync,
  realpathSync,
} from "fs";
import { join, relative, dirname } from "path";
import { createInterface } from "readline";
import { detectProject } from "../lib/detect/project.js";
import type { ProjectInfo } from "../lib/detect/project.js";

const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;

/** Resolve a path and verify it stays within the project root. */
function withinProject(cwd: string, fullPath: string): boolean {
  // Resolve symlinks in the longest existing ancestor
  let check = fullPath;
  while (!existsSync(check) && check !== cwd) {
    check = dirname(check);
  }
  try {
    const realCheck = realpathSync(check);
    const realCwd = realpathSync(cwd);
    return realCheck === realCwd || realCheck.startsWith(realCwd + "/");
  } catch {
    return false;
  }
}

// ANSI colors
const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;
const cyan = (s: string) => `\x1b[36m${s}\x1b[0m`;

// Skill recommendations by detected language/framework
const SKILL_MAP: Record<string, string[]> = {
  TypeScript: ["typescript-development"],
  Python: ["python-development"],
  Ruby: ["ruby-development"],
  Go: ["go-development"],
  "Next.js": ["typescript-development", "interface-design"],
  React: ["typescript-development", "interface-design"],
  FastAPI: ["python-development", "database-design"],
  Django: ["python-development", "database-design"],
  Rails: ["ruby-development", "database-design"],
  Flask: ["python-development"],
};

// Directories to scaffold (relative to project root)
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

// Files to scaffold: [relative path, content generator]
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

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]
`,
  ],
];

function askYesNo(question: string): Promise<boolean> {
  if (!process.stdin.isTTY) {
    return Promise.resolve(false);
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
        resolve(false);
      }
    });
    rl.question(question, (answer) => {
      resolved = true;
      rl.close();
      resolve(answer.trim().toLowerCase().startsWith("y"));
    });
  });
}

function printDetected(info: ProjectInfo): void {
  console.log(`  ${bold("Detected:")}`);

  if (info.languages.length === 0 && info.frameworks.length === 0) {
    console.log(`    ${gray("No languages or frameworks detected")}`);
  } else {
    for (const lang of info.languages) {
      console.log(`    ${green("\u2713")} ${lang.name} ${gray(`(${lang.indicator})`)}`);
    }
    for (const fw of info.frameworks) {
      console.log(`    ${green("\u2713")} ${fw.name} ${gray(`(${fw.indicator})`)}`);
    }
  }

  console.log();
  console.log(`  ${bold("Existing:")}`);

  const checks: Array<[boolean, string]> = [
    [info.existing.hasAgentsDir, ".agents/ directory"],
    [info.existing.hasAgentsMd, ".agents/AGENTS.md"],
    [info.existing.hasDocsDir, "docs/ directory"],
    [info.existing.hasChangelog, "CHANGELOG.md"],
    [info.existing.hasClaudeDir, ".claude/ directory"],
    [info.existing.hasLoafJson, ".agents/loaf.json"],
  ];

  for (const [exists, label] of checks) {
    if (exists) {
      console.log(`    ${green("\u2713")} ${label}`);
    } else {
      console.log(`    ${gray("\u2717")} ${label}`);
    }
  }
}

function scaffoldDirs(cwd: string): { created: string[]; skipped: string[] } {
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

function scaffoldFiles(cwd: string): { created: string[]; skipped: string[] } {
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

function getRecommendedSkills(info: ProjectInfo): string[] {
  const skills = new Set<string>(["foundations"]);

  for (const lang of info.languages) {
    const mapped = SKILL_MAP[lang.name];
    if (mapped) {
      for (const s of mapped) skills.add(s);
    }
  }

  for (const fw of info.frameworks) {
    const mapped = SKILL_MAP[fw.name];
    if (mapped) {
      for (const s of mapped) skills.add(s);
    }
  }

  return Array.from(skills);
}

function fileOrSymlinkExists(path: string): boolean {
  try {
    // lstatSync does not follow symlinks, so it detects broken symlinks too
    lstatSync(path);
    return true;
  } catch {
    return false;
  }
}

export function registerInitCommand(program: Command): void {
  program
    .command("init")
    .description("Initialize a project with Loaf structure")
    .option("--no-symlinks", "Skip symlink creation prompts")
    .action(async (options: { symlinks: boolean }) => {
      const cwd = process.cwd();

      console.log(`\n${bold("loaf init")}\n`);

      // Phase 1: Scan
      process.stdout.write(`  ${cyan("scanning")} project...\n\n`);
      const info = detectProject(cwd);

      // Phase 2: Report
      printDetected(info);
      console.log();

      // Phase 3: Scaffold
      const dirs = scaffoldDirs(cwd);
      const files = scaffoldFiles(cwd);
      const allSkipped = [...dirs.skipped, ...files.skipped];

      if (dirs.created.length > 0 || files.created.length > 0) {
        console.log(`  ${bold("Creating:")}`);
        for (const dir of dirs.created) {
          console.log(`    ${green("+")} ${dir}`);
        }
        for (const file of files.created) {
          console.log(`    ${green("+")} ${file}`);
        }
        console.log();
      } else {
        console.log(`  ${gray("Nothing to create — all files exist")}\n`);
      }

      if (allSkipped.length > 0) {
        console.log(`  ${yellow("Skipped")} (symlink points outside project):`);
        for (const s of allSkipped) {
          console.log(`    ${yellow("!")} ${s}`);
        }
        console.log();
      }

      // Phase 4: Symlinks + Recommendations
      if (options.symlinks) {
        const agentsMdPath = join(cwd, ".agents", "AGENTS.md");

        // Only offer symlinks if the target .agents/AGENTS.md exists
        if (existsSync(agentsMdPath)) {
          console.log(`  ${bold("Symlinks:")}`);

          // .claude/CLAUDE.md -> .agents/AGENTS.md
          const claudeSymlink = join(cwd, ".claude", "CLAUDE.md");
          if (!fileOrSymlinkExists(claudeSymlink)) {
            const yes = await askYesNo(
              `    Create .claude/CLAUDE.md \u2192 .agents/AGENTS.md? [y/N] `,
            );
            if (yes) {
              const claudeDir = join(cwd, ".claude");
              if (!existsSync(claudeDir)) {
                mkdirSync(claudeDir, { recursive: true });
              }
              const relTarget = relative(claudeDir, agentsMdPath);
              symlinkSync(relTarget, claudeSymlink);
              console.log(`    ${green("\u2713")} Created .claude/CLAUDE.md`);
            }
          }

          // ./AGENTS.md -> .agents/AGENTS.md
          const rootSymlink = join(cwd, "AGENTS.md");
          if (!fileOrSymlinkExists(rootSymlink)) {
            const yes = await askYesNo(
              `    Create ./AGENTS.md \u2192 .agents/AGENTS.md? [y/N] `,
            );
            if (yes) {
              const relTarget = relative(cwd, agentsMdPath);
              symlinkSync(relTarget, rootSymlink);
              console.log(`    ${green("\u2713")} Created ./AGENTS.md`);
            }
          }

          console.log();
        }
      }

      // Skill recommendations
      const skills = getRecommendedSkills(info);
      if (skills.length > 0) {
        console.log(`  ${bold("Recommended skills:")}`);

        // Group skills by source for display
        const stackParts: string[] = [];
        for (const lang of info.languages) {
          if (SKILL_MAP[lang.name]) stackParts.push(lang.name);
        }
        for (const fw of info.frameworks) {
          if (SKILL_MAP[fw.name]) stackParts.push(fw.name);
        }

        const nonFoundation = skills.filter((s) => s !== "foundations");
        if (nonFoundation.length > 0) {
          const stackLabel =
            stackParts.length > 0
              ? gray(`(for ${stackParts.join(" + ")})`)
              : "";
          console.log(
            `    \u2022 ${nonFoundation.join(", ")}  ${stackLabel}`,
          );
        }
        console.log(`    \u2022 foundations  ${gray("(always)")}`);
        console.log();
      }

      // Done
      console.log(`  ${green("\u2713")} Project initialized\n`);
      console.log(`  ${bold("Next steps:")}`);
      console.log(`    1. Edit .agents/AGENTS.md with your project details`);
      console.log(`    2. Run ${cyan("loaf install")} to set up your AI tools`);
      console.log();
    });
}
