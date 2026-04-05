/**
 * MCP detection and registry for all supported targets.
 *
 * Supports: Claude Code, Cursor, OpenCode, Codex, Gemini, Amp.
 * Each target has its own config format and file paths.
 */

import { existsSync, readFileSync } from "fs";
import { execFileSync } from "child_process";
import { join } from "path";

import { readLoafConfig } from "../config/agents-config.js";

function home(): string {
  return process.env.HOME || process.env.USERPROFILE || "";
}

export type McpTier = "recommended" | "optional";

export type McpScope = "global" | "project" | null;

export interface McpStackStatus {
  configured: boolean;
  scope: McpScope;
}

export interface McpStatus {
  id: string;
  displayName: string;
  tier: McpTier;
  targets: Record<string, McpStackStatus>;
}

export interface McpDefinition {
  id: string;
  displayName: string;
  tier: McpTier;
  defaultArgs: string[];
  targetArgs?: Partial<Record<string, string[]>>;
  manualHint: string;
}

/* ── Target MCP config paths ────────────────────────────────────────── */

interface TargetMcpConfig {
  /** Absolute paths (after ~ expansion) to scan for global MCP config */
  globalPaths: string[];
  /** Paths relative to project root to scan for project MCP config */
  projectPaths: string[];
  /** JSON key where MCP servers live (dot-separated for nested keys) */
  mcpKey: string;
  /** Config file format */
  format: "json" | "toml";
}

function expandHome(p: string): string {
  return p.startsWith("~/") ? join(home(), p.slice(2)) : p;
}

const TARGET_MCP_CONFIGS: Record<string, TargetMcpConfig> = {
  "claude-code": {
    globalPaths: ["~/.claude/settings.json", "~/.claude/settings.local.json"],
    projectPaths: [".claude/settings.json", ".claude/settings.local.json", ".mcp.json"],
    mcpKey: "mcpServers",
    format: "json",
  },
  cursor: {
    globalPaths: ["~/.cursor/mcp.json"],
    projectPaths: [".cursor/mcp.json"],
    mcpKey: "mcpServers",
    format: "json",
  },
  opencode: {
    globalPaths: ["~/.config/opencode/opencode.json"],
    projectPaths: ["opencode.json"],
    mcpKey: "mcp",
    format: "json",
  },
  codex: {
    globalPaths: ["~/.codex/config.toml"],
    projectPaths: [".codex/config.toml"],
    mcpKey: "mcp_servers",
    format: "toml",
  },
  gemini: {
    globalPaths: ["~/.gemini/settings.json"],
    projectPaths: [".gemini/settings.json"],
    mcpKey: "mcpServers",
    format: "json",
  },
  amp: {
    globalPaths: ["~/.config/amp/settings.json"],
    projectPaths: [".amp/settings.json"],
    mcpKey: "amp.mcpServers",
    format: "json",
  },
};

export function getTargetMcpConfig(target: string): TargetMcpConfig | undefined {
  return TARGET_MCP_CONFIGS[target];
}

export function mcpSupportedTargets(): string[] {
  return Object.keys(TARGET_MCP_CONFIGS);
}

/* ── MCP registry ───────────────────────────────────────────────────── */

const SERENA_BASE_ARGS = [
  "uvx", "-p", "3.13", "--from",
  "git+https://github.com/oraios/serena",
  "serena", "start-mcp-server",
];

export const MCP_REGISTRY: McpDefinition[] = [
  {
    id: "linear",
    displayName: "Linear",
    tier: "recommended",
    defaultArgs: ["npx", "-y", "mcp-remote", "https://mcp.linear.app/mcp"],
    manualHint:
      "npx -y mcp-remote https://mcp.linear.app/mcp",
  },
  {
    id: "serena",
    displayName: "Serena",
    tier: "optional",
    defaultArgs: [...SERENA_BASE_ARGS, "--project-from-cwd"],
    targetArgs: {
      "claude-code": [...SERENA_BASE_ARGS, "--context", "claude-code", "--project-from-cwd"],
      cursor: [...SERENA_BASE_ARGS, "--context", "cursor", "--project-from-cwd"],
    },
    manualHint:
      "uvx -p 3.13 --from git+https://github.com/oraios/serena serena start-mcp-server --project-from-cwd",
  },
];

/* ── Blob matchers (MCP-specific content detection) ─────────────────── */

function blobMatchesLinear(blob: string): boolean {
  const b = blob.toLowerCase();
  return b.includes("mcp.linear.app") || /\blinear\b/.test(b);
}

function blobMatchesSerena(blob: string): boolean {
  const b = blob.toLowerCase();
  return (
    b.includes("oraios/serena") ||
    b.includes("serena start-mcp-server") ||
    (b.includes("serena") && b.includes("uvx"))
  );
}

function getMatcher(mcpId: string): ((blob: string) => boolean) | null {
  if (mcpId === "linear") return blobMatchesLinear;
  if (mcpId === "serena") return blobMatchesSerena;
  return null;
}

/* ── Claude CLI detection (special case) ────────────────────────────── */

export function parseClaudeMcpListOutput(output: string): Set<string> {
  const names = new Set<string>();
  for (const line of output.split(/\r?\n/)) {
    const t = line.trim();
    if (!t) continue;
    const m = t.match(/^(?:[\d.]+\s+|[•*-]\s+)?([a-zA-Z0-9_:-]+)\b/);
    if (!m) continue;
    const full = m[1].toLowerCase();
    names.add(full);
    const parts = full.split(":");
    if (parts.length > 1) {
      names.add(parts[parts.length - 1]);
    }
  }
  return names;
}

let claudeListCache: Set<string> | null = null;

