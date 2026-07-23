import test from "node:test";
import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, realpathSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { delimiter, join } from "node:path";
import { tmpdir } from "node:os";
import { buildEnvironment, collectTextValues, modelVisibleProof, opencodeVersionMatches, parseOpenCodeJSONL, resolveExecutable, sanitizeError, sanitizedStderr } from "./u8-opencode-smoke.mjs";

test("build environment passes XDG dirs through only when set", () => {
  const xdgKeys = ["XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"];
  const saved = Object.fromEntries(xdgKeys.map((key) => [key, process.env[key]]));
  try {
    for (const key of xdgKeys) delete process.env[key];
    let env = buildEnvironment();
    for (const key of xdgKeys) assert.equal(key in env, false);
    process.env.XDG_CACHE_HOME = "/tmp/u8-test-cache";
    env = buildEnvironment();
    assert.equal(env.XDG_CACHE_HOME, "/tmp/u8-test-cache");
    assert.equal("XDG_CONFIG_HOME" in env, false);
  } finally {
    for (const key of xdgKeys) {
      if (saved[key] === undefined) delete process.env[key];
      else process.env[key] = saved[key];
    }
  }
});

test("skips multiplexer shims whose realpath basename differs from the command", () => {
  const tempRoot = mkdtempSync(join(tmpdir(), "u8-resolve-test-"));
  const savedPath = process.env.PATH;
  try {
    const shimDir = join(tempRoot, "shims");
    const realDir = join(tempRoot, "installs");
    mkdirSync(shimDir, { recursive: true });
    mkdirSync(realDir, { recursive: true });
    const mux = join(tempRoot, "mise");
    writeFileSync(mux, "#!/bin/sh\n", { mode: 0o755 });
    symlinkSync(mux, join(shimDir, "opencode"));
    const genuine = join(realDir, "opencode");
    writeFileSync(genuine, "#!/bin/sh\n", { mode: 0o755 });
    process.env.PATH = [shimDir, realDir].join(delimiter);
    assert.equal(resolveExecutable("opencode"), realpathSync(genuine));
    process.env.PATH = shimDir;
    assert.throws(() => resolveExecutable("opencode"), /unavailable/);
  } finally {
    process.env.PATH = savedPath;
    rmSync(tempRoot, { recursive: true, force: true });
  }
});

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
