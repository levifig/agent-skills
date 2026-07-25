#!/usr/bin/env node

import { createHash, randomBytes } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join, relative, resolve, sep } from "node:path";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { parseRunnerArgs, publishReceiptIfSuccessful } from "./capability-runner-utils.mjs";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(scriptDir, "../..");
const platform = `${process.platform}-${process.arch}`;
const candidatePluginPath = "plugins/loaf";
const markerPattern = /^LOAF_CLAUDE_STARTUP_SMOKE_[A-F0-9]{12}$/;

export function parseClaudeStreamOutput(raw, marker) {
  const events = raw.split(/\r?\n/).filter(Boolean).map((line) => JSON.parse(line));
  const hook = events.find((event) => event.type === "system" && event.subtype === "hook_response" && event.hook_event === "SessionStart");
  if (!hook || typeof hook.output !== "string") throw new Error("SessionStart hook response was not observed");
  const native = JSON.parse(hook.output);
  const hookSpecific = native.hookSpecificOutput;
  const additionalContext = hookSpecific?.additionalContext;
  const assistantTexts = [];
  for (const event of events) {
    if (event.type === "assistant") {
      for (const block of event.message?.content ?? []) if (typeof block.text === "string") assistantTexts.push(block.text);
    }
    if (event.type === "result" && typeof event.result === "string") assistantTexts.push(event.result);
  }
  return {
    native,
    hookObservation: {
      event_name: "SessionStart:startup",
      native_json: hookSpecific?.hookEventName === "SessionStart" && typeof additionalContext === "string",
      hook_event_name: hookSpecific?.hookEventName ?? "",
      additional_context_marker: typeof additionalContext === "string" && additionalContext.includes(marker),
    },
    assistantMarkerMatch: assistantTexts.some((text) => text.trim() === marker),
  };
}

export function claudeVersionMatches(output, expectedVersion) {
  const match = output.trim().match(/^([0-9]+\.[0-9]+\.[0-9]+) \(Claude Code\)$/);
  return match?.[1] === expectedVersion;
}

function run(command, args, cwd, env = {}, timeout = 120000) {
  const result = spawnSync(command, args, {
    cwd,
    env: { ...process.env, LOAF_DB: undefined, ...env },
    encoding: "utf8",
    timeout,
    maxBuffer: 16 * 1024 * 1024,
  });
  return {
    status: result.status ?? 1,
    stdout: result.stdout ?? "",
    stderr: result.stderr ?? "",
    error: result.error,
  };
}

function sha256(path) {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}

