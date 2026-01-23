/**
 * Remote Skills Build Target
 *
 * Generates remote-fetchable skills at repo root for Cursor, Codex, Copilot:
 * loaf/
 * ├── skills/          # Remote-fetchable skills
 * │   ├── python/
 * │   │   ├── SKILL.md
 * │   │   └── references/
 * │   ├── typescript/
 * │   └── ...
 * ├── agents/          # Remote-fetchable agents (subagent format)
 * │   ├── pm.md
 * │   ├── backend-dev.md
 * │   └── ...
 * ├── commands/        # Remote-fetchable commands
 * │   ├── start-session.md
 * │   └── ...
 * └── hooks.json       # Cursor-compatible hooks
 *
 * This allows Cursor users to fetch directly:
 *   /skill add https://github.com/levifig/loaf/skills/python
 *   /agent add https://github.com/levifig/loaf/agents/backend-dev.md
 *
 * Reads frontmatter from sidecars (e.g., pm.remote.yaml, SKILL.remote.yaml)
 */

import {
  mkdirSync,
  cpSync,
  writeFileSync,
  readFileSync,
  readdirSync,
  existsSync,
  rmSync,
} from "fs";
import matter from "gray-matter";
import { join, extname, dirname, basename } from "path";
import { parse as parseYaml } from "yaml";
import { loadSkillFrontmatter } from "../lib/sidecar.js";

const TARGET_NAME = "remote";

// Default frontmatter for agents
const DEFAULT_AGENT_FRONTMATTER = {
  model: "inherit",
  is_background: true,
};

// PM runs foreground (orchestrates others)
const PM_AGENT_FRONTMATTER = {
  model: "inherit",
  is_background: false,
};

/**
 * Build remote distribution to repo root
 */
export async function build({
  config,
  targetConfig,
  targetsConfig,
  rootDir,
  srcDir,
  distDir,
}) {
  // distDir is the repo root for remote target
  const skillsDir = join(distDir, "skills");
  const agentsDir = join(distDir, "agents");
  const commandsDir = join(distDir, "commands");

  // Clean existing directories
  for (const dir of [skillsDir, agentsDir, commandsDir]) {
    if (existsSync(dir)) {
      rmSync(dir, { recursive: true });
    }
    mkdirSync(dir, { recursive: true });
  }

  // Copy all skills
  copySkills(srcDir, skillsDir);

  // Copy and transform agents with Cursor-compatible frontmatter
  copyAgents(srcDir, agentsDir);

  // Copy commands
  copyCommands(srcDir, commandsDir);

  // Generate Cursor-compatible hooks.json
  generateHooksJson(config, distDir);
}

/**
 * Copy all skills with frontmatter from SKILL.md
 */
