import assert from "node:assert/strict";
import {
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readlinkSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { refreshDevBuildLink } from "./dev-build-link.mjs";

test("last successful dev build owns the local Loaf link", (t) => {
  const fixture = devLinkFixture(t);
  const first = fixture.loafLauncher("first");
  const second = fixture.loafLauncher("second");

  assert.equal(refreshDevBuildLink(first, { home: fixture.home }).status, "linked");
  assert.equal(readlinkSync(fixture.link), fixture.pointer);
  assert.equal(readlinkSync(fixture.pointer), first);
  assert.equal(refreshDevBuildLink(second, { home: fixture.home }).status, "linked");
  assert.equal(readlinkSync(fixture.link), fixture.pointer);
  assert.equal(readlinkSync(fixture.pointer), second);
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

test("injected race after observe: operator real file is not displaced", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");
  mkdirSync(dirname(fixture.link), { recursive: true });

  const warnings = [];
  const result = refreshDevBuildLink(launcher, {
    home: fixture.home,
    warn: (message) => warnings.push(message),
    beforeClaimPublic() {
      writeFileSync(fixture.link, "operator-owned\n");
    },
  });

  assert.equal(result.status, "conflict");
  assert.equal(readFileSync(fixture.link, "utf8"), "operator-owned\n");
  assert.equal(lstatSync(fixture.link).isSymbolicLink(), false);
  assert.ok(warnings.some((message) => message.startsWith("WARN:")));
});

test("injected race after observe: unrelated symlink is not displaced", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");
  const unrelated = join(fixture.root, "other", "bin", "loaf");
  mkdirSync(dirname(fixture.link), { recursive: true });
  mkdirSync(dirname(unrelated), { recursive: true });
  writeFileSync(unrelated, "foreign\n");

  const result = refreshDevBuildLink(launcher, {
    home: fixture.home,
    warn: () => {},
    beforeClaimPublic() {
      symlinkSync(unrelated, fixture.link);
    },
  });

  assert.equal(result.status, "conflict");
  assert.equal(readlinkSync(fixture.link), unrelated);
});

test("injected race after observe: directory is not displaced", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");
  mkdirSync(dirname(fixture.link), { recursive: true });

  const result = refreshDevBuildLink(launcher, {
    home: fixture.home,
    warn: () => {},
    beforeClaimPublic() {
      mkdirSync(fixture.link);
    },
  });

  assert.equal(result.status, "conflict");
  assert.equal(lstatSync(fixture.link).isDirectory(), true);
});

test("permission failure does not throw", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");
  const warnings = [];
  const result = refreshDevBuildLink(launcher, {
    home: fixture.home,
    warn: (message) => warnings.push(message),
    fs: {
      symlinkSync(target, dest) {
        if (dest === fixture.link) {
          const error = new Error("permission denied");
          error.code = "EACCES";
          throw error;
        }
        return symlinkSync(target, dest);
      },
    },
  });

  assert.equal(result.status, "failed");
  assert.ok(warnings.some((message) => /failed to link latest dev build/.test(message)));
});

test("read-only home / EROFS does not throw", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");
  const result = refreshDevBuildLink(launcher, {
    home: fixture.home,
    warn: () => {},
    fs: {
      mkdirSync(path, options) {
        if (String(path).includes(`${join(".local", "bin")}`)) {
          const error = new Error("read-only file system");
          error.code = "EROFS";
          throw error;
        }
        return mkdirSync(path, options);
      },
    },
  });

  assert.equal(result.status, "failed");
});

test("symlink failure does not throw", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");
  const result = refreshDevBuildLink(launcher, {
    home: fixture.home,
    warn: () => {},
    fs: {
      symlinkSync() {
        const error = new Error("operation not permitted");
        error.code = "EPERM";
        throw error;
      },
    },
  });

  assert.equal(result.status, "failed");
});

test("stale temp state is ignored", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");
  mkdirSync(dirname(fixture.pointer), { recursive: true });
  const staleOther = `${fixture.pointer}.tmp-99999`;
  const stalePid = `${fixture.pointer}.tmp-${process.pid}`;
  writeFileSync(staleOther, "other-pid\n");
  writeFileSync(stalePid, "same-pid\n");

  const result = refreshDevBuildLink(launcher, { home: fixture.home });
  assert.equal(result.status, "linked");
  assert.equal(readlinkSync(fixture.link), fixture.pointer);
  assert.equal(readlinkSync(fixture.pointer), launcher);
  assert.equal(readFileSync(staleOther, "utf8"), "other-pid\n");
  assert.throws(() => lstatSync(stalePid), { code: "ENOENT" });
});

