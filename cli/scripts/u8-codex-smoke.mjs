#!/usr/bin/env node

import { createHash, randomBytes } from "node:crypto";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join, relative, resolve, sep } from "node:path";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(scriptDir, "../..");
const researchDir = join(repoRoot, "docs/changes/20260710-journal-reliability-foundation/research");
const evidencePath = join(researchDir, "u8-codex-0.144.1-isolated-smoke.json");
const expectedVersion = "0.144.1";
const platform = `${process.platform}-${process.arch}`;
const candidateHooksPath = "dist/codex/.codex/hooks.json";
const candidateNativeRoot = join(repoRoot, "bin", "native");

export function codexVersionMatches(output, expectedVersion) {
  const match = output.match(/(?:^|\s)codex-cli\s+([0-9]+\.[0-9]+\.[0-9]+)(?:\s|$)/);
  return match?.[1] === expectedVersion;
}

export function shellQuote(value) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

export function parseCodexJSONL(raw, marker) {
  const events = raw.split(/\r?\n/).filter(Boolean).map((line) => JSON.parse(line));
  const serialized = events.map((event) => JSON.stringify(event));
  const nativeJSON = serialized.some((event) => event.includes("hookEventName") && event.includes(marker));
  const assistantTexts = events
    .filter((event) => event.type === "item.completed" && event.item?.type === "agent_message")
    .map((event) => event.item.text)
    .filter((text) => typeof text === "string");
  return {
    hookObservation: {
      event_name: "SessionStart:startup",
      native_json: nativeJSON,
      hook_event_name: nativeJSON ? "SessionStart" : "",
      additional_context_marker: nativeJSON,
    },
    assistantMarkerMatch: assistantTexts.some((text) => text.trim() === marker),
  };
}

export function parseCodexHookObservation(raw, marker) {
  const output = JSON.parse(raw);
  const topLevelKeys = Object.keys(output).sort();
  const hookSpecificOutput = output.hookSpecificOutput;
  const hookKeys = hookSpecificOutput && typeof hookSpecificOutput === "object" ? Object.keys(hookSpecificOutput).sort() : [];
  const nativeJSON = topLevelKeys.length === 1 && topLevelKeys[0] === "hookSpecificOutput" && hookKeys.length === 2 && hookKeys[0] === "additionalContext" && hookKeys[1] === "hookEventName";
  const hookEventName = nativeJSON && hookSpecificOutput.hookEventName === "SessionStart" ? hookSpecificOutput.hookEventName : "";
  const additionalContextMarker = nativeJSON && hookEventName === "SessionStart" && typeof hookSpecificOutput.additionalContext === "string" && hookSpecificOutput.additionalContext.includes(marker);
  return {
    eventName: "SessionStart:startup",
    nativeJSON,
    hookEventName,
    additionalContextMarker,
  };
}

function run(command, args, cwd, env = {}, timeout = 180000) {
  const result = spawnSync(command, args, {
    cwd,
    env: { ...process.env, LOAF_DB: undefined, ...env },
    encoding: "utf8",
    timeout,
    maxBuffer: 16 * 1024 * 1024,
    stdio: ["ignore", "pipe", "pipe"],
  });
  return { status: result.status ?? 1, stdout: result.stdout ?? "", stderr: result.stderr ?? "", error: result.error };
}

function sha256(path) {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}

function nativeBinaryPath() {
  const path = join(candidateNativeRoot, platform, "loaf");
  if (!existsSync(path)) throw new Error(`candidate native runtime ${platform} is missing`);
  return path;
}

function sanitizeError(error, tempRoot) {
  const text = error instanceof Error ? error.message : String(error);
  return text.replaceAll(repoRoot, "<repo>").replaceAll(tempRoot, "<disposable>").replaceAll(process.env.HOME ?? "", "<home>").replaceAll(/\s+/g, " ").trim().slice(0, 400);
}

function sanitizedStderr(stderr) {
  const trimmed = stderr.trim();
  if (trimmed === "") return "";
  if (trimmed === "Reading additional input from stdin...") return trimmed;
  return "unexpected stderr";
}

