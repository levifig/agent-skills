import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, readlinkSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { refreshDevBuildLink } from "./dev-build-link.mjs";

test("last successful dev build owns the local Loaf link", (t) => {
  const fixture = devLinkFixture(t);
  const first = fixture.loafLauncher("first");
  const second = fixture.loafLauncher("second");

  assert.equal(refreshDevBuildLink(first, { home: fixture.home }).status, "linked");
  assert.equal(readlinkSync(fixture.link), first);
  assert.equal(refreshDevBuildLink(second, { home: fixture.home }).status, "linked");
  assert.equal(readlinkSync(fixture.link), second);
});

test("a real file is never overwritten", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");
  mkdirSync(join(fixture.home, ".local", "bin"), { recursive: true });
  writeFileSync(fixture.link, "operator-owned\n");

  const warnings = [];
  assert.equal(refreshDevBuildLink(launcher, { home: fixture.home, warn: (message) => warnings.push(message) }).status, "conflict");
  assert.equal(warnings.length, 1);
  assert.equal(readFileSync(fixture.link, "utf8"), "operator-owned\n");
});

test("an unrelated symlink is never overwritten", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");
  const unrelated = join(fixture.root, "other", "bin", "loaf");
  mkdirSync(join(fixture.home, ".local", "bin"), { recursive: true });
  mkdirSync(join(fixture.root, "other", "bin"), { recursive: true });
  symlinkSync(unrelated, fixture.link);

  const warnings = [];
  assert.equal(refreshDevBuildLink(launcher, { home: fixture.home, warn: (message) => warnings.push(message) }).status, "conflict");
  assert.equal(warnings.length, 1);
  assert.equal(readlinkSync(fixture.link), unrelated);
});

function devLinkFixture(t) {
  const root = mkdtempSync(join(tmpdir(), "loaf-dev-link-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const home = join(root, "home");
  return {
    root,
    home,
    link: join(home, ".local", "bin", "loaf"),
    loafLauncher(name) {
      const packageRoot = join(root, name);
      const launcher = join(packageRoot, "bin", "loaf");
      mkdirSync(join(packageRoot, "bin"), { recursive: true });
      writeFileSync(join(packageRoot, "package.json"), '{"name":"loaf"}\n');
      writeFileSync(launcher, "#!/usr/bin/env node\n");
      return launcher;
    },
  };
}
