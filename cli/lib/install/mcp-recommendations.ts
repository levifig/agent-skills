/**
 * Post-install MCP recommendation flow.
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "fs";
import { createInterface } from "readline";
import { execFileSync } from "child_process";
import { join } from "path";

import { mergeAgentsConfigIntegrations, readAgentsConfig } from "../config/agents-config.js";
import {
  buildMcpStatuses,
  getMcpDefinition,
  MCP_REGISTRY,
  type McpDefinition,
  type McpStatus,
} from "../detect/mcp.js";
import { detectClaudeCode } from "../detect/tools.js";

const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;
const white = (s: string) => `\x1b[97m${s}\x1b[0m`;

export interface McpRecommendationOptions {
  projectRoot: string;
  upgrade: boolean;
  hasClaudeCode: boolean;
  cursorTargetThisRun: boolean;
  hasAnyDetectedTool: boolean;
  installedTargets?: string[];
}

function askGpn(question: string): Promise<"global" | "project" | "no"> {
  const rl = createInterface({
    input: process.stdin,
    output: process.stdout,
  });
  return new Promise((resolve) => {
    rl.question(question, (answer) => {
      rl.close();
      const a = answer.trim().toLowerCase();
      if (a.startsWith("g")) resolve("global");
      else if (a.startsWith("p")) resolve("project");
      else resolve("no");
    });
  });
}

function claudeScopeFlag(scope: "global" | "project"): string {
  return scope === "global" ? "user" : "project";
}

function installViaClaude(
  def: McpDefinition,
  scope: "global" | "project",
): void {
  execFileSync(
    "claude",
    ["mcp", "add", "--scope", claudeScopeFlag(scope), def.id, "--", ...def.claudeArgs],
    { stdio: "inherit" },
  );
}

function mergeCursorMcpServer(
  projectRoot: string,
  def: McpDefinition,
  scope: "global" | "project",
): void {
  const home = process.env.HOME || process.env.USERPROFILE || "";
  const dir =
    scope === "global" ? join(home, ".cursor") : join(projectRoot, ".cursor");
  const p = join(dir, "mcp.json");
  let data: Record<string, unknown> = {};
  if (existsSync(p)) {
    try {
      data = JSON.parse(readFileSync(p, "utf-8")) as Record<string, unknown>;
    } catch {
      data = {};
    }
  }
  if (!data.mcpServers || typeof data.mcpServers !== "object") {
    data.mcpServers = {};
  }
  const argv = def.cursorArgs ?? def.claudeArgs;
  (data.mcpServers as Record<string, unknown>)[def.id] = {
    command: argv[0],
    args: argv.slice(1),
  };
  if (!existsSync(dir)) {
    mkdirSync(dir, { recursive: true });
  }
  writeFileSync(p, `${JSON.stringify(data, null, 2)}\n`, "utf-8");
}

export function mcpIntegrationDoneForSession(
  st: McpStatus,
  canClaude: boolean,
  cursorTargetThisRun: boolean,
): boolean {
  const hasAutoPath = canClaude || cursorTargetThisRun;
  if (!hasAutoPath) {
    return false;
  }
  return (
    (!canClaude || st.claude.configured) &&
    (!cursorTargetThisRun || st.cursor.configured)
  );
}

function formatStackLine(
  label: string,
  st: { configured: boolean; scope: string | null },
): string {
  if (st.configured) {
    const where =
      st.scope === "global"
        ? "global"
        : st.scope === "project"
          ? "project"
          : "";
    return `    ${green("✓")} ${label}${where ? ` (${where})` : ""}`;
  }
  return `    ${yellow("⚡")} ${label}: not configured`;
}

/**
 * Run after target installation. Skipped when --upgrade.
 * Non-interactive stdin writes safe defaults (disabled) and returns.
 */
