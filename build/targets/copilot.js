/**
 * Copilot Build Target
 *
 * Generates GitHub Copilot instructions:
 * dist/copilot/
 * └── .github/
 *     └── copilot-instructions.md
 *
 * Note: Copilot doesn't support hooks, so we include manual check instructions.
 */

import {
  mkdirSync,
  readFileSync,
  writeFileSync,
  existsSync,
  readdirSync,
} from "fs";
import { join } from "path";

/**
 * Build Copilot distribution
 */
export async function build({ config, rootDir, srcDir, distDir }) {
  const githubDir = join(distDir, ".github");
  mkdirSync(githubDir, { recursive: true });

  // Generate combined instructions file
  generateInstructions(config, srcDir, githubDir);
}

/**
 * Generate copilot-instructions.md
 */
function generateInstructions(config, srcDir, githubDir) {
  let content = `# Copilot Instructions

This file provides context and guidelines for GitHub Copilot to generate better code suggestions.

## Project Overview

This project uses the following technologies and patterns. Please follow these guidelines when generating code.

`;

  // Add agent summaries
  content += generateAgentSection(srcDir);

  // Add skill summaries
  content += generateSkillsSection(srcDir);

  // Add quality checks (manual since no hooks)
  content += generateQualitySection(config);

  writeFileSync(join(githubDir, "copilot-instructions.md"), content);
}

/**
 * Generate agent section
 */
function generateAgentSection(srcDir) {
  const agentsDir = join(srcDir, "agents");
  if (!existsSync(agentsDir)) {
    return "";
  }

  let section = `## Roles and Responsibilities

When working on different parts of the codebase, consider these specialized perspectives:

`;

  const agentFiles = readdirSync(agentsDir).filter((f) => f.endsWith(".md"));

  for (const agentFile of agentFiles) {
    const agentPath = join(agentsDir, agentFile);
    const agentContent = readFileSync(agentPath, "utf-8");

    // Extract frontmatter
    const frontmatterMatch = agentContent.match(/^---\n([\s\S]*?)\n---/);
    if (frontmatterMatch) {
      const frontmatter = frontmatterMatch[1];
      const nameMatch = frontmatter.match(/name:\s*(.+)/);
      const descMatch = frontmatter.match(/description:\s*(.+)/);

      if (nameMatch && descMatch) {
        section += `### ${nameMatch[1]}\n${descMatch[1]}\n\n`;
      }
    }
  }

  return section;
}

/**
 * Generate skills section
 */
function generateSkillsSection(srcDir) {
  const skillsDir = join(srcDir, "skills");
  if (!existsSync(skillsDir)) {
    return "";
  }

  let section = `## Technology Guidelines

`;

  const skills = readdirSync(skillsDir, { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .map((d) => d.name);

  for (const skill of skills) {
    const skillMdPath = join(skillsDir, skill, "SKILL.md");
    if (!existsSync(skillMdPath)) {
      continue;
    }

    const skillContent = readFileSync(skillMdPath, "utf-8");

    // Extract key sections (first 2000 chars to keep file manageable)
    const truncatedContent = extractKeyContent(skillContent, skill);
    section += `### ${capitalize(skill)}\n\n${truncatedContent}\n\n`;
  }

  return section;
}

/**
 * Generate quality checks section
 */
function generateQualitySection(config) {
  let section = `## Quality Checks (Manual)

Since Copilot doesn't support automated hooks, ensure you run these checks:

### Before Committing

`;

  // Gather all pre-tool hooks
  const preHooks = config.hooks["pre-tool"] || [];
  const uniqueDescriptions = [...new Set(preHooks.map((h) => h.description))];

  for (const desc of uniqueDescriptions.slice(0, 15)) {
    section += `- ${desc}\n`;
  }

  section += `
### After Making Changes

`;

  // Gather all post-tool hooks
  const postHooks = config.hooks["post-tool"] || [];
  const uniquePostDescriptions = [
    ...new Set(postHooks.map((h) => h.description)),
  ];

  for (const desc of uniquePostDescriptions) {
    section += `- ${desc}\n`;
  }

  section += `
### General Guidelines

- Always add type hints/annotations
- Write tests for new functionality
- Follow existing code style
- Validate all user input
- Handle errors appropriately
- Keep functions small and focused
`;

  return section;
}

/**
 * Extract key content from skill file
 */
function extractKeyContent(content, skill) {
  // Remove frontmatter
  const withoutFrontmatter = content.replace(/^---\n[\s\S]*?\n---\n/, "");

  // Get first significant section
  const lines = withoutFrontmatter.split("\n");
  const extracted = [];
  let charCount = 0;
  const maxChars = 1500;

  for (const line of lines) {
    if (charCount > maxChars) {
      extracted.push("\n...(see full documentation)");
      break;
    }
    extracted.push(line);
    charCount += line.length;
  }

  return extracted.join("\n").trim();
}

/**
 * Capitalize first letter
 */
function capitalize(str) {
  return str.charAt(0).toUpperCase() + str.slice(1);
}
