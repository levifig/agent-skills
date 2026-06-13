#!/usr/bin/env node
"use strict";

const { existsSync } = require("node:fs");
const { join } = require("node:path");
const { spawnSync } = require("node:child_process");

const binDir = __dirname;
const target = `${process.platform}-${process.arch}`;
const nativeName = process.platform === "win32" ? "loaf.exe" : "loaf";
const nativePath = join(binDir, "native", target, nativeName);
const args = process.argv.slice(2);

if (existsSync(nativePath)) {
  const result = spawnSync(nativePath, args, { stdio: "inherit" });
  if (result.error) {
    console.error(`error: failed to start native Loaf runtime at ${nativePath}: ${result.error.message}`);
    process.exit(1);
  }
  if (result.signal) {
    console.error(`error: native Loaf runtime exited from signal ${result.signal}`);
    process.exit(1);
  }
  process.exit(result.status ?? 1);
}

console.error(
  `error: native Loaf runtime not found for ${target} at ${nativePath}; ` +
    "reinstall Loaf or run `npm run build:go` in a checkout."
);
process.exit(1);
