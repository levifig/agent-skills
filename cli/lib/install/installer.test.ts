import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "fs";
import { join } from "path";

import { installCursor } from "./installer.js";

const TEST_ROOT = join(process.cwd(), ".test-install-script");

describe("installCursor", () => {
  const originalHome = process.env.HOME;

  beforeEach(() => {
    rmSync(TEST_ROOT, { recursive: true, force: true });
    mkdirSync(TEST_ROOT, { recursive: true });
    process.env.HOME = TEST_ROOT;
  });

  afterEach(() => {
    process.env.HOME = originalHome;
    rmSync(TEST_ROOT, { recursive: true, force: true });
  });

  it("replaces legacy Loaf hooks without removing user loaf commands", () => {
    const distDir = join(TEST_ROOT, "dist");
    const configDir = join(TEST_ROOT, ".cursor");
    const hooksPath = join(configDir, "hooks.json");

    mkdirSync(distDir, { recursive: true });
    mkdirSync(configDir, { recursive: true });
    mkdirSync(join(configDir, "hooks"), { recursive: true });
    writeFileSync(join(configDir, "hooks", "custom.sh"), "#!/usr/bin/env bash\n", "utf-8");

    writeFileSync(
      hooksPath,
      JSON.stringify(
        {
          version: 1,
          hooks: {
            postToolUse: [
              {
                command: "loaf task refresh",
                matcher: "Edit|Write",
              },
              {
                prompt: "KNOWLEDGE BASE: The file you just edited may have stale knowledge coverage.",
                matcher: "Edit|Write",
              },
              {
                command: "loaf kb review docs/knowledge/guide.md",
                matcher: "Bash",
              },
              {
                command: "loaf task refresh",
                matcher: "Bash",
              },
              {
                command: "bash $HOME/.cursor/hooks/custom.sh",
                matcher: "Bash",
              },
            ],
          },
        },
        null,
        2
      ),
      "utf-8"
    );

    writeFileSync(
      join(distDir, "hooks.json"),
      JSON.stringify(
        {
          version: 1,
          hooks: {
            postToolUse: [
              {
                "loaf-managed": true,
                command: "loaf task refresh",
                matcher: "Edit|Write",
              },
              {
                "loaf-managed": true,
                prompt: "KNOWLEDGE BASE: The file you just edited may have stale knowledge coverage.",
                matcher: "Edit|Write",
              },
            ],
          },
        },
        null,
        2
      ),
      "utf-8"
    );

    installCursor(distDir, configDir);

    const installed = JSON.parse(readFileSync(hooksPath, "utf-8"));
    const postToolUse = installed.hooks.postToolUse;
    expect(readFileSync(join(configDir, "hooks", "custom.sh"), "utf-8")).toContain("#!/usr/bin/env bash");

    expect(postToolUse).toEqual([
      {
        command: "loaf kb review docs/knowledge/guide.md",
        matcher: "Bash",
      },
      {
        command: "loaf task refresh",
        matcher: "Bash",
      },
      {
        command: "bash $HOME/.cursor/hooks/custom.sh",
        matcher: "Bash",
      },
      {
        "loaf-managed": true,
        command: "loaf task refresh",
        matcher: "Edit|Write",
      },
      {
        "loaf-managed": true,
        prompt: "KNOWLEDGE BASE: The file you just edited may have stale knowledge coverage.",
        matcher: "Edit|Write",
      },
    ]);
  });

  it("replaces the legacy session end hook with the hook-safe variant", () => {
    const distDir = join(TEST_ROOT, "dist-session-end");
    const configDir = join(TEST_ROOT, ".cursor-session-end");
    const hooksPath = join(configDir, "hooks.json");

    mkdirSync(distDir, { recursive: true });
    mkdirSync(configDir, { recursive: true });

    writeFileSync(
      hooksPath,
      JSON.stringify(
        {
          version: 1,
          hooks: {
            sessionEnd: [
              {
                command: "loaf session end",
              },
            ],
          },
        },
        null,
        2
      ),
      "utf-8"
    );

    writeFileSync(
      join(distDir, "hooks.json"),
      JSON.stringify(
        {
          version: 1,
          hooks: {
            sessionEnd: [
              {
                "loaf-managed": true,
                command: "loaf session end --if-active",
              },
            ],
          },
        },
        null,
        2
      ),
      "utf-8"
    );

    installCursor(distDir, configDir);

    const installed = JSON.parse(readFileSync(hooksPath, "utf-8"));
    expect(installed.hooks.sessionEnd).toEqual([
      {
        "loaf-managed": true,
        command: "loaf session end --if-active",
      },
    ]);
  });

  it("removes hook types that only contained obsolete Loaf-managed hooks", () => {
    const distDir = join(TEST_ROOT, "dist-obsolete-hook-types");
    const configDir = join(TEST_ROOT, ".cursor-obsolete-hook-types");
    const hooksPath = join(configDir, "hooks.json");

    mkdirSync(distDir, { recursive: true });
    mkdirSync(configDir, { recursive: true });

    writeFileSync(
      hooksPath,
      JSON.stringify(
        {
          version: 1,
          hooks: {
            postCompact: [
              {
                "loaf-managed": true,
                command: "loaf session context for-resumption",
              },
            ],
            userpromptsubmit: [
              {
                "loaf-managed": true,
                command: "loaf session context for-prompt",
              },
            ],
            taskcompleted: [
              {
                "loaf-managed": true,
                command: "loaf session log --from-hook",
              },
            ],
            beforeSubmitPrompt: [
              {
                command: "custom prompt hook",
              },
            ],
          },
        },
        null,
        2
      ),
      "utf-8"
    );

    writeFileSync(
      join(distDir, "hooks.json"),
      JSON.stringify(
        {
          version: 1,
          hooks: {
            beforeSubmitPrompt: [
              {
                "loaf-managed": true,
                command: "loaf session context for-prompt",
              },
            ],
          },
        },
        null,
        2
      ),
      "utf-8"
    );

    installCursor(distDir, configDir);

    const installed = JSON.parse(readFileSync(hooksPath, "utf-8"));
    expect(installed.hooks.postCompact).toBeUndefined();
    expect(installed.hooks.userpromptsubmit).toBeUndefined();
    expect(installed.hooks.taskcompleted).toBeUndefined();
    expect(installed.hooks.beforeSubmitPrompt).toEqual([
      {
        command: "custom prompt hook",
      },
      {
        "loaf-managed": true,
        command: "loaf session context for-prompt",
      },
    ]);
  });

  it("removes obsolete Loaf hook scripts during cursor upgrades", () => {
    const distDir = join(TEST_ROOT, "dist-upgrade");
    const configDir = join(TEST_ROOT, ".cursor-upgrade");
    const hooksDir = join(configDir, "hooks", "session");

    mkdirSync(join(distDir, "hooks", "session"), { recursive: true });
    mkdirSync(hooksDir, { recursive: true });
    writeFileSync(join(hooksDir, "session-start.sh"), "old\n", "utf-8");
    writeFileSync(join(distDir, "hooks", "session", "compact.sh"), "new\n", "utf-8");

    installCursor(distDir, configDir, true);

    expect(existsSync(join(hooksDir, "session-start.sh"))).toBe(false);
    expect(readFileSync(join(hooksDir, "compact.sh"), "utf-8")).toBe("new\n");
  });
});
