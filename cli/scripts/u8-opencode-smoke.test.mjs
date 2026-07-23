import test from "node:test";
import assert from "node:assert/strict";
import { collectTextValues, modelVisibleProof, opencodeVersionMatches, parseOpenCodeJSONL, sanitizeError, sanitizedStderr } from "./u8-opencode-smoke.mjs";

test("requires the exact OpenCode version token", () => {
  assert.equal(opencodeVersionMatches("1.18.4\n"), true);
  assert.equal(opencodeVersionMatches("1.18.40"), false);
  assert.equal(opencodeVersionMatches("v1.18.4"), false);
  assert.equal(opencodeVersionMatches("1.18.4-dev"), false);
});

test("recursively extracts only string values at text keys", () => {
  const texts = collectTextValues({
    text: "top",
    nested: [{ text: "nested" }, { value: { text: "deep" } }],
    notText: "ignored",
  });
  assert.deepEqual(texts, ["top", "nested", "deep"]);
});

test("parses JSONL recursively and matches an exact trimmed marker", () => {
  const marker = "LOAF_OPENCODE_U8_ABCDEF123456";
  const raw = [
    JSON.stringify({ type: "message", parts: [{ type: "text", text: `context ${marker}` }] }),
    JSON.stringify({ type: "result", data: { nested: { text: ` ${marker} ` } } }),
  ].join("\n");
  assert.equal(parseOpenCodeJSONL(raw, marker).assistantMarkerMatch, true);
  assert.equal(parseOpenCodeJSONL(JSON.stringify({ type: "message", text: "wrong" }), marker).assistantMarkerMatch, false);
});

test("rejects malformed and non-object JSONL events", () => {
  assert.throws(() => parseOpenCodeJSONL("{not-json}", "marker"), SyntaxError);
  assert.throws(() => parseOpenCodeJSONL("null", "marker"), SyntaxError);
  assert.throws(() => parseOpenCodeJSONL("[]", "marker"), SyntaxError);
});

test("assistant text alone is not model-visible proof without hook observation", () => {
  assert.equal(modelVisibleProof(true, false), false);
  assert.equal(modelVisibleProof(true, true), true);
});

test("sanitizes paths and bounds failure text", () => {
  const failure = sanitizeError(new Error("failed at /Users/test/.cache/loaf/tmp/repo with\nsecret details"), [
    ["/Users/test/.cache/loaf", "<home>"],
    ["/tmp/repo", "<disposable>"],
  ]);
  assert.equal(failure, "failed at <home><disposable> with secret details");
  assert.ok(failure.length <= 400);
  assert.equal(sanitizeError(new Error("\n\t"), []), "smoke failed");
});

test("sanitizes stderr to the receipt contract", () => {
  assert.equal(sanitizedStderr("\n"), "");
  assert.equal(sanitizedStderr("credential=secret\n"), "unexpected stderr");
});
