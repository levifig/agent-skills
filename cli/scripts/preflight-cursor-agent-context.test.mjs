import test from "node:test";
import assert from "node:assert/strict";
import { classifyCursorPreflight } from "./preflight-cursor-agent-context.mjs";

test("classifies an unsafe Cursor preflight without claiming execution", () => {
  const preflight = classifyCursorPreflight("candidate-42\n", "Usage: agent\n", "candidate-42");
  assert.equal(preflight.exactVersion, true);
  assert.equal(preflight.noSessionPersistence, false);
  assert.equal(preflight.smokeExecuted, false);
  assert.match(preflight.blocker, /no-session-persistence/);
});

test("rejects an unexpected installed version before any smoke", () => {
  const preflight = classifyCursorPreflight("unexpected\n", "--no-session-persistence\n", "candidate-42");
  assert.equal(preflight.exactVersion, false);
  assert.equal(preflight.noSessionPersistence, true);
  assert.equal(preflight.smokeExecuted, false);
  assert.match(preflight.blocker, /does not match/);
});
