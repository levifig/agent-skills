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
    resolveCommit: () => "bbbbbbb",
    refreshLink: () => {
      throw new Error("refreshLink must not run after a compile failure");
    },
    buildTarget: failingSecondTarget(fixture),
  });

  assert.notEqual(result.status, 0);
  assert.equal(readFileSync(fixture.linux, "utf8"), "old-linux");
  assert.equal(readFileSync(fixture.windows, "utf8"), "old-win");
  assert.equal(readFileSync(fixture.marker, "utf8"), "aaaaaaa\n");
  assert.equal(existsSync(fixture.staging), false);
});

test("successful multi-target publishes matching marker", (t) => {
  const fixture = nativeFixture(t);
  let linked = 0;
  const result = runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    resolveCommit: () => "bbbbbbb",
    refreshLink: () => {
      linked += 1;
      return { status: "linked", link: "/tmp/loaf" };
    },
    buildTarget: succeedingTargets(),
  });

  assert.equal(result.status, 0);
  assert.equal(readFileSync(fixture.linux, "utf8"), "new-linux-x64");
  assert.equal(readFileSync(fixture.windows, "utf8"), "new-win32-x64");
  assert.equal(readFileSync(fixture.marker, "utf8"), "bbbbbbb\n");
  assert.equal(existsSync(fixture.staging), false);
  assert.equal(linked, 1);
});

test("compile failure never publishes a new binary with the old marker", (t) => {
  const fixture = nativeFixture(t);
  runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    resolveCommit: () => "bbbbbbb",
    refreshLink: () => ({ status: "skipped" }),
    buildTarget: failingSecondTarget(fixture),
  });

  assert.notEqual(readFileSync(fixture.linux, "utf8"), "new-linux-x64");
  assert.equal(readFileSync(fixture.marker, "utf8"), "aaaaaaa\n");
});

test("non-Git successful compile publishes binaries and removes marker", (t) => {
  const fixture = nativeFixture(t);
  const warnings = [];
  const result = runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    resolveCommit: () => "",
    warn: (message) => warnings.push(message),
    refreshLink: () => {
      throw new Error("refreshLink must not run without a recorded commit");
    },
    buildTarget: succeedingTargets(),
  });

  assert.equal(result.status, 0);
  assert.equal(readFileSync(fixture.linux, "utf8"), "new-linux-x64");
  assert.equal(existsSync(fixture.marker), false);
  assert.ok(warnings.some((message) => /could not record the source commit/.test(message)));
});

test("detached HEAD still records rev-parse HEAD", (t) => {
  const repo = gitRepo(t);
  writeFileSync(join(repo, "tracked.txt"), "one\n");
  git(repo, "add", "tracked.txt");
  git(repo, "commit", "-m", "one");
  git(repo, "checkout", "--detach");
  const fixture = nativeFixture(t, { root: repo });
  const want = git(repo, "rev-parse", "--short=7", "HEAD").stdout.trim().toLowerCase();

  const result = runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    refreshLink: () => ({ status: "skipped" }),
    buildTarget: succeedingTargets(),
  });

  assert.equal(result.status, 0);
  assert.equal(readFileSync(fixture.marker, "utf8"), `${want}\n`);
});

test("shallow clone still records HEAD", (t) => {
  const source = gitRepo(t);
  writeFileSync(join(source, "tracked.txt"), "one\n");
  git(source, "add", "tracked.txt");
  git(source, "commit", "-m", "one");
  const clone = mkdtempSync(join(tmpdir(), "loaf-shallow-"));
  t.after(() => rmSync(clone, { recursive: true, force: true }));
  const cloned = spawnSync("git", ["clone", "--depth", "1", source, clone], { encoding: "utf8" });
  assert.equal(cloned.status, 0, cloned.stderr);
  const fixture = nativeFixture(t, { root: clone });
  const want = git(clone, "rev-parse", "--short=7", "HEAD").stdout.trim().toLowerCase();

  const result = runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    refreshLink: () => ({ status: "skipped" }),
    buildTarget: succeedingTargets(),
  });

  assert.equal(result.status, 0);
  assert.equal(readFileSync(fixture.marker, "utf8"), `${want}\n`);
});

