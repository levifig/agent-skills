/**
 * Claude Code Build Target
 *
 * Generates plugin marketplace structure at repo root:
 * agent-skills/
 * ├── .claude-plugin/
 * │   └── marketplace.json
 * └── plugins/
 *     ├── orchestration/
 *     │   ├── .claude-plugin/plugin.json
 *     │   ├── agents/
 *     │   ├── skills/
 *     │   ├── commands/
 *     │   └── hooks/
 *     └── ...
 *
 * This allows Claude Code to use:
 *   /plugin marketplace add levifig/agent-skills
 */

import {
  mkdirSync,
  cpSync,
  writeFileSync,
  readFileSync,
  existsSync,
  rmSync,
} from "fs";
import { join } from "path";

const VERSION = "1.0.0";
const REPOSITORY = "https://github.com/levifig/agent-skills";

/**
 * Build Claude Code distribution to repo root
 */
export async function build({ config, rootDir, srcDir, distDir }) {
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

  // Create marketplace.json
  createMarketplace(config, marketplaceDir);

  // Build each plugin group
  const pluginGroups = config["plugin-groups"];
  for (const [pluginName, pluginConfig] of Object.entries(pluginGroups)) {
    buildPlugin(pluginName, pluginConfig, config, srcDir, pluginsDir);
  }
}

/**
 * Create marketplace.json for plugin discovery
 */
function createMarketplace(config, marketplaceDir) {
  const pluginGroups = config["plugin-groups"];

  const marketplace = {
    name: "levifig-agent-skills",
    owner: {
      name: "Levi Figueira",
      email: "me@levifig.com",
    },
    metadata: {
      description:
        "Universal agent skills for AI coding assistants. PM orchestration, code foundations, and language-specific plugins.",
      version: VERSION,
    },
    plugins: Object.entries(pluginGroups).map(([name, cfg]) => ({
      name,
      description: cfg.description,
      source: `./plugins/${name}`,
      version: VERSION,
      license: "MIT",
      repository: REPOSITORY,
    })),
  };

  writeFileSync(
    join(marketplaceDir, "marketplace.json"),
    JSON.stringify(marketplace, null, 2)
  );
}

/**
 * Build a single plugin
 */
function buildPlugin(pluginName, pluginConfig, config, srcDir, pluginsDir) {
  const pluginDir = join(pluginsDir, pluginName);
  mkdirSync(pluginDir, { recursive: true });

  // Create plugin.json
  createPluginJson(pluginName, pluginConfig, config, pluginDir);

  // Copy agents
  copyAgents(pluginConfig.agents, srcDir, pluginDir);

  // Copy skills
  copySkills(pluginConfig.skills, srcDir, pluginDir);

  // Copy commands (only for orchestration)
  if (pluginConfig.commands) {
    copyCommands(pluginConfig.commands, srcDir, pluginDir);
  }

  // Copy hooks
  copyHooks(pluginConfig.hooks, config, srcDir, pluginDir);
}

/**
 * Create plugin.json with hook configuration
 */
