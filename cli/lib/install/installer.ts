/**
 * Target Installation Logic
 *
 * Handles copying built output to tool config directories.
 * Ported from install.sh.
 */

import {
  mkdirSync,
  cpSync,
  writeFileSync,
  rmSync,
  existsSync,
  readdirSync,
  readFileSync,
} from "fs";
import { join, dirname } from "path";
import { execFileSync } from "child_process";
import { fileURLToPath } from "url";

const LOAF_MARKER_FILE = ".loaf-version";
const LOAF_HOOK_MARKER = "loaf-managed";
const LEGACY_LOAF_SIGNATURES = new Set([
  "command:loaf check --hook check-secrets|matcher:Edit|Write|Bash|if:",
  "command:loaf check --hook security-audit|matcher:Bash|if:",
  "command:loaf check --hook validate-push|matcher:Bash|if:",
  "command:loaf check --hook workflow-pre-pr|matcher:Bash|if:",
  "command:loaf check --hook validate-commit|matcher:Bash|if:",
  "command:loaf session log --detect-linear|matcher:Bash|if:",
  "command:loaf task refresh|matcher:Edit|Write|if:",
  "command:loaf session log --from-hook|matcher:Bash|if:Bash(git commit:*)",
  "command:loaf session log --from-hook|matcher:Bash|if:Bash(gh pr create:*)",
  "command:loaf session log --from-hook|matcher:Bash|if:Bash(gh pr merge:*)",
  "command:loaf session start|matcher:|if:",
  "command:loaf session end|matcher:|if:",
  "command:bash $HOME/.cursor/hooks/post-tool/kb-staleness-nudge.sh|matcher:Edit|Write|if:",
  "command:bash $HOME/.cursor/hooks/session/compact.sh|matcher:|if:",
]);
const LEGACY_LOAF_COMMANDS = new Set([
  "bash $HOME/.cursor/hooks/session/session-start-soul.sh",
  "bash $HOME/.cursor/hooks/session/session-start.sh",
  "bash $HOME/.cursor/hooks/session/kb-session-start.sh",
  "bash $HOME/.cursor/hooks/session/session-end.sh",
  "bash $HOME/.cursor/hooks/session/kb-session-end.sh",
  "bash $HOME/.cursor/hooks/session/pre-compact-archive.sh",
]);
const OBSOLETE_CURSOR_HOOK_FILES = [
  "session/check-sessions.sh",
  "session/kb-session-end.sh",
  "session/kb-session-start.sh",
  "session/pre-compact-archive.sh",
  "session/session-end-simple.sh",
  "session/session-end.sh",
  "session/session-start-soul.sh",
  "session/session-start.sh",
];
const LEGACY_LOAF_PROMPT_PREFIXES = [
  "STOP. Before running gh pr merge",
  "ADVISORY: You are about to run `git push`",
  "KNOWLEDGE BASE:",
  "POST-MERGE HOUSEKEEPING:",
  "CONTEXT COMPACTION IMMINENT:",
  "SESSION JOURNAL NUDGE:",
];

function getVersion(): string {
  const __dirname = dirname(fileURLToPath(import.meta.url));
  for (const candidate of [
    join(__dirname, "..", "package.json"),
    join(__dirname, "..", "..", "package.json"),
    join(__dirname, "..", "..", "..", "package.json"),
  ]) {
    try {
      const pkg = JSON.parse(readFileSync(candidate, "utf-8"));
      if (pkg.name === "loaf") return pkg.version;
    } catch {
      continue;
    }
  }
  return "0.0.0";
}

