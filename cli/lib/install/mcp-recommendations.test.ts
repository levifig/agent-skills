import { describe, it, expect } from "vitest";
import {
  ensureSerenaNativeInstall,
  mcpIntegrationDoneForSession,
  type SerenaNativeInstallDeps,
} from "./mcp-recommendations.js";
import type { McpStackStatus, McpStatus } from "../detect/mcp.js";

function linearStatus(
  targets: Record<string, McpStackStatus>,
): McpStatus {
  return {
    id: "linear",
    displayName: "Linear",
    tier: "recommended",
    targets,
  };
}

const G: McpStackStatus = { configured: true, scope: "global" };
const P: McpStackStatus = { configured: true, scope: "project" };
const N: McpStackStatus = { configured: false, scope: null };

describe("mcpIntegrationDoneForSession", () => {
  it("returns true when all available targets are configured", () => {
    const st = linearStatus({ "claude-code": G, cursor: P });
    expect(mcpIntegrationDoneForSession(st, ["claude-code", "cursor"])).toBe(true);
  });

  it("returns false when any available target is unconfigured", () => {
    const st = linearStatus({ "claude-code": G, cursor: N });
    expect(mcpIntegrationDoneForSession(st, ["claude-code", "cursor"])).toBe(false);
  });

  it("returns true when checking a subset of configured targets", () => {
    const st = linearStatus({ "claude-code": G, cursor: N });
    expect(mcpIntegrationDoneForSession(st, ["claude-code"])).toBe(true);
  });

  it("returns false when no targets are available", () => {
    const st = linearStatus({ "claude-code": G });
    expect(mcpIntegrationDoneForSession(st, [])).toBe(false);
  });

  it("works with all 6 targets", () => {
    const st = linearStatus({
      "claude-code": G, cursor: P, opencode: G, codex: G, gemini: P, amp: N,
    });
    expect(mcpIntegrationDoneForSession(st, ["claude-code", "cursor", "opencode", "codex", "gemini", "amp"])).toBe(false);
    expect(mcpIntegrationDoneForSession(st, ["claude-code", "cursor", "opencode", "codex", "gemini"])).toBe(true);
  });
});

function serenaDeps(params: {
  hasSerena?: boolean;
  hasUv?: boolean;
  answer?: string;
  serenaAfterInstall?: boolean;
} = {}): SerenaNativeInstallDeps & { logs: string[]; runs: string[][]; asks: string[] } {
  const logs: string[] = [];
  const runs: string[][] = [];
  const asks: string[] = [];
  let installRan = false;

  return {
    logs,
    runs,
    asks,
    commandExists(command: string): boolean {
      if (command === "serena") {
        return params.hasSerena === true || (installRan && params.serenaAfterInstall === true);
      }
      if (command === "uv") return params.hasUv === true;
      return false;
    },
    async ask(question: string): Promise<string> {
      asks.push(question);
      return params.answer ?? "";
    },
    run(command: string, args: string[]): void {
      runs.push([command, ...args]);
      if (command === "uv") installRan = true;
    },
    log(message: string): void {
      logs.push(message);
    },
  };
}

describe("ensureSerenaNativeInstall", () => {
  it("passes without prompting when serena is already installed", async () => {
    const deps = serenaDeps({ hasSerena: true });
    const result = await ensureSerenaNativeInstall(deps);

    expect(result.ok).toBe(true);
    expect(deps.logs).toEqual([]);
    expect(deps.asks).toEqual([]);
    expect(deps.runs).toEqual([]);
  });

  it("prints native install instructions and fails when uv is missing", async () => {
    const deps = serenaDeps({ hasSerena: false, hasUv: false });
    const result = await ensureSerenaNativeInstall(deps);

    expect(result.ok).toBe(false);
    expect(result.message).toContain("uv not found");
    expect(deps.logs.join("\n")).toContain("uv tool install -p 3.13 serena-agent@latest --prerelease=allow");
    expect(deps.logs.join("\n")).toContain("serena init");
    expect(deps.asks).toEqual([]);
    expect(deps.runs).toEqual([]);
  });

  it("does not write MCP config path prerequisites when the user declines uv install", async () => {
    const deps = serenaDeps({ hasSerena: false, hasUv: true, answer: "n" });
    const result = await ensureSerenaNativeInstall(deps);

    expect(result.ok).toBe(false);
    expect(result.message).toContain("Skipped native Serena setup");
    expect(deps.asks).toHaveLength(1);
    expect(deps.runs).toEqual([]);
  });

  it("offers uv install, initializes Serena, and succeeds when serena appears on PATH", async () => {
    const deps = serenaDeps({
      hasSerena: false,
      hasUv: true,
      answer: "y",
      serenaAfterInstall: true,
    });
    const result = await ensureSerenaNativeInstall(deps);

    expect(result.ok).toBe(true);
    expect(deps.runs).toEqual([
      ["uv", "tool", "install", "-p", "3.13", "serena-agent@latest", "--prerelease=allow"],
      ["serena", "init"],
    ]);
  });
});
