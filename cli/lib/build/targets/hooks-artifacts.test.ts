/**
 * Build Hook Artifacts Regression Tests
 *
 * Tests that verify generated hook artifacts (hooks.json, plugin.json)
 * preserve critical fields like `if`, `failClosed`, `matcher`, `timeout`, and `command`.
 * 
 * @vitest-environment node
 */

import { describe, it, expect, beforeAll } from "vitest";
import { existsSync, readFileSync } from "fs";
import { join } from "path";

// ─────────────────────────────────────────────────────────────────────────────
// Test Setup
// ─────────────────────────────────────────────────────────────────────────────

const DIST_DIR = join(process.cwd(), "dist");
const PLUGINS_DIR = join(process.cwd(), "plugins");

interface CursorHook {
  timeout?: number;
  matcher?: string;
  failClosed?: boolean;
  command?: string;
  prompt?: string;
  if?: string;
}

interface CursorHooksJson {
  version: number;
  hooks: {
    preToolUse?: CursorHook[];
    postToolUse?: CursorHook[];
    sessionStart?: CursorHook[];
    sessionEnd?: CursorHook[];
    preCompact?: CursorHook[];
    stop?: CursorHook[];
  };
}

interface ClaudeHook {
  type: "command" | "prompt";
  command?: string;
  prompt?: string;
  if?: string;
  timeout?: number;
  description?: string;
  failClosed?: boolean;
}

