/**
 * Post-install MCP recommendation flow.
 *
 * For each registered MCP:
 *   1. Ask once: install? [n]o / [g]lobal / [p]roject (default: no)
 *   2. If yes: which targets? [a]ll / comma-separated list
 *   3. Install for selected targets, record in loaf.json
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "fs";
import { createInterface } from "readline";
import { execFileSync } from "child_process";
import { join } from "path";

import { mergeLoafConfigIntegrations, readLoafConfig } from "../config/agents-config.js";
import {
  buildMcpStatuses,
  getMcpArgs,
  getMcpDefinition,
  getTargetMcpConfig,
  MCP_REGISTRY,
  type McpDefinition,
  type McpStatus,
} from "../detect/mcp.js";
import { detectClaudeCode } from "../detect/tools.js";

const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;

export interface McpRecommendationOptions {
  projectRoot: string;
  upgrade: boolean;
  availableTargets: string[];
}

/* ── Prompts ────────────────────────────────────────────────────────── */

function ask(question: string): Promise<string> {
  const rl = createInterface({
    input: process.stdin,
    output: process.stdout,
  });
  return new Promise((resolve) => {
    rl.question(question, (answer) => {
      rl.close();
      resolve(answer.trim().toLowerCase());
    });
  });
}

function parseScope(answer: string): "global" | "project" | "no" {
  if (answer.startsWith("g")) return "global";
  if (answer.startsWith("p")) return "project";
  return "no";
}

/* ── Target installation dispatch ───────────────────────────────────── */

function installViaClaude(
  def: McpDefinition,
  scope: "global" | "project",
): void {
  const scopeFlag = scope === "global" ? "user" : "project";
  const args = getMcpArgs(def, "claude-code");
  execFileSync(
    "claude",
    ["mcp", "add", "--scope", scopeFlag, def.id, "--", ...args],
    { stdio: "inherit" },
  );
}

function mergeJsonMcpConfig(
  filePath: string,
  mcpKey: string,
  serverId: string,
  args: string[],
): void {
  const dir = join(filePath, "..");
  if (!existsSync(dir)) mkdirSync(dir, { recursive: true });

  let data: Record<string, unknown> = {};
  if (existsSync(filePath)) {
    try {
      data = JSON.parse(readFileSync(filePath, "utf-8")) as Record<string, unknown>;
    } catch {
      data = {};
    }
  }

  // Resolve dot-key (e.g., "amp.mcpServers") — use literal key for top-level
  let servers: Record<string, unknown>;
  if (mcpKey in data && typeof data[mcpKey] === "object" && data[mcpKey] !== null) {
    servers = data[mcpKey] as Record<string, unknown>;
  } else if (mcpKey.includes(".")) {
    // Also check nested path
    const parts = mcpKey.split(".");
    let current = data;
    for (const part of parts.slice(0, -1)) {
      if (!current[part] || typeof current[part] !== "object") {
        current[part] = {};
      }
      current = current[part] as Record<string, unknown>;
    }
    const lastPart = parts[parts.length - 1];
    if (!current[lastPart] || typeof current[lastPart] !== "object") {
      current[lastPart] = {};
    }
    servers = current[lastPart] as Record<string, unknown>;
  } else {
    data[mcpKey] = {};
    servers = data[mcpKey] as Record<string, unknown>;
  }

  servers[serverId] = {
    command: args[0],
    args: args.slice(1),
  };

  writeFileSync(filePath, `${JSON.stringify(data, null, 2)}\n`, "utf-8");
}

function mergeOpencodeMcpConfig(
  filePath: string,
  serverId: string,
  args: string[],
): void {
  const dir = join(filePath, "..");
  if (!existsSync(dir)) mkdirSync(dir, { recursive: true });

  let data: Record<string, unknown> = {};
  if (existsSync(filePath)) {
    try {
      data = JSON.parse(readFileSync(filePath, "utf-8")) as Record<string, unknown>;
    } catch {
      data = {};
    }
  }

  if (!data.mcp || typeof data.mcp !== "object") {
    data.mcp = {};
  }
  const servers = data.mcp as Record<string, unknown>;
  // OpenCode uses command as array, requires type field
  servers[serverId] = {
    type: "local",
    command: args,
    enabled: true,
  };

  writeFileSync(filePath, `${JSON.stringify(data, null, 2)}\n`, "utf-8");
}

function installMcpForTarget(
  target: string,
  def: McpDefinition,
  scope: "global" | "project",
  projectRoot: string,
): { ok: boolean; message: string } {
  const args = getMcpArgs(def, target);
  const config = getTargetMcpConfig(target);
  if (!config) return { ok: false, message: `Unknown target: ${target}` };

  try {
    if (target === "claude-code") {
      // Use Claude CLI for native installation
      if (!detectClaudeCode()) {
        return { ok: false, message: "Claude Code CLI not found" };
      }
      installViaClaude(def, scope);
      return { ok: true, message: `${scope} scope` };
    }

    if (target === "codex") {
      // Codex uses CLI; fall back to manual hint for project scope
      try {
        execFileSync("codex", ["mcp", "add", def.id, "--", ...args], {
          stdio: "inherit",
        });
        return { ok: true, message: scope === "global" ? "global (via CLI)" : "project (via CLI)" };
      } catch {
        return { ok: false, message: "Codex CLI failed — configure manually in .codex/config.toml" };
      }
    }

    // All other JSON targets: file merge
    const filePath = scope === "global"
      ? join(process.env.HOME || "", config.globalPaths[0].replace("~/", ""))
      : join(projectRoot, config.projectPaths[0]);

    if (target === "opencode") {
      mergeOpencodeMcpConfig(filePath, def.id, args);
    } else {
      mergeJsonMcpConfig(filePath, config.mcpKey, def.id, args);
    }

    const loc = scope === "global" ? config.globalPaths[0] : config.projectPaths[0];
    return { ok: true, message: `merged into ${loc}` };
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    return { ok: false, message: msg };
  }
}

