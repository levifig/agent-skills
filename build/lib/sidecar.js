/**
 * Sidecar Metadata System
 *
 * Skills: Frontmatter (name, description) in SKILL.md source file.
 *         Optional extensions for Claude Code in SKILL.claude-code.yaml.
 *
 * Agents: Full frontmatter in sidecars (target-specific formats differ significantly).
 */

import { readFileSync, existsSync } from "fs";
import { join, dirname, basename } from "path";
import { parse as parseYaml } from "yaml";
import matter from "gray-matter";

// ─────────────────────────────────────────────────────────────────────────────
// Skills
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Load base frontmatter from SKILL.md
 *
 * @param {string} skillDir - Path to skill directory
 * @returns {Object} Frontmatter with name and description
 */
export function loadSkillFrontmatter(skillDir) {
  const skillMdPath = join(skillDir, "SKILL.md");

  if (!existsSync(skillMdPath)) {
    const skillName = basename(skillDir);
    throw new Error(`Missing SKILL.md in ${skillDir}`);
  }

  const content = readFileSync(skillMdPath, "utf-8");
  const { data } = matter(content);
  return data;
}

/**
 * Load optional Claude Code extensions from sidecar
 *
 * @param {string} skillDir - Path to skill directory
 * @returns {Object} Extensions (empty object if sidecar doesn't exist)
 */
export function loadSkillExtensions(skillDir) {
  const sidecarPath = join(skillDir, "SKILL.claude-code.yaml");

  if (!existsSync(sidecarPath)) {
    return {};
  }

  const content = readFileSync(sidecarPath, "utf-8");
  return parseYaml(content) || {};
}

/**
 * Merge base frontmatter with Claude Code extensions
 *
 * @param {Object} base - Base frontmatter from SKILL.md
 * @param {Object} extensions - Claude Code extensions from sidecar
 * @returns {Object} Merged frontmatter
 */
export function mergeSkillFrontmatter(base, extensions) {
  return { ...base, ...extensions };
}

// ─────────────────────────────────────────────────────────────────────────────
// Agents
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Load required sidecar for an agent file
 *
 * @param {string} sourcePath - Path to the agent source file (e.g., src/agents/pm.md)
 * @param {string} target - Target name (e.g., 'claude-code', 'opencode')
 * @returns {Object} Sidecar frontmatter content
 * @throws {Error} If sidecar is missing
 */
export function loadAgentSidecar(sourcePath, target) {
  const dir = dirname(sourcePath);
  const baseName = basename(sourcePath, ".md");
  const sidecarPath = join(dir, `${baseName}.${target}.yaml`);

  if (!existsSync(sidecarPath)) {
    throw new Error(
      `Missing required sidecar: ${sidecarPath}\n` +
        `Agent '${baseName}' requires a sidecar for target '${target}'`
    );
  }

  const content = readFileSync(sidecarPath, "utf-8");
  return parseYaml(content);
}

// ─────────────────────────────────────────────────────────────────────────────
// Commands
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Load optional sidecar for a command file
 *
 * @param {string} sourcePath - Path to the command source file (e.g., src/commands/start-session.md)
 * @param {string} target - Target name (e.g., 'opencode', 'cursor')
 * @returns {Object} Sidecar frontmatter content (empty object if not found)
 */
export function loadCommandSidecar(sourcePath, target) {
  const dir = dirname(sourcePath);
  const baseName = basename(sourcePath, ".md");
  const sidecarPath = join(dir, `${baseName}.${target}.yaml`);

  if (!existsSync(sidecarPath)) {
    return {};
  }

  const content = readFileSync(sidecarPath, "utf-8");
  return parseYaml(content) || {};
}

// ─────────────────────────────────────────────────────────────────────────────
// Config
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Load targets.yaml configuration
 *
 * @param {string} configDir - Path to config directory
 * @returns {Object} Targets configuration
 */
export function loadTargetsConfig(configDir) {
  const configPath = join(configDir, "targets.yaml");
  if (!existsSync(configPath)) {
    return { targets: {} };
  }
  const content = readFileSync(configPath, "utf-8");
  return parseYaml(content);
}

/**
 * Get output directory for a target
 *
 * @param {Object} targetsConfig - Full targets.yaml config
 * @param {string} target - Target name
 * @returns {string|null} Output directory path or null
 */
export function getTargetOutput(targetsConfig, target) {
  return targetsConfig.targets?.[target]?.output || null;
}