test("loaf-owned pointer still updates", (t) => {
  const fixture = devLinkFixture(t);
  const first = fixture.loafLauncher("first");
  const second = fixture.loafLauncher("second");
  mkdirSync(dirname(fixture.pointer), { recursive: true });
  mkdirSync(dirname(fixture.link), { recursive: true });
  symlinkSync(first, fixture.pointer);
  symlinkSync(fixture.pointer, fixture.link);

  assert.equal(refreshDevBuildLink(second, { home: fixture.home }).status, "linked");
  assert.equal(readlinkSync(fixture.link), fixture.pointer);
  assert.equal(readlinkSync(fixture.pointer), second);
});

test("missing public creates loaf-owned activation", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");

  assert.equal(refreshDevBuildLink(launcher, { home: fixture.home }).status, "linked");
  assert.equal(lstatSync(fixture.link).isSymbolicLink(), true);
  assert.equal(readlinkSync(fixture.link), fixture.pointer);
  assert.equal(readlinkSync(fixture.pointer), launcher);
});

test("missing public does not steal a later operator file", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");
  mkdirSync(dirname(fixture.link), { recursive: true });

  assert.equal(
    refreshDevBuildLink(launcher, {
      home: fixture.home,
      warn: () => {},
      beforeClaimPublic() {
        writeFileSync(fixture.link, "operator-owned\n");
      },
    }).status,
    "conflict",
  );
  assert.equal(
    refreshDevBuildLink(launcher, { home: fixture.home, warn: () => {} }).status,
    "conflict",
  );
  assert.equal(readFileSync(fixture.link, "utf8"), "operator-owned\n");
});

test("legacy loaf checkout symlink is not replaced", (t) => {
  const fixture = devLinkFixture(t);
  const first = fixture.loafLauncher("first");
  const second = fixture.loafLauncher("second");
  mkdirSync(dirname(fixture.link), { recursive: true });
  symlinkSync(first, fixture.link);

  const warnings = [];
  const result = refreshDevBuildLink(second, {
    home: fixture.home,
    warn: (message) => warnings.push(message),
  });

  assert.equal(result.status, "conflict");
  assert.equal(readlinkSync(fixture.link), first);
  assert.ok(warnings.some((message) => /already points at a Loaf checkout/.test(message)));
});

test("broken symlink is not replaced", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");
  const missing = join(fixture.root, "missing", "loaf");
  mkdirSync(dirname(fixture.link), { recursive: true });
  symlinkSync(missing, fixture.link);

  const result = refreshDevBuildLink(launcher, { home: fixture.home, warn: () => {} });
  assert.equal(result.status, "conflict");
  assert.equal(readlinkSync(fixture.link), missing);
});

test("pointer regular file is not renamed onto", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");
  mkdirSync(dirname(fixture.pointer), { recursive: true });
  writeFileSync(fixture.pointer, "nope\n");

  const result = refreshDevBuildLink(launcher, { home: fixture.home, warn: () => {} });
  assert.notEqual(result.status, "linked");
  assert.equal(readFileSync(fixture.pointer, "utf8"), "nope\n");
  assert.throws(() => lstatSync(fixture.link), { code: "ENOENT" });
});

test("concurrent EEXIST on public is linked if racer created our pointer", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");
  mkdirSync(dirname(fixture.link), { recursive: true });

  const result = refreshDevBuildLink(launcher, {
    home: fixture.home,
    beforeClaimPublic() {
      mkdirSync(dirname(fixture.pointer), { recursive: true });
      try {
        symlinkSync(launcher, fixture.pointer);
      } catch (error) {
        if (error.code !== "EEXIST") throw error;
      }
      symlinkSync(fixture.pointer, fixture.link);
    },
  });

  assert.equal(result.status, "linked");
  assert.equal(readlinkSync(fixture.link), fixture.pointer);
});

test("refreshDevBuildLink never throws on unexpected error", (t) => {
  const fixture = devLinkFixture(t);
  const launcher = fixture.loafLauncher("candidate");
  const result = refreshDevBuildLink(launcher, {
    home: fixture.home,
    warn: () => {},
    fs: {
      renameSync() {
        throw new Error("boom");
      },
    },
  });

  assert.equal(result.status, "failed");
});

function devLinkFixture(t) {
  const root = mkdtempSync(join(tmpdir(), "loaf-dev-link-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const home = join(root, "home");
  return {
    root,
    home,
    link: join(home, ".local", "bin", "loaf"),
    pointer: join(home, ".local", "share", "loaf", "current-dev-launcher"),
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
