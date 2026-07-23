#!/usr/bin/env node

import { createHash, randomBytes } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join, relative, resolve } from "node:path";
import { spawnSync } from "node:child_process";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(scriptDir, "../..");
const researchDir = join(repoRoot, "docs/changes/20260710-journal-reliability-foundation/research");
const evidencePath = join(researchDir, "u8-claude-code-2.1.218-candidate-smoke.json");
const expectedVersion = "2.1.218";
const platform = `${process.platform}-${process.arch}`;
const candidatePluginPath = "plugins/loaf";

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

function main() {
  mkdirSync(researchDir, { recursive: true });
  const marker = `LOAF_U8_SMOKE_${randomBytes(6).toString("hex").toUpperCase()}`;
  const timestamp = new Date().toISOString();
  const tempRoot = mkdtempSync(join(repoRoot, ".u8-claude-smoke-"));
  const cleanup = () => rmSync(tempRoot, { recursive: true, force: true });
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
  try {
    const buildGo = run("npm", ["run", "build:go"], repoRoot);
    if (buildGo.status !== 0) throw new Error("candidate Go build failed");
    const buildClaude = run("bin/loaf", ["build", "--target", "claude-code"], repoRoot);
    if (buildClaude.status !== 0) throw new Error("candidate Claude build failed");
    if (!existsSync(candidateBinary)) throw new Error("candidate plugin binary is missing");
    const version = run("claude", ["--version"], repoRoot);
    if (version.status !== 0 || !claudeVersionMatches(version.stdout, expectedVersion)) throw new Error("installed Claude version is not 2.1.218");
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
    const claude = run("claude", [
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
    smoke ??= {
      evidence_version: 2, timestamp, target: "claude-code", surface: "cli", version: expectedVersion, platform,
      installed_mode: "plugin-dir", context_mode: "startup", adapter: "claude-session-start-v1", mode: "explicit-plugin-dir",
      invocation: { command: "claude", args: [], cwd: "<disposable-repo>" }, setup: [], candidate_plugin_path: candidatePluginPath,
      exit_code: 1, stderr_empty: false, model_visible_marker_observed: false, assistant_marker_match: false, marker,
      hook_observation: { event_name: "", native_json: false, hook_event_name: "", additional_context_marker: false },
      candidate_artifacts: { hooks_path: "plugins/loaf/hooks/hooks.json", hooks_sha256: "", native_binary_path: "", native_binary_sha256: "" },
      failure_reason: error instanceof Error ? error.message : "smoke failed",
    };
    smoke.failure_reason = error instanceof Error ? error.message : "smoke failed";
    smoke.exit_code = smoke.exit_code ?? 1;
  } finally {
    cleanup();
  }
  writeFileSync(evidencePath, `${JSON.stringify(smoke, null, 2)}\n`);
  process.stdout.write(`${JSON.stringify({ evidence_path: "docs/changes/20260710-journal-reliability-foundation/research/u8-claude-code-2.1.218-candidate-smoke.json", exit_code: smoke.exit_code, assistant_marker_match: smoke.assistant_marker_match }, null, 2)}\n`);
  if (smoke.exit_code !== 0 || !smoke.assistant_marker_match) process.exitCode = 1;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
