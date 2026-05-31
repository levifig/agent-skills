#!/usr/bin/env node
"use strict";

const { existsSync } = require("node:fs");
const { dirname, join } = require("node:path");
const { spawnSync } = require("node:child_process");

const GO_ONLY_COMMANDS = new Set([
  "state",
  "trace",
  "brainstorm",
  "idea",
  "spark",
  "tag",
  "bundle",
  "link",
]);

const binDir = __dirname;
const rootDir = dirname(binDir);
const target = `${process.platform}-${process.arch}`;
const nativeName = process.platform === "win32" ? "loaf.exe" : "loaf";
const nativePath = join(binDir, "native", target, nativeName);
const fallbackScript = findFallbackScript();
const args = process.argv.slice(2);

if (existsSync(nativePath)) {
  const env = { ...process.env };
  if (fallbackScript && !env.LOAF_LEGACY_CLI) {
    env.LOAF_LEGACY_CLI = fallbackScript;
  }
  const result = spawnSync(nativePath, args, { stdio: "inherit", env });
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

if (!fallbackScript) {
  console.error("error: TypeScript fallback not found; reinstall Loaf or run `npm run build:cli` in a checkout.");
  process.exit(1);
}

const command = firstCommand(args);
if (GO_ONLY_COMMANDS.has(command)) {
  console.error(
    `error: loaf ${command} requires a native Loaf runtime for ${target}; ` +
      "this package can still run legacy TypeScript commands on this platform."
  );
  console.error("hint: full per-platform native packaging will replace this launcher after the Go CLI port is complete.");
  process.exit(2);
}

const fallback = spawnSync(process.execPath, [fallbackScript, ...args], { stdio: "inherit" });
if (fallback.error) {
  console.error(`error: failed to start TypeScript fallback at ${fallbackScript}: ${fallback.error.message}`);
  process.exit(1);
}
if (fallback.signal) {
  console.error(`error: TypeScript fallback exited from signal ${fallback.signal}`);
  process.exit(1);
}
process.exit(fallback.status ?? 1);

function findFallbackScript() {
  const candidates = [
    join(rootDir, "dist-cli", "index.js"),
    join(binDir, "..", "share", "loaf", "dist-cli", "index.js"),
  ];
  for (const candidate of candidates) {
    if (existsSync(candidate)) return candidate;
  }
  return null;
}

function firstCommand(argv) {
  for (const arg of argv) {
    if (arg === "--") return null;
    if (!arg.startsWith("-")) return arg;
  }
  return null;
}