/* ── Status display ─────────────────────────────────────────────────── */

function formatTargetLine(
  target: string,
  st: { configured: boolean; scope: string | null },
): string {
  if (st.configured) {
    const where = st.scope === "global" ? "global" : st.scope === "project" ? "project" : "";
    return `    ${green("✓")} ${target}${where ? ` (${where})` : ""}`;
  }
  return `    ${yellow("⚡")} ${target}: not configured`;
}

/* ── Integration-done check ─────────────────────────────────────────── */

/**
 * Returns true when an MCP is already configured on all available targets.
 */
export function mcpIntegrationDoneForSession(
  st: McpStatus,
  availableTargets: string[],
): boolean {
  if (availableTargets.length === 0) return false;
  return availableTargets.every((t) => st.targets[t]?.configured);
}

/* ── Main flow ──────────────────────────────────────────────────────── */

/**
 * Run after target installation. Skipped when --upgrade.
 * Non-interactive stdin writes safe defaults (disabled) and returns.
 */
export async function runMcpRecommendations(
  opts: McpRecommendationOptions,
): Promise<void> {
  if (opts.upgrade) return;
  if (!process.stdin.isTTY) {
    const existing = readLoafConfig(opts.projectRoot);
    for (const def of MCP_REGISTRY) {
      if (existing.integrations?.[def.id] === undefined) {
        mergeLoafConfigIntegrations(opts.projectRoot, {
          [def.id]: { enabled: false },
        });
      }
    }
    return;
  }

  const { projectRoot, availableTargets } = opts;
  if (availableTargets.length === 0) return;

  console.log(`\n${bold("Recommended MCP servers")}\n`);

  const statuses = buildMcpStatuses(projectRoot, availableTargets);

  for (const st of statuses) {
    const def = getMcpDefinition(st.id);
    if (!def) continue;

    const tierLabel = def.tier === "optional" ? `${gray("Optional — ")}` : "";
    console.log(`  ${tierLabel}${bold(def.displayName)}`);

    // Show current status across all available targets
    for (const t of availableTargets) {
      const ts = st.targets[t];
      if (ts) console.log(formatTargetLine(t, ts));
    }

    // Already configured everywhere?
    if (mcpIntegrationDoneForSession(st, availableTargets)) {
      mergeLoafConfigIntegrations(projectRoot, {
        [def.id]: { enabled: true },
      });
      console.log();
      continue;
    }

    // Ask once: scope (combines yes/no + scope selection)
    const scopeAnswer = await ask(
      `    Install? [n]o / [g]lobal / [p]roject (default: no): `,
    );
    const scope = parseScope(scopeAnswer);

    if (scope === "no") {
      mergeLoafConfigIntegrations(projectRoot, {
        [def.id]: { enabled: false },
      });
      console.log(`    ${gray("○")} Skipped — recorded in .agents/loaf.json`);
      console.log();
      continue;
    }

    // Find targets that need installation
    const unconfigured = availableTargets.filter(
      (t) => !st.targets[t]?.configured,
    );

    // Ask which targets (only if multiple unconfigured)
    let selectedTargets: string[];
    if (unconfigured.length === 1) {
      selectedTargets = unconfigured;
    } else {
      console.log(`\n    Unconfigured targets:`);
      unconfigured.forEach((t, i) => {
        console.log(`      ${i + 1}. ${t}`);
      });
      const targetAnswer = await ask(
        `    Which targets? [a]ll / comma-separated numbers: `,
      );
      if (targetAnswer.startsWith("a") || targetAnswer === "") {
        selectedTargets = unconfigured;
      } else {
        const indices = targetAnswer
          .split(",")
          .map((s) => parseInt(s.trim(), 10) - 1)
          .filter((i) => i >= 0 && i < unconfigured.length);
        selectedTargets = indices.length > 0
          ? indices.map((i) => unconfigured[i])
          : unconfigured;
      }
    }

    // Install for each selected target
    let allOk = true;
    for (const target of selectedTargets) {
      const result = installMcpForTarget(target, def, scope, projectRoot);
      if (result.ok) {
        console.log(`    ${green("✓")} ${target}: ${result.message}`);
      } else {
        console.log(`    ${yellow("⚠")} ${target}: ${result.message}`);
        allOk = false;
      }
    }

    // Check final state: all available targets configured?
    const alreadyConfigured = availableTargets.filter(
      (t) => st.targets[t]?.configured,
    );
    const totalOk = alreadyConfigured.length + (allOk ? selectedTargets.length : 0);
    const enabled = totalOk === availableTargets.length;

    mergeLoafConfigIntegrations(projectRoot, {
      [def.id]: { enabled },
    });

    if (!enabled) {
      console.log(
        `    ${yellow("⚠")} ${gray(`integrations.${def.id}.enabled remains false until all targets succeed.`)}`,
      );
    }

    console.log();
  }

  console.log(
    `${green("✓")} Integration choices recorded under ${gray("integrations")} in ${gray(".agents/loaf.json")}`,
  );
  console.log();
}
