/**
 * Claude Code Build Target
 *
 * Generates a single unified plugin at repo root:
 * plugins/loaf/
 * ├── .claude-plugin/plugin.json
 * ├── agents/
 * ├── skills/
 * └── hooks/
 *
 * Also creates .claude-plugin/marketplace.json at repo root.
 * Scoping: /loaf:implement, Task(loaf:backend-dev)
 */

import {
  mkdirSync,
  cpSync,
  writeFileSync,
  readFileSync,
  existsSync,
  readdirSync,
  rmSync,
  copyFileSync,
  chmodSync,
} from "fs";
import { join } from "path";
import {
  loadSkillExtensions,
} from "../lib/sidecar.js";
import { getVersion } from "../lib/version.js";
import { copySkills } from "../lib/skills.js";
import { copyAgents } from "../lib/agents.js";
import { createCommandSubstituter } from "../lib/commands.js";
import { copySharedTemplates } from "../lib/shared-templates.js";
import { copyDirWithTransform, discoverSkills } from "../lib/copy-utils.js";
import type { BuildContext, HooksConfig, HookDefinition, SkillFrontmatter } from "../types.js";

const TARGET_NAME = "claude-code";
const PLUGIN_NAME = "loaf";
const PLUGIN_DESCRIPTION = "Loaf - An Opinionated Agentic Framework";
const REPOSITORY = "https://github.com/levifig/loaf";

