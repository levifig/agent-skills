/**
 * Claude Code Build Target
 *
 * Generates a single unified plugin at repo root:
 * loaf/
 * ├── .claude-plugin/
 * │   └── marketplace.json
 * └── plugins/
 *     └── loaf/
 *         ├── .claude-plugin/plugin.json
 *         ├── agents/
 *         ├── skills/
 *         ├── commands/
 *         └── hooks/
 *
 * This allows Claude Code to use:
 *   /plugin marketplace add levifig/loaf
 *
 * Scoping: /loaf:start-session, Task(loaf:backend-dev)
 *
 * Reads frontmatter from sidecars (e.g., pm.claude-code.yaml, SKILL.claude-code.yaml)
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
import { join } from "path";
import {
  loadAgentSidecar,
  loadSkillFrontmatter,
  loadSkillExtensions,
  mergeSkillFrontmatter,
  loadCommandSidecar,
} from "../lib/sidecar.js";
import { getVersion } from "../lib/version.js";

// Version is loaded dynamically from package.json at build time
let VERSION = "0.0.0";
const REPOSITORY = "https://github.com/levifig/loaf";
const TARGET_NAME = "claude-code";
const PLUGIN_NAME = "loaf";
const PLUGIN_DESCRIPTION = "Loaf - Levi's Opinionated Agentic Framework";

// LSP Servers for code intelligence
const LSP_SERVERS = {
  go: {
    command: "gopls",
    args: ["serve"],
    extensionToLanguage: { ".go": "go" },
  },
  python: {
    command: "pyright-langserver",
    args: ["--stdio"],
    extensionToLanguage: { ".py": "python", ".pyi": "python" },
  },
  typescript: {
    command: "typescript-language-server",
    args: ["--stdio"],
    extensionToLanguage: {
      ".ts": "typescript",
      ".tsx": "typescriptreact",
      ".js": "javascript",
      ".jsx": "javascriptreact",
    },
  },
  ruby: {
    command: "solargraph",
    args: ["stdio"],
    extensionToLanguage: { ".rb": "ruby", ".rake": "ruby", ".gemspec": "ruby" },
  },
};

// MCP Servers bundled with the plugin
const MCP_SERVERS = {
  "sequential-thinking": {
    command: "npx",
    args: ["-y", "@modelcontextprotocol/server-sequential-thinking"],
  },
  linear: {
    command: "npx",
    args: ["-y", "mcp-remote", "https://mcp.linear.app/mcp"],
  },
  serena: {
    command: "uvx",
    args: [
      "--from",
      "git+https://github.com/oraios/serena",
      "serena",
      "start-mcp-server",
    ],
  },
};

/**
 * Build Claude Code distribution to repo root
 */
export async function build({
  config,
  targetConfig,
  targetsConfig,
  rootDir,
  srcDir,
  distDir,
  targetName,
}) {
  // Load version from package.json at build time
  VERSION = getVersion(rootDir);

  // distDir is the repo root for claude-code target
  const pluginsDir = join(distDir, "plugins");
  const marketplaceDir = join(distDir, ".claude-plugin");

  // Clean existing plugin directories
  if (existsSync(pluginsDir)) {
    rmSync(pluginsDir, { recursive: true });
  }
  if (existsSync(marketplaceDir)) {
    rmSync(marketplaceDir, { recursive: true });
  }

  // Create directories
  mkdirSync(pluginsDir, { recursive: true });
  mkdirSync(marketplaceDir, { recursive: true });

  // Create marketplace.json with single plugin
  createMarketplace(marketplaceDir);

  // Build the single unified plugin
  buildUnifiedPlugin(config, srcDir, pluginsDir);
}

/**
 * Create marketplace.json for plugin discovery
 */
