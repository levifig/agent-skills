import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { runNativeBuild } from "./build-go.mjs";

test("partial multi-target failure leaves previous successful set", (t) => {
  const fixture = nativeFixture(t);
  const result = runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    refreshLink: () => {
      throw new Error("refreshLink must not run after a compile failure");
    },
    buildTarget: failingSecondTarget(),
  });

  assert.notEqual(result.status, 0);
  assert.equal(readFileSync(fixture.linux, "utf8"), "old-linux");
  assert.equal(readFileSync(fixture.windows, "utf8"), "old-win");
  assert.equal(existsSync(fixture.staging), false);
});

test("successful multi-target publishes every binary and links the dev build", (t) => {
  const fixture = nativeFixture(t);
  let linked = 0;
  const result = runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    refreshLink: () => {
      linked += 1;
      return { status: "linked", link: "/tmp/loaf" };
    },
    buildTarget: succeedingTargets(),
  });

  assert.equal(result.status, 0);
  assert.equal(readFileSync(fixture.linux, "utf8"), "new-linux-x64");
  assert.equal(readFileSync(fixture.windows, "utf8"), "new-win32-x64");
  assert.equal(existsSync(fixture.staging), false);
  assert.equal(linked, 1);
});

test("compile failure never publishes a partial set", (t) => {
  const fixture = nativeFixture(t);
  runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    refreshLink: () => ({ status: "skipped" }),
    buildTarget: failingSecondTarget(),
  });

  assert.equal(readFileSync(fixture.linux, "utf8"), "old-linux");
  assert.equal(readFileSync(fixture.windows, "utf8"), "old-win");
});

test("release env skips user-local link", (t) => {
  const fixture = nativeFixture(t);
  let linked = 0;
  const result = runNativeBuild({
    rootDir: fixture.root,
    env: { ...fixture.env, LOAF_BUILD_COMMIT: "abc1234" },
    refreshLink: () => {
      linked += 1;
      return { status: "linked" };
    },
    buildTarget: succeedingTargets(),
  });

  assert.equal(result.status, 0);
  assert.equal(readFileSync(fixture.linux, "utf8"), "new-linux-x64");
  assert.equal(linked, 0);
});

test("LOAF_DEV_LINK=0 skips user-local link", (t) => {
  const fixture = nativeFixture(t);
  let linked = 0;
  const result = runNativeBuild({
    rootDir: fixture.root,
    env: { ...fixture.env, LOAF_DEV_LINK: "0" },
    refreshLink: () => {
      linked += 1;
      return { status: "linked" };
    },
    buildTarget: succeedingTargets(),
  });

  assert.equal(result.status, 0);
  assert.equal(linked, 0);
});

test("dry-run does not touch dest", (t) => {
  const fixture = nativeFixture(t);
  const result = runNativeBuild({
    rootDir: fixture.root,
    env: { ...fixture.env, LOAF_NATIVE_ARTIFACT_DRY_RUN: "1" },
    refreshLink: () => {
      throw new Error("refreshLink must not run during dry-run");
    },
    buildTarget: () => {
      throw new Error("buildTarget must not run during dry-run");
    },
  });

  assert.equal(result.status, 0);
  assert.equal(readFileSync(fixture.linux, "utf8"), "old-linux");
  assert.equal(readFileSync(fixture.windows, "utf8"), "old-win");
});

test("activation failure does not fail a successful native build", (t) => {
  const fixture = nativeFixture(t);
  const warnings = [];
  const result = runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    warn: (message) => warnings.push(message),
    refreshLink: () => {
      const error = new Error("permission denied");
      error.code = "EACCES";
      throw error;
    },
    buildTarget: succeedingTargets(),
  });

  assert.equal(result.status, 0);
  assert.equal(readFileSync(fixture.linux, "utf8"), "new-linux-x64");
  assert.ok(warnings.some((message) => /failed to link latest dev build/.test(message)));
});