const LSP_SERVERS: Record<string, unknown> = {
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

// Hooks that use `${CLAUDE_PLUGIN_ROOT}/bin/loaf` binary path
const BINARY_PATH_HOOKS = new Set([
  // Enforcement hooks
  "check-secrets",
  "validate-push",
  "validate-commit",
  "workflow-pre-pr",
  "security-audit",
  // Session lifecycle hooks
  "session-start-loaf",
  "session-end-loaf",
  "session-context-inject",
  "post-compact",
  // Journal auto-entry hooks
  "journal-post-commit",
  "journal-post-pr",
  "journal-post-merge",
  // Task management hooks
  "generate-task-board",
  // Task journal hooks
  "journal-task-completed",
  // Linear integration hooks
  "detect-linear-magic",
]);

let VERSION = "0.0.0";

export async function build({
  config,
  targetsConfig,
  rootDir,
  srcDir,
  distDir,
}: BuildContext): Promise<void> {
  VERSION = getVersion(rootDir);

  // distDir is the repo root for claude-code target
  const pluginsDir = join(distDir, "plugins");
  const marketplaceDir = join(distDir, ".claude-plugin");

  if (existsSync(pluginsDir)) rmSync(pluginsDir, { recursive: true });
  if (existsSync(marketplaceDir)) rmSync(marketplaceDir, { recursive: true });

  mkdirSync(pluginsDir, { recursive: true });
  mkdirSync(marketplaceDir, { recursive: true });

  createMarketplace(marketplaceDir);
  buildUnifiedPlugin(config as HooksConfig, rootDir, srcDir, pluginsDir, targetsConfig);
}

function createMarketplace(marketplaceDir: string): void {
  const marketplace = {
    name: "levifig-loaf",
    owner: { name: "Levi Figueira", email: "me@levifig.com" },
    metadata: { description: PLUGIN_DESCRIPTION, version: VERSION },
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

function buildUnifiedPlugin(
  config: HooksConfig,
  rootDir: string,
  srcDir: string,
  pluginsDir: string,
  targetsConfig: BuildContext["targetsConfig"]
): void {
  const pluginDir = join(pluginsDir, PLUGIN_NAME);
  mkdirSync(pluginDir, { recursive: true });

  // Discover all skills to determine knownCommands for scoping
  const allSkills = discoverSkills(join(rootDir, "dist"));

  const knownCommands: string[] = [];
  for (const skill of allSkills) {
    const skillDir = join(rootDir, "content", "skills", skill);
    if (existsSync(skillDir)) {
      const extensions = loadSkillExtensions(skillDir);
      // Only include user-invocable skills (default true)
      if (extensions["user-invocable"] !== false) {
        knownCommands.push(skill);
      }
    }
  }

  // Create base command substituter (universal unscoped substitution)
  const baseTransform = createCommandSubstituter("claude-code");

  // Create scoping transform that applies /loaf: prefix to known commands
  const scopingTransform = (content: string): string => {
    let result = content;
    for (const cmd of knownCommands) {
      // Match /cmd that is not already scoped (e.g., not /loaf:cmd or /other:cmd)
      const pattern = new RegExp(
        `(?<!/\\w+:)\\/${cmd}(?=\\s|\\)|\\]|,|$|\`)`,
        "g"
      );
      result = result.replace(pattern, `/loaf:${cmd}`);
    }
    return result;
  };

  // Combine transforms: base first, then scoping
  const transformMd = (content: string): string =>
    scopingTransform(baseTransform(content));

  createPluginJson(config, pluginDir);

  // Copy agents using shared module with required sidecar
  const agentsDir = join(pluginDir, "agents");
  mkdirSync(agentsDir, { recursive: true });
  copyAgents({
    srcDir,
    destDir: agentsDir,
    targetName: TARGET_NAME,
    version: VERSION,
    sidecarRequired: true,
  });

  // Copy skills using shared module with Claude-specific sidecar merge and scoping
  const skillsDir = join(pluginDir, "skills");
  mkdirSync(skillsDir, { recursive: true });
  copySkills({
    srcDir: join(rootDir, "dist"), // Read from intermediate dist/skills/
    destDir: skillsDir,
    targetName: TARGET_NAME,
    version: VERSION,
    targetsConfig,
    transformMd,
    mergeFrontmatter: (base, skillDir) => {
      // skillDir is from dist/skills/, extract skill name
      const skillName = skillDir.split("/").pop() || "";
      // Load sidecar from content/skills/ (sidecars are not in intermediate)
      const contentSkillDir = join(srcDir, "skills", skillName);
      const extensions = loadSkillExtensions(contentSkillDir);
      const merged = { ...base, ...extensions } as SkillFrontmatter;
      
      // Truncate description at 250 chars for Claude Code (accounting for ellipsis)
      if (merged.description && merged.description.length > 250) {
        merged.description = merged.description.substring(0, 247) + "...";
      }
      
      return merged;
    },
  });

  // Hooks are now defined as direct commands in plugin.json, no need to copy scripts
  // Keep the copyAllHooks for backward compatibility and session hooks
  copyAllHooks(config, srcDir, pluginDir);

  writeFileSync(join(pluginDir, ".lsp.json"), JSON.stringify(LSP_SERVERS, null, 2));

  // Copy plugin-root templates (e.g. soul.md for SessionStart hook self-healing)
  const pluginTemplatesDir = join(pluginDir, "templates");
  const soulTemplateSrc = join(srcDir, "templates", "soul.md");
  if (existsSync(soulTemplateSrc)) {
    mkdirSync(pluginTemplatesDir, { recursive: true });
    cpSync(soulTemplateSrc, join(pluginTemplatesDir, "soul.md"));
  }

  const setupSrc = join(srcDir, "SETUP.md");
  if (existsSync(setupSrc)) {
    cpSync(setupSrc, join(pluginDir, "SETUP.md"));
  }

  // Copy CLI binary to plugin bin/ directory for enforcement hooks
  const binDir = join(pluginDir, "bin");
  mkdirSync(binDir, { recursive: true });
  const cliSource = join(rootDir, "dist-cli", "index.js");
  if (existsSync(cliSource)) {
    copyFileSync(cliSource, join(binDir, "loaf"));
    chmodSync(join(binDir, "loaf"), 0o755);
  }
  
  // Write minimal package.json so the binary can find its version
  const pluginPackageJson = {
    name: PLUGIN_NAME,
    version: VERSION,
  };
  writeFileSync(
    join(pluginDir, "package.json"),
    JSON.stringify(pluginPackageJson, null, 2)
  );
}

function groupByMatcher(hooks: HookDefinition[]): Record<string, HookDefinition[]> {
  const groups: Record<string, HookDefinition[]> = {};
  for (const hook of hooks) {
    const matcher = hook.matcher || "Edit|Write";
    if (!groups[matcher]) groups[matcher] = [];
    groups[matcher].push(hook);
  }
  return groups;
}

function getClaudeHookCommand(hook: HookDefinition): string {
  // Instruction hooks: cat the instruction file from the plugin root
  if (hook.instruction) {
    return `cat "\${CLAUDE_PLUGIN_ROOT}/hooks/${hook.instruction}"`;
  }

  // For binary path hooks (enforcement + session + journal), handle specially
  if (BINARY_PATH_HOOKS.has(hook.id)) {
    // Enforcement hooks don't have a command field - construct it
    if (!hook.command) {
      return `"\${CLAUDE_PLUGIN_ROOT}/bin/loaf" check --hook ${hook.id}`;
    }
    // Session and journal hooks have commands - just substitute the loaf binary path
    return hook.command.replace(/\bloaf\b/g, '"${CLAUDE_PLUGIN_ROOT}/bin/loaf"');
  }

  // If hook has direct command field (not in BINARY_PATH_HOOKS), use as-is
  if (hook.command) {
    return hook.command.replace(/\$\{CLAUDE_PLUGIN_ROOT\}/g, "${CLAUDE_PLUGIN_ROOT}");
  }

  // Otherwise build command from script path (fallback for legacy hooks)
  const parts = hook.script!.split("/");
  const filename = parts[parts.length - 1];
  const hookPath = `hooks/${filename}`;

  if (filename.endsWith(".py")) {
    return `python3 \${CLAUDE_PLUGIN_ROOT}/${hookPath}`;
  }
  return `bash \${CLAUDE_PLUGIN_ROOT}/${hookPath}`;
}

function createPluginJson(config: HooksConfig, pluginDir: string): void {
  // plugin.json — metadata only, no hooks (all hooks go to hooks/hooks.json)
  const pluginJson: Record<string, unknown> = {
    name: PLUGIN_NAME,
    version: VERSION,
    description: PLUGIN_DESCRIPTION,
    repository: REPOSITORY,
    license: "MIT",
  };

  // All hooks go to hooks/hooks.json for consistent loading
  const allPreToolHooks = config.hooks["pre-tool"] || [];
  const allPostToolHooks = config.hooks["post-tool"] || [];
  const allSessionHooks = config.hooks.session || [];
  const allHooks: Record<string, unknown[]> = {};

  if (allPreToolHooks.length > 0) {
    const preToolByMatcher = groupByMatcher(allPreToolHooks);
    allHooks.PreToolUse = Object.entries(preToolByMatcher).map(([matcher, hookList]) => ({
      matcher,
      hooks: hookList.map((h) => {
        if (h.type === "prompt") {
          return {
            type: "prompt" as const,
            prompt: h.prompt!,
            ...(h.if && { if: h.if }),
            ...(h.timeout && { timeout: Math.floor((h.timeout || 5000) / 1000) }),
          };
        }
        return {
          type: "command" as const,
          command: getClaudeHookCommand(h),
          ...(h.if && { if: h.if }),
          ...(h.timeout && { timeout: Math.floor((h.timeout || 30000) / 1000) }),
          ...(h.description && { description: h.description }),
          ...(h.failClosed && { failClosed: h.failClosed }),
        };
      }),
    }));
  }

  if (allPostToolHooks.length > 0) {
    const postToolByMatcher = groupByMatcher(allPostToolHooks);
    allHooks.PostToolUse = Object.entries(postToolByMatcher).map(([matcher, hookList]) => ({
      matcher,
      hooks: hookList.map((h) => {
        if (h.type === "prompt") {
          return {
            type: "prompt" as const,
            prompt: h.prompt!,
            ...(h.if && { if: h.if }),
            ...(h.timeout && { timeout: Math.floor((h.timeout || 5000) / 1000) }),
            ...(h.description && { description: h.description }),
          };
        }
        return {
          type: "command" as const,
          command: getClaudeHookCommand(h),
          ...(h.if && { if: h.if }),
          ...(h.description && { description: h.description }),
          ...(h.timeout && { timeout: Math.floor((h.timeout || 30000) / 1000) }),
          ...(h.failClosed && { failClosed: h.failClosed }),
        };
      }),
    }));
  }

  for (const hook of allSessionHooks) {
    const eventName = hook.event!;
    if (!allHooks[eventName]) allHooks[eventName] = [];

    const hookEntry: Record<string, unknown> = {
      type: hook.type || "command",
      ...(hook.timeout && { timeout: Math.floor((hook.timeout || 60000) / 1000) }),
      ...(hook.description && { description: hook.description }),
      ...(hook.if && { if: hook.if }),
    };

    if (hook.type === "prompt") {
      hookEntry.prompt = hook.prompt!;
    } else {
      hookEntry.command = getClaudeHookCommand(hook);
      if (hook.failClosed) hookEntry.failClosed = hook.failClosed;
    }

    allHooks[eventName].push({ hooks: [hookEntry] });
  }

  // Write hooks/hooks.json
  const hooksJsonDir = join(pluginDir, "hooks");
  mkdirSync(hooksJsonDir, { recursive: true });
  writeFileSync(
    join(hooksJsonDir, "hooks.json"),
    JSON.stringify({ hooks: allHooks }, null, 2)
  );

  const pluginJsonDir = join(pluginDir, ".claude-plugin");
  mkdirSync(pluginJsonDir, { recursive: true });
  writeFileSync(join(pluginJsonDir, "plugin.json"), JSON.stringify(pluginJson, null, 2));
}

function copyAllHooks(config: HooksConfig, srcDir: string, pluginDir: string): void {
  const hooksDir = join(pluginDir, "hooks");
  mkdirSync(hooksDir, { recursive: true });

  // Copy lib directory as-is
  const libSrc = join(srcDir, "hooks", "lib");
  if (existsSync(libSrc)) {
    cpSync(libSrc, join(hooksDir, "lib"), { recursive: true });
  }

  // Collect all hook IDs that still need script files
  // (only session hooks and legacy hooks without direct commands)
  const scriptHookIds = new Set<string>();
  for (const hook of config.hooks["pre-tool"] || []) {
    if (hook.script && !BINARY_PATH_HOOKS.has(hook.id)) {
      scriptHookIds.add(hook.id);
    }
  }
  for (const hook of config.hooks["post-tool"] || []) {
    if (hook.script && !BINARY_PATH_HOOKS.has(hook.id)) {
      scriptHookIds.add(hook.id);
    }
  }
  for (const hook of config.hooks.session || []) {
    if (hook.script) {
      scriptHookIds.add(hook.id);
    }
  }

  // Find and copy each hook script
  for (const hookId of scriptHookIds) {
    const hookDef = config.hooks["pre-tool"]?.find((h) => h.id === hookId) || 
                    config.hooks["post-tool"]?.find((h) => h.id === hookId) || 
                    config.hooks.session?.find((h) => h.id === hookId);

    if (hookDef && hookDef.script) {
      const parts = hookDef.script.split("/");
      const filename = parts[parts.length - 1];
      const src = join(srcDir, hookDef.script);
      const dest = join(hooksDir, filename);
      if (existsSync(src)) cpSync(src, dest);
    }
  }

  // Copy subagent hooks as-is
  const subagentHooksSrc = join(srcDir, "hooks", "subagent");
  if (existsSync(subagentHooksSrc)) {
    const files = readdirSync(subagentHooksSrc);
    for (const file of files) {
      cpSync(join(subagentHooksSrc, file), join(hooksDir, file));
    }
  }

  // Copy instructions directory (used by workflow hooks for inline output)
  const instructionsSrc = join(srcDir, "hooks", "instructions");
  if (existsSync(instructionsSrc)) {
    cpSync(instructionsSrc, join(hooksDir, "instructions"), { recursive: true });
  }
}
