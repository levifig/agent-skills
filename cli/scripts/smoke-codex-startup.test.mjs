import test from "node:test";
import assert from "node:assert/strict";
import { buildCodexArgs, codexVersionMatches, parseCodexHookObservation, parseCodexJSONL, shellQuote } from "./smoke-codex-startup.mjs";
import { parseRunnerArgs } from "./capability-runner-utils.mjs";

test("parses native SessionStart marker and exact assistant marker", () => {
  const marker = "LOAF_CODEX_STARTUP_SMOKE_ABCDEF123456";
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
  const marker = "LOAF_CODEX_STARTUP_SMOKE_ABCDEF123456";
  const raw = JSON.stringify({ type: "item.completed", item: { type: "agent_message", text: marker } });
  const parsed = parseCodexJSONL(raw, marker);
  assert.equal(parsed.hookObservation.native_json, false);
  assert.equal(parsed.assistantMarkerMatch, true);
});

test("validates the exact native hook stdout observation", () => {
  const marker = "LOAF_CODEX_STARTUP_SMOKE_ABCDEF123456";
  const observed = JSON.stringify({ hookSpecificOutput: { hookEventName: "SessionStart", additionalContext: `digest ${marker}` } });
  const parsed = parseCodexHookObservation(observed, marker);
  assert.deepEqual(parsed, { eventName: "SessionStart:startup", nativeJSON: true, hookEventName: "SessionStart", additionalContextMarker: true });
});

test("rejects extra fields in the native hook stdout observation", () => {
  const marker = "LOAF_CODEX_STARTUP_SMOKE_ABCDEF123456";
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

test("omits model selection unless a model is requested", () => {
  const args = buildCodexArgs(undefined);
  assert.equal(args.includes("-m"), false);
  assert.deepEqual(args.slice(0, 7), ["exec", "--ephemeral", "--ignore-rules", "--dangerously-bypass-hook-trust", "--sandbox", "read-only", "--json"]);
});

test("selects the requested model on the command line", () => {
  const args = buildCodexArgs("gpt-5.3-codex-spark");
  assert.deepEqual(args.slice(6, 8), ["-m", "gpt-5.3-codex-spark"]);
  assert.equal(args.at(-2), "<disposable-repo>");
});

test("accepts an optional model identity and rejects an unsafe one", () => {
  const base = ["--client", "codex", "--expected-version", "9.8.7", "--receipt", "proof.json"];
  assert.equal(parseRunnerArgs(base, ["codex-model"]).optional["codex-model"], undefined);
  assert.equal(parseRunnerArgs([...base, "--codex-model", "gpt-5.3-codex-spark"], ["codex-model"]).optional["codex-model"], "gpt-5.3-codex-spark");
  assert.throws(() => parseRunnerArgs([...base, "--codex-model", "-evil"], ["codex-model"]), /exact safe identity/);
  assert.throws(() => parseRunnerArgs([...base, "--codex-model", "model"]), /unknown option/);
});

test("requires the exact Codex CLI version token", () => {
  assert.equal(codexVersionMatches("codex-cli 9.8.7\n", "9.8.7"), true);
  assert.equal(codexVersionMatches("codex-cli 9.8.70\n", "9.8.7"), false);
  assert.equal(codexVersionMatches("other-cli 9.8.7\n", "9.8.7"), false);
});
