#!/usr/bin/env node
/**
 * Build the Go front controller used as the public loaf command.
 */

import { spawnSync } from "node:child_process";
import {
  chmodSync,
  copyFileSync,
  mkdirSync,
  readFileSync,
  renameSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { refreshDevBuildLink } from "./dev-build-link.mjs";
import { goBuildArgs } from "./go-build-flags.mjs";

const supportedTargets = {
  "darwin-arm64": { goos: "darwin", goarch: "arm64" },
  "darwin-x64": { goos: "darwin", goarch: "amd64" },
  "linux-arm64": { goos: "linux", goarch: "arm64" },
  "linux-x64": { goos: "linux", goarch: "amd64" },
  "win32-arm64": { goos: "windows", goarch: "arm64" },
  "win32-x64": { goos: "windows", goarch: "amd64" },
};

export function runNativeBuild(options = {}) {
  const rootDir = options.rootDir || process.cwd();
  const env = options.env || process.env;
  const fs = {
    chmodSync,
    copyFileSync,
    mkdirSync,
    readFileSync,
    renameSync,
    rmSync,
    writeFileSync,
    ...options.fs,
  };
  const spawn = options.spawnSync || spawnSync;
  const log = options.log || console.log;
  const warn = options.warn || console.warn;
  const platform = options.platform || process.platform;
  const refreshLink = options.refreshLink || refreshDevBuildLink;
  const resolveCommit = options.resolveCommit || ((cwd, buildEnv) => defaultResolveCommit(cwd, buildEnv, spawn));
  const buildTarget = options.buildTarget || ((dest, target, buildEnv) => defaultBuildTarget(dest, target, buildEnv, rootDir, spawn));

  const launcherSource = join(rootDir, "cli", "runtime", "loaf-launcher.cjs");
  const launcherOutput = join(rootDir, "bin", "loaf");
  const nativeRoot = join(rootDir, "bin", "native");
  const stagingRoot = join(nativeRoot, ".staging");
  const devCommitOutput = join(rootDir, "bin", ".loaf-dev-commit");
  const baseEnv = {
    ...env,
    CGO_ENABLED: "0",
    ...pinnedGoToolchainEnv(rootDir, fs),
  };
  const targets = readBuildTargets(baseEnv, rootDir, spawn);
  const dryRun = baseEnv.LOAF_NATIVE_ARTIFACT_DRY_RUN === "1";

  fs.mkdirSync(dirname(launcherOutput), { recursive: true });
  if (dryRun) {
    log(`DRY RUN: would copy Loaf launcher to ${launcherOutput}`);
  } else {
    fs.copyFileSync(launcherSource, launcherOutput);
    fs.chmodSync(launcherOutput, 0o755);
    fs.writeFileSync(join(rootDir, "bin", "package.json"), JSON.stringify({ type: "commonjs" }, null, 2) + "\n");
  }

  const staged = [];
  if (!dryRun) {
    fs.rmSync(stagingRoot, { recursive: true, force: true });
  }

  try {
    for (const target of targets) {
      const nativeName = target.goos === "windows" ? "loaf.exe" : "loaf";
      const nativeOutput = join(nativeRoot, target.runtimeID, nativeName);
      const stagedOutput = join(stagingRoot, target.runtimeID, nativeName);

      if (dryRun) {
        log(`DRY RUN: would build ${target.runtimeID} at ${nativeOutput}`);
        continue;
      }

      fs.mkdirSync(dirname(stagedOutput), { recursive: true });
      const result = buildTarget(stagedOutput, target, {
        ...baseEnv,
        GOOS: target.goos,
        GOARCH: target.goarch,
      });

      if ((result.status ?? 1) !== 0) {
        return { status: result.status ?? 1 };
      }

      fs.chmodSync(stagedOutput, 0o755);
      log(`✓ Built Go front controller (${target.runtimeID}): ${nativeOutput}`);
      staged.push({ stagedOutput, nativeOutput });
    }

    if (dryRun) {
      return { status: 0 };
    }

    fs.rmSync(devCommitOutput, { force: true });
    for (const artifact of staged) {
      fs.mkdirSync(dirname(artifact.nativeOutput), { recursive: true });
      try {
        fs.renameSync(artifact.stagedOutput, artifact.nativeOutput);
      } catch {
        fs.rmSync(artifact.nativeOutput, { force: true });
        fs.renameSync(artifact.stagedOutput, artifact.nativeOutput);
      }
    }
  } finally {
    if (!dryRun) {
      fs.rmSync(stagingRoot, { recursive: true, force: true });
    }
  }

  const devCommit = recordDevBuildCommit({
    outputPath: devCommitOutput,
    env: baseEnv,
    rootDir,
    resolveCommit,
    fs,
    warn,
    log,
  });
  if (devCommit && platform !== "win32" && !isReleaseBuild(baseEnv) && baseEnv.LOAF_DEV_LINK !== "0") {
    try {
      const result = refreshLink(launcherOutput);
      if (result.status === "linked") {
        log(`✓ Linked latest dev build: ${result.link} -> ${launcherOutput}`);
      }
    } catch (error) {
      warn(`WARN: failed to link latest dev build (${error.code || error.message})`);
    }
  }
  log(`✓ Built Loaf launcher: ${launcherOutput}`);
  return { status: 0, commit: devCommit };
}

function recordDevBuildCommit({ outputPath, env, rootDir, resolveCommit, fs, warn, log }) {
  const commit = resolveCommit(rootDir, env);
  if (!/^[0-9a-f]{7}$/.test(commit)) {
    fs.rmSync(outputPath, { force: true });
    warn("WARN: could not record the source commit for this dev build");
    return "";
  }
  fs.writeFileSync(outputPath, commit + "\n");
  log(`✓ Recorded dev build commit: ${commit}`);
  return commit;
}

function defaultResolveCommit(rootDir, buildEnv, spawn) {
  const result = spawn("git", ["rev-parse", "--short=7", "HEAD"], {
    cwd: rootDir,
    env: buildEnv,
    encoding: "utf8",
  });
  return result.status === 0 ? result.stdout.trim().toLowerCase() : "";
}

function defaultBuildTarget(dest, target, buildEnv, rootDir, spawn) {
  return spawn("go", goBuildArgs(dest, buildEnv), {
    cwd: rootDir,
    env: buildEnv,
    stdio: "inherit",
  });
}

function isReleaseBuild(buildEnv) {
  return Boolean((buildEnv.LOAF_BUILD_COMMIT || "").trim() || (buildEnv.LOAF_BUILD_DATE || "").trim());
}

function readBuildTargets(goEnv, rootDir, spawn) {
  const requested = targetListFromEnv(goEnv);
  if (requested.length > 0) {
    return requested.map((runtimeID) => targetFromRuntimeID(runtimeID));
  }
  const current = readCurrentGoTarget(goEnv, rootDir, spawn);
  return [{ ...current, runtimeID: `${nodePlatform(current.goos)}-${nodeArch(current.goarch)}` }];
}

function pinnedGoToolchainEnv(rootDir, fs) {
  const goMod = fs.readFileSync(join(rootDir, "go.mod"), "utf8");
  const match = goMod.match(/^toolchain\s+(\S+)\s*$/m);
  return match ? { GOTOOLCHAIN: match[1] } : {};
}

function targetListFromEnv(goEnv) {
  return (goEnv.LOAF_BUILD_TARGETS || goEnv.LOAF_NATIVE_TARGETS || "")
    .split(",")
    .map((value) => value.trim())
    .filter(Boolean)
    .filter((value, index, values) => values.indexOf(value) === index);
}

function readCurrentGoTarget(goEnv, rootDir, spawn) {
  const result = spawn("go", ["env", "GOOS", "GOARCH"], {
    cwd: rootDir,
    env: goEnv,
    encoding: "utf8",
  });
  if (result.status !== 0) {
    process.stderr.write(result.stderr || "");
    throw new Error("could not determine Go target from `go env GOOS GOARCH`");
  }
  const [goos, goarch] = result.stdout.trim().split(/\s+/);
  if (!goos || !goarch) {
    throw new Error("could not determine Go target from `go env GOOS GOARCH`");
  }
  return { goos, goarch };
}

function targetFromRuntimeID(runtimeID) {
  const target = supportedTargets[runtimeID];
  if (!target) {
    throw new Error(`unsupported LOAF_BUILD_TARGETS entry ${JSON.stringify(runtimeID)}.`);
  }
  return { runtimeID, ...target };
}

function nodePlatform(goos) {
  if (goos === "windows") return "win32";
  return goos;
}

function nodeArch(goarch) {
  switch (goarch) {
    case "amd64":
      return "x64";
    case "386":
      return "ia32";
    default:
      return goarch;
  }
}

function main() {
  try {
    const result = runNativeBuild({
      rootDir: process.cwd(),
      env: process.env,
    });
    process.exit(result.status ?? 0);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (message.startsWith("unsupported LOAF_BUILD_TARGETS entry")) {
      console.error(`ERROR: ${message}`);
      console.error(`Supported targets: ${Object.keys(supportedTargets).join(", ")}`);
      process.exit(1);
    }
    if (message.includes("could not determine Go target")) {
      console.error(`ERROR: ${message}`);
      process.exit(1);
    }
    throw error;
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main();
}
