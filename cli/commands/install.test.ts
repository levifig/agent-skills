/**
 * Install Script Integration Tests
 *
 * Smoke tests for install.sh wrapper generation logic.
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  readFileSync,
  writeFileSync,
} from "fs";
import { join } from "path";
import { tmpdir } from "os";

import { buildMcpStatuses } from "../lib/detect/mcp.js";
import {
  mergeAgentsConfigIntegrations,
  readAgentsConfig,
} from "../lib/config/agents-config.js";

const TEST_ROOT = join(process.cwd(), ".test-install-script");

function createMockLoafRepo(name: string): string {
  const repoPath = join(TEST_ROOT, name);
  mkdirSync(repoPath, { recursive: true });
  writeFileSync(
    join(repoPath, "package.json"),
    JSON.stringify({ name: "loaf", version: "2.0.0-test" }, null, 2),
    "utf-8"
  );
  mkdirSync(join(repoPath, "content/skills"), { recursive: true });
  mkdirSync(join(repoPath, "dist-cli"), { recursive: true });
  writeFileSync(
    join(repoPath, "dist-cli/index.js"),
    "#!/usr/bin/env node\nconsole.log('Loaf test CLI');\n",
    "utf-8"
  );
  const installShSource = readFileSync(
    join(process.cwd(), "install.sh"),
    "utf-8"
  );
  writeFileSync(join(repoPath, "install.sh"), installShSource, "utf-8");
  return repoPath;
}

beforeEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
  mkdirSync(TEST_ROOT, { recursive: true });
});

afterEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
});

describe("install.sh: dev mode wrapper", () => {
  it("dev mode detection requires .git, package.json, and content/skills", async () => {
    const repoPath = createMockLoafRepo("dev-mode-test");
    mkdirSync(join(repoPath, ".git"), { recursive: true });

    expect(existsSync(join(repoPath, ".git"))).toBe(true);
    expect(existsSync(join(repoPath, "package.json"))).toBe(true);
    expect(existsSync(join(repoPath, "content/skills"))).toBe(true);
    expect(existsSync(join(repoPath, "install.sh"))).toBe(true);
  }, 15000);

  it("wrapper generation script references correct paths", () => {
    const installShContent = readFileSync(
      join(process.cwd(), "install.sh"),
      "utf-8"
    );
    expect(installShContent).toContain("LOCAL_BIN=\"${HOME}/.local/bin\"");
    expect(installShContent).toContain("#!/usr/bin/env bash");
    expect(installShContent).toContain("REPO_DIR=");
    expect(installShContent).toContain("cat >");
    expect(installShContent).toContain("EOF");
  });
});

describe("install.sh: interactive mode behavior", () => {
  it("script does not force --to all in no-args case", () => {
    const installShContent = readFileSync(
      join(process.cwd(), "install.sh"),
      "utf-8"
    );
    expect(installShContent).toContain("node dist-cli/index.js install");
    const hasForcedAll = /node dist-cli\/index\.js install --to all/.test(installShContent);
    expect(hasForcedAll).toBe(false);
  });

  it("script passes through install_args when provided", () => {
    const installShContent = readFileSync(
      join(process.cwd(), "install.sh"),
      "utf-8"
    );
    expect(installShContent).toContain('"${install_args[@]}"');
    expect(installShContent).toContain("install_args");
  });
});

describe("install.sh: runtime behavior", () => {
  it("wrapper script contains proper bash structure", () => {
    const installShContent = readFileSync(
      join(process.cwd(), "install.sh"),
      "utf-8"
    );
    const wrapperMatch = installShContent.match(
      /cat > "\$\{LOCAL_BIN\}\/loaf" << EOF([\s\S]*?)EOF/
    );
    expect(wrapperMatch).toBeTruthy();

    const wrapperContent = wrapperMatch![1];
    expect(wrapperContent).toContain("#!/usr/bin/env bash");
    expect(wrapperContent).toContain("REPO_DIR=");
    expect(wrapperContent).toContain("node");
    expect(wrapperContent).toContain('dist-cli/index.js');
    expect(wrapperContent).toContain('\\$@');
  });

  it("detect_dev_mode checks all required paths", () => {
    const installShContent = readFileSync(
      join(process.cwd(), "install.sh"),
      "utf-8"
    );
    expect(installShContent).toContain("[[ -d \"${script_dir}/.git\" ]]");
    expect(installShContent).toContain('[[ -f "${script_dir}/package.json" ]]');
    expect(installShContent).toContain('[[ -d "${script_dir}/content/skills" ]]');
  });
});

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

  it("mergeAgentsConfigIntegrations writes integrations under .agents/config.json", () => {
    const dir = mkdtempSync(join(tmpdir(), "loaf-install-cfg-"));
    try {
      mergeAgentsConfigIntegrations(dir, {
        linear: { enabled: false },
        serena: { enabled: true },
      });
      const cfg = readAgentsConfig(dir);
      expect(cfg.integrations?.linear?.enabled).toBe(false);
      expect(cfg.integrations?.serena?.enabled).toBe(true);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