test("dev identity from git is linked into every compile", (t) => {
  const repo = gitRepo(t);
  const fixture = nativeFixture(t, { root: repo });
  git(repo, "add", "-A");
  git(repo, "commit", "-m", "fixture");
  const head = git(repo, "rev-parse", "HEAD").stdout.trim();
  const seen = [];

  const clean = runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    refreshLink: () => ({ status: "skipped" }),
    buildTarget: capturingTargets(seen),
  });
  assert.equal(clean.status, 0);
  assert.equal(seen.length, 2);
  for (const env of seen) {
    assert.equal(env.LOAF_DEV_COMMIT, head);
    assert.equal(env.LOAF_DEV_MODIFIED, "0");
  }

  writeFileSync(join(repo, "tracked.txt"), "dirty\n");
  seen.length = 0;
  const dirty = runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    refreshLink: () => ({ status: "skipped" }),
    buildTarget: capturingTargets(seen),
  });
  assert.equal(dirty.status, 0);
  assert.equal(seen[0].LOAF_DEV_COMMIT, head);
  assert.equal(seen[0].LOAF_DEV_MODIFIED, "1");
});

test("non-Git root compiles without a dev identity and warns", (t) => {
  const fixture = nativeFixture(t);
  const warnings = [];
  const seen = [];
  const result = runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    warn: (message) => warnings.push(message),
    refreshLink: () => ({ status: "skipped" }),
    buildTarget: capturingTargets(seen),
  });

  assert.equal(result.status, 0);
  assert.equal(readFileSync(fixture.linux, "utf8"), "new-linux-x64");
  assert.equal(seen[0].LOAF_DEV_COMMIT, undefined);
  assert.ok(warnings.some((message) => /could not resolve a Git identity/.test(message)));
});

test("release env never resolves a dev identity", (t) => {
  const fixture = nativeFixture(t);
  const seen = [];
  const result = runNativeBuild({
    rootDir: fixture.root,
    env: { ...fixture.env, LOAF_BUILD_COMMIT: "abc1234" },
    resolveDevIdentity: () => {
      throw new Error("resolveDevIdentity must not run for a release build");
    },
    refreshLink: () => ({ status: "skipped" }),
    buildTarget: capturingTargets(seen),
  });

  assert.equal(result.status, 0);
  assert.equal(seen[0].LOAF_DEV_COMMIT, undefined);
  assert.equal(seen[0].LOAF_BUILD_COMMIT, "abc1234");
});

function nativeFixture(t, options = {}) {
  const root = options.root || mkdtempSync(join(tmpdir(), "loaf-native-build-"));
  if (!options.root) {
    t.after(() => rmSync(root, { recursive: true, force: true }));
  }
  mkdirSync(join(root, "cli", "runtime"), { recursive: true });
  writeFileSync(join(root, "cli", "runtime", "loaf-launcher.cjs"), "launcher\n");
  writeFileSync(join(root, "go.mod"), "module example.com/loaf\n\ngo 1.22\n");
  const linux = join(root, "bin", "native", "linux-x64", "loaf");
  const windows = join(root, "bin", "native", "win32-x64", "loaf.exe");
  mkdirSync(dirname(linux), { recursive: true });
  mkdirSync(dirname(windows), { recursive: true });
  writeFileSync(linux, "old-linux");
  writeFileSync(windows, "old-win");
  return {
    root,
    linux,
    windows,
    staging: join(root, "bin", "native", ".staging"),
    env: {
      ...process.env,
      LOAF_BUILD_TARGETS: "linux-x64,win32-x64",
      LOAF_DEV_LINK: "",
    },
  };
}

function succeedingTargets() {
  return (dest) => {
    mkdirSync(dirname(dest), { recursive: true });
    writeFileSync(dest, dest.includes("win32") ? "new-win32-x64" : "new-linux-x64");
    return { status: 0 };
  };
}

function capturingTargets(seen) {
  return (dest, _target, env) => {
    seen.push(env);
    mkdirSync(dirname(dest), { recursive: true });
    writeFileSync(dest, dest.includes("win32") ? "new-win32-x64" : "new-linux-x64");
    return { status: 0 };
  };
}

function gitRepo(t) {
  const root = mkdtempSync(join(tmpdir(), "loaf-git-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  git(root, "init", "-q");
  git(root, "config", "user.email", "dev@example.com");
  git(root, "config", "user.name", "Dev");
  git(root, "config", "commit.gpgsign", "false");
  return root;
}

function git(cwd, ...args) {
  const result = spawnSync("git", args, { cwd, encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr);
  return result;
}

function failingSecondTarget() {
  let count = 0;
  return (dest) => {
    count += 1;
    mkdirSync(dirname(dest), { recursive: true });
    writeFileSync(dest, dest.includes("win32") ? "new-win32-x64" : "new-linux-x64");
    if (count === 2) {
      return { status: 1 };
    }
    return { status: 0 };
  };
}
