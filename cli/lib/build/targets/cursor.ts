/**
 * Cursor Build Target
 *
 * Cursor supports: skills, agents (subagents), hooks
 * Generates hooks.json for Cursor's hook system.
 */

import {
  mkdirSync,
  cpSync,
  writeFileSync,
  readdirSync,
  existsSync,
} from "fs";
import { join } from "path";
import { resetTargetOutput } from "../lib/target-output.js";
import { loadTargetSkillSidecar } from "../lib/sidecar.js";
import { getVersion, injectVersion } from "../lib/version.js";

import { copySkills } from "../lib/skills.js";
import { copyAgents } from "../lib/agents.js";
import type { BuildContext, HooksConfig, HookDefinition } from "../types.js";

const TARGET_NAME = "cursor";
const LOAF_HOOK_MARKER = "loaf-managed";

const DEFAULT_AGENT_FRONTMATTER = {
  model: "inherit",
  is_background: true,
};

// Hooks that use `loaf` binary path
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
  // Journal auto-entry hooks
  "journal-post-commit",
  "journal-post-pr",
  "journal-post-merge",
]);

function substituteCommands(content: string): string {
  return content
    .replace(/\{\{IMPLEMENT_CMD\}\}/g, "/implement")
    .replace(/\{\{RESUME_CMD\}\}/g, "/resume")
    .replace(/\{\{ORCHESTRATE_CMD\}\}/g, "/implement");
}

export async function build({
  config,
  targetConfig,
  targetsConfig,
  rootDir,
  srcDir,
  distDir,
}: BuildContext): Promise<void> {
  const version = getVersion(rootDir);
  const transformMd = (content: string) => substituteCommands(content);

  resetTargetOutput(distDir, ["skills", "agents", "hooks"]);

  const skillsDir = join(distDir, "skills");
  const agentsDir = join(distDir, "agents");
  const hooksDir = join(distDir, "hooks");

  // Copy skills using shared module with Cursor-specific extensions
  // The shared module expects srcDir/skills/, so we pass the intermediate directory
  // which has skills/ directly in it
  copySkills({
    srcDir: join(rootDir, "dist"),
    destDir: skillsDir,
    targetName: TARGET_NAME,
    version,
    targetsConfig,
    transformMd,
    extraDirs: ["assets"],
    mergeFrontmatter: (base, skillDir) => {
      const sidecarFrontmatter = loadTargetSkillSidecar(skillDir, TARGET_NAME);
      return injectVersion({ ...base, ...sidecarFrontmatter }, version);
    },
  });

  // Copy agents using shared module with optional sidecars
  copyAgents({
    srcDir,
    destDir: agentsDir,
    targetName: TARGET_NAME,
    version,
    sidecarRequired: false,
    defaults: DEFAULT_AGENT_FRONTMATTER,
  });
  
  // Copy remaining shell hooks used by session/pre-compact flows.
  copyHooks(srcDir, hooksDir);
  generateHooksJson(config as HooksConfig, distDir);

  // Copy plugin-root templates (e.g. soul.md for SessionStart hook)
  const soulTemplateSrc = join(srcDir, "templates", "soul.md");
  if (existsSync(soulTemplateSrc)) {
    const templatesDir = join(distDir, "templates");
    mkdirSync(templatesDir, { recursive: true });
    cpSync(soulTemplateSrc, join(templatesDir, "soul.md"));
  }
}

function copyHooks(srcDir: string, destDir: string): void {
  const hooksSrc = join(srcDir, "hooks");
  if (!existsSync(hooksSrc)) return;

  const subdirs = ["session", "post-tool", "lib", "instructions"];

  for (const subdir of subdirs) {
    const subSrc = join(hooksSrc, subdir);
    const subDest = join(destDir, subdir);

    if (existsSync(subSrc)) {
      mkdirSync(subDest, { recursive: true });
      copyHookFiles(subSrc, subDest);
    }
  }

  // Copy top-level hook files
  const entries = readdirSync(hooksSrc, { withFileTypes: true });
  for (const entry of entries) {
    if (entry.isFile()) {
      cpSync(join(hooksSrc, entry.name), join(destDir, entry.name));
    }
  }
}

/**
 * Copy hook files with Cursor-specific override support.
 * If foo.cursor.sh exists, it replaces foo.sh in the output.
 */
function copyHookFiles(srcDir: string, destDir: string): void {
  const entries = readdirSync(srcDir, { withFileTypes: true });

  const cursorOverrides = new Set<string>();
  for (const entry of entries) {
    if (entry.isFile() && entry.name.endsWith(".cursor.sh")) {
      cursorOverrides.add(entry.name.replace(".cursor.sh", ".sh"));
    }
  }

  for (const entry of entries) {
    if (entry.isDirectory()) {
      const subSrc = join(srcDir, entry.name);
      const subDest = join(destDir, entry.name);
      mkdirSync(subDest, { recursive: true });
      copyHookFiles(subSrc, subDest);
    } else if (entry.isFile()) {
      if (entry.name.endsWith(".cursor.sh")) {
        const destName = entry.name.replace(".cursor.sh", ".sh");
        cpSync(join(srcDir, entry.name), join(destDir, destName));
      } else if (!cursorOverrides.has(entry.name)) {
        cpSync(join(srcDir, entry.name), join(destDir, entry.name));
      }
    }
  }
}

