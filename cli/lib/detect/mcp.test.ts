import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import {
  detectClaudeStackMcp,
  detectCursorStackMcp,
  detectMcpForTarget,
  buildMcpStatuses,
  parseClaudeMcpListOutput,
  mcpSupportedTargets,
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

  it("detects Linear in project .gemini/settings.json", () => {
    const prevHome = process.env.HOME;
    const fakeHome = join(TMP, "home");
    mkdirSync(fakeHome, { recursive: true });
    process.env.HOME = fakeHome;

    mkdirSync(join(TMP, ".gemini"), { recursive: true });
    writeFileSync(
      join(TMP, ".gemini", "settings.json"),
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
    const result = detectMcpForTarget(TMP, "gemini", "linear");
    process.env.HOME = prevHome;
    expect(result.configured).toBe(true);
    expect(result.scope).toBe("project");
  });

  it("detects Serena in project opencode.json via mcp key", () => {
    const prevHome = process.env.HOME;
    const fakeHome = join(TMP, "home");
    mkdirSync(fakeHome, { recursive: true });
    process.env.HOME = fakeHome;

    writeFileSync(
      join(TMP, "opencode.json"),
      JSON.stringify({
        mcp: {
          serena: {
            type: "local",
            command: ["uvx", "-p", "3.13", "--from", "git+https://github.com/oraios/serena", "serena", "start-mcp-server"],
          },
        },
      }),
      "utf-8",
    );
    const result = detectMcpForTarget(TMP, "opencode", "serena");
    process.env.HOME = prevHome;
    expect(result.configured).toBe(true);
    expect(result.scope).toBe("project");
  });

  it("detects Linear in .amp/settings.json via amp.mcpServers key", () => {
    const prevHome = process.env.HOME;
    const fakeHome = join(TMP, "home");
    mkdirSync(fakeHome, { recursive: true });
    process.env.HOME = fakeHome;

    mkdirSync(join(TMP, ".amp"), { recursive: true });
    writeFileSync(
      join(TMP, ".amp", "settings.json"),
      JSON.stringify({
        "amp.mcpServers": {
          linear: {
            command: "npx",
            args: ["-y", "mcp-remote", "https://mcp.linear.app/mcp"],
          },
        },
      }),
      "utf-8",
    );
    const result = detectMcpForTarget(TMP, "amp", "linear");
    process.env.HOME = prevHome;
    expect(result.configured).toBe(true);
    expect(result.scope).toBe("project");
  });

  it("detects Linear in .codex/config.toml via string scanning", () => {
    const prevHome = process.env.HOME;
    const fakeHome = join(TMP, "home");
    mkdirSync(fakeHome, { recursive: true });
    process.env.HOME = fakeHome;

    mkdirSync(join(TMP, ".codex"), { recursive: true });
    writeFileSync(
      join(TMP, ".codex", "config.toml"),
      `[mcp_servers.linear]\ncommand = "npx"\nargs = ["-y", "mcp-remote", "https://mcp.linear.app/mcp"]\n`,
      "utf-8",
    );
    const result = detectMcpForTarget(TMP, "codex", "linear");
    process.env.HOME = prevHome;
    expect(result.configured).toBe(true);
    expect(result.scope).toBe("project");
  });

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
    const rows = buildMcpStatuses(join(TMP, "proj"), ["claude-code", "cursor"]);
    const linear = rows.find((r) => r.id === "linear");
    expect(linear?.targets["claude-code"].configured).toBe(true);
    expect(linear?.targets.cursor.configured).toBe(false);
    process.env.HOME = prevHome;
  });

  it("buildMcpStatuses returns linear + serena entries", () => {
    const rows = buildMcpStatuses(TMP, ["claude-code", "cursor"]);
    expect(rows).toHaveLength(2);
    expect(rows.map((r) => r.id).sort()).toEqual(["linear", "serena"]);
  });
});

describe("mcpSupportedTargets", () => {
  it("includes all 6 targets", () => {
    const targets = mcpSupportedTargets();
    expect(targets).toContain("claude-code");
    expect(targets).toContain("cursor");
    expect(targets).toContain("opencode");
    expect(targets).toContain("codex");
    expect(targets).toContain("gemini");
    expect(targets).toContain("amp");
    expect(targets).toHaveLength(6);
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
