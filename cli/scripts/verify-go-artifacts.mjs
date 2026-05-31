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

const requiredFiles = [
  "cli/runtime/loaf-launcher.cjs",
  "bin/loaf",
  "bin/package.json",
  currentNativeArtifact("bin"),
  "dist-cli/index.js",
  "plugins/loaf/bin/loaf",
  "plugins/loaf/bin/package.json",
  currentNativeArtifact("plugins/loaf/bin"),
  "plugins/loaf/dist-cli/index.js",
];

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
assertSame(currentNativeArtifact("bin"), currentNativeArtifact("plugins/loaf/bin"));
assertSame("dist-cli/index.js", "plugins/loaf/dist-cli/index.js");
assertReproducibleGoBinary();

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

function assertReproducibleGoBinary() {
  const tempDir = mkdtempSync(join(tmpdir(), "loaf-go-verify-"));
  const tempBinary = join(tempDir, nativeBinaryName());
  try {
    const result = spawnSync("go", ["build", "-trimpath", "-o", tempBinary, "./cmd/loaf"], {
      cwd: rootDir,
      env: { ...process.env, CGO_ENABLED: "0" },
      stdio: "inherit",
    });
    if (result.status !== 0) {
      fail(`go rebuild failed with exit code ${result.status ?? 1}`);
    }
    const committed = readFileSync(join(rootDir, currentNativeArtifact("bin")));
    const rebuilt = readFileSync(tempBinary);
    if (!committed.equals(rebuilt)) {
      fail(`${currentNativeArtifact("bin")} is stale; run npm run build:go`);
    }
  } finally {
    rmSync(tempDir, { recursive: true, force: true });
  }
}

function currentNativeArtifact(binRoot) {
  return join(binRoot, "native", `${process.platform}-${process.arch}`, nativeBinaryName());
}

function nativeBinaryName() {
  return process.platform === "win32" ? "loaf.exe" : "loaf";
}

function fail(message) {
  console.error(`ERROR: ${message}`);
  process.exit(1);
}
