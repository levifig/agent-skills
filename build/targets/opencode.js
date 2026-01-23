/**
 * OpenCode Build Target
 *
 * Generates flat OpenCode structure:
 * dist/opencode/
 * ├── skill/
 * │   ├── python/
 * │   ├── typescript/
 * │   └── ...
 * ├── agent/
 * │   ├── pm.md
 * │   ├── backend-dev.md
 * │   └── ...
 * ├── command/
 * │   ├── start-session.md
 * │   └── ...
 * └── plugin/
 *     └── hooks.js
 *
 * Reads frontmatter from sidecars (e.g., pm.opencode.yaml, SKILL.opencode.yaml)
 */

import {
  mkdirSync,
  cpSync,
  writeFileSync,
  readFileSync,
  readdirSync,
  existsSync,
} from "fs";
import matter from "gray-matter";
import { join } from "path";
import { loadAgentSidecar, loadSkillFrontmatter, loadCommandSidecar } from "../lib/sidecar.js";
import { getVersion } from "../lib/version.js";

const TARGET_NAME = "opencode";

// Version is loaded dynamically from package.json at build time
let VERSION = "0.0.0";

/**
 * Build OpenCode distribution
 */
export async function build({
  config,
  targetConfig,
  targetsConfig,
  rootDir,
  srcDir,
  distDir,
}) {
  // Load version from package.json at build time
  VERSION = getVersion(rootDir);

  // Clean and create dist directory
  mkdirSync(distDir, { recursive: true });

  // Copy skills with frontmatter from sidecars
  copySkills(srcDir, distDir);

  // Copy and transform agents with frontmatter from sidecars
  copyAgents(srcDir, distDir);

  // Copy commands
  copyCommands(srcDir, distDir);

  // Generate hooks.js
  generateHooks(config, srcDir, distDir);
}

/**
 * Copy all skills with frontmatter from sidecars
 */