function createPluginJson(pluginName, pluginConfig, config, pluginDir) {
  const pluginJson = {
    name: pluginName,
    version: VERSION,
    description: pluginConfig.description,
    repository: REPOSITORY,
    license: "MIT",
    agents: pluginConfig.agents.map((a) => `./agents/${a}.md`),
    hooks: {},
  };

  // Add commands if present
  if (pluginConfig.commands) {
    pluginJson.commands = pluginConfig.commands.map(
      (c) => `./commands/${c}.md`
    );
  }

  // Add skills if present
  if (pluginConfig.skills) {
    pluginJson.skills = pluginConfig.skills.map(
      (s) => `./skills/${s}/SKILL.md`
    );
  }

  // Build hooks configuration
  const hooks = pluginConfig.hooks || {};

  // Pre-tool hooks
  if (hooks["pre-tool"]) {
    const preToolHooks = hooks["pre-tool"]
      .map((hookId) => config.hooks["pre-tool"].find((h) => h.id === hookId))
      .filter(Boolean);

    const preToolByMatcher = groupByMatcher(preToolHooks);
    pluginJson.hooks.PreToolUse = Object.entries(preToolByMatcher).map(
      ([matcher, hookList]) => ({
        matcher,
        hooks: hookList.map((h) => ({
          type: "command",
          command: `bash \${CLAUDE_PLUGIN_ROOT}/${getHookPath(h)}`,
          ...(h.timeout && { timeout: h.timeout }),
          ...(h.description && { description: h.description }),
        })),
      })
    );
  }

  // Post-tool hooks
  if (hooks["post-tool"]) {
    const postToolHooks = hooks["post-tool"]
      .map((hookId) => config.hooks["post-tool"].find((h) => h.id === hookId))
      .filter(Boolean);

    const postToolByMatcher = groupByMatcher(postToolHooks);
    pluginJson.hooks.PostToolUse = Object.entries(postToolByMatcher).map(
      ([matcher, hookList]) => ({
        matcher,
        hooks: hookList.map((h) => ({
          type: "command",
          command: `bash \${CLAUDE_PLUGIN_ROOT}/${getHookPath(h)}`,
          ...(h.description && { description: h.description }),
        })),
      })
    );
  }

  // Session hooks
  if (hooks.session) {
    const sessionHooks = hooks.session
      .map((hookId) => config.hooks.session.find((h) => h.id === hookId))
      .filter(Boolean);

    for (const hook of sessionHooks) {
      const eventName = hook.event; // SessionStart, SessionEnd, PreCompact
      if (!pluginJson.hooks[eventName]) {
        pluginJson.hooks[eventName] = [];
      }
      pluginJson.hooks[eventName].push({
        hooks: [
          {
            type: "command",
            command: `bash \${CLAUDE_PLUGIN_ROOT}/${getHookPath(hook)}`,
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
 * Copy agent files
 */
function copyAgents(agents, srcDir, pluginDir) {
  const agentsDir = join(pluginDir, "agents");
  mkdirSync(agentsDir, { recursive: true });

  for (const agent of agents) {
    const src = join(srcDir, "agents", `${agent}.md`);
    const dest = join(agentsDir, `${agent}.md`);
    if (existsSync(src)) {
      cpSync(src, dest);
    }
  }
}

/**
 * Copy skill directories
 */
function copySkills(skills, srcDir, pluginDir) {
  const skillsDir = join(pluginDir, "skills");
  mkdirSync(skillsDir, { recursive: true });

  for (const skill of skills) {
    const src = join(srcDir, "skills", skill);
    const dest = join(skillsDir, skill);
    if (existsSync(src)) {
      cpSync(src, dest, { recursive: true });
    }
  }
}

/**
 * Copy command files
 */
function copyCommands(commands, srcDir, pluginDir) {
  const commandsDir = join(pluginDir, "commands");
  mkdirSync(commandsDir, { recursive: true });

  for (const command of commands) {
    const src = join(srcDir, "commands", `${command}.md`);
    const dest = join(commandsDir, `${command}.md`);
    if (existsSync(src)) {
      cpSync(src, dest);
    }
  }
}

/**
 * Copy hook scripts
 */
function copyHooks(hooks, config, srcDir, pluginDir) {
  if (!hooks) return;

  const hooksDir = join(pluginDir, "hooks");
  mkdirSync(hooksDir, { recursive: true });

  // Copy lib directory
  const libSrc = join(srcDir, "hooks", "lib");
  const libDest = join(hooksDir, "lib");
  if (existsSync(libSrc)) {
    cpSync(libSrc, libDest, { recursive: true });
  }

  // Collect all hook IDs
  const hookIds = [
    ...(hooks["pre-tool"] || []),
    ...(hooks["post-tool"] || []),
    ...(hooks.session || []),
  ];

  // Find and copy each hook script
  for (const hookId of hookIds) {
    // Find hook definition
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
}
