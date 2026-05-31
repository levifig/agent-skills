#!/usr/bin/env node
/**
 * Build the Go front controller used as the public loaf command.
 */

import { spawnSync } from "node:child_process";
import { chmodSync, copyFileSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";

const rootDir = process.cwd();
const launcherSource = join(rootDir, "cli", "runtime", "loaf-launcher.cjs");
const launcherOutput = join(rootDir, "bin", "loaf");

const env = {
  ...process.env,
  CGO_ENABLED: "0",
};

const goTarget = readGoTarget(env);
const nodeTarget = `${nodePlatform(goTarget.goos)}-${nodeArch(goTarget.goarch)}`;
const nativeName = goTarget.goos === "windows" ? "loaf.exe" : "loaf";
const nativeOutput = join(rootDir, "bin", "native", nodeTarget, nativeName);

mkdirSync(dirname(launcherOutput), { recursive: true });
copyFileSync(launcherSource, launcherOutput);
chmodSync(launcherOutput, 0o755);
writeFileSync(join(rootDir, "bin", "package.json"), JSON.stringify({ type: "commonjs" }, null, 2) + "\n");

mkdirSync(dirname(nativeOutput), { recursive: true });
const result = spawnSync("go", ["build", "-trimpath", "-o", nativeOutput, "./cmd/loaf"], {
  cwd: rootDir,
  env,
  stdio: "inherit",
});

if (result.status !== 0) {
  process.exit(result.status ?? 1);
}

chmodSync(nativeOutput, 0o755);
console.log(`✓ Built Loaf launcher: ${launcherOutput}`);
console.log(`✓ Built Go front controller: ${nativeOutput}`);

function readGoTarget(goEnv) {
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
