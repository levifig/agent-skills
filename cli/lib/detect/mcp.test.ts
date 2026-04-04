import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import {
  detectClaudeStackMcp,
  detectCursorStackMcp,
  buildMcpStatuses,
  parseClaudeMcpListOutput,
} from "./mcp.js";
import {
  mergeLoafConfigIntegrations,
  readLoafConfig,
} from "../config/agents-config.js";

let TMP: string;

beforeEach(() => {
  TMP = mkdtempSync(join(tmpdir(), "loaf-mcp-"));
});

afterEach(() => {
  rmSync(TMP, { recursive: true, force: true });
});

describe("parseClaudeMcpListOutput", () => {
  it("parses server names from list-like output", () => {
    const s = parseClaudeMcpListOutput("linear\n1. serena\n• foo");
    expect(s.has("linear")).toBe(true);
    expect(s.has("serena")).toBe(true);
    expect(s.has("foo")).toBe(true);
  });

  it("extracts server name from plugin:namespace:server format", () => {
    const s = parseClaudeMcpListOutput("plugin:serena:serena\nplugin:context7:context7");
    expect(s.has("serena")).toBe(true);
    expect(s.has("context7")).toBe(true);
    expect(s.has("plugin:serena:serena")).toBe(true);
  });
});

describe("MCP stack detection", () => {
  it(
    "detects Linear in project .cursor/mcp.json",
    () => {
      const prevHome = process.env.HOME;
      const fakeHome = join(TMP, "home");
      mkdirSync(fakeHome, { recursive: true });
      process.env.HOME = fakeHome;

      mkdirSync(join(TMP, ".cursor"), { recursive: true });
      writeFileSync(
        join(TMP, ".cursor", "mcp.json"),
        JSON.stringify({
          mcpServers: {
            linear: {
              command: "npx",
              args: ["-y", "mcp-remote", "https://mcp.linear.app/mcp"],
            },
          },
        }),
        "utf-8",
      );
      const cur = detectCursorStackMcp(TMP, "linear");
      process.env.HOME = prevHome;
      expect(cur.configured).toBe(true);
      expect(cur.scope).toBe("project");
      const cl = detectClaudeStackMcp(TMP, "linear");
      expect(cl.configured).toBe(false);
    },
    15_000,
  );

  it("keeps Claude vs Cursor detection separate when only Claude is configured", () => {
    const prevHome = process.env.HOME;
    const fakeHome = join(TMP, "h2");
    mkdirSync(join(fakeHome, ".claude"), { recursive: true });
    process.env.HOME = fakeHome;
    writeFileSync(
      join(fakeHome, ".claude", "settings.json"),
      JSON.stringify({
        mcpServers: {
          linear: {
            command: "npx",
            args: ["-y", "mcp-remote", "https://mcp.linear.app/mcp"],
          },
        },
      }),
      "utf-8",
    );
    const rows = buildMcpStatuses(join(TMP, "proj"));
    const linear = rows.find((r) => r.id === "linear");
    expect(linear?.claude.configured).toBe(true);
    expect(linear?.cursor.configured).toBe(false);
    process.env.HOME = prevHome;
  });

  it("buildMcpStatuses returns linear + serena entries", () => {
    const rows = buildMcpStatuses(TMP);
    expect(rows).toHaveLength(2);
    expect(rows.map((r) => r.id).sort()).toEqual(["linear", "serena"]);
  });
});

describe("agents-config merge", () => {
  it("merges integrations without dropping other keys", () => {
    mkdirSync(join(TMP, ".agents"), { recursive: true });
    writeFileSync(
      join(TMP, ".agents", "loaf.json"),
      JSON.stringify({ linear: { workspace: "acme" } }, null, 2),
      "utf-8",
    );
    mergeLoafConfigIntegrations(TMP, { linear: { enabled: true } });
    const cfg = readLoafConfig(TMP);
    expect(cfg.linear).toEqual({ workspace: "acme" });
    expect(cfg.integrations?.linear).toEqual({ enabled: true });
  });
});
