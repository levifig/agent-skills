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

  it("preserves 'if' on pre-tool prompt hooks", () => {
    const preToolUse = hooksJson.hooks.preToolUse || [];
    
    // Find workflow-pre-merge hook (should have if: Bash(gh pr merge:*))
    const workflowPreMerge = preToolUse.find(
      h => h.prompt && h.prompt.includes("squash merge")
    );
    expect(workflowPreMerge).toBeDefined();
    expect(workflowPreMerge?.if).toBe("Bash(gh pr merge:*)");
    
    // Find workflow-pre-push hook (should have if: Bash(git push:*))
    const workflowPrePush = preToolUse.find(
      h => h.prompt && h.prompt.includes("git push")
    );
    expect(workflowPrePush).toBeDefined();
    expect(workflowPrePush?.if).toBe("Bash(git push:*)");
  });

  it("preserves 'if' on post-tool prompt hooks", () => {
    const postToolUse = hooksJson.hooks.postToolUse || [];
    
    // Find workflow-post-merge hook (should have if: Bash(gh pr merge:*))
    const workflowPostMerge = postToolUse.find(
      h => h.prompt && h.prompt.includes("POST-MERGE HOUSEKEEPING")
    );
    expect(workflowPostMerge).toBeDefined();
    expect(workflowPostMerge?.if).toBe("Bash(gh pr merge:*)");
  });

  it("preserves 'if' on post-tool command hooks (journal auto-entry)", () => {
    const postToolUse = hooksJson.hooks.postToolUse || [];
    
    // Find journal-post-commit hook
    const journalCommit = postToolUse.find(
      h => h.command === "loaf session log --from-hook" && h.if?.includes("git commit")
    );
    expect(journalCommit).toBeDefined();
    expect(journalCommit?.if).toBe("Bash(git commit:*)");
    
    // Find journal-post-pr hook
    const journalPr = postToolUse.find(
      h => h.command === "loaf session log --from-hook" && h.if?.includes("gh pr create")
    );
    expect(journalPr).toBeDefined();
    expect(journalPr?.if).toBe("Bash(gh pr create:*)");
    
    // Find journal-post-merge hook
    const journalMerge = postToolUse.find(
      h => h.command === "loaf session log --from-hook" && h.if?.includes("gh pr merge")
    );
    expect(journalMerge).toBeDefined();
    expect(journalMerge?.if).toBe("Bash(gh pr merge:*)");
  });

  it("preserves 'failClosed' on enforcement hooks", () => {
    const preToolUse = hooksJson.hooks.preToolUse || [];
    
    // Find validate-push hook
    const validatePush = preToolUse.find(
      h => h.command?.includes("validate-push")
    );
    expect(validatePush).toBeDefined();
    expect(validatePush?.failClosed).toBe(true);
    
    // Find check-secrets hook
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

describe("claude-code plugin.json", () => {
  let pluginJson: ClaudePluginJson;

  beforeAll(() => {
    const pluginPath = join(PLUGINS_DIR, "loaf", ".claude-plugin", "plugin.json");
    if (!existsSync(pluginPath)) {
      throw new Error("plugin.json not found. Run `npm run build` first.");
    }
    pluginJson = JSON.parse(readFileSync(pluginPath, "utf-8")) as ClaudePluginJson;
  });

  it("has required top-level fields", () => {
    expect(pluginJson.name).toBe("loaf");
    expect(pluginJson.version).toBeDefined();
    expect(pluginJson.hooks).toBeDefined();
  });

  it("preserves 'if' on pre-tool prompt hooks", () => {
    const preToolUse = pluginJson.hooks.PreToolUse || [];
    
    // Find workflow-pre-merge hook
    const bashMatcher = preToolUse.find(m => m.matcher === "Bash");
    expect(bashMatcher).toBeDefined();
    
    const workflowPreMerge = bashMatcher?.hooks.find(
      h => h.type === "prompt" && h.prompt?.includes("squash merge")
    );
    expect(workflowPreMerge).toBeDefined();
    expect(workflowPreMerge?.if).toBe("Bash(gh pr merge:*)");
  });

  it("preserves 'if' on post-tool command hooks (journal auto-entry)", () => {
    const postToolUse = pluginJson.hooks.PostToolUse || [];
    
    const bashMatcher = postToolUse.find(m => m.matcher === "Bash");
    expect(bashMatcher).toBeDefined();
    
    // Find journal-post-pr hook
    const journalPr = bashMatcher?.hooks.find(
      h => h.type === "command" && 
           h.command?.includes("session log --from-hook") &&
           h.if?.includes("gh pr create")
    );
    expect(journalPr).toBeDefined();
    expect(journalPr?.if).toBe("Bash(gh pr create:*)");
    
    // Find journal-post-merge hook
    const journalMerge = bashMatcher?.hooks.find(
      h => h.type === "command" && 
           h.command?.includes("session log --from-hook") &&
           h.if?.includes("gh pr merge")
    );
    expect(journalMerge).toBeDefined();
    expect(journalMerge?.if).toBe("Bash(gh pr merge:*)");
  });

  it("preserves 'failClosed' on enforcement hooks", () => {
    const preToolUse = pluginJson.hooks.PreToolUse || [];
    
    const bashMatcher = preToolUse.find(m => m.matcher === "Bash");
    expect(bashMatcher).toBeDefined();
    
    // Find validate-push hook
    const validatePush = bashMatcher?.hooks.find(
      h => h.type === "command" && h.command?.includes("validate-push")
    );
    expect(validatePush).toBeDefined();
    expect(validatePush?.failClosed).toBe(true);
  });

  it("preserves 'matcher' on PreToolUse hooks", () => {
    const preToolUse = pluginJson.hooks.PreToolUse || [];
    
    for (const matcherGroup of preToolUse) {
      expect(matcherGroup.matcher).toBeDefined();
    }
  });

  it("preserves 'timeout' on hooks", () => {
    const preToolUse = pluginJson.hooks.PreToolUse || [];
    
    for (const matcherGroup of preToolUse) {
      for (const hook of matcherGroup.hooks) {
        expect(hook.timeout).toBeDefined();
        expect(typeof hook.timeout).toBe("number");
      }
    }
  });

  it("preserves 'description' on hooks", () => {
    const preToolUse = pluginJson.hooks.PreToolUse || [];
    
    const bashMatcher = preToolUse.find(m => m.matcher === "Bash");
    expect(bashMatcher).toBeDefined();
    
    // validate-push should have a description
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
    expect(postToolHooksData).toContain('"Bash(gh pr create:*)"');
    expect(postToolHooksData).toContain('"Bash(gh pr merge:*)"');
  });

  it("runtime checks if condition before executing hook", () => {
    // The generated code should check if condition before running hook
    expect(hooksContent).toContain("if (!matchesIfCondition");
    expect(hooksContent).toContain("continue");
  });
});
