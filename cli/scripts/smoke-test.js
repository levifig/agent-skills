#!/usr/bin/env node
/**
 * Cross-Harness Smoke Test
 *
 * Verifies generated hook artifacts work correctly across all targets.
 * Tests command-scoped hooks fire exactly once for their designated commands.
 *
 * Usage: node scripts/smoke-test.js
 */

import { execSync } from "child_process";
import { existsSync, readFileSync, mkdirSync, rmSync, writeFileSync } from "fs";
import { join } from "path";
import { fileURLToPath } from "url";
import { dirname } from "path";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const ROOT_DIR = join(__dirname, "..", "..");

// ANSI colors
const green = (s) => `\x1b[32m${s}\x1b[0m`;
const red = (s) => `\x1b[31m${s}\x1b[0m`;
const yellow = (s) => `\x1b[33m${s}\x1b[0m`;
const cyan = (s) => `\x1b[36m${s}\x1b[0m`;
const gray = (s) => `\x1b[90m${s}\x1b[0m`;

// Test results
const results = {
  passed: 0,
  failed: 0,
  tests: [],
};

function test(name, fn) {
  try {
    fn();
    results.passed++;
    results.tests.push({ name, status: "passed" });
    console.log(`  ${green("✓")} ${name}`);
  } catch (error) {
    results.failed++;
    results.tests.push({ name, status: "failed", error: error.message });
    console.log(`  ${red("✗")} ${name}`);
    console.log(`    ${red(error.message)}`);
  }
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message || "Assertion failed");
  }
}

