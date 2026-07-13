import test from "node:test";
import assert from "node:assert/strict";
import { claudeVersionMatches, parseClaudeStreamOutput } from "./u8-claude-smoke.mjs";

test("parses native SessionStart output and exact assistant marker", () => {
  const marker = "LOAF_U8_SMOKE_TEST";
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
  assert.equal(claudeVersionMatches("2.1.207 (Claude Code)\n", "2.1.207"), true);
  assert.equal(claudeVersionMatches("2.1.2070 (Claude Code)", "2.1.207"), false);
  assert.equal(claudeVersionMatches("Claude Code 2.1.207", "2.1.207"), false);
});
