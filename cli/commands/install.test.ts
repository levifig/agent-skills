/**
 * Install Command Tests
 *
 * Tests for the loaf install CLI command and its helpers.
 */

import { describe, it, expect } from "vitest";
import {
  mkdtempSync,
  rmSync,
  readFileSync,
  writeFileSync,
} from "fs";
import { join } from "path";
import { tmpdir } from "os";

import { buildMcpStatuses } from "../lib/detect/mcp.js";
import {
  mergeLoafConfigIntegrations,
  readLoafConfig,
} from "../lib/config/agents-config.js";

describe("loaf install: MCP recommendation helpers", () => {
  it("buildMcpStatuses lists Linear and Serena", () => {
    const dir = mkdtempSync(join(tmpdir(), "loaf-install-mcp-"));
    try {
      const rows = buildMcpStatuses(dir);
      expect(rows.map((r) => r.id).sort()).toEqual(["linear", "serena"]);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  it("mergeLoafConfigIntegrations writes integrations under .agents/loaf.json", () => {
    const dir = mkdtempSync(join(tmpdir(), "loaf-install-cfg-"));
    try {
      mergeLoafConfigIntegrations(dir, {
        linear: { enabled: false },
        serena: { enabled: true },
      });
      const cfg = readLoafConfig(dir);
      expect(cfg.integrations?.linear?.enabled).toBe(false);
      expect(cfg.integrations?.serena?.enabled).toBe(true);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
