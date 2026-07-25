import test from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { claudeVersionMatches, parseClaudeStreamOutput } from "./smoke-claude-code-startup.mjs";
import { parseRunnerArgs, publishReceiptIfSuccessful } from "./capability-runner-utils.mjs";

test("parses native SessionStart output and exact assistant marker", () => {
  const marker = "LOAF_CLAUDE_STARTUP_SMOKE_ABCDEF123456";
  const raw = [
    JSON.stringify({ type: "system", subtype: "hook_response", hook_event: "SessionStart", output: JSON.stringify({ hookSpecificOutput: { hookEventName: "SessionStart", additionalContext: `digest ${marker}` } }) }),
    JSON.stringify({ type: "assistant", message: { content: [{ type: "text", text: marker }] } }),
    JSON.stringify({ type: "result", result: marker }),
  ].join("\n");
  const parsed = parseClaudeStreamOutput(raw, marker);
  assert.equal(parsed.hookObservation.native_json, true);
  assert.equal(parsed.hookObservation.additional_context_marker, true);
  assert.equal(parsed.assistantMarkerMatch, true);
});

test("rejects a stream without SessionStart hook response", () => {
  assert.throws(() => parseClaudeStreamOutput(JSON.stringify({ type: "result", result: "no hook" }), "marker"), /SessionStart hook response/);
});

test("requires the exact Claude Code version token", () => {
  assert.equal(claudeVersionMatches("9.8.7 (Claude Code)\n", "9.8.7"), true);
  assert.equal(claudeVersionMatches("9.8.70 (Claude Code)", "9.8.7"), false);
  assert.equal(claudeVersionMatches("Claude Code 9.8.7", "9.8.7"), false);
});

test("requires one safe value for every runner option", () => {
  const parsed = parseRunnerArgs(["--client", "/opt/claude", "--expected-version", "9.8.7", "--receipt", "proof.json"]);
  assert.equal(parsed.client, "/opt/claude");
  assert.equal(parsed.expectedVersion, "9.8.7");
  assert.ok(parsed.receiptPath.endsWith("proof.json"));
  assert.throws(() => parseRunnerArgs(["--client", "claude"]), /missing required option/);
  assert.throws(() => parseRunnerArgs(["--client", "claude", "--client", "other", "--expected-version", "9.8.7", "--receipt", "proof.json"]), /duplicate option/);
  assert.throws(() => parseRunnerArgs(["--unknown", "value", "--client", "claude", "--expected-version", "9.8.7", "--receipt", "proof.json"]), /unknown option/);
  assert.throws(() => parseRunnerArgs(["--client", "claude\nunsafe", "--expected-version", "9.8.7", "--receipt", "proof.json"]), /safe executable/);
  assert.throws(() => parseRunnerArgs(["--client", "claude", "--expected-version", "9.8.7 unsafe", "--receipt", "proof.json"]), /exact safe identity/);
  assert.throws(() => parseRunnerArgs(["--client", "claude", "--expected-version", "9.8.7", "--receipt", "proof.txt"]), /safe JSON path/);
});

test("publishes success atomically and preserves an existing receipt on failure", () => {
  const root = mkdtempSync(join(tmpdir(), "loaf-capability-receipt-test-"));
  const receiptPath = join(root, "receipt.json");
  try {
    writeFileSync(receiptPath, "existing\n");
    assert.equal(publishReceiptIfSuccessful(receiptPath, { status: "failed" }, false), false);
    assert.equal(readFileSync(receiptPath, "utf8"), "existing\n");
    assert.equal(publishReceiptIfSuccessful(receiptPath, { status: "passed" }, true), true);
    assert.deepEqual(JSON.parse(readFileSync(receiptPath, "utf8")), { status: "passed" });
    assert.deepEqual(readdirSync(root), ["receipt.json"]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