test("dirty tree is HEAD-only", (t) => {
  const repo = gitRepo(t);
  writeFileSync(join(repo, "tracked.txt"), "clean\n");
  git(repo, "add", "tracked.txt");
  git(repo, "commit", "-m", "clean");
  writeFileSync(join(repo, "tracked.txt"), "dirty\n");
  const fixture = nativeFixture(t, { root: repo });
  const want = git(repo, "rev-parse", "--short=7", "HEAD").stdout.trim().toLowerCase();
  const seen = [];

  const result = runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    spawnSync(command, args, options) {
      if (command === "git") seen.push(args.slice());
      return spawnSync(command, args, options);
    },
    refreshLink: () => ({ status: "skipped" }),
    buildTarget: succeedingTargets(),
  });

  assert.equal(result.status, 0);
  assert.equal(readFileSync(fixture.marker, "utf8"), `${want}\n`);
  assert.deepEqual(seen, [["rev-parse", "--short=7", "HEAD"]]);
});

test("invalid git sha removes marker after successful publish", (t) => {
  const fixture = nativeFixture(t);
  const warnings = [];
  const result = runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    resolveCommit: () => "abc",
    warn: (message) => warnings.push(message),
    refreshLink: () => {
      throw new Error("refreshLink must not run without a recorded commit");
    },
    buildTarget: succeedingTargets(),
  });

  assert.equal(result.status, 0);
  assert.equal(readFileSync(fixture.linux, "utf8"), "new-linux-x64");
  assert.equal(existsSync(fixture.marker), false);
  assert.ok(warnings.some((message) => /could not record the source commit/.test(message)));
});

test("release env skips user-local link", (t) => {
  const fixture = nativeFixture(t);
  let linked = 0;
  const result = runNativeBuild({
    rootDir: fixture.root,
    env: { ...fixture.env, LOAF_BUILD_COMMIT: "abc1234" },
    resolveCommit: () => "bbbbbbb",
    refreshLink: () => {
      linked += 1;
      return { status: "linked" };
    },
    buildTarget: succeedingTargets(),
  });

  assert.equal(result.status, 0);
  assert.equal(readFileSync(fixture.marker, "utf8"), "bbbbbbb\n");
  assert.equal(linked, 0);
});

test("dry-run does not touch marker or dest", (t) => {
  const fixture = nativeFixture(t);
  const result = runNativeBuild({
    rootDir: fixture.root,
    env: { ...fixture.env, LOAF_NATIVE_ARTIFACT_DRY_RUN: "1" },
    resolveCommit: () => "bbbbbbb",
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
  assert.equal(readFileSync(fixture.marker, "utf8"), "aaaaaaa\n");
});

test("activation failure does not fail a successful native build", (t) => {
  const fixture = nativeFixture(t);
  const warnings = [];
  const result = runNativeBuild({
    rootDir: fixture.root,
    env: fixture.env,
    resolveCommit: () => "bbbbbbb",
    warn: (message) => warnings.push(message),
    refreshLink: () => {
      const error = new Error("permission denied");
      error.code = "EACCES";
      throw error;
    },
    buildTarget: succeedingTargets(),
  });

  assert.equal(result.status, 0);
  assert.equal(readFileSync(fixture.marker, "utf8"), "bbbbbbb\n");
  assert.ok(warnings.some((message) => /failed to link latest dev build/.test(message)));
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
  const marker = join(root, "bin", ".loaf-dev-commit");
  writeFileSync(marker, "aaaaaaa\n");
  return {
    root,
    linux,
    windows,
    marker,
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

function failingSecondTarget(fixture) {
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

function gitRepo(t) {
  const root = mkdtempSync(join(tmpdir(), "loaf-git-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  git(root, "init");
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
