#!/usr/bin/env node

import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import { parseRunnerArgs, publishReceiptIfSuccessful } from "./capability-runner-utils.mjs";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(scriptDir, "../..");
const platform = `${process.platform}-${process.arch}`;
const candidateHooksPath = "dist/cursor/hooks.json";
const candidateBinaryPath = `bin/native/${platform}/loaf`;

export function classifyCursorPreflight(versionOutput, helpOutput, expectedVersion) {
  const observedVersion = versionOutput.trim();
  const exactVersion = observedVersion === expectedVersion;
  const noSessionPersistence = helpOutput.includes("--no-session-persistence");
  return {
    observedVersion,
    exactVersion,
    noSessionPersistence,
    smokeExecuted: false,
    blocker: !exactVersion
      ? `installed cursor-agent version ${observedVersion || "<missing>"} does not match ${expectedVersion}`
      : !noSessionPersistence
        ? "installed cursor-agent does not expose --no-session-persistence; refusing a model-visible smoke that could persist session state globally"
        : "smoke implementation is not enabled for this installed CLI",
  };
}

function run(command, args, cwd, env = {}, timeout = 120000) {
  const result = spawnSync(command, args, {
    cwd,
    env: { ...process.env, ...env },
    encoding: "utf8",
    timeout,
    maxBuffer: 16 * 1024 * 1024,
  });
  return { status: result.status ?? 1, stdout: result.stdout ?? "", stderr: result.stderr ?? "", error: result.error };
}

function sha256(path) {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}

function candidateArtifacts() {
  const hooksPath = join(repoRoot, candidateHooksPath);
  const nativeBinaryPath = join(repoRoot, candidateBinaryPath);
  if (!existsSync(hooksPath) || !existsSync(nativeBinaryPath)) throw new Error("Cursor candidate artifacts are missing");
  const hooks = JSON.parse(readFileSync(hooksPath, "utf8"));
  if (hooks.version !== 1 || !Array.isArray(hooks.hooks?.sessionStart) || hooks.hooks.sessionStart.length !== 1 || hooks.hooks.sessionStart[0]?.command !== "loaf journal context --from-hook --cursor-hook") {
    throw new Error("Cursor candidate hooks artifact does not contain the exact sessionStart adapter");
  }
  for (const field of ["beforeSubmitPrompt", "preCompact", "stop", "sessionEnd"]) {
    if (hooks.hooks?.[field] !== undefined) throw new Error(`Cursor candidate hooks artifact unexpectedly contains ${field}`);
  }
  return {
    hooks_path: candidateHooksPath,
    hooks_sha256: sha256(hooksPath),
    native_binary_path: candidateBinaryPath,
    native_binary_sha256: sha256(nativeBinaryPath),
  };
}

function main(argv = process.argv.slice(2)) {
  const { client, expectedVersion, receiptPath } = parseRunnerArgs(argv);
  const timestamp = new Date().toISOString();
  const buildGo = run("npm", ["run", "build:go"], repoRoot);
  if (buildGo.status !== 0) throw new Error("candidate Go build failed");
  const buildCursor = run("bin/loaf", ["build", "--target", "cursor"], repoRoot);
  if (buildCursor.status !== 0) throw new Error("candidate Cursor target build failed");
  const artifacts = candidateArtifacts();
  const version = run(client, ["--version"], repoRoot);
  const help = run(client, ["--help"], repoRoot);
  if (version.status !== 0 || help.status !== 0) throw new Error("installed cursor-agent version/help preflight failed");
  const preflight = classifyCursorPreflight(version.stdout, help.stdout, expectedVersion);
  if (!preflight.exactVersion) throw new Error(preflight.blocker);
  if (preflight.noSessionPersistence) throw new Error("installed cursor-agent exposes --no-session-persistence, but a model-visible isolated smoke is not implemented");
  const smoke = {
    evidence_version: 2,
    timestamp,
    target: "cursor",
    surface: "cursor-agent",
    version: preflight.observedVersion || "unknown",
    platform,
    installed_mode: "candidate-build",
    context_mode: "new-composer",
    adapter: "cursor-session-start-v1",
    mode: "candidate-preflight",
    setup: [
      "candidate dist/cursor/hooks.json is built but not installed globally",
      "an isolated disposable Git repository and absolute disposable LOAF_DB are required for a future smoke",
      "the preflight refuses execution unless the installed CLI exposes --no-session-persistence",
    ],
    candidate_target_path: "dist/cursor",
    smoke_executed: preflight.smokeExecuted,
    cli_version_exact: preflight.exactVersion,
    no_session_persistence_supported: preflight.noSessionPersistence,
    candidate_artifacts: artifacts,
    blocker: preflight.blocker,
    retry_condition: "Rerun after the installed CLI exposes a documented no-session-persistence mode or after an explicitly approved isolated session-store boundary is available.",
  };
  publishReceiptIfSuccessful(receiptPath, smoke, true);
  process.stdout.write(`${JSON.stringify({ receipt: receiptPath, status: "safely-refused", reason: smoke.blocker }, null, 2)}\n`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    main();
  } catch (error) {
    process.stderr.write(`${error instanceof Error ? error.message : "Cursor agent context preflight failed"}\n`);
    process.exitCode = 1;
  }
}
