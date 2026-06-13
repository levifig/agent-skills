#!/usr/bin/env node
/**
 * Verify generated Go command artifacts are present and synchronized.
 */

import { existsSync, readFileSync } from "node:fs";
import { mkdtempSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawnSync } from "node:child_process";

const rootDir = process.cwd();
const packageJSON = JSON.parse(readFileSync(join(rootDir, "package.json"), "utf8"));
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
const targets = readVerifyTargets(baseEnv);
const dryRun = baseEnv.LOAF_NATIVE_ARTIFACT_DRY_RUN === "1";

const requiredFiles = [
  "cli/runtime/loaf-launcher.cjs",
  "bin/loaf",
  "bin/package.json",
  "plugins/loaf/bin/loaf",
  "plugins/loaf/bin/package.json",
];
for (const target of targets) {
  requiredFiles.push(nativeArtifact("bin", target), nativeArtifact("plugins/loaf/bin", target));
}

if (dryRun) {
  for (const file of requiredFiles) {
    console.log(`DRY RUN: would verify ${file}`);
  }
  process.exit(0);
}

for (const file of requiredFiles) {
  if (!existsSync(join(rootDir, file))) {
    fail(`missing required artifact: ${file}`);
  }
}

assertSinglePublicCommand(packageJSON);
assertPortablePackage(packageJSON);
assertSame("cli/runtime/loaf-launcher.cjs", "bin/loaf");
assertSame("cli/runtime/loaf-launcher.cjs", "plugins/loaf/bin/loaf");
assertSame("bin/loaf", "plugins/loaf/bin/loaf");
assertSame("bin/package.json", "plugins/loaf/bin/package.json");
for (const target of targets) {
  assertSame(nativeArtifact("bin", target), nativeArtifact("plugins/loaf/bin", target));
  assertReproducibleGoBinary(target);
}

console.log("Go command artifacts are present and synchronized.");

function assertSinglePublicCommand(manifest) {
  const bin = manifest.bin;
  if (!bin || typeof bin !== "object" || Array.isArray(bin)) {
    fail("package.json must expose a bin object with one public command");
  }
  const entries = Object.entries(bin);
  if (entries.length !== 1 || entries[0][0] !== "loaf" || entries[0][1] !== "bin/loaf") {
    fail("package.json must expose exactly one public command: loaf -> bin/loaf");
  }
}

function assertPortablePackage(manifest) {
  if ("os" in manifest || "cpu" in manifest) {
    fail("package.json must not restrict os/cpu while bin/loaf is a portable launcher");
  }
}

function assertSame(left, right) {
  const leftBytes = readFileSync(join(rootDir, left));
  const rightBytes = readFileSync(join(rootDir, right));
  if (!leftBytes.equals(rightBytes)) {
    fail(`${right} is stale; run npm run build`);
  }
}

function assertReproducibleGoBinary(target) {
  const tempDir = mkdtempSync(join(tmpdir(), "loaf-go-verify-"));
  const tempBinary = join(tempDir, nativeBinaryName(target));
  try {
    const result = spawnSync("go", ["build", "-trimpath", "-ldflags", "-buildid=", "-o", tempBinary, "./cmd/loaf"], {
      cwd: rootDir,
      env: {
        ...baseEnv,
        GOOS: target.goos,
        GOARCH: target.goarch,
      },
      stdio: "inherit",
    });
    if (result.status !== 0) {
      fail(`go rebuild failed with exit code ${result.status ?? 1}`);
    }
    const committed = readFileSync(join(rootDir, nativeArtifact("bin", target)));
    const rebuilt = readFileSync(tempBinary);
    if (!committed.equals(rebuilt)) {
      fail(`${nativeArtifact("bin", target)} is stale; run npm run build:go`);
    }
  } finally {
    rmSync(tempDir, { recursive: true, force: true });
  }
}

function pinnedGoToolchainEnv() {
  const goMod = readFileSync(join(rootDir, "go.mod"), "utf8");
  const match = goMod.match(/^toolchain\s+(\S+)\s*$/m);
  return match ? { GOTOOLCHAIN: match[1] } : {};
}

function nativeArtifact(binRoot, target) {
  return join(binRoot, "native", target.runtimeID, nativeBinaryName(target));
}

function nativeBinaryName(target) {
  return target.goos === "windows" ? "loaf.exe" : "loaf";
}

function readVerifyTargets(goEnv) {
  const requested = targetListFromEnv(goEnv);
  if (requested.length > 0) {
    return requested.map((runtimeID) => targetFromRuntimeID(runtimeID));
  }
  const current = readCurrentGoTarget(goEnv);
  return [{ ...current, runtimeID: `${nodePlatform(current.goos)}-${nodeArch(current.goarch)}` }];
}

function targetListFromEnv(goEnv) {
  return (goEnv.LOAF_VERIFY_TARGETS || goEnv.LOAF_BUILD_TARGETS || goEnv.LOAF_NATIVE_TARGETS || "")
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
    fail(result.stderr || "`go env GOOS GOARCH` failed");
  }
  const [goos, goarch] = result.stdout.trim().split(/\s+/);
  if (!goos || !goarch) {
    fail("could not determine Go target from `go env GOOS GOARCH`");
  }
  return { goos, goarch };
}

function targetFromRuntimeID(runtimeID) {
  const target = supportedTargets[runtimeID];
  if (!target) {
    fail(`unsupported native target ${JSON.stringify(runtimeID)}. Supported targets: ${Object.keys(supportedTargets).join(", ")}`);
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

function fail(message) {
  console.error(`ERROR: ${message}`);
  process.exit(1);
}