function copySkills(srcDir, destDir) {
  const src = join(srcDir, "skills");

  if (!existsSync(src)) {
    return;
  }

  const skills = readdirSync(src, { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .map((d) => d.name);

  for (const skill of skills) {
    const skillSrc = join(src, skill);
    const skillDest = join(destDir, skill);
    mkdirSync(skillDest, { recursive: true });

    // Load frontmatter from SKILL.md (with optional remote sidecar)
    const baseFrontmatter = loadSkillFrontmatter(skillSrc);
    const sidecarFrontmatter = loadRemoteSkillSidecar(skillSrc);
    const frontmatter = { ...baseFrontmatter, ...sidecarFrontmatter };

    // Read SKILL.md body (strip existing frontmatter if any)
    const skillMdPath = join(skillSrc, "SKILL.md");
    if (existsSync(skillMdPath)) {
      const content = readFileSync(skillMdPath, "utf-8");
      const { content: body } = matter(content);

      // Write SKILL.md with merged frontmatter
      const transformed = matter.stringify(body, frontmatter);
      writeFileSync(join(skillDest, "SKILL.md"), transformed);
    }

    // Copy references directory
    const refSrc = join(skillSrc, "references");
    const refDest = join(skillDest, "references");
    if (existsSync(refSrc)) {
      cpSync(refSrc, refDest, { recursive: true });
    }

    // Copy scripts directory
    const scriptsSrc = join(skillSrc, "scripts");
    const scriptsDest = join(skillDest, "scripts");
    if (existsSync(scriptsSrc)) {
      cpSync(scriptsSrc, scriptsDest, { recursive: true });
    }

    // Copy assets directory (Cursor-specific)
    const assetsSrc = join(skillSrc, "assets");
    const assetsDest = join(skillDest, "assets");
    if (existsSync(assetsSrc)) {
      cpSync(assetsSrc, assetsDest, { recursive: true });
    }
  }
}

/**
 * Load optional remote sidecar for skill
 */
function loadRemoteSkillSidecar(skillDir) {
  const sidecarPath = join(skillDir, `SKILL.${TARGET_NAME}.yaml`);

  if (!existsSync(sidecarPath)) {
    return {};
  }

  const content = readFileSync(sidecarPath, "utf-8");
  return parseYaml(content) || {};
}

/**
 * Copy and transform all agents with Cursor-compatible frontmatter
 */
function copyAgents(srcDir, destDir) {
  const src = join(srcDir, "agents");

  if (!existsSync(src)) {
    return;
  }

  const files = readdirSync(src).filter((f) => f.endsWith(".md"));

  for (const file of files) {
    const srcPath = join(src, file);
    const destPath = join(destDir, file);
    const agentName = file.replace(".md", "");

    // Read source body (strip existing frontmatter if any)
    const content = readFileSync(srcPath, "utf-8");
    const { content: body, data: sourceFrontmatter } = matter(content);

    // Load frontmatter from sidecar or use defaults
    const sidecarFrontmatter = loadRemoteAgentSidecar(srcPath);

    // Determine base defaults (PM is foreground, others are background)
    const defaults =
      agentName === "pm" ? PM_AGENT_FRONTMATTER : DEFAULT_AGENT_FRONTMATTER;

    // Merge: defaults < source frontmatter < sidecar frontmatter
    const frontmatter = {
      ...defaults,
      name: sourceFrontmatter.name || agentName,
      description:
        sourceFrontmatter.description ||
        `${agentName} agent for specialized tasks`,
      ...sidecarFrontmatter,
    };

    // Reconstruct the file with Cursor-compatible frontmatter
    const transformed = matter.stringify(body, frontmatter);
    writeFileSync(destPath, transformed);
  }
}

/**
 * Load optional remote sidecar for agent
 */
function loadRemoteAgentSidecar(sourcePath) {
  const dir = dirname(sourcePath);
  const baseName = basename(sourcePath, ".md");
  const sidecarPath = join(dir, `${baseName}.${TARGET_NAME}.yaml`);

  if (!existsSync(sidecarPath)) {
    return {};
  }

  const content = readFileSync(sidecarPath, "utf-8");
  return parseYaml(content) || {};
}

/**
 * Copy all commands
 */
function copyCommands(srcDir, destDir) {
  const src = join(srcDir, "commands");

  if (!existsSync(src)) {
    return;
  }

  const files = readdirSync(src).filter((f) => f.endsWith(".md"));

  for (const file of files) {
    const srcPath = join(src, file);
    const destPath = join(destDir, file);
    cpSync(srcPath, destPath);
  }
}

/**
 * Generate Cursor-compatible hooks.json
 *
 * Cursor hook format:
 * {
 *   "version": 1,
 *   "hooks": {
 *     "preToolUse": [...],
 *     "postToolUse": [...],
 *     "sessionStart": [...],
 *     "sessionEnd": [...],
 *     "preCompact": [...]
 *   }
 * }
 */
function generateHooksJson(config, distDir) {
  const preToolHooks = config.hooks["pre-tool"] || [];
  const postToolHooks = config.hooks["post-tool"] || [];
  const sessionHooks = config.hooks.session || [];

  const hooksJson = {
    version: 1,
    hooks: {},
  };

  // Pre-tool hooks -> preToolUse
  if (preToolHooks.length > 0) {
    hooksJson.hooks.preToolUse = preToolHooks.map((hook) => ({
      command: getHookCommand(hook),
      timeout: Math.floor((hook.timeout || 60000) / 1000), // Cursor uses seconds
      ...(hook.matcher && { matcher: { tool_name: hook.matcher } }),
    }));
  }

  // Post-tool hooks -> postToolUse
  if (postToolHooks.length > 0) {
    hooksJson.hooks.postToolUse = postToolHooks.map((hook) => ({
      command: getHookCommand(hook),
      timeout: 30, // Default 30s for post-tool
      ...(hook.matcher && { matcher: { tool_name: hook.matcher } }),
    }));
  }

  // Session hooks -> sessionStart, sessionEnd, preCompact
  for (const hook of sessionHooks) {
    const eventName = mapSessionEvent(hook.event);
    if (!hooksJson.hooks[eventName]) {
      hooksJson.hooks[eventName] = [];
    }
    hooksJson.hooks[eventName].push({
      command: getHookCommand(hook),
      timeout: Math.floor((hook.timeout || 60000) / 1000),
    });
  }

  writeFileSync(
    join(distDir, "hooks.json"),
    JSON.stringify(hooksJson, null, 2)
  );
}

/**
 * Map session event names from Claude Code to Cursor format
 */
function mapSessionEvent(event) {
  const mapping = {
    SessionStart: "sessionStart",
    SessionEnd: "sessionEnd",
    PreCompact: "preCompact",
  };
  return mapping[event] || event.toLowerCase();
}

/**
 * Get hook command for Cursor format
 */
function getHookCommand(hook) {
  const parts = hook.script.split("/");
  const filename = parts[parts.length - 1];

  // Cursor supports TypeScript via Bun, but we keep bash for compatibility
  if (filename.endsWith(".py")) {
    return `python3 hooks/${parts.slice(-2).join("/")}`;
  } else if (filename.endsWith(".ts")) {
    return `bun run hooks/${parts.slice(-2).join("/")}`;
  } else {
    return `bash hooks/${parts.slice(-2).join("/")}`;
  }
}
