import { describe, it, expect } from "vitest";
import { mcpIntegrationDoneForSession } from "./mcp-recommendations.js";
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
