/**
 * Cursor Build Target
 *
 * Generates full Cursor distribution:
 * dist/cursor/
 * ├── skills/
 * │   ├── python/
 * │   │   ├── SKILL.md         # version in frontmatter
 * │   │   ├── references/
 * │   │   └── scripts/
 * │   └── ...
 * ├── commands/
 * │   ├── start-session.md     # version-only frontmatter
 * │   └── ...
 * ├── agents/
 * │   ├── pm.md                # version + Cursor subagent fields
 * │   ├── backend-dev.md
 * │   └── ...
 * ├── hooks/
 * │   ├── pre-tool/
 * │   ├── post-tool/
 * │   ├── session/
 * │   └── lib/
 * └── hooks.json               # Cursor hook config
 *
 * Cursor supports: skills, commands (as rules), agents (subagents), hooks
 */

import {
  mkdirSync,
  cpSync,
  writeFileSync,
  readFileSync,
  existsSync,
  readdirSync,
  rmSync,
} from "fs";
import matter from "gray-matter";
import { join, dirname, basename } from "path";
import { parse as parseYaml } from "yaml";
import { loadSkillFrontmatter } from "../lib/sidecar.js";
import { getVersion, injectVersion } from "../lib/version.js";

const TARGET_NAME = "cursor";

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
 * Build Cursor distribution
 */
export async function build({
  config,
  targetConfig,
  targetsConfig,
  rootDir,
  srcDir,
  distDir,
}) {
  const version = getVersion(rootDir);

  const skillsDir = join(distDir, "skills");
  const agentsDir = join(distDir, "agents");
  const commandsDir = join(distDir, "commands");
  const hooksDir = join(distDir, "hooks");

  // Clean existing directories
  for (const dir of [skillsDir, agentsDir, commandsDir, hooksDir]) {
    if (existsSync(dir)) {
      rmSync(dir, { recursive: true });
    }
    mkdirSync(dir, { recursive: true });
  }

  // Copy all skills with version injection
  copySkills(srcDir, skillsDir, version);

  // Copy and transform agents with Cursor-compatible frontmatter
  copyAgents(srcDir, agentsDir, targetConfig, version);

  // Copy commands with version-only frontmatter
  copyCommands(srcDir, commandsDir, version);

  // Copy hooks directory structure
  copyHooks(srcDir, hooksDir);

  // Generate Cursor-compatible hooks.json
  generateHooksJson(config, distDir);
}

/**
 * Copy all skills with version injection
 */
function copySkills(srcDir, destDir, version) {
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

    // Load frontmatter from SKILL.md (with optional cursor sidecar)
    const baseFrontmatter = loadSkillFrontmatter(skillSrc);
    const sidecarFrontmatter = loadCursorSkillSidecar(skillSrc);
    const frontmatter = injectVersion(
      { ...baseFrontmatter, ...sidecarFrontmatter },
      version
    );

    // Read SKILL.md body (strip existing frontmatter if any)
    const skillMdPath = join(skillSrc, "SKILL.md");
    if (existsSync(skillMdPath)) {
      const content = readFileSync(skillMdPath, "utf-8");
      const { content: body } = matter(content);

      // Write SKILL.md with merged frontmatter + version
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
 * Load optional Cursor sidecar for skill
 */
function loadCursorSkillSidecar(skillDir) {
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
function copyAgents(srcDir, destDir, targetConfig, version) {
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
    const sidecarFrontmatter = loadCursorAgentSidecar(srcPath);

    // Determine base defaults (PM is foreground, others are background)
    const defaults =
      agentName === "pm"
        ? PM_AGENT_FRONTMATTER
        : targetConfig?.defaults?.agents?.frontmatter || DEFAULT_AGENT_FRONTMATTER;

    // Merge: defaults < source frontmatter < sidecar frontmatter
    const frontmatter = injectVersion(
      {
        ...defaults,
        name: sourceFrontmatter.name || agentName,
        description:
          sourceFrontmatter.description ||
          `${agentName} agent for specialized tasks`,
        ...sidecarFrontmatter,
      },
      version
    );

    // Reconstruct the file with Cursor-compatible frontmatter
    const transformed = matter.stringify(body, frontmatter);
    writeFileSync(destPath, transformed);
  }
}

/**
 * Load optional Cursor sidecar for agent
 */
function loadCursorAgentSidecar(sourcePath) {
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
 * Copy all commands with version-only frontmatter
 *
 * Cursor strips most frontmatter fields from commands, so we only keep version.
 */
function copyCommands(srcDir, destDir, version) {
  const src = join(srcDir, "commands");

  if (!existsSync(src)) {
    return;
  }

  const files = readdirSync(src).filter((f) => f.endsWith(".md"));

  for (const file of files) {
    const srcPath = join(src, file);
    const destPath = join(destDir, file);

    // Read source content
    const content = readFileSync(srcPath, "utf-8");
    const { content: body } = matter(content);

    // Write with version-only frontmatter
    const frontmatter = { version };
    const transformed = matter.stringify(body, frontmatter);
    writeFileSync(destPath, transformed);
  }
}

/**
 * Copy hooks directory structure
 */
function copyHooks(srcDir, destDir) {
  const hooksSrc = join(srcDir, "hooks");

  if (!existsSync(hooksSrc)) {
    return;
  }

  // Copy all subdirectories (pre-tool, post-tool, session, lib)
  const subdirs = ["pre-tool", "post-tool", "session", "lib"];

  for (const subdir of subdirs) {
    const subSrc = join(hooksSrc, subdir);
    const subDest = join(destDir, subdir);

    if (existsSync(subSrc)) {
      cpSync(subSrc, subDest, { recursive: true });
    }
  }

  // Also copy any top-level hook files
  const entries = readdirSync(hooksSrc, { withFileTypes: true });
  for (const entry of entries) {
    if (entry.isFile()) {
      cpSync(join(hooksSrc, entry.name), join(destDir, entry.name));
    }
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
  // Keep the full relative path structure for Cursor
  const scriptPath = hook.script;

  if (scriptPath.endsWith(".py")) {
    return `python3 hooks/${scriptPath.replace(/^hooks\//, "")}`;
  } else if (scriptPath.endsWith(".ts")) {
    return `bun run hooks/${scriptPath.replace(/^hooks\//, "")}`;
  } else {
    return `bash hooks/${scriptPath.replace(/^hooks\//, "")}`;
  }
}
