import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";
import { classifyReleaseTag } from "./classify-release-tag.mjs";

const cases = [
  { tag: "v0.3+gabcdef0", status: "invalid" },
  { tag: "v0.3.1", status: "release" },
  { tag: "v0.3.1+gabcdef0", status: "dev" },
  { tag: "v0.3.1-alpha.1", status: "release" },
  { tag: "v1.0.0", status: "release" },
  { tag: "v0.3.1+gabc", status: "release" },
  { tag: "v0.3.1+foo", status: "release" },
  { tag: "v0.2.1786022455", status: "dev" },
  { tag: "v0.2.20+gabcdef0", status: "dev" },
  { tag: "v0.2.20+build.9.gabc1234", status: "dev" },
  { tag: "v0.2.1000000000", status: "dev" },
  { tag: "v0.2.999999999", status: "release" },
  { tag: "v0.3.1+gABCDEF0", status: "release" },
  { tag: "v0.3.1+", status: "invalid" },
  { tag: "v02.1.0", status: "invalid" },
  { tag: "v0.3", status: "invalid" },
  { tag: "v", status: "invalid" },
  { tag: "", status: "invalid" },
  { tag: "0.3.1", status: "invalid" },
  { tag: "v0.3.1+gabcdef0.extra", status: "dev" },
];

for (const tc of cases) {
  test(`classifies ${JSON.stringify(tc.tag) || "(empty)"} as ${tc.status}`, () => {
    const result = classifyReleaseTag(tc.tag);
    assert.equal(result.status, tc.status);
  });
}

test("CLI fails malformed tags instead of skipping them as dev", () => {
  const script = join(dirname(fileURLToPath(import.meta.url)), "classify-release-tag.mjs");
  const result = spawnSync(process.execPath, [script, "v0.3+gabcdef0"], { encoding: "utf8" });
  assert.notEqual(result.status, 0);
  assert.doesNotMatch(result.stdout, /dev=true/);
  assert.match(result.stderr, /valid SemVer|must start with v/);
});

test("CLI skips only a valid dev identity", () => {
  const script = join(dirname(fileURLToPath(import.meta.url)), "classify-release-tag.mjs");
  const result = spawnSync(process.execPath, [script, "v0.3.1+gabcdef0"], { encoding: "utf8" });
  assert.equal(result.status, 0);
  assert.match(result.stdout, /^tag=v0\.3\.1\+gabcdef0$/m);
  assert.match(result.stdout, /^ref=refs\/tags\/v0\.3\.1\+gabcdef0$/m);
  assert.match(result.stdout, /^dev=true$/m);
  assert.match(result.stderr, /carries a dev build identity/);
});