function createMarketplace(marketplaceDir) {
  const marketplace = {
    name: "levifig-loaf",
    owner: {
      name: "Levi Figueira",
      email: "me@levifig.com",
    },
    metadata: {
      description: PLUGIN_DESCRIPTION,
      version: VERSION,
    },
    plugins: [
      {
        name: PLUGIN_NAME,
        description: PLUGIN_DESCRIPTION,
        source: `./plugins/${PLUGIN_NAME}`,
        version: VERSION,
        license: "MIT",
        repository: REPOSITORY,
      },
    ],
  };

  writeFileSync(
    join(marketplaceDir, "marketplace.json"),
    JSON.stringify(marketplace, null, 2)
  );
}

/**
 * Build the single unified plugin with all agents, commands, skills, and hooks
 */
function buildUnifiedPlugin(config, srcDir, pluginsDir) {
  const pluginDir = join(pluginsDir, PLUGIN_NAME);
  mkdirSync(pluginDir, { recursive: true });

  // Discover all agents, commands, and skills from src/
  const allAgents = discoverAgents(srcDir);
  const allCommands = discoverCommands(srcDir);
  const allSkills = discoverSkills(srcDir);

  // Create plugin.json with all hooks
  createPluginJson(config, pluginDir, allAgents, allCommands, allSkills);

  // Copy all agents
  copyAgents(allAgents, srcDir, pluginDir);

  // Copy all skills
  copySkills(allSkills, srcDir, pluginDir);

  // Copy all commands
  copyCommands(allCommands, srcDir, pluginDir);

  // Copy all hooks
  copyAllHooks(config, srcDir, pluginDir);

  // Create .lsp.json for language server configurations
  writeFileSync(
    join(pluginDir, ".lsp.json"),
    JSON.stringify(LSP_SERVERS, null, 2)
  );

  // Copy SETUP.md with installation instructions
  const setupSrc = join(srcDir, "SETUP.md");
  if (existsSync(setupSrc)) {
    cpSync(setupSrc, join(pluginDir, "SETUP.md"));
  }
}

/**
 * Discover all agent files in src/agents/
 */
function discoverAgents(srcDir) {
  const agentsDir = join(srcDir, "agents");
  if (!existsSync(agentsDir)) return [];

  return readdirSync(agentsDir)
    .filter((f) => f.endsWith(".md"))
    .map((f) => f.replace(".md", ""));
}

/**
 * Discover all command files in src/commands/
 */
function discoverCommands(srcDir) {
  const commandsDir = join(srcDir, "commands");
  if (!existsSync(commandsDir)) return [];

  return readdirSync(commandsDir)
    .filter((f) => f.endsWith(".md"))
    .map((f) => f.replace(".md", ""));
}

/**
 * Discover all skill directories in src/skills/
 */
function discoverSkills(srcDir) {
  const skillsDir = join(srcDir, "skills");
  if (!existsSync(skillsDir)) return [];

  return readdirSync(skillsDir).filter((f) => {
    const skillPath = join(skillsDir, f);
    return (
      existsSync(join(skillPath, "SKILL.md")) ||
      existsSync(join(skillPath, "references"))
    );
  });
}

/**
 * Create plugin.json with all hook configurations and MCP servers
 */
