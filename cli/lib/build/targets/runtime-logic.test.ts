/**
 * Runtime Logic Unit Tests
 *
 * Tests for the runtime hook matching and filtering logic.
 * These test the actual functions that would be generated in the runtime plugins.
 * 
 * @vitest-environment node
 */

import { describe, it, expect, beforeAll } from "vitest";
import { existsSync, readFileSync } from "fs";
import { join } from "path";
import ts from "typescript";

// ─────────────────────────────────────────────────────────────────────────────
// Runtime Functions (copied from generated plugin for testing)
// ─────────────────────────────────────────────────────────────────────────────

function matchesTool(toolName: string, pattern: string): boolean {
  const patterns = pattern.split("|");
  return patterns.some((p) => {
    const trimmed = p.trim();
    if (trimmed.endsWith("*")) {
      return toolName.startsWith(trimmed.slice(0, -1));
    }
    return toolName === trimmed;
  });
}

function matchesIfCondition(toolName: string, toolInput: unknown, ifCondition: string | undefined): boolean {
  if (!ifCondition) return true;
  
  // Parse pattern like "Bash(gh pr merge:*)" or "Bash(git push:*)"
  const match = ifCondition.match(/^(\w+)\(([^)]+)\)$/);
  if (!match) return true;
  
  const [, expectedTool, commandPattern] = match;
  if (toolName !== expectedTool) return false;
  
  const input = toolInput as Record<string, unknown> | undefined;
  const command = input?.command as string | undefined;
  if (!command) return false;
  
  // Handle glob patterns with :* suffix (e.g., "git commit:*" means "starts with git commit")
  if (commandPattern.endsWith(':*')) {
    const prefix = commandPattern.slice(0, -2);
    return command.startsWith(prefix);
  }
  
  if (commandPattern.endsWith(':')) {
    const prefix = commandPattern.slice(0, -1);
    return command.startsWith(prefix);
  }
  
  // For other patterns, convert glob to regex
  let regexPattern = commandPattern
    .replace(/[.+^\$"{}()|[\]\\]/g, '\\$&')
    .replace(/\*/g, '.*')
    .replace(/\?/g, '.');
  
  const regex = new RegExp('^' + regexPattern + '$');
  return regex.test(command);
}

// ─────────────────────────────────────────────────────────────────────────────
// matchesTool Tests
// ─────────────────────────────────────────────────────────────────────────────

describe("matchesTool", () => {
  it("matches exact tool names", () => {
    expect(matchesTool("Bash", "Bash")).toBe(true);
    expect(matchesTool("Edit", "Edit")).toBe(true);
    expect(matchesTool("Write", "Write")).toBe(true);
  });

  it("does not match different tool names", () => {
    expect(matchesTool("Bash", "Edit")).toBe(false);
    expect(matchesTool("Edit", "Bash")).toBe(false);
  });

  it("matches union patterns (Edit|Write)", () => {
    expect(matchesTool("Edit", "Edit|Write")).toBe(true);
    expect(matchesTool("Write", "Edit|Write")).toBe(true);
    expect(matchesTool("Bash", "Edit|Write")).toBe(false);
  });

  it("matches wildcard patterns", () => {
    expect(matchesTool("EditFile", "Edit*")).toBe(true);
    expect(matchesTool("EditSomething", "Edit*")).toBe(true);
    expect(matchesTool("WriteFile", "Edit*")).toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// matchesIfCondition Tests
// ─────────────────────────────────────────────────────────────────────────────

describe("matchesIfCondition", () => {
  it("returns true when no if condition", () => {
    expect(matchesIfCondition("Bash", { command: "git commit" }, undefined)).toBe(true);
  });

  it("returns true for malformed if condition", () => {
    expect(matchesIfCondition("Bash", { command: "git commit" }, "invalid")).toBe(true);
  });

  it("matches git commit commands", () => {
    const toolInput = { command: 'git commit -m "feat: add feature"' };
    expect(matchesIfCondition("Bash", toolInput, "Bash(git commit:*)")).toBe(true);
  });

  it("matches gh pr create commands", () => {
    const toolInput = { command: 'gh pr create --title "Test" --body "Desc"' };
    expect(matchesIfCondition("Bash", toolInput, "Bash(gh pr create:*)")).toBe(true);
  });

  it("matches gh pr merge commands", () => {
    const toolInput = { command: "gh pr merge 123" };
    expect(matchesIfCondition("Bash", toolInput, "Bash(gh pr merge:*)")).toBe(true);
  });

  it("does not match wrong tool name", () => {
    const toolInput = { command: "git commit -m test" };
    expect(matchesIfCondition("Edit", toolInput, "Bash(git commit:*)")).toBe(false);
  });

  it("does not match wrong command pattern", () => {
    const toolInput = { command: "ls -la" };
    expect(matchesIfCondition("Bash", toolInput, "Bash(git commit:*)")).toBe(false);
    expect(matchesIfCondition("Bash", toolInput, "Bash(gh pr create:*)")).toBe(false);
    expect(matchesIfCondition("Bash", toolInput, "Bash(gh pr merge:*)")).toBe(false);
  });

  it("requires command in toolInput", () => {
    expect(matchesIfCondition("Bash", {}, "Bash(git commit:*)")).toBe(false);
    expect(matchesIfCondition("Bash", { command: undefined }, "Bash(git commit:*)")).toBe(false);
    expect(matchesIfCondition("Bash", null, "Bash(git commit:*)")).toBe(false);
  });

  it("handles wildcard at end of pattern", () => {
    const toolInput = { command: "git push origin main" };
    expect(matchesIfCondition("Bash", toolInput, "Bash(git push:*)")).toBe(true);
  });

  it("handles complex command patterns", () => {
    const toolInput = { command: 'gh pr create --title "feat: add feature" --body "Description" --draft' };
    expect(matchesIfCondition("Bash", toolInput, "Bash(gh pr create:*)")).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Command-Scoped Hook Routing Matrix Tests
// ─────────────────────────────────────────────────────────────────────────────

describe("command-scoped hook routing matrix", () => {
  const postToolHooks: Array<{ id: string; matcher: string; if?: string }> = [
    { id: "journal-post-commit", matcher: "Bash", if: "Bash(git commit:*)" },
    { id: "journal-post-pr", matcher: "Bash", if: "Bash(gh pr create:*)" },
    { id: "journal-post-merge", matcher: "Bash", if: "Bash(gh pr merge:*)" },
    { id: "generate-task-board", matcher: "Edit|Write" },
  ];

  function getMatchingHooks(toolName: string, toolInput: { command?: string }): string[] {
    const matched: string[] = [];
    for (const hook of postToolHooks) {
      if (matchesTool(toolName, hook.matcher)) {
        if (matchesIfCondition(toolName, toolInput, hook.if)) {
          matched.push(hook.id);
        }
      }
    }
    return matched;
  }

  it("git commit runs only journal-post-commit", () => {
    const result = getMatchingHooks("Bash", { command: 'git commit -m "feat: x"' });
    expect(result).toEqual(["journal-post-commit"]);
  });

  it("gh pr create runs only journal-post-pr", () => {
    const result = getMatchingHooks("Bash", { command: 'gh pr create --title "Test" --body "Desc"' });
    expect(result).toEqual(["journal-post-pr"]);
  });

  it("gh pr merge runs only journal-post-merge", () => {
    const result = getMatchingHooks("Bash", { command: "gh pr merge 123" });
    expect(result).toEqual(["journal-post-merge"]);
  });

  it("unrelated Bash command runs none of the journal hooks", () => {
    const result = getMatchingHooks("Bash", { command: "ls -la" });
    expect(result).toEqual([]);
  });

  it("Edit tool runs generate-task-board", () => {
    const result = getMatchingHooks("Edit", {});
    expect(result).toContain("generate-task-board");
    expect(result).not.toContain("journal-post-commit");
  });

  it("Write tool runs generate-task-board", () => {
    const result = getMatchingHooks("Write", {});
    expect(result).toContain("generate-task-board");
  });

  it("non-Bash tool runs no command-scoped hooks", () => {
    const result = getMatchingHooks("Read", {});
    expect(result).not.toContain("journal-post-commit");
    expect(result).not.toContain("journal-post-pr");
    expect(result).not.toContain("journal-post-merge");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Generated Plugin Syntax Validity Tests
//
// Catch template-literal escape regressions in runtime-plugin.ts.
// The generator embeds regex literals inside a template string — any
// unrecognised escape sequence (e.g. \* → *, \? → ?) silently drops
// the backslash, producing invalid regex like /*/g or /?/g that break
// the entire generated file at parse time.
// ─────────────────────────────────────────────────────────────────────────────

describe("generated plugin syntax validity", () => {
  const TARGETS = [
    { name: "opencode hooks.ts", path: join(process.cwd(), "dist", "opencode", "plugins", "hooks.ts") },
    { name: "amp loaf.js", path: join(process.cwd(), "dist", "amp", "plugins", "loaf.js") },
  ];

  for (const target of TARGETS) {
    describe(target.name, () => {
      let content: string;

      beforeAll(() => {
        if (!existsSync(target.path)) {
          throw new Error(`${target.path} not found — run \`npm run build\` first.`);
        }
        content = readFileSync(target.path, "utf-8");
      });

      it("parses without TypeScript syntax errors", () => {
        const sourceFile = ts.createSourceFile(
          target.path,
          content,
          ts.ScriptTarget.ES2022,
          /* setParentNodes */ false,
        );

        // TypeScript parser reports diagnostics on the source file itself
        const diagnostics = (sourceFile as ts.SourceFile & { parseDiagnostics?: ts.Diagnostic[] })
          .parseDiagnostics ?? [];

        const errors = diagnostics.filter(d => d.category === ts.DiagnosticCategory.Error);
        const messages = errors.map(d =>
          `Line ${sourceFile.getLineAndCharacterOfPosition(d.start ?? 0).line + 1}: ${
            typeof d.messageText === "string" ? d.messageText : d.messageText.messageText
          }`
        );

        expect(errors.length, `Syntax errors in ${target.name}:\n${messages.join("\n")}`).toBe(0);
      });

      it("contains correctly-escaped glob-to-regex pattern (no bare /*/g or /?/g)", () => {
        // These patterns are the broken forms produced by the template-literal escape bug.
        // A bare /*/g is parsed as a block comment start; /?/g is an invalid quantifier.
        expect(content).not.toMatch(/\.replace\(\/\*\/g/);
        expect(content).not.toMatch(/\.replace\(\/\?\/g/);
      });

      it("contains properly-escaped \\* and \\? in replace calls", () => {
        // The correct generated output uses /\*/g and /\?/g (backslash preserved).
        expect(content).toContain(".replace(/\\*/g");
        expect(content).toContain(".replace(/\\?/g");
      });

      it("contains properly-escaped character class in first replace", () => {
        // Generated replace should contain the correctly escaped character class.
        // The class must include [\]\\] (escaped ] and \) not the malformed []\].
        expect(content).toContain(".replace(/[.+^$\"{}()|[\\]\\\\]/g");
      });
    });
  }
});