export async function runMcpRecommendations(
  opts: McpRecommendationOptions,
): Promise<void> {
  if (opts.upgrade) return;
  if (!process.stdin.isTTY) {
    const existing = readAgentsConfig(opts.projectRoot);
    for (const def of MCP_REGISTRY) {
      if (existing.integrations?.[def.id] === undefined) {
        mergeAgentsConfigIntegrations(opts.projectRoot, {
          [def.id]: { enabled: false },
        });
      }
    }
    return;
  }

  const { projectRoot, hasClaudeCode, cursorTargetThisRun, hasAnyDetectedTool } =
    opts;
  const installedTargets = opts.installedTargets ?? [];
  if (!hasAnyDetectedTool && !hasClaudeCode) {
    return;
  }

  const canClaude = hasClaudeCode && detectClaudeCode();
  const hasAutoPath = canClaude || cursorTargetThisRun;

  console.log(`\n${bold("Recommended MCP servers")}\n`);

  const statuses = buildMcpStatuses(projectRoot);

  for (const st of statuses) {
    const def = getMcpDefinition(st.id);
    if (!def) continue;

    const tierLabel =
      def.tier === "optional" ? `${gray("Optional — ")}` : "";
    console.log(`  ${tierLabel}${bold(def.displayName)}`);

    if (canClaude) {
      console.log(formatStackLine("Claude Code", st.claude));
    }
    if (cursorTargetThisRun) {
      console.log(formatStackLine("Cursor", st.cursor));
    }

    const doneForSession = mcpIntegrationDoneForSession(
      st,
      canClaude,
      cursorTargetThisRun,
    );

    if (doneForSession) {
      mergeAgentsConfigIntegrations(projectRoot, {
        [def.id]: { enabled: true },
      });
      console.log();
      continue;
    }

    const needsClaudeInstall = canClaude && !st.claude.configured;
    const needsCursorInstall = cursorTargetThisRun && !st.cursor.configured;

    if (!hasAutoPath) {
      console.log(`    ${gray("Manual:")} ${white(def.manualHint)}`);
      mergeAgentsConfigIntegrations(projectRoot, {
        [def.id]: { enabled: false },
      });
      console.log();
      continue;
    }

    const choice = await askGpn(
      `    Install missing MCP(s)? [g]lobal / [p]roject / [n]o: `,
    );

    if (choice === "no") {
      mergeAgentsConfigIntegrations(projectRoot, {
        [def.id]: { enabled: false },
      });
      console.log(`    ${gray("○")} Skipped — recorded in .agents/config.json`);
      console.log();
      continue;
    }

    let claudeSuccess = false;
    let cursorSuccess = false;

    if (needsClaudeInstall) {
      try {
        installViaClaude(def, choice);
        claudeSuccess = true;
        console.log(
          `    ${green("✓")} ${def.displayName} added for Claude Code (${claudeScopeFlag(choice)} scope)`,
        );
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        console.log(`    ${yellow("⚠")} Claude CLI failed: ${msg}`);
        console.log(`    ${gray("Manual:")} ${def.manualHint}`);
      }
    }

    if (needsCursorInstall) {
      try {
        mergeCursorMcpServer(projectRoot, def, choice);
        cursorSuccess = true;
        const loc =
          choice === "global" ? "~/.cursor/mcp.json" : ".cursor/mcp.json";
        console.log(
          `    ${green("✓")} ${def.displayName} merged into ${gray(loc)}`,
        );
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        console.log(`    ${yellow("⚠")} Cursor mcp.json merge failed: ${msg}`);
      }
    }

    const stacksOk =
      (!canClaude || st.claude.configured || claudeSuccess) &&
      (!cursorTargetThisRun || st.cursor.configured || cursorSuccess);
    mergeAgentsConfigIntegrations(projectRoot, {
      [def.id]: { enabled: stacksOk },
    });
    if (!stacksOk) {
      console.log(
        `    ${yellow("⚠")} ${gray(`integrations.${def.id}.enabled remains false until every required stack succeeds.`)}`,
      );
    }

    const otherHint = installedTargets.filter((t) =>
      ["opencode", "codex", "amp"].includes(t),
    );
    if (otherHint.length > 0) {
      console.log(
        `    ${gray(`Also configure this MCP in ${otherHint.join(", ")} via each tool's MCP settings.`)}`,
      );
      console.log(`    ${gray("Reference:")} ${white(def.manualHint)}`);
    }

    console.log();
  }

  console.log(
    `${green("✓")} Integration choices recorded under ${gray("integrations")} in ${gray(".agents/config.json")}`,
  );
  console.log();
}