function hasRsync(): boolean {
  try {
    execFileSync("which", ["rsync"], { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}

function syncDir(src: string, dest: string): void {
  mkdirSync(dest, { recursive: true });

  if (hasRsync()) {
    execFileSync("rsync", ["-a", "--delete", `${src}/`, `${dest}/`], {
      stdio: "inherit",
    });
  } else {
    // Fallback: remove and copy
    const entries = readdirSync(dest);
    for (const entry of entries) {
      rmSync(join(dest, entry), { recursive: true, force: true });
    }
    cpSync(src, dest, { recursive: true });
  }
}

function mergeDir(src: string, dest: string): void {
  mkdirSync(dest, { recursive: true });
  cpSync(src, dest, { recursive: true, force: true });
}

function writeMarker(configDir: string): void {
  mkdirSync(configDir, { recursive: true });
  writeFileSync(join(configDir, LOAF_MARKER_FILE), `${getVersion()}\n`);
}

// ─────────────────────────────────────────────────────────────────────────────
// Hook Management Helpers (for user-hooks coexistence)
// ─────────────────────────────────────────────────────────────────────────────

interface CodexHooksJson {
  version?: number;
  hooks?: {
    [key: string]: Array<Record<string, unknown>>;
  };
}

function getHookSignature(hook: Record<string, unknown>): string | null {
  const command = hook.command as string | undefined;
  const prompt = hook.prompt as string | undefined;
  const matcher = hook.matcher as string | undefined;
  const condition = hook.if as string | undefined;

  if (command) {
    return `command:${command}|matcher:${matcher || ""}|if:${condition || ""}`;
  }

  if (prompt) {
    return `prompt:${prompt}|matcher:${matcher || ""}|if:${condition || ""}`;
  }

  return null;
}

function isLegacyLoafHook(hook: Record<string, unknown>): boolean {
  const signature = getHookSignature(hook);
  if (signature && LEGACY_LOAF_SIGNATURES.has(signature)) {
    return true;
  }

  const command = hook.command as string | undefined;
  const prompt = hook.prompt as string | undefined;

  if (command) {
    return LEGACY_LOAF_COMMANDS.has(command);
  }

  if (prompt) {
    return LEGACY_LOAF_PROMPT_PREFIXES.some((prefix) => prompt.startsWith(prefix));
  }

  return false;
}

function isLoafHook(hook: Record<string, unknown>): boolean {
  if (hook[LOAF_HOOK_MARKER] === true) {
    return true;
  }

  return isLegacyLoafHook(hook);
}

function loadHooksJson(path: string): CodexHooksJson {
  if (!existsSync(path)) {
    return { version: 1, hooks: {} };
  }
  try {
    const content = readFileSync(path, "utf-8");
    return JSON.parse(content) as CodexHooksJson;
  } catch {
    return { version: 1, hooks: {} };
  }
}

function saveHooksJson(path: string, hooks: CodexHooksJson): void {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, JSON.stringify(hooks, null, 2));
}

// ─────────────────────────────────────────────────────────────────────────────
// Install Functions
// ─────────────────────────────────────────────────────────────────────────────

export function installOpencode(distDir: string, configDir: string, _upgrade: boolean = false): void {
  const dirs = ["skills", "agents", "commands", "plugins", "templates"];

  for (const dir of dirs) {
    const src = join(distDir, dir);
    const dest = join(configDir, dir);
    if (existsSync(src)) {
      syncDir(src, dest);
    }
  }

  writeMarker(configDir);
}

export function installCursor(distDir: string, configDir: string, upgrade: boolean = false): void {
  // Skills → .agents/skills/ (converged path)
  const homeDir = process.env.HOME || process.env.USERPROFILE || "";
  const skillsDest = join(homeDir, ".agents/skills");
  const skillsSrc = join(distDir, "skills");
  if (existsSync(skillsSrc)) {
    syncDir(skillsSrc, skillsDest);
  }

  // Agents, hooks.json, hook scripts, templates → ~/.cursor/
  const staleCommands = join(configDir, "commands");
  if (existsSync(staleCommands)) {
    rmSync(staleCommands, { recursive: true });
  }

  const agentsSrc = join(distDir, "agents");
  if (existsSync(agentsSrc)) {
    syncDir(agentsSrc, join(configDir, "agents"));
  }

  // Hooks → merge with existing (preserve user hooks, update Loaf hooks)
  const hooksSrc = join(distDir, "hooks.json");
  if (existsSync(hooksSrc)) {
    const hooksPath = join(configDir, "hooks.json");
    const existing = loadHooksJson(hooksPath);
    const loafHooks = loadHooksJson(hooksSrc);

    // Merge: keep user hooks, replace Loaf hooks (or add if new)
    const merged: CodexHooksJson = { version: 1, hooks: {} };
    
    // Process each hook type (PreToolUse, PostToolUse, etc.)
    const allHookTypes = new Set([
      ...Object.keys(existing.hooks || {}),
      ...Object.keys(loafHooks.hooks || {})
    ]);
    
    for (const hookType of allHookTypes) {
      const existingHooks = (existing.hooks?.[hookType] || []) as Record<string, unknown>[];
      const loafHooksList = (loafHooks.hooks?.[hookType] || []) as Record<string, unknown>[];
      
      // Filter out existing Loaf hooks
      const userHooks = existingHooks.filter((hook) => !isLoafHook(hook));
      
      merged.hooks![hookType] = [...userHooks, ...loafHooksList];
    }

    saveHooksJson(hooksPath, merged);
  }

  const hooksDir = join(distDir, "hooks");
  if (existsSync(hooksDir)) {
    if (upgrade) {
      const installedHooksDir = join(configDir, "hooks");
      for (const file of OBSOLETE_CURSOR_HOOK_FILES) {
        rmSync(join(installedHooksDir, file), { force: true });
      }
    }

    mergeDir(hooksDir, join(configDir, "hooks"));
  }

  const templatesSrc = join(distDir, "templates");
  if (existsSync(templatesSrc)) {
    syncDir(templatesSrc, join(configDir, "templates"));
  }

  writeMarker(configDir);
}

export function installCodex(distDir: string, configDir: string, upgrade: boolean = false): void {
  const homeDir = process.env.HOME || process.env.USERPROFILE || "";
  const codexHome = process.env.CODEX_HOME || join(homeDir, ".codex");

  // Skills → .agents/skills/ (converged path)
  const skillsDest = join(homeDir, ".agents/skills");
  const skillsSrc = join(distDir, "skills");
  if (existsSync(skillsSrc)) {
    syncDir(skillsSrc, skillsDest);
  }

  // Hooks → $CODEX_HOME/hooks.json (merge with existing)
  const loafHooksPath = join(distDir, ".codex/hooks.json");
  if (existsSync(loafHooksPath)) {
    const hooksPath = join(codexHome, "hooks.json");
    const existing = loadHooksJson(hooksPath);
    const loafHooks = loadHooksJson(loafHooksPath);

    // Merge: keep user hooks, replace Loaf hooks (or add if new)
    const merged: CodexHooksJson = { version: 1, hooks: {} };
    
    // Process each hook type (PreToolUse, PostToolUse, etc.)
    const allHookTypes = new Set([
      ...Object.keys(existing.hooks || {}),
      ...Object.keys(loafHooks.hooks || {})
    ]);
    
    for (const hookType of allHookTypes) {
      const existingHooks = (existing.hooks?.[hookType] || []) as Record<string, unknown>[];
      const loafHooksList = (loafHooks.hooks?.[hookType] || []) as Record<string, unknown>[];
      
      // Filter out existing Loaf hooks
      const userHooks = existingHooks.filter((hook) => !isLoafHook(hook));
      
      merged.hooks![hookType] = [...userHooks, ...loafHooksList];
    }

    saveHooksJson(hooksPath, merged);
  }

  writeMarker(configDir);
}

export function installGemini(distDir: string, configDir: string, _upgrade: boolean = false): void {
  const homeDir = process.env.HOME || process.env.USERPROFILE || "";
  
  // Skills → .agents/skills/ (converged path)
  const skillsDest = join(homeDir, ".agents/skills");
  const skillsSrc = join(distDir, "skills");
  if (existsSync(skillsSrc)) {
    syncDir(skillsSrc, skillsDest);
  }

  writeMarker(configDir);
}

export function installAmp(distDir: string, configDir: string, _upgrade: boolean = false): void {
  const homeDir = process.env.HOME || process.env.USERPROFILE || "";

  // Skills → .agents/skills/ or ~/.config/agents/skills/
  const skillsDest = process.env.AMP_SKILLS_HOME || join(homeDir, ".config/agents/skills");
  const skillsSrc = join(distDir, "skills");
  if (existsSync(skillsSrc)) {
    syncDir(skillsSrc, skillsDest);
  }

  // Plugins → ~/.amp/plugins/
  const pluginsDest = process.env.AMP_PLUGINS_DIR || join(homeDir, ".amp/plugins");
  const pluginSrc = join(distDir, "plugins/loaf.js");
  if (existsSync(pluginSrc)) {
    mkdirSync(pluginsDest, { recursive: true });
    cpSync(pluginSrc, join(pluginsDest, "loaf.js"));
  }

  writeMarker(configDir);
}

export const INSTALLERS: Record<
  string,
  (distDir: string, configDir: string, upgrade?: boolean) => void
> = {
  opencode: installOpencode,
  cursor: installCursor,
  codex: installCodex,
  gemini: installGemini,
  amp: installAmp,
};
