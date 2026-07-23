import test from "node:test";
import assert from "node:assert/strict";
import { codexVersionMatches, parseCodexHookObservation, parseCodexJSONL, shellQuote } from "./u8-codex-smoke.mjs";

test("parses native SessionStart marker and exact assistant marker", () => {
  const marker = "LOAF_CODEX_U8_ABCDEF123456";
  const raw = [
    JSON.stringify({ type: "item.completed", item: { type: "command_execution", output: JSON.stringify({ hookSpecificOutput: { hookEventName: "SessionStart", additionalContext: `digest ${marker}` } }) } }),
    JSON.stringify({ type: "item.completed", item: { type: "agent_message", text: marker } }),
  ].join("\n");
  const parsed = parseCodexJSONL(raw, marker);
  assert.equal(parsed.hookObservation.native_json, true);
  assert.equal(parsed.hookObservation.additional_context_marker, true);
  assert.equal(parsed.assistantMarkerMatch, true);
});

test("does not treat a guessed assistant marker as native hook evidence", () => {
  const marker = "LOAF_CODEX_U8_ABCDEF123456";
  const raw = JSON.stringify({ type: "item.completed", item: { type: "agent_message", text: marker } });
  const parsed = parseCodexJSONL(raw, marker);
  assert.equal(parsed.hookObservation.native_json, false);
  assert.equal(parsed.assistantMarkerMatch, true);
});

test("validates the exact native hook stdout observation", () => {
  const marker = "LOAF_CODEX_U8_ABCDEF123456";
  const observed = JSON.stringify({ hookSpecificOutput: { hookEventName: "SessionStart", additionalContext: `digest ${marker}` } });
  const parsed = parseCodexHookObservation(observed, marker);
  assert.deepEqual(parsed, { eventName: "SessionStart:startup", nativeJSON: true, hookEventName: "SessionStart", additionalContextMarker: true });
});

test("rejects extra fields in the native hook stdout observation", () => {
  const marker = "LOAF_CODEX_U8_ABCDEF123456";
  const observed = JSON.stringify({ hookSpecificOutput: { hookEventName: "SessionStart", additionalContext: marker, unexpected: true } });
  const parsed = parseCodexHookObservation(observed, marker);
  assert.equal(parsed.nativeJSON, false);
  assert.equal(parsed.additionalContextMarker, false);
});

test("shell quotes executable paths literally", () => {
  assert.equal(shellQuote("/trusted/Loaf $release/o'brien/loaf"), "'/trusted/Loaf $release/o'\\''brien/loaf'");
});

test("rejects malformed JSONL", () => {
  assert.throws(() => parseCodexJSONL("not-json", "marker"), SyntaxError);
});

test("requires the exact Codex CLI version token", () => {
  assert.equal(codexVersionMatches("codex-cli 0.145.0\n", "0.145.0"), true);
  assert.equal(codexVersionMatches("codex-cli 0.145.00\n", "0.145.0"), false);
  assert.equal(codexVersionMatches("other-cli 0.145.0\n", "0.145.0"), false);
});