function mapSessionEvent(event: string): string {
  const mapping: Record<string, string> = {
    SessionStart: "sessionStart",
    SessionEnd: "sessionEnd",
    PreCompact: "preCompact",
    Stop: "stop",
  };
  return mapping[event] || event.toLowerCase();
}

function getCursorHookCommand(hook: HookDefinition): string {
  // Instruction hooks: cat the instruction file from the Cursor hooks directory
  if (hook.instruction) {
    return `cat "$HOME/.cursor/hooks/${hook.instruction}"`;
  }

  // For binary path hooks (enforcement + session + journal), handle specially
  if (BINARY_PATH_HOOKS.has(hook.id)) {
    // Enforcement hooks don't have a command field - construct it
    if (!hook.command) {
      return `loaf check --hook ${hook.id}`;
    }
    // Session and journal hooks have commands - keep them as-is
    return hook.command;
  }

  // If hook has direct command field (not in BINARY_PATH_HOOKS), use it
  if (hook.command) {
    return hook.command;
  }

  // Otherwise build command from script path (fallback for legacy hooks)
  const scriptPath = hook.script!.replace(/^hooks\//, "");
  const basePath = "$HOME/.cursor/hooks";

  if (hook.script!.endsWith(".py")) {
    return `python3 ${basePath}/${scriptPath}`;
  } else if (hook.script!.endsWith(".ts")) {
    return `bun run ${basePath}/${scriptPath}`;
  }
  return `bash ${basePath}/${scriptPath}`;
}

function generateHooksJson(config: HooksConfig, distDir: string): void {
  const preToolHooks = config.hooks["pre-tool"] || [];
  const postToolHooks = config.hooks["post-tool"] || [];
  const sessionHooks = config.hooks.session || [];

  const hooksJson: Record<string, unknown> = {
    version: 1,
    hooks: {} as Record<string, unknown>,
  };

  const hooks = hooksJson.hooks as Record<string, unknown>;

  if (preToolHooks.length > 0) {
    hooks.preToolUse = preToolHooks.map((hook) => {
      const result: Record<string, unknown> = {
        [LOAF_HOOK_MARKER]: true,
        timeout: Math.floor((hook.timeout || 60000) / 1000),
        ...(hook.matcher && { matcher: hook.matcher }),
        ...(hook.failClosed && { failClosed: hook.failClosed }),
      };
      
      // Command hooks get a command field; prompt hooks get prompt
      // Both can have optional 'if' condition for command-scoped filtering
      if (hook.type === "prompt") {
        if (hook.prompt) result.prompt = hook.prompt;
      } else {
        result.command = getCursorHookCommand(hook);
      }
      if (hook.if) result.if = hook.if;
      
      return result;
    });
  }

  if (postToolHooks.length > 0) {
    hooks.postToolUse = postToolHooks.map((hook) => {
      const result: Record<string, unknown> = {
        [LOAF_HOOK_MARKER]: true,
        timeout: Math.floor((hook.timeout || 30000) / 1000),
        ...(hook.matcher && { matcher: hook.matcher }),
        ...(hook.failClosed && { failClosed: hook.failClosed }),
      };
      
      // Command hooks get a command field; prompt hooks get prompt
      // Both can have optional 'if' condition for command-scoped filtering
      if (hook.type === "prompt") {
        if (hook.prompt) result.prompt = hook.prompt;
      } else {
        result.command = getCursorHookCommand(hook);
      }
      if (hook.if) result.if = hook.if;
      
      return result;
    });
  }

  for (const hook of sessionHooks) {
    const eventName = mapSessionEvent(hook.event || "");
    if (!hooks[eventName]) hooks[eventName] = [];
    
    const result: Record<string, unknown> = {
      [LOAF_HOOK_MARKER]: true,
      timeout: Math.floor((hook.timeout || 60000) / 1000),
    };
    
    // Command hooks get a command field; prompt hooks get prompt
    // Both can have optional 'if' condition for command-scoped filtering
    if (hook.type === "prompt") {
      if (hook.prompt) result.prompt = hook.prompt;
    } else {
      result.command = getCursorHookCommand(hook);
    }
    if (hook.if) result.if = hook.if;
    if (hook.failClosed) result.failClosed = hook.failClosed;
    
    (hooks[eventName] as unknown[]).push(result);
  }

  writeFileSync(
    join(distDir, "hooks.json"),
    JSON.stringify(hooksJson, null, 2),
  );
}
