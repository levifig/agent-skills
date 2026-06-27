#!/usr/bin/env node
/**
 * Build the Go front controller used as the public loaf command.
 */

import { spawnSync } from "node:child_process";
import { chmodSync, copyFileSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { goBuildArgs } from "./go-build-flags.mjs";

const rootDir = process.cwd();
const launcherSource = join(rootDir, "cli", "runtime", "loaf-launcher.cjs");
const launcherOutput = join(rootDir, "bin", "loaf");

const baseEnv = {
  ...process.env,
  CGO_ENABLED: "0",
  ...pinnedGoToolchainEnv(),
};
const supportedTargets = {
  "darwin-arm64": { goos: "darwin", goarch: "arm64" },
  "darwin-x64": { goos: "darwin", goarch: "amd64" },
  "linux-arm64": { goos: "linux", goarch: "arm64" },
  "linux-x64": { goos: "linux", goarch: "amd64" },
  "win32-arm64": { goos: "windows", goarch: "arm64" },
  "win32-x64": { goos: "windows", goarch: "amd64" },
};
const targets = readBuildTargets(baseEnv);
const dryRun = baseEnv.LOAF_NATIVE_ARTIFACT_DRY_RUN === "1";

mkdirSync(dirname(launcherOutput), { recursive: true });
if (dryRun) {
  console.log(`DRY RUN: would copy Loaf launcher to ${launcherOutput}`);
} else {
  copyFileSync(launcherSource, launcherOutput);
  chmodSync(launcherOutput, 0o755);
  writeFileSync(join(rootDir, "bin", "package.json"), JSON.stringify({ type: "commonjs" }, null, 2) + "\n");
}

for (const target of targets) {
  const nativeName = target.goos === "windows" ? "loaf.exe" : "loaf";
  const nativeOutput = join(rootDir, "bin", "native", target.runtimeID, nativeName);

  if (dryRun) {
    console.log(`DRY RUN: would build ${target.runtimeID} at ${nativeOutput}`);
    continue;
  }

  mkdirSync(dirname(nativeOutput), { recursive: true });
  const result = spawnSync("go", goBuildArgs(nativeOutput, baseEnv), {
    cwd: rootDir,
    env: {
      ...baseEnv,
      GOOS: target.goos,
      GOARCH: target.goarch,
    },
    stdio: "inherit",
  });

  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }

  chmodSync(nativeOutput, 0o755);
  console.log(`✓ Built Go front controller (${target.runtimeID}): ${nativeOutput}`);
}

if (!dryRun) {
  console.log(`✓ Built Loaf launcher: ${launcherOutput}`);
}

function readBuildTargets(goEnv) {
  const requested = targetListFromEnv(goEnv);
  if (requested.length > 0) {
    return requested.map((runtimeID) => targetFromRuntimeID(runtimeID));
  }
  const current = readCurrentGoTarget(goEnv);
  return [{ ...current, runtimeID: `${nodePlatform(current.goos)}-${nodeArch(current.goarch)}` }];
}

function pinnedGoToolchainEnv() {
  const goMod = readFileSync(join(rootDir, "go.mod"), "utf8");
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

function readCurrentGoTarget(goEnv) {
  const result = spawnSync("go", ["env", "GOOS", "GOARCH"], {
    cwd: rootDir,
    env: goEnv,
    encoding: "utf8",
  });
  if (result.status !== 0) {
    process.stderr.write(result.stderr || "");
    process.exit(result.status ?? 1);
  }
  const [goos, goarch] = result.stdout.trim().split(/\s+/);
  if (!goos || !goarch) {
    console.error("ERROR: could not determine Go target from `go env GOOS GOARCH`");
    process.exit(1);
  }
  return { goos, goarch };
}

function targetFromRuntimeID(runtimeID) {
  const target = supportedTargets[runtimeID];
  if (!target) {
    console.error(`ERROR: unsupported LOAF_BUILD_TARGETS entry ${JSON.stringify(runtimeID)}.`);
    console.error(`Supported targets: ${Object.keys(supportedTargets).join(", ")}`);
    process.exit(1);
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
