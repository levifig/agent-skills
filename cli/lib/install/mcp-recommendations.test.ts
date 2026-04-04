import { describe, it, expect } from "vitest";
import { mcpIntegrationDoneForSession } from "./mcp-recommendations.js";
import type { McpStatus } from "../detect/mcp.js";

function linearStatus(
  claude: McpStatus["claude"],
  cursor: McpStatus["cursor"],
): McpStatus {
  return {
    id: "linear",
    displayName: "Linear",
    tier: "recommended",
    claude,
    cursor,
  };
}

describe("mcpIntegrationDoneForSession", () => {
  it("treats integration as done when Claude is configured and Cursor was not a target this run (even if Cursor stack is empty)", () => {
    const st = linearStatus(
      { configured: true, scope: "global" },
      { configured: false, scope: null },
    );
    expect(mcpIntegrationDoneForSession(st, true, false)).toBe(true);
  });

  it("requires Cursor stack when Cursor was a target this run and Linear is missing there", () => {
    const st = linearStatus(
      { configured: true, scope: "global" },
      { configured: false, scope: null },
    );
    expect(mcpIntegrationDoneForSession(st, true, true)).toBe(false);
  });

  it("is done when both stacks are configured and Cursor was targeted", () => {
    const st = linearStatus(
      { configured: true, scope: "global" },
      { configured: true, scope: "project" },
    );
    expect(mcpIntegrationDoneForSession(st, true, true)).toBe(true);
  });

  it("does not inherit Claude/Cursor file state when install is manual-only (e.g. Codex only)", () => {
    const st = linearStatus(
      { configured: true, scope: "global" },
      { configured: true, scope: "project" },
    );
    expect(mcpIntegrationDoneForSession(st, false, false)).toBe(false);
  });
});