function claudeListNames(): Set<string> {
  if (process.env.VITEST === "true" || process.env.LOAF_SKIP_CLAUDE_MCP_LIST === "1") {
    return new Set();
  }
  if (claudeListCache) return claudeListCache;
  try {
    const out = execFileSync("claude", ["mcp", "list"], {
      encoding: "utf-8",
      stdio: ["ignore", "pipe", "pipe"],
      maxBuffer: 2 * 1024 * 1024,
      timeout: 2500,
    });
    claudeListCache = parseClaudeMcpListOutput(out);
  } catch {
    claudeListCache = new Set();
  }
  return claudeListCache;
}

/* ── Generic JSON scanner ───────────────────────────────────────────── */

function safeJson(path: string): unknown {
  try {
    if (!existsSync(path)) return null;
    return JSON.parse(readFileSync(path, "utf-8"));
  } catch {
    return null;
  }
}

/**
 * Resolve a potentially dot-separated key (e.g. "amp.mcpServers") into
 * the nested object value.
 */
function resolveKey(obj: unknown, key: string): unknown {
  if (!obj || typeof obj !== "object") return undefined;
  // Try direct key first (handles "amp.mcpServers" as a literal key)
  const o = obj as Record<string, unknown>;
  if (key in o) return o[key];
  // Try dot-path traversal
  const parts = key.split(".");
  let current: unknown = obj;
  for (const part of parts) {
    if (!current || typeof current !== "object") return undefined;
    current = (current as Record<string, unknown>)[part];
  }
  return current;
}

function scanMcpServersGeneric(
  raw: unknown,
  mcpKey: string,
  id: string,
  match: (blob: string) => boolean,
): boolean {
  const servers = resolveKey(raw, mcpKey);
  if (!servers || typeof servers !== "object") return false;
  const entries = servers as Record<string, unknown>;
  if (id in entries) return true;
  for (const [, spec] of Object.entries(entries)) {
    if (match(JSON.stringify(spec))) return true;
  }
  return false;
}

/**
 * Scan a TOML config file for MCP server presence.
 * Uses simple string matching — avoids needing a TOML parser.
 */
function scanTomlForMcp(
  path: string,
  id: string,
  match: (blob: string) => boolean,
): boolean {
  try {
    if (!existsSync(path)) return false;
    const content = readFileSync(path, "utf-8");
    // Check for [mcp_servers.{id}] section header
    if (content.includes(`[mcp_servers.${id}]`)) return true;
    // Fall back to blob matching
    return match(content);
  } catch {
    return false;
  }
}

/* ── Per-target detection ───────────────────────────────────────────── */

export function detectMcpForTarget(
  projectRoot: string,
  target: string,
  mcpId: string,
): McpStackStatus {
  const config = TARGET_MCP_CONFIGS[target];
  if (!config) return { configured: false, scope: null };

  const match = getMatcher(mcpId);
  if (!match) return { configured: false, scope: null };

  if (config.format === "toml") {
    // TOML targets: simple string scan (avoids parser dependency)
    for (const p of config.globalPaths) {
      if (scanTomlForMcp(expandHome(p), mcpId, match)) {
        return { configured: true, scope: "global" };
      }
    }
    for (const p of config.projectPaths) {
      if (scanTomlForMcp(join(projectRoot, p), mcpId, match)) {
        return { configured: true, scope: "project" };
      }
    }
  } else {
    // JSON targets: structured scan
    for (const p of config.globalPaths) {
      const j = safeJson(expandHome(p));
      if (scanMcpServersGeneric(j, config.mcpKey, mcpId, match)) {
        return { configured: true, scope: "global" };
      }
    }
    for (const p of config.projectPaths) {
      const j = safeJson(join(projectRoot, p));
      if (scanMcpServersGeneric(j, config.mcpKey, mcpId, match)) {
        return { configured: true, scope: "project" };
      }
    }
  }

  // Claude Code special case: check `claude mcp list` output
  if (target === "claude-code") {
    if (claudeListNames().has(mcpId)) {
      return { configured: true, scope: "global" };
    }
  }

  return { configured: false, scope: null };
}

/* ── Backward-compatible wrappers ───────────────────────────────────── */

export function detectClaudeStackMcp(
  projectRoot: string,
  id: "linear" | "serena",
): McpStackStatus {
  return detectMcpForTarget(projectRoot, "claude-code", id);
}

export function detectCursorStackMcp(
  projectRoot: string,
  id: "linear" | "serena",
): McpStackStatus {
  return detectMcpForTarget(projectRoot, "cursor", id);
}

/* ── Status builders ────────────────────────────────────────────────── */

export function buildMcpStatuses(
  projectRoot: string,
  targets?: string[],
): McpStatus[] {
  const targetList = targets ?? Object.keys(TARGET_MCP_CONFIGS);
  return MCP_REGISTRY.map((def) => {
    const targetStatuses: Record<string, McpStackStatus> = {};
    for (const t of targetList) {
      targetStatuses[t] = detectMcpForTarget(projectRoot, t, def.id);
    }
    return {
      id: def.id,
      displayName: def.displayName,
      tier: def.tier,
      targets: targetStatuses,
    };
  });
}

export function getMcpDefinition(id: string): McpDefinition | undefined {
  return MCP_REGISTRY.find((d) => d.id === id);
}

/**
 * Get the effective args for an MCP on a specific target.
 */
export function getMcpArgs(def: McpDefinition, target: string): string[] {
  return def.targetArgs?.[target] ?? def.defaultArgs;
}

export function isLinearIntegrationDisabled(projectRoot: string): boolean {
  const cfg = readLoafConfig(projectRoot);
  return cfg.integrations?.linear?.enabled === false;
}
