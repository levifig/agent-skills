/**
 * MCP detection for Linear and Serena across Claude Code and Cursor.
 */

import { existsSync, readFileSync } from "fs";
import { execFileSync } from "child_process";
import { join } from "path";

import { readAgentsConfig } from "../config/agents-config.js";

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
  claude: McpStackStatus;
  cursor: McpStackStatus;
}

export interface McpDefinition {
  id: "linear" | "serena";
  displayName: string;
  tier: McpTier;
  claudeArgs: string[];
  cursorArgs?: string[];
  manualHint: string;
}

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
    claudeArgs: ["npx", "-y", "mcp-remote", "https://mcp.linear.app/mcp"],
    manualHint:
      "claude mcp add [--scope user|project] linear -- npx -y mcp-remote https://mcp.linear.app/mcp",
  },
  {
    id: "serena",
    displayName: "Serena",
    tier: "optional",
    claudeArgs: [...SERENA_BASE_ARGS, "--context", "claude-code", "--project-from-cwd"],
    cursorArgs: [...SERENA_BASE_ARGS, "--context", "cursor", "--project-from-cwd"],
    manualHint:
      "claude mcp add [--scope user|project] serena -- uvx -p 3.13 --from git+https://github.com/oraios/serena serena start-mcp-server --context claude-code --project-from-cwd",
  },
];

function safeJson(path: string): unknown {
  try {
    if (!existsSync(path)) return null;
    return JSON.parse(readFileSync(path, "utf-8"));
  } catch {
    return null;
  }
}

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

export function parseClaudeMcpListOutput(output: string): Set<string> {
  const names = new Set<string>();
  for (const line of output.split(/\r?\n/)) {
    const t = line.trim();
    if (!t) continue;
    const m = t.match(/^(?:[\d.]+\s+|[•*-]\s+)?([a-zA-Z0-9_:-]+)\b/);
    if (!m) continue;
    const full = m[1].toLowerCase();
    names.add(full);
    // Plugin MCPs use plugin:<plugin>:<server> format -- extract the server name
    const parts = full.split(":");
    if (parts.length > 1) {
      names.add(parts[parts.length - 1]);
    }
  }
  return names;
}

function scanMcpServers(
  raw: unknown,
  id: string,
  match: (blob: string) => boolean,
): boolean {
  if (!raw || typeof raw !== "object") return false;
  const o = raw as Record<string, unknown>;
  const servers = o.mcpServers;
  if (!servers || typeof servers !== "object") return false;
  const entries = servers as Record<string, unknown>;
  if (id in entries) return true;
  for (const [, spec] of Object.entries(entries)) {
    if (match(JSON.stringify(spec))) return true;
  }
  return false;
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

export function detectClaudeStackMcp(
  projectRoot: string,
  id: "linear" | "serena",
): McpStackStatus {
  const match = id === "linear" ? blobMatchesLinear : blobMatchesSerena;
  const globalPath = join(home(), ".claude", "settings.json");
  const globalLocal = join(home(), ".claude", "settings.local.json");
  for (const p of [globalPath, globalLocal]) {
    const j = safeJson(p);
    if (scanMcpServers(j, id, match)) {
      return { configured: true, scope: "global" };
    }
  }
  for (const p of [
    join(projectRoot, ".claude", "settings.json"),
    join(projectRoot, ".claude", "settings.local.json"),
    join(projectRoot, ".mcp.json"),
  ]) {
    const j = safeJson(p);
    if (scanMcpServers(j, id, match)) {
      return { configured: true, scope: "project" };
    }
  }
  if (claudeListNames().has(id)) {
    return { configured: true, scope: "global" };
  }
  return { configured: false, scope: null };
}

export function detectCursorStackMcp(
  projectRoot: string,
  id: "linear" | "serena",
): McpStackStatus {
  const match = id === "linear" ? blobMatchesLinear : blobMatchesSerena;
  const globalPath = join(home(), ".cursor", "mcp.json");
  const projectPath = join(projectRoot, ".cursor", "mcp.json");
  const g = scanMcpServers(safeJson(globalPath), id, match);
  const p = scanMcpServers(safeJson(projectPath), id, match);
  if (p) return { configured: true, scope: "project" };
  if (g) return { configured: true, scope: "global" };
  return { configured: false, scope: null };
}

export function buildMcpStatuses(projectRoot: string): McpStatus[] {
  return MCP_REGISTRY.map((def) => {
    const claude = detectClaudeStackMcp(projectRoot, def.id);
    const cursor = detectCursorStackMcp(projectRoot, def.id);
    return {
      id: def.id,
      displayName: def.displayName,
      tier: def.tier,
      claude,
      cursor,
    };
  });
}

export function getMcpDefinition(id: string): McpDefinition | undefined {
  return MCP_REGISTRY.find((d) => d.id === id);
}

export function isLinearIntegrationDisabled(projectRoot: string): boolean {
  const cfg = readAgentsConfig(projectRoot);
  return cfg.integrations?.linear?.enabled === false;
}