interface ClaudePluginJson {
  name: string;
  version: string;
  hooks: {
    PreToolUse?: Array<{
      matcher: string;
      hooks: ClaudeHook[];
    }>;
    PostToolUse?: Array<{
      matcher: string;
      hooks: ClaudeHook[];
    }>;
    SessionStart?: Array<{ hooks: ClaudeHook[] }>;
    SessionEnd?: Array<{ hooks: ClaudeHook[] }>;
    PreCompact?: Array<{ hooks: ClaudeHook[] }>;
    Stop?: Array<{ hooks: ClaudeHook[] }>;
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// Cursor Tests
// ─────────────────────────────────────────────────────────────────────────────

describe("cursor hooks.json", () => {
  let hooksJson: CursorHooksJson;

  beforeAll(() => {
    const hooksPath = join(DIST_DIR, "cursor", "hooks.json");
    if (!existsSync(hooksPath)) {
      throw new Error("hooks.json not found. Run `npm run build` first.");
    }
    hooksJson = JSON.parse(readFileSync(hooksPath, "utf-8")) as CursorHooksJson;
  });

  it("has correct version", () => {
    expect(hooksJson.version).toBe(1);
  });

  it("preserves 'if' on pre-tool instruction hooks", () => {
    const preToolUse = hooksJson.hooks.preToolUse || [];

    // Find workflow-pre-merge hook (instruction hook with if: Bash(gh pr merge:*))
    const workflowPreMerge = preToolUse.find(
      h => h.command?.includes("instructions/pre-merge.md")
    );
    expect(workflowPreMerge).toBeDefined();
    expect(workflowPreMerge?.if).toBe("Bash(gh pr merge:*)");

    // Find workflow-pre-push hook (instruction hook with if: Bash(git push:*))
    const workflowPrePush = preToolUse.find(
      h => h.command?.includes("instructions/pre-push.md")
    );
    expect(workflowPrePush).toBeDefined();
    expect(workflowPrePush?.if).toBe("Bash(git push:*)");
  });

  it("preserves 'if' on post-tool instruction hooks", () => {
    const postToolUse = hooksJson.hooks.postToolUse || [];

    // Find workflow-post-merge hook (instruction hook with if: Bash(gh pr merge:*))
    const workflowPostMerge = postToolUse.find(
      h => h.command?.includes("instructions/post-merge.md")
    );
    expect(workflowPostMerge).toBeDefined();
    expect(workflowPostMerge?.if).toBe("Bash(gh pr merge:*)");
  });

  it("preserves 'if' on post-tool command hooks (journal auto-entry)", () => {
    const postToolUse = hooksJson.hooks.postToolUse || [];

    // Find journal-git-events hook (git commit)
    const journalGit = postToolUse.find(
      h => h.command === "loaf session log --from-hook" && h.if?.includes("git commit")
    );
    expect(journalGit).toBeDefined();
    expect(journalGit?.if).toBe("Bash(git commit:*)");

    // Find journal-gh-events hook (gh pr create + merge)
    const journalGh = postToolUse.find(
      h => h.command === "loaf session log --from-hook" && h.if?.includes("gh pr")
    );
    expect(journalGh).toBeDefined();
    expect(journalGh?.if).toBe("Bash(gh pr:*)");
  });

  it("preserves 'failClosed' on enforcement hooks", () => {
    const preToolUse = hooksJson.hooks.preToolUse || [];

    // validate-push is advisory (no failClosed) — downgraded in dev.12
    const validatePush = preToolUse.find(
      h => h.command?.includes("validate-push")
    );
    expect(validatePush).toBeDefined();
    expect(validatePush?.failClosed).toBeUndefined();

    // check-secrets is blocking (failClosed: true)
    const checkSecrets = preToolUse.find(
      h => h.command?.includes("check-secrets")
    );
    expect(checkSecrets).toBeDefined();
    expect(checkSecrets?.failClosed).toBe(true);
  });

  it("preserves 'matcher' on all tool hooks", () => {
    const preToolUse = hooksJson.hooks.preToolUse || [];
    
    // All pre-tool hooks should have a matcher
    for (const hook of preToolUse) {
      expect(hook.matcher).toBeDefined();
    }
    
    const postToolUse = hooksJson.hooks.postToolUse || [];
    
    // All post-tool hooks should have a matcher
    for (const hook of postToolUse) {
      expect(hook.matcher).toBeDefined();
    }
  });

  it("preserves 'timeout' on all hooks", () => {
    const preToolUse = hooksJson.hooks.preToolUse || [];
    
    // All pre-tool hooks should have a timeout
    for (const hook of preToolUse) {
      expect(hook.timeout).toBeDefined();
      expect(typeof hook.timeout).toBe("number");
    }
    
    const postToolUse = hooksJson.hooks.postToolUse || [];
    
    // All post-tool hooks should have a timeout
    for (const hook of postToolUse) {
      expect(hook.timeout).toBeDefined();
      expect(typeof hook.timeout).toBe("number");
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Claude Code Tests
// ─────────────────────────────────────────────────────────────────────────────

describe("claude-code hooks.json", () => {
  let ccHooks: ClaudePluginJson;

  beforeAll(() => {
    const hooksPath = join(PLUGINS_DIR, "loaf", "hooks", "hooks.json");
    if (!existsSync(hooksPath)) {
      throw new Error("hooks/hooks.json not found. Run `npm run build` first.");
    }
    ccHooks = JSON.parse(readFileSync(hooksPath, "utf-8")) as ClaudePluginJson;
  });

  it("has hooks top-level key", () => {
    expect(ccHooks.hooks).toBeDefined();
  });

  it("plugin.json has no hooks key (all hooks in hooks.json)", () => {
    const pluginPath = join(PLUGINS_DIR, "loaf", ".claude-plugin", "plugin.json");
    const pluginJson = JSON.parse(readFileSync(pluginPath, "utf-8"));
    expect(pluginJson.hooks).toBeUndefined();
  });

  it("preserves 'if' on pre-tool instruction hooks", () => {
    const preToolUse = ccHooks.hooks.PreToolUse || [];

    const bashMatcher = preToolUse.find(m => m.matcher === "Bash");
    expect(bashMatcher).toBeDefined();

    const workflowPreMerge = bashMatcher?.hooks.find(
      h => h.type === "command" && h.command?.includes("instructions/pre-merge.md")
    );
    expect(workflowPreMerge).toBeDefined();
    expect(workflowPreMerge?.if).toBe("Bash(gh pr merge:*)");
  });

  it("preserves 'if' on post-tool command hooks (journal auto-entry)", () => {
    const postToolUse = ccHooks.hooks.PostToolUse || [];

    const bashMatcher = postToolUse.find(m => m.matcher === "Bash");
    expect(bashMatcher).toBeDefined();

    // Find journal-gh-events hook (gh pr create + merge)
    const journalGh = bashMatcher?.hooks.find(
      h => h.type === "command" &&
           h.command?.includes("session log --from-hook") &&
           h.if?.includes("gh pr")
    );
    expect(journalGh).toBeDefined();
    expect(journalGh?.if).toBe("Bash(gh pr:*)");
  });

  it("preserves 'failClosed' on enforcement hooks", () => {
    const preToolUse = ccHooks.hooks.PreToolUse || [];

    const bashMatcher = preToolUse.find(m => m.matcher === "Bash");
    expect(bashMatcher).toBeDefined();

    // validate-push is advisory (no failClosed) — downgraded in dev.12
    const validatePush = bashMatcher?.hooks.find(
      h => h.type === "command" && h.command?.includes("validate-push")
    );
    expect(validatePush).toBeDefined();
    expect(validatePush?.failClosed).toBeUndefined();
  });

  it("preserves 'matcher' on PreToolUse hooks", () => {
    const preToolUse = ccHooks.hooks.PreToolUse || [];

    for (const matcherGroup of preToolUse) {
      expect(matcherGroup.matcher).toBeDefined();
    }
  });

  it("preserves 'timeout' on hooks", () => {
    const preToolUse = ccHooks.hooks.PreToolUse || [];

    for (const matcherGroup of preToolUse) {
      for (const hook of matcherGroup.hooks) {
        expect(hook.timeout).toBeDefined();
        expect(typeof hook.timeout).toBe("number");
      }
    }
  });

  it("preserves 'description' on hooks", () => {
    const preToolUse = ccHooks.hooks.PreToolUse || [];

    const bashMatcher = preToolUse.find(m => m.matcher === "Bash");
    expect(bashMatcher).toBeDefined();

    const validatePush = bashMatcher?.hooks.find(
      h => h.type === "command" && h.command?.includes("validate-push")
    );
    expect(validatePush?.description).toBeDefined();
    expect(validatePush?.description).toContain("build");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Runtime Behavioral Tests
// ─────────────────────────────────────────────────────────────────────────────

describe("runtime if matching behavior", () => {
  const hooksPath = join(DIST_DIR, "opencode", "plugins", "hooks.ts");
  let hooksContent: string;

  beforeAll(() => {
    if (!existsSync(hooksPath)) {
      throw new Error("hooks.ts not found. Run `npm run build` first.");
    }
    hooksContent = readFileSync(hooksPath, "utf-8");
  });

  it("correctly matches glob patterns in if conditions", () => {
    // Verify the matchesIfCondition function handles glob wildcards
    expect(hooksContent).toContain("matchesIfCondition");
    
    // Should handle :* suffix patterns (e.g., "git commit:*" matches commands starting with "git commit")
    expect(hooksContent).toContain("endsWith(':*')");
    
    // Should parse if condition format: Tool(command:pattern)
    expect(hooksContent).toMatch(/ifCondition\.match\(\s*\/.*\w.*\(/);
  });

  it("includes all command-scoped hooks in runtime data", () => {
    // Extract the postToolHooks data from the generated code
    const postToolHooksMatch = hooksContent.match(/const postToolHooks: Record<string, HookEntry\[\]> = ({[\s\S]*?});/);
    expect(postToolHooksMatch).toBeTruthy();
    
    const postToolHooksData = postToolHooksMatch?.[1] || "";
    
    // Should include journal hooks with if conditions
    expect(postToolHooksData).toContain('"Bash(git commit:*)"');
    expect(postToolHooksData).toContain('"Bash(gh pr:*)"');
  });

  it("runtime checks if condition before executing hook", () => {
    // The generated code should check if condition before running hook
    expect(hooksContent).toContain("if (!matchesIfCondition");
    expect(hooksContent).toContain("continue");
  });
});