function createPluginJson(config, pluginDir, agents, commands, skills) {
  const pluginJson = {
    name: PLUGIN_NAME,
    version: VERSION,
    description: PLUGIN_DESCRIPTION,
    repository: REPOSITORY,
    license: "MIT",
    agents: agents.map((a) => `./agents/${a}.md`),
    commands: commands.map((c) => `./commands/${c}.md`),
    skills: skills.map((s) => `./skills/${s}/SKILL.md`),
    hooks: {},
    mcpServers: MCP_SERVERS,
  };

  // Collect all hooks from config
  const allPreToolHooks = config.hooks["pre-tool"] || [];
  const allPostToolHooks = config.hooks["post-tool"] || [];
  const allSessionHooks = config.hooks.session || [];

  // Pre-tool hooks
  if (allPreToolHooks.length > 0) {
    const preToolByMatcher = groupByMatcher(allPreToolHooks);
    pluginJson.hooks.PreToolUse = Object.entries(preToolByMatcher).map(
      ([matcher, hookList]) => ({
        matcher,
        hooks: hookList.map((h) => ({
          type: "command",
          command: getHookCommand(h),
          ...(h.timeout && { timeout: h.timeout }),
          ...(h.description && { description: h.description }),
        })),
      })
    );
  }

  // Post-tool hooks
  if (allPostToolHooks.length > 0) {
    const postToolByMatcher = groupByMatcher(allPostToolHooks);
    pluginJson.hooks.PostToolUse = Object.entries(postToolByMatcher).map(
      ([matcher, hookList]) => ({
        matcher,
        hooks: hookList.map((h) => ({
          type: "command",
          command: getHookCommand(h),
          ...(h.description && { description: h.description }),
        })),
      })
    );
  }

  // Session hooks
  if (allSessionHooks.length > 0) {
    for (const hook of allSessionHooks) {
      const eventName = hook.event; // SessionStart, SessionEnd, PreCompact
      if (!pluginJson.hooks[eventName]) {
        pluginJson.hooks[eventName] = [];
      }
      pluginJson.hooks[eventName].push({
        hooks: [
          {
            type: "command",
            command: getHookCommand(hook),
            ...(hook.description && { description: hook.description }),
            ...(hook.timeout && { timeout: hook.timeout }),
          },
        ],
      });
    }
  }

  // Write plugin.json
  const pluginJsonDir = join(pluginDir, ".claude-plugin");
  mkdirSync(pluginJsonDir, { recursive: true });
  writeFileSync(
    join(pluginJsonDir, "plugin.json"),
    JSON.stringify(pluginJson, null, 2)
  );
}

/**
 * Group hooks by matcher pattern
 */
function groupByMatcher(hooks) {
  const groups = {};
  for (const hook of hooks) {
    const matcher = hook.matcher || "Edit|Write";
    if (!groups[matcher]) {
      groups[matcher] = [];
    }
    groups[matcher].push(hook);
  }
  return groups;
}

/**
 * Get hook path relative to plugin root
 */
function getHookPath(hook) {
  // Script path like hooks/pre-tool/python-type-check.sh
  // becomes hooks/python-type-check.sh in plugin
  const parts = hook.script.split("/");
  const filename = parts[parts.length - 1];
  return `hooks/${filename}`;
}

/**
 * Get the command to run a hook script with the appropriate interpreter
 */
function getHookCommand(hook) {
  const hookPath = getHookPath(hook);
  const filename = hookPath.split("/").pop();

  // Determine interpreter based on file extension
  if (filename.endsWith(".py")) {
    return `python3 \${CLAUDE_PLUGIN_ROOT}/${hookPath}`;
  } else if (filename.endsWith(".sh")) {
    return `bash \${CLAUDE_PLUGIN_ROOT}/${hookPath}`;
  } else {
    // Default to bash for unknown extensions
    return `bash \${CLAUDE_PLUGIN_ROOT}/${hookPath}`;
  }
}

/**
 * Copy agent files with frontmatter from sidecars
 */
function copyAgents(agents, srcDir, pluginDir) {
  const agentsDir = join(pluginDir, "agents");
  mkdirSync(agentsDir, { recursive: true });

  for (const agent of agents) {
    const srcPath = join(srcDir, "agents", `${agent}.md`);
    const destPath = join(agentsDir, `${agent}.md`);

    if (!existsSync(srcPath)) {
      continue;
    }

    // Load frontmatter from sidecar
    const frontmatter = loadAgentSidecar(srcPath, TARGET_NAME);

    // Read body from source (strip existing frontmatter if any)
    const content = readFileSync(srcPath, "utf-8");
    const { content: body } = matter(content);

    // Write with sidecar frontmatter
    const transformed = matter.stringify(body, frontmatter);
    writeFileSync(destPath, transformed);
  }
}