function main(argv = process.argv.slice(2)) {
  const { client, expectedVersion, receiptPath } = parseRunnerArgs(argv);
  const marker = `LOAF_CLAUDE_STARTUP_SMOKE_${randomBytes(6).toString("hex").toUpperCase()}`;
  if (!markerPattern.test(marker)) throw new Error("generated marker does not match the required format");
  const timestamp = new Date().toISOString();
  const tempRoot = mkdtempSync(join(tmpdir(), "loaf-claude-code-startup-smoke-"));
  if (resolve(tempRoot).startsWith(`${repoRoot}${sep}`)) throw new Error("disposable Claude Code smoke root must be outside the repository");
  const cleanup = () => {
    try {
      rmSync(tempRoot, { recursive: true, force: true, maxRetries: 3, retryDelay: 50 });
    } catch {
      return false;
    }
    return !existsSync(tempRoot);
  };
  for (const signal of ["SIGINT", "SIGTERM", "SIGHUP"]) {
    process.once(signal, () => {
      cleanup();
      process.exit(128 + ({ SIGINT: 2, SIGTERM: 15, SIGHUP: 1 }[signal] ?? 1));
    });
  }
  const disposableRepo = join(tempRoot, "repo");
  const dbDir = join(tempRoot, "state");
  mkdirSync(disposableRepo, { recursive: true });
  mkdirSync(dbDir, { recursive: true });
  const dbPath = join(dbDir, "loaf.sqlite");
  const candidatePlugin = join(repoRoot, candidatePluginPath);
  const candidateBinary = join(candidatePlugin, "bin", "loaf");
  let smoke;
  let failure;
  let cleanupSucceeded = false;
  try {
    const buildGo = run("npm", ["run", "build:go"], repoRoot);
    if (buildGo.status !== 0) throw new Error("candidate Go build failed");
    const buildClaude = run("bin/loaf", ["build", "--target", "claude-code"], repoRoot);
    if (buildClaude.status !== 0) throw new Error("candidate Claude build failed");
    if (!existsSync(candidateBinary)) throw new Error("candidate plugin binary is missing");
    const version = run(client, ["--version"], repoRoot);
    if (version.status !== 0 || !claudeVersionMatches(version.stdout, expectedVersion)) throw new Error(`installed Claude Code version does not match ${expectedVersion}`);
    if (run("git", ["init", "-q"], disposableRepo).status !== 0) throw new Error("disposable Git initialization failed");
    const candidateEnv = { LOAF_DB: dbPath };
    if (run(candidateBinary, ["state", "init", "--json"], disposableRepo, candidateEnv).status !== 0) throw new Error("isolated Loaf state initialization failed");
    if (run(candidateBinary, ["journal", "log", `discover(smoke): ${marker}`], disposableRepo, candidateEnv).status !== 0) throw new Error("isolated journal marker write failed");
    const claudeArgs = [
      "--plugin-dir", "<repo>/plugins/loaf",
      "--strict-mcp-config", "--mcp-config", '{"mcpServers":{}}',
      "--no-session-persistence", "--setting-sources", "", "--tools", "",
      "--include-hook-events", "--output-format", "stream-json", "-p",
      "Reply with exactly the unique marker present in Loaf continuity context, and nothing else.",
    ];
    const claude = run(client, [
      "--plugin-dir", candidatePlugin, "--strict-mcp-config", "--mcp-config", '{"mcpServers":{}}',
      "--no-session-persistence", "--setting-sources", "", "--tools", "", "--include-hook-events",
      "--output-format", "stream-json", "-p", "Reply with exactly the unique marker present in Loaf continuity context, and nothing else.",
    ], disposableRepo, candidateEnv);
    const parsed = parseClaudeStreamOutput(claude.stdout, marker);
    smoke = {
      evidence_version: 2,
      timestamp,
      target: "claude-code",
      surface: "cli",
      version: expectedVersion,
      platform,
      installed_mode: "plugin-dir",
      context_mode: "startup",
      adapter: "claude-session-start-v1",
      mode: "explicit-plugin-dir",
      invocation: { command: "claude", args: claudeArgs, cwd: "<disposable-repo>" },
      setup: ["build candidate Go binary and Claude plugin", "create disposable Git repository", "initialize absolute disposable LOAF_DB", "write random marker to isolated journal"],
      candidate_plugin_path: candidatePluginPath,
      exit_code: claude.status,
      stderr_empty: claude.stderr.length === 0,
      model_visible_marker_observed: parsed.hookObservation.additional_context_marker,
      assistant_marker_match: parsed.assistantMarkerMatch,
      marker,
      hook_observation: parsed.hookObservation,
      candidate_artifacts: {
        hooks_path: "plugins/loaf/hooks/hooks.json",
        hooks_sha256: sha256(join(repoRoot, "plugins/loaf/hooks/hooks.json")),
        native_binary_path: relative(repoRoot, join(candidatePlugin, "bin", "native", platform, "loaf")),
        native_binary_sha256: sha256(join(candidatePlugin, "bin", "native", platform, "loaf")),
      },
    };
    if (claude.status !== 0 || claude.stderr.length !== 0 || !parsed.hookObservation.additional_context_marker || !parsed.assistantMarkerMatch) throw new Error("model-visible marker smoke did not pass");
  } catch (error) {
    failure = error;
  } finally {
    cleanupSucceeded = cleanup();
  }
  if (failure) throw failure;
  if (!cleanupSucceeded) throw new Error("disposable Claude Code smoke cleanup failed");
  publishReceiptIfSuccessful(receiptPath, smoke, true);
  process.stdout.write(`${JSON.stringify({ receipt: receiptPath, exit_code: smoke.exit_code, assistant_marker_match: smoke.assistant_marker_match, cleanup_succeeded: true }, null, 2)}\n`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    main();
  } catch (error) {
    process.stderr.write(`${error instanceof Error ? error.message : "Claude Code startup smoke failed"}\n`);
    process.exitCode = 1;
  }
}