function copySkills(srcDir, distDir) {
  const src = join(srcDir, "skills");
  const dest = join(distDir, "skill");

  if (!existsSync(src)) {
    return;
  }

  mkdirSync(dest, { recursive: true });

  const skills = readdirSync(src, { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .map((d) => d.name);

  for (const skill of skills) {
    const skillSrc = join(src, skill);
    const skillDest = join(dest, skill);
    mkdirSync(skillDest, { recursive: true });

    // Load frontmatter from SKILL.md (standard format works for OpenCode)
    const frontmatter = loadSkillFrontmatter(skillSrc);

    // Read SKILL.md body (strip existing frontmatter if any)
    const skillMdPath = join(skillSrc, "SKILL.md");
    if (existsSync(skillMdPath)) {
      const content = readFileSync(skillMdPath, "utf-8");
      const { content: body } = matter(content);

      // Write SKILL.md with frontmatter
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
  }
}

/**
 * Copy and transform all agents with frontmatter from sidecars
 */
function copyAgents(srcDir, distDir) {
  const src = join(srcDir, "agents");
  const dest = join(distDir, "agent");

  if (!existsSync(src)) {
    return;
  }

  mkdirSync(dest, { recursive: true });

  const files = readdirSync(src).filter((f) => f.endsWith(".md"));

  for (const file of files) {
    const srcPath = join(src, file);
    const destPath = join(dest, file);

    // Read source body (strip existing frontmatter if any)
    const content = readFileSync(srcPath, "utf-8");
    const { content: body } = matter(content);

    // Load frontmatter from sidecar
    const frontmatter = loadAgentSidecar(srcPath, TARGET_NAME);

    // Reconstruct the file with sidecar frontmatter
    const transformed = matter.stringify(body, frontmatter);
    writeFileSync(destPath, transformed);
  }
}

/**
 * Copy commands with optional sidecar frontmatter
 *
 * OpenCode supports assigning commands to agents via frontmatter:
 * - agent: which agent executes the command
 * - subtask: false to run in main context (not as subagent)
 *
 * Sidecar files: {command}.opencode.yaml
 */
function copyCommands(srcDir, distDir) {
  const src = join(srcDir, "commands");
  const dest = join(distDir, "command");

  if (!existsSync(src)) {
    return;
  }

  mkdirSync(dest, { recursive: true });

  const files = readdirSync(src).filter((f) => f.endsWith(".md"));

  for (const file of files) {
    const srcPath = join(src, file);
    const destPath = join(dest, file);

    // Read source content
    const content = readFileSync(srcPath, "utf-8");
    const { content: body, data: frontmatter } = matter(content);

    // Load optional sidecar for OpenCode-specific frontmatter
    const sidecar = loadCommandSidecar(srcPath, TARGET_NAME);

    // Merge: source frontmatter < sidecar overrides < version
    const mergedFrontmatter = {
      ...frontmatter,
      ...sidecar,
      version: VERSION,
    };

    // Write with merged frontmatter
    const transformed = matter.stringify(body, mergedFrontmatter);
    writeFileSync(destPath, transformed);
  }
}

/**
 * Generate OpenCode hooks.js
 *
 * OpenCode hook format:
 * - tool.execute.pre.<matcher>
 * - tool.execute.post.<matcher>
 * - session.start
 * - session.end
 */
function generateHooks(config, srcDir, distDir) {
  const pluginDir = join(distDir, "plugin");
  mkdirSync(pluginDir, { recursive: true });

  // Copy hooks lib and scripts
  const hooksSrc = join(srcDir, "hooks");
  const hooksDest = join(pluginDir, "hooks");
  if (existsSync(hooksSrc)) {
    cpSync(hooksSrc, hooksDest, { recursive: true });
  }

  // Generate hooks.js
  const hooksJs = generateHooksJs(config);
  writeFileSync(join(pluginDir, "hooks.js"), hooksJs);
}

/**
 * Generate hooks.js content
 *
 * OpenCode plugin format: export default async function that returns hooks object
 * Hook names: tool.execute.before, tool.execute.after, event
 * Hook signature: (input, output) => Promise<void>
 */
function generateHooksJs(config) {
  const preToolHooks = config.hooks["pre-tool"] || [];
  const postToolHooks = config.hooks["post-tool"] || [];
  const sessionHooks = config.hooks.session || [];

  // Group pre-tool by matcher
  const preToolByMatcher = {};
  for (const hook of preToolHooks) {
    const matcher = hook.matcher || "Edit|Write";
    if (!preToolByMatcher[matcher]) {
      preToolByMatcher[matcher] = [];
    }
    preToolByMatcher[matcher].push(hook);
  }

  // Group post-tool by matcher
  const postToolByMatcher = {};
  for (const hook of postToolHooks) {
    const matcher = hook.matcher || "Edit|Write";
    if (!postToolByMatcher[matcher]) {
      postToolByMatcher[matcher] = [];
    }
    postToolByMatcher[matcher].push(hook);
  }

  return `/**
 * OpenCode Plugin - Agent Skills Hooks
 * Auto-generated by loaf build system
 *
 * This plugin provides quality gates and automation hooks.
 */

import { execFileSync } from 'child_process';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const HOOKS_DIR = join(__dirname, 'hooks');

/**
 * Execute a hook script safely
 */
function runHook(script, toolName, toolInput, timeout = 60000) {
  try {
    const scriptPath = join(HOOKS_DIR, script);
    const interpreter = script.endsWith('.py') ? 'python3' : 'bash';
    const result = execFileSync(interpreter, [scriptPath], {
      cwd: process.cwd(),
      env: {
        ...process.env,
        TOOL_NAME: toolName || '',
        TOOL_INPUT: JSON.stringify(toolInput || {}),
      },
      encoding: 'utf-8',
      timeout,
    });
    return { success: true, output: result };
  } catch (error) {
    return { success: false, error: error.message };
  }
}

/**
 * Check if tool matches pattern
 */
function matchesTool(toolName, pattern) {
  const patterns = pattern.split('|');
  return patterns.includes(toolName);
}

/**
 * Pre-tool hooks by matcher
 */
const preToolHooks = {
${Object.entries(preToolByMatcher)
  .map(
    ([matcher, hooks]) => `  '${matcher}': [
${hooks.map((h) => `    { id: '${h.id}', script: '${getScriptFilename(h.script)}', timeout: ${h.timeout || 60000} },`).join("\n")}
  ],`
  )
  .join("\n")}
};

/**
 * Post-tool hooks by matcher
 */
const postToolHooks = {
${Object.entries(postToolByMatcher)
  .map(
    ([matcher, hooks]) => `  '${matcher}': [
${hooks.map((h) => `    { id: '${h.id}', script: '${getScriptFilename(h.script)}', timeout: ${h.timeout || 60000} },`).join("\n")}
  ],`
  )
  .join("\n")}
};

/**
 * Session hooks
 */
const sessionHooks = {
${sessionHooks.map((h) => `  '${h.event.toLowerCase()}': { id: '${h.id}', script: '${getScriptFilename(h.script)}', timeout: ${h.timeout || 60000} },`).join("\n")}
};

/**
 * OpenCode Plugin Entry Point
 *
 * Receives context: { client, $, directory, project, worktree }
 * Returns hooks object
 */
export default async function AgentSkillsPlugin({ client, $ }) {
  return {
    /**
     * Called before tool execution
     */
    'tool.execute.before': async (input, output) => {
      const toolName = input?.tool?.name;
      if (!toolName) return;

      for (const [matcher, hookList] of Object.entries(preToolHooks)) {
        if (matchesTool(toolName, matcher)) {
          for (const hook of hookList) {
            const result = runHook(hook.script, toolName, input?.tool?.input, hook.timeout);
            if (!result.success) {
              console.warn(\`[loaf] Hook \${hook.id} failed: \${result.error}\`);
            }
          }
        }
      }
    },

    /**
     * Called after tool execution
     */
    'tool.execute.after': async (input, output) => {
      const toolName = input?.tool?.name;
      if (!toolName) return;

      for (const [matcher, hookList] of Object.entries(postToolHooks)) {
        if (matchesTool(toolName, matcher)) {
          for (const hook of hookList) {
            const result = runHook(hook.script, toolName, input?.tool?.input, hook.timeout);
            if (!result.success) {
              console.warn(\`[loaf] Hook \${hook.id} failed: \${result.error}\`);
            }
          }
        }
      }
    },

    /**
     * Called on session events
     */
    'event': async ({ event }) => {
      if (event.type === 'session.created' && sessionHooks.sessionstart) {
        runHook(sessionHooks.sessionstart.script, 'session', {}, sessionHooks.sessionstart.timeout);
      }
      if (event.type === 'session.ended' && sessionHooks.sessionend) {
        runHook(sessionHooks.sessionend.script, 'session', {}, sessionHooks.sessionend.timeout);
      }
    },
  };
}
`;
}

/**
 * Get script filename from path
 */
function getScriptFilename(scriptPath) {
  const parts = scriptPath.split("/");
  // Keep the subdirectory structure: pre-tool/script.sh, post-tool/script.sh
  return parts.slice(-2).join("/");
}