/**
 * Copy skill directories with frontmatter from SKILL.md + optional extensions
 */
function copySkills(skills, srcDir, pluginDir) {
  const skillsDir = join(pluginDir, "skills");
  mkdirSync(skillsDir, { recursive: true });

  for (const skill of skills) {
    const skillSrc = join(srcDir, "skills", skill);
    const skillDest = join(skillsDir, skill);

    if (!existsSync(skillSrc)) {
      continue;
    }

    mkdirSync(skillDest, { recursive: true });

    // Load base frontmatter from SKILL.md
    const baseFrontmatter = loadSkillFrontmatter(skillSrc);

    // Load optional Claude Code extensions from sidecar
    const extensions = loadSkillExtensions(skillSrc);

    // Merge base with extensions
    const frontmatter = mergeSkillFrontmatter(baseFrontmatter, extensions);

    // Read SKILL.md body
    const skillMdPath = join(skillSrc, "SKILL.md");
    if (existsSync(skillMdPath)) {
      const content = readFileSync(skillMdPath, "utf-8");
      const { content: body } = matter(content);

      // Write with merged frontmatter
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
 * Copy command files with optional sidecar frontmatter and version
 *
 * Sidecar files: {command}.claude-code.yaml
 * Version is injected from package.json at build time
 */
function copyCommands(commands, srcDir, pluginDir) {
  const commandsDir = join(pluginDir, "commands");
  mkdirSync(commandsDir, { recursive: true });

  for (const command of commands) {
    const srcPath = join(srcDir, "commands", `${command}.md`);
    const destPath = join(commandsDir, `${command}.md`);

    if (!existsSync(srcPath)) {
      continue;
    }

    // Read source content
    const content = readFileSync(srcPath, "utf-8");
    const { content: body, data: frontmatter } = matter(content);

    // Load optional sidecar for Claude Code-specific frontmatter
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
 * Copy all hook scripts from all categories
 */
function copyAllHooks(config, srcDir, pluginDir) {
  const hooksDir = join(pluginDir, "hooks");
  mkdirSync(hooksDir, { recursive: true });

  // Copy lib directory
  const libSrc = join(srcDir, "hooks", "lib");
  const libDest = join(hooksDir, "lib");
  if (existsSync(libSrc)) {
    cpSync(libSrc, libDest, { recursive: true });
  }

  // Collect all hook IDs from all categories
  const allHookIds = new Set();

  // Pre-tool hooks
  for (const hook of config.hooks["pre-tool"] || []) {
    allHookIds.add(hook.id);
  }

  // Post-tool hooks
  for (const hook of config.hooks["post-tool"] || []) {
    allHookIds.add(hook.id);
  }

  // Session hooks
  for (const hook of config.hooks.session || []) {
    allHookIds.add(hook.id);
  }

  // Find and copy each hook script
  for (const hookId of allHookIds) {
    // Find hook definition in any category
    const hookDef =
      config.hooks["pre-tool"]?.find((h) => h.id === hookId) ||
      config.hooks["post-tool"]?.find((h) => h.id === hookId) ||
      config.hooks.session?.find((h) => h.id === hookId);

    if (hookDef) {
      const parts = hookDef.script.split("/");
      const filename = parts[parts.length - 1];
      const src = join(srcDir, hookDef.script);
      const dest = join(hooksDir, filename);
      if (existsSync(src)) {
        cpSync(src, dest);
      }
    }
  }

  // Copy subagent hooks
  const subagentHooksSrc = join(srcDir, "hooks", "subagent");
  if (existsSync(subagentHooksSrc)) {
    const files = readdirSync(subagentHooksSrc);
    for (const file of files) {
      const src = join(subagentHooksSrc, file);
      const dest = join(hooksDir, file);
      cpSync(src, dest);
    }
  }
}