function writeHookObservationWrapper(wrapperPath, candidateBinary, observationPath) {
  const wrapper = `#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { chmodSync, readFileSync, writeFileSync } from "node:fs";

const result = spawnSync(${JSON.stringify(candidateBinary)}, process.argv.slice(2), {
  env: process.env,
  input: readFileSync(0),
  encoding: "buffer",
  stdio: ["pipe", "pipe", "pipe"],
});
const stdout = result.stdout ?? Buffer.alloc(0);
writeFileSync(${JSON.stringify(observationPath)}, stdout, { mode: 0o600 });
chmodSync(${JSON.stringify(observationPath)}, 0o600);
process.stdout.write(stdout);
if (result.stderr?.length) process.stderr.write(result.stderr);
if (result.error) process.stderr.write(String(result.error));
process.exitCode = result.status ?? 1;
`;
  writeFileSync(wrapperPath, wrapper, { mode: 0o700 });
  chmodSync(wrapperPath, 0o700);
}

function main() {
  mkdirSync(researchDir, { recursive: true });
  const marker = `LOAF_CODEX_U8_${randomBytes(6).toString("hex").toUpperCase()}`;
  const timestamp = new Date().toISOString();
  const tempRoot = mkdtempSync(join(tmpdir(), "loaf-u8-codex-smoke-"));
  const cleanup = () => rmSync(tempRoot, { recursive: true, force: true });
  for (const signal of ["SIGINT", "SIGTERM", "SIGHUP"]) {
    process.once(signal, () => {
      cleanup();
      process.exit(128 + ({ SIGINT: 2, SIGTERM: 15, SIGHUP: 1 }[signal] ?? 1));
    });
  }
  if (resolve(tempRoot).startsWith(`${repoRoot}${sep}`)) throw new Error("disposable Codex smoke root must be outside the repository");
  const disposableRepo = join(tempRoot, "repo");
  const codexHome = join(tempRoot, "codex-home");
  const stateDir = join(tempRoot, "state");
  const dbPath = join(stateDir, "loaf.sqlite");
  const candidateBinary = nativeBinaryPath();
  let smoke;
  try {
    mkdirSync(disposableRepo, { recursive: true, mode: 0o700 });
    mkdirSync(codexHome, { recursive: true, mode: 0o700 });
    mkdirSync(stateDir, { recursive: true, mode: 0o700 });
    chmodSync(tempRoot, 0o700);
    chmodSync(codexHome, 0o700);
    const buildGo = run("npm", ["run", "build:go"], repoRoot);
    if (buildGo.status !== 0) throw new Error("candidate Go build failed");
    const buildCodex = run("bin/loaf", ["build", "--target", "codex"], repoRoot);
    if (buildCodex.status !== 0) throw new Error("candidate Codex build failed");
    if (!existsSync(candidateBinary)) throw new Error("candidate native binary is missing");
    const version = run("codex", ["--version"], repoRoot);
    if (version.status !== 0 || !codexVersionMatches(version.stdout, expectedVersion)) throw new Error("installed Codex version is not 0.144.1");
    if (run("git", ["init", "-q"], disposableRepo).status !== 0) throw new Error("disposable Git initialization failed");
    const authPath = join(process.env.CODEX_HOME ?? join(process.env.HOME ?? "", ".codex"), "auth.json");
    if (!existsSync(authPath)) throw new Error("installed Codex auth.json is unavailable");
    writeFileSync(join(codexHome, "auth.json"), readFileSync(authPath), { mode: 0o600 });
    writeFileSync(join(codexHome, "config.toml"), "[features]\nhooks = true\n", { mode: 0o600 });
    const sourceHooks = JSON.parse(readFileSync(join(repoRoot, candidateHooksPath), "utf8"));
    const wrapperPath = join(tempRoot, "codex-hook-observer.mjs");
    const observationPath = join(tempRoot, "codex-hook-observation.json");
    writeHookObservationWrapper(wrapperPath, candidateBinary, observationPath);
    const hookCommand = shellQuote(wrapperPath) + " journal context --from-hook --codex-hook";
    sourceHooks.hooks.SessionStart = sourceHooks.hooks.SessionStart.map((group) => ({
      ...group,
      hooks: group.hooks.map((hook) => ({ ...hook, command: hook.command.replace("{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook", hookCommand) })),
    }));
    writeFileSync(join(codexHome, "hooks.json"), `${JSON.stringify(sourceHooks, null, 2)}\n`, { mode: 0o600 });
    const candidateEnv = { CODEX_HOME: codexHome, LOAF_DB: dbPath };
    if (run(candidateBinary, ["state", "init", "--json"], disposableRepo, candidateEnv).status !== 0) throw new Error("isolated Loaf state initialization failed");
    if (run(candidateBinary, ["journal", "log", `discover(smoke): ${marker}`], disposableRepo, candidateEnv).status !== 0) throw new Error("isolated journal marker write failed");
    const codexArgs = ["exec", "--ephemeral", "--ignore-rules", "--dangerously-bypass-hook-trust", "--sandbox", "read-only", "--json", "-C", "<disposable-repo>", "Return exactly the unique marker supplied by SessionStart context, and nothing else."];
    const codex = run("codex", [...codexArgs.slice(0, -2), disposableRepo, codexArgs.at(-1)], disposableRepo, candidateEnv);
    const parsed = parseCodexJSONL(codex.stdout, marker);
    if (!existsSync(observationPath)) throw new Error("Codex hook observation file was not written");
    if ((statSync(wrapperPath).mode & 0o777) !== 0o700) throw new Error("Codex hook observation wrapper is not mode 0700");
    if ((statSync(observationPath).mode & 0o777) !== 0o600) throw new Error("Codex hook observation is not mode 0600");
    const observedHook = parseCodexHookObservation(readFileSync(observationPath, "utf8"), marker);
    smoke = {
      evidence_version: 2,
      timestamp,
      target: "codex",
      surface: "cli",
      version: expectedVersion,
      platform,
      installed_mode: "isolated-codex-home",
      context_mode: "startup",
      adapter: "codex-session-start-v1",
      mode: "isolated-codex-home",
      invocation: { command: "codex", args: codexArgs, cwd: "<disposable-repo>" },
      setup: ["build candidate Go binary and Codex target", "create disposable Git repository", "create isolated CODEX_HOME with hooks enabled", "copy installed auth.json into isolated CODEX_HOME with mode 0600", "initialize absolute disposable LOAF_DB", "write random marker to isolated journal", "observe hook stdout through a mode-0700 disposable wrapper and mode-0600 file"],
      exit_code: codex.status,
      stderr_empty: codex.stderr.length === 0,
      stderr: sanitizedStderr(codex.stderr),
      model_visible_marker_observed: observedHook.additionalContextMarker && parsed.assistantMarkerMatch,
      assistant_marker_match: parsed.assistantMarkerMatch,
      marker,
      hook_observation: {
        event_name: observedHook.eventName,
        native_json: observedHook.nativeJSON,
        hook_event_name: observedHook.hookEventName,
        additional_context_marker: observedHook.additionalContextMarker,
      },
      candidate_artifacts: {
        hooks_path: candidateHooksPath,
        hooks_sha256: sha256(join(repoRoot, candidateHooksPath)),
        native_binary_path: relative(repoRoot, candidateBinary),
        native_binary_sha256: sha256(candidateBinary),
      },
    };
    if (codex.status !== 0 || !observedHook.additionalContextMarker || !parsed.assistantMarkerMatch || smoke.stderr === "unexpected stderr") throw new Error("model-visible Codex marker smoke did not pass");
  } catch (error) {
    smoke ??= {
      evidence_version: 2, timestamp, target: "codex", surface: "cli", version: expectedVersion, platform, installed_mode: "isolated-codex-home", context_mode: "startup", adapter: "codex-session-start-v1", mode: "isolated-codex-home",
      invocation: { command: "codex", args: [], cwd: "<disposable-repo>" }, setup: [], exit_code: 1, stderr_empty: false, stderr: "", model_visible_marker_observed: false, assistant_marker_match: false, marker,
      hook_observation: { event_name: "", native_json: false, hook_event_name: "", additional_context_marker: false }, candidate_artifacts: { hooks_path: candidateHooksPath, hooks_sha256: "", native_binary_path: "", native_binary_sha256: "" },
    };
    smoke.failure_reason = sanitizeError(error, tempRoot);
    smoke.exit_code = smoke.exit_code ?? 1;
  } finally {
    cleanup();
  }
  writeFileSync(evidencePath, `${JSON.stringify(smoke, null, 2)}\n`);
  process.stdout.write(`${JSON.stringify({ evidence_path: "docs/changes/20260710-journal-reliability-foundation/research/u8-codex-0.144.1-isolated-smoke.json", exit_code: smoke.exit_code, assistant_marker_match: smoke.assistant_marker_match }, null, 2)}\n`);
  if (smoke.exit_code !== 0 || !smoke.assistant_marker_match) process.exitCode = 1;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