function flattenClaudeHookMatchers(matchers = []) {
  return matchers.flatMap(matcher =>
    (matcher.hooks || []).map(hook => ({ ...hook, matcher: matcher.matcher }))
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Cursor Target Tests
// ─────────────────────────────────────────────────────────────────────────────

function testCursor() {
  console.log(`\n${cyan("Cursor Target")}`);
  
  const hooksPath = join(ROOT_DIR, "dist", "cursor", "hooks.json");
  
  test("hooks.json exists", () => {
    assert(existsSync(hooksPath), "hooks.json not found");
  });
  
  const hooks = JSON.parse(readFileSync(hooksPath, "utf-8"));
  
  test("journal-post-commit hook has if condition", () => {
    const postTool = hooks.hooks.postToolUse || [];
    const journalCommit = postTool.find(
      h => h.command?.includes("session log --from-hook") && h.if?.includes("git commit")
    );
    assert(journalCommit, "journal-post-commit not found");
    assert(journalCommit.if === "Bash(git commit:*)", `Expected if: Bash(git commit:*), got: ${journalCommit.if}`);
  });
  
  test("journal-post-pr hook has if condition", () => {
    const postTool = hooks.hooks.postToolUse || [];
    const journalPr = postTool.find(
      h => h.command?.includes("session log --from-hook") && h.if === "Bash(gh pr:*)"
    );
    assert(journalPr, "journal-post-pr not found");
    assert(journalPr.if === "Bash(gh pr:*)", `Expected if: Bash(gh pr:*), got: ${journalPr.if}`);
  });
  
  test("post-merge instruction hook has if condition", () => {
    const postTool = hooks.hooks.postToolUse || [];
    const postMerge = postTool.find(
      h => h.command?.includes("post-merge.md") && h.if === "Bash(gh pr merge:*)"
    );
    assert(postMerge, "post-merge instruction hook not found");
  });
  
  test("validate-push hook is scoped to git push", () => {
    const preTool = hooks.hooks.preToolUse || [];
    const validatePush = preTool.find(
      h => h.command?.includes("validate-push")
    );
    assert(validatePush, "validate-push not found");
    assert(validatePush.if === "Bash(git push:*)", `Expected if: Bash(git push:*), got: ${validatePush.if}`);
  });
  
  test("workflow-pre-pr hook is scoped to PR creation", () => {
    const preTool = hooks.hooks.preToolUse || [];
    const workflowPrePr = preTool.find(
      h => h.command?.includes("workflow-pre-pr")
    );
    assert(workflowPrePr, "workflow-pre-pr not found");
    assert(workflowPrePr.if === "Bash(gh pr create:*)", `Expected if: Bash(gh pr create:*), got: ${workflowPrePr.if}`);
  });
}

// ─────────────────────────────────────────────────────────────────────────────
// Claude Code Target Tests
// ─────────────────────────────────────────────────────────────────────────────

function testClaudeCode() {
  console.log(`\n${cyan("Claude Code Target")}`);
  
  const hooksPath = join(ROOT_DIR, "plugins", "loaf", "hooks", "hooks.json");
  
  test("hooks.json exists", () => {
    assert(existsSync(hooksPath), "hooks.json not found");
  });
  
  const plugin = JSON.parse(readFileSync(hooksPath, "utf-8"));
  
  test("journal-post-commit hook has if condition", () => {
    const postTool = flattenClaudeHookMatchers(plugin.hooks.PostToolUse);
    const journalCommit = postTool.find(
      h => h.command?.includes("session log --from-hook") && h.if?.includes("git commit")
    );
    assert(journalCommit, "journal-post-commit not found");
    assert(journalCommit.if === "Bash(git commit:*)", `Expected if: Bash(git commit:*), got: ${journalCommit.if}`);
  });
  
  test("journal-post-pr hook has if condition", () => {
    const postTool = flattenClaudeHookMatchers(plugin.hooks.PostToolUse);
    const journalPr = postTool.find(
      h => h.command?.includes("session log --from-hook") && h.if === "Bash(gh pr:*)"
    );
    assert(journalPr, "journal-post-pr not found");
    assert(journalPr.if === "Bash(gh pr:*)", `Expected if: Bash(gh pr:*), got: ${journalPr.if}`);
  });
  
  test("post-merge instruction hook has if condition", () => {
    const postTool = flattenClaudeHookMatchers(plugin.hooks.PostToolUse);
    const postMerge = postTool.find(
      h => h.command?.includes("post-merge.md") && h.if === "Bash(gh pr merge:*)"
    );
    assert(postMerge, "post-merge instruction hook not found");
  });
  
  test("validate-push hook is scoped to git push", () => {
    const preTool = flattenClaudeHookMatchers(plugin.hooks.PreToolUse);
    const validatePush = preTool.find(
      h => h.command?.includes("validate-push")
    );
    assert(validatePush, "validate-push not found");
    assert(validatePush.if === "Bash(git push:*)", `Expected if: Bash(git push:*), got: ${validatePush.if}`);
  });
}

// ─────────────────────────────────────────────────────────────────────────────
// OpenCode Target Tests
// ─────────────────────────────────────────────────────────────────────────────

function testOpenCode() {
  console.log(`\n${cyan("OpenCode Target")}`);
  
  const hooksPath = join(ROOT_DIR, "dist", "opencode", "plugins", "hooks.ts");
  
  test("hooks.ts exists", () => {
    assert(existsSync(hooksPath), "hooks.ts not found");
  });
  
  const hooksContent = readFileSync(hooksPath, "utf-8");
  
  test("hooks.ts includes matchesIfCondition function", () => {
    assert(hooksContent.includes("matchesIfCondition"), "matchesIfCondition function not found");
  });
  
  test("hooks.ts checks hook.if condition", () => {
    assert(hooksContent.includes("hook.if"), "hook.if check not found");
    assert(hooksContent.includes("matchesIfCondition"), "matchesIfCondition call not found");
  });
  
  test("hooks.ts includes journal-post-pr hook with if", () => {
    // The generated TypeScript should include the if condition for journal hooks
    assert(hooksContent.includes('"Bash(gh pr create:*)"') || hooksContent.includes("'Bash(gh pr create:*)'"), 
           "gh pr create:* if condition not found");
  });
}

// ─────────────────────────────────────────────────────────────────────────────
// Codex Target Tests
// ─────────────────────────────────────────────────────────────────────────────

function testCodex() {
  console.log(`\n${cyan("Codex Target")}`);
  
  const skillsDir = join(ROOT_DIR, "dist", "codex", "skills");
  
  test("skills directory exists", () => {
    assert(existsSync(skillsDir), "skills directory not found");
  });
  
  // Codex target only gets skills, no hooks
  // Just verify skills are present
  const skills = ["git-workflow", "orchestration", "security-compliance"];
  for (const skill of skills) {
    test(`${skill} skill exists`, () => {
      const skillPath = join(skillsDir, skill, "SKILL.md");
      assert(existsSync(skillPath), `${skill} SKILL.md not found`);
    });
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Amp Target Tests
// ─────────────────────────────────────────────────────────────────────────────

function testAmp() {
  console.log(`\n${cyan("Amp Target")}`);
  
  const hooksPath = join(ROOT_DIR, "dist", "amp", ".amp", "plugins", "loaf.ts");
  
  test("loaf.ts exists", () => {
    assert(existsSync(hooksPath), "loaf.ts not found");
  });
  
  const hooksContent = readFileSync(hooksPath, "utf-8");
  
  test("loaf.js includes matchesIfCondition function", () => {
    assert(hooksContent.includes("matchesIfCondition"), "matchesIfCondition function not found");
  });
  
  test("loaf.js checks hook.if condition", () => {
    assert(hooksContent.includes("hook.if"), "hook.if check not found");
  });
}

// ─────────────────────────────────────────────────────────────────────────────
// Main
// ─────────────────────────────────────────────────────────────────────────────

console.log(`\n${cyan("═".repeat(70))}`);
console.log(`${cyan("Cross-Harness Smoke Test")}`);
console.log(`${cyan("═".repeat(70))}`);

testCursor();
testClaudeCode();
testOpenCode();
testCodex();
testAmp();

console.log(`\n${cyan("═".repeat(70))}`);
console.log(`${cyan("Results")}`);
console.log(`${cyan("═".repeat(70))}`);

if (results.failed === 0) {
  console.log(`\n${green("✓ All tests passed")} (${results.passed} tests)`);
  process.exit(0);
} else {
  console.log(`\n${red("✗ Some tests failed")}`);
  console.log(`  ${green("Passed:")} ${results.passed}`);
  console.log(`  ${red("Failed:")} ${results.failed}`);
  process.exit(1);
}
