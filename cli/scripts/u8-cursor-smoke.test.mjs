import test from "node:test";
import assert from "node:assert/strict";
import { classifyCursorPreflight } from "./u8-cursor-smoke.mjs";

test("classifies an unsafe Cursor preflight without claiming execution", () => {
  const preflight = classifyCursorPreflight("2026.05.09-0afadcc\n", "Usage: agent\n");
  assert.equal(preflight.exactVersion, true);
  assert.equal(preflight.noSessionPersistence, false);
  assert.equal(preflight.smokeExecuted, false);
  assert.match(preflight.blocker, /no-session-persistence/);
});

test("rejects an unexpected installed version before any smoke", () => {
  const preflight = classifyCursorPreflight("3.11.19\n", "--no-session-persistence\n");
  assert.equal(preflight.exactVersion, false);
  assert.equal(preflight.noSessionPersistence, true);
  assert.equal(preflight.smokeExecuted, false);
  assert.match(preflight.blocker, /does not match/);
});
