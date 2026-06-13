#!/usr/bin/env node
/**
 * Build the native-only package artifacts expected before publishing.
 */

import { spawnSync } from "node:child_process";

const releaseTargets = [
  "darwin-arm64",
  "darwin-x64",
  "linux-arm64",
  "linux-x64",
  "win32-arm64",
  "win32-x64",
];

const targets = (process.env.LOAF_RELEASE_TARGETS || process.env.LOAF_BUILD_TARGETS || releaseTargets.join(",")).trim();
if (!targets) {
  console.error("ERROR: release build target list is empty.");
  process.exit(1);
}

const env = {
  ...process.env,
  LOAF_BUILD_TARGETS: targets,
  LOAF_VERIFY_TARGETS: process.env.LOAF_VERIFY_TARGETS || targets,
};

if (process.env.LOAF_RELEASE_DRY_RUN === "1") {
  console.log(`DRY RUN: would run npm run build for native release targets: ${env.LOAF_BUILD_TARGETS}`);
  console.log(`DRY RUN: LOAF_VERIFY_TARGETS=${env.LOAF_VERIFY_TARGETS}`);
  process.exit(0);
}

const npm = process.platform === "win32" ? "npm.cmd" : "npm";
const result = spawnSync(npm, ["run", "build"], {
  cwd: process.cwd(),
  env,
  stdio: "inherit",
});

process.exit(result.status ?? 1);
