#!/usr/bin/env node

import { createHash, randomBytes } from "node:crypto";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, statSync, writeFileSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import { basename, delimiter, dirname, join, resolve, sep } from "node:path";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(scriptDir, "../..");
const researchDir = join(repoRoot, "docs/changes/20260710-journal-reliability-foundation/research");
const evidencePath = join(researchDir, "u8-opencode-1.18.4-isolated-smoke.json");
const expectedVersion = "1.18.4";
const platform = `${process.platform}-${process.arch}`;
const candidateHooksPath = "dist/opencode/plugins/hooks.ts";
const candidateNativePath = `bin/native/${platform}/loaf`;
const markerPattern = /^LOAF_OPENCODE_U8_[A-F0-9]{12}$/;
const prompt = "Reply with exactly the unique marker present in Loaf continuity context, and nothing else.";
const setupSteps = [
  "build candidate Go binary and OpenCode target",
  "create disposable Git repository with isolated XDG directories",
  "initialize isolated Loaf state and write a random journal marker",
  "load only the candidate plugin through OPENCODE_CONFIG_CONTENT",
  "observe candidate hook stdout through a mode-0700 wrapper and mode-0600 file",
];

const safeEnvironmentKeys = ["LANG", "LC_ALL", "PATH", "TERM", "TZ", "XDG_CACHE_HOME"];

export function opencodeVersionMatches(output, expected = expectedVersion) {
  return output.trim() === expected;
}

export function collectTextValues(value, texts = []) {
  if (Array.isArray(value)) {
    for (const item of value) collectTextValues(item, texts);
    return texts;
  }
  if (!value || typeof value !== "object") return texts;
  for (const [key, child] of Object.entries(value)) {
    if (key === "text" && typeof child === "string") texts.push(child);
    if (child && typeof child === "object") collectTextValues(child, texts);
  }
  return texts;
}

export function parseOpenCodeJSONL(raw, marker) {
  const lines = raw.split(/\r?\n/).filter((line) => line.trim() !== "");
  const texts = [];
  for (const [index, line] of lines.entries()) {
    let event;
    try {
      event = JSON.parse(line);
    } catch (error) {
      throw new SyntaxError(`OpenCode JSONL line ${index + 1} is malformed: ${error.message}`);
    }
    if (!event || typeof event !== "object" || Array.isArray(event)) {
      throw new SyntaxError(`OpenCode JSONL line ${index + 1} is not an object event`);
    }
    collectTextValues(event, texts);
  }
  return { assistantMarkerMatch: texts.some((text) => text.trim() === marker) };
}

export function modelVisibleProof(assistantMarkerMatch, hookObservationMarker) {
  return assistantMarkerMatch === true && hookObservationMarker === true;
}

export function sanitizeError(error, paths = []) {
  const text = error instanceof Error ? error.message : String(error);
  let sanitized = text;
  for (const [path, replacement] of paths) {
    if (path) sanitized = sanitized.replaceAll(path, replacement);
  }
  return sanitized.replaceAll(/\s+/g, " ").trim().slice(0, 400) || "smoke failed";
}

export function sanitizedStderr(stderr) {
  return stderr.trim() === "" ? "" : "unexpected stderr";
}

function run(command, args, cwd, env, timeout = 300000) {
  const result = spawnSync(command, args, {
    cwd,
    env,
    encoding: "utf8",
    timeout,
    maxBuffer: 32 * 1024 * 1024,
    stdio: ["pipe", "pipe", "pipe"],
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

export function resolveExecutable(command) {
  for (const directory of (process.env.PATH ?? "").split(delimiter)) {
    if (!directory) continue;
    const candidate = resolve(directory, command);
    try {
      const info = statSync(candidate);
      if (info.isFile() && (info.mode & 0o111) !== 0) {
        // Multiplexer shims (mise et al.) realpath to a differently named dispatcher binary that keys off argv[0], which realpath destroys; skip them so the smoke runs the real standalone binary under the isolated env.
        const resolved = realpathSync(candidate);
        if (basename(resolved) === command) return resolved;
      }
    } catch {
      // Continue through the explicit PATH allowlist.
    }
  }
  throw new Error(`installed ${command} executable is unavailable`);
}

export function buildEnvironment() {
  const env = {};
  for (const key of safeEnvironmentKeys) if (process.env[key] !== undefined) env[key] = process.env[key];
  // mise-shimmed toolchains resolve user env templates that may reference XDG dirs, so builds need them passed through; the disposable harness env stays fully isolated.
  for (const key of ["XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"]) {
    if (process.env[key] !== undefined) env[key] = process.env[key];
  }
  if (process.env.HOME !== undefined) env.HOME = process.env.HOME;
  if (process.env.USERPROFILE !== undefined) env.USERPROFILE = process.env.USERPROFILE;
  return env;
}

function candidateArtifacts() {
  const hooksPath = join(repoRoot, candidateHooksPath);
  const nativePath = join(repoRoot, candidateNativePath);
  if (!existsSync(hooksPath) || !existsSync(nativePath)) throw new Error("OpenCode candidate artifacts are missing");
  const pluginSource = readFileSync(hooksPath, "utf8");
  if (!pluginSource.includes("client.session.get({ path: { id: sessionID } })") || !pluginSource.includes("parentID")) {
    throw new Error("OpenCode candidate adapter is missing the fail-closed root-session lookup gate");
  }
  return {
    hooks_path: candidateHooksPath,
    hooks_sha256: sha256(hooksPath),
    native_binary_path: candidateNativePath,
    native_binary_sha256: sha256(nativePath),
    root_gate_present: true,
  };
}

function disposableEnvironment(tempRoot, dbPath, pluginURL) {
  const home = join(tempRoot, "home");
  const xdgData = join(tempRoot, "xdg-data");
  const xdgConfig = join(tempRoot, "xdg-config");
  const xdgState = join(tempRoot, "xdg-state");
  const xdgCache = join(tempRoot, "xdg-cache");
  const tmp = join(tempRoot, "tmp");
  for (const path of [home, xdgData, xdgConfig, xdgState, xdgCache, tmp]) mkdirSync(path, { recursive: true, mode: 0o700 });
  const env = {};
  for (const key of safeEnvironmentKeys) if (process.env[key] !== undefined) env[key] = process.env[key];
  return {
    ...env,
    HOME: home,
    USERPROFILE: home,
    XDG_DATA_HOME: xdgData,
    XDG_CONFIG_HOME: xdgConfig,
    XDG_STATE_HOME: xdgState,
    XDG_CACHE_HOME: xdgCache,
    TMPDIR: tmp,
    OPENCODE_DB: ":memory:",
    OPENCODE_DISABLE_PROJECT_CONFIG: "1",
    OPENCODE_DISABLE_AUTOUPDATE: "1",
    OPENCODE_DISABLE_MODELS_FETCH: "1",
    OPENCODE_CONFIG_CONTENT: JSON.stringify({ plugin: [pluginURL] }),
    LOAF_DB: dbPath,
  };
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

function baseReceipt(marker, invocation, artifacts) {
  return {
    evidence_version: 2,
    timestamp: new Date().toISOString(),
    target: "opencode",
    surface: "cli",
    version: expectedVersion,
    platform,
    installed_mode: "isolated-xdg",
    context_mode: "request",
    adapter: "opencode-plugin-v1",
    mode: "isolated-xdg",
    invocation,
    setup: setupSteps,
    exit_code: 1,
    stderr_empty: false,
    stderr: "",
    model_visible_marker_observed: false,
    assistant_marker_match: false,
    plugin_loaded: false,
    root_session_lookup_proven: false,
    no_auth_supplied: true,
    cleanup_succeeded: false,
    marker,
    candidate_artifacts: artifacts,
  };
}

function main() {
  mkdirSync(researchDir, { recursive: true });
  const marker = `LOAF_OPENCODE_U8_${randomBytes(6).toString("hex").toUpperCase()}`;
  if (!markerPattern.test(marker)) throw new Error("generated marker does not match the required format");
  const tempRoot = mkdtempSync(join(tmpdir(), "loaf-u8-opencode-smoke-"));
  chmodSync(tempRoot, 0o700);
  if (resolve(tempRoot).startsWith(`${repoRoot}${sep}`)) throw new Error("disposable OpenCode smoke root must be outside the repository");
  const disposableRepo = join(tempRoot, "repo");
  const stateDir = join(tempRoot, "state");
  const dbPath = join(stateDir, "loaf.sqlite");
  const candidateBinary = join(repoRoot, candidateNativePath);
  const candidatePlugin = join(repoRoot, candidateHooksPath);
  const installedOpenCode = resolveExecutable("opencode");
  const invocation = {
    command: "opencode",
    args: ["run", "--format", "json", "--model", "opencode/deepseek-v4-flash-free", "--dir", "<disposable-repo>", prompt],
    cwd: "<disposable-repo>",
  };
  let artifacts = { hooks_path: candidateHooksPath, hooks_sha256: "", native_binary_path: candidateNativePath, native_binary_sha256: "" };
  let smoke = baseReceipt(marker, invocation, artifacts);
  let cleanupSucceeded = false;
  const cleanup = () => {
    for (let attempt = 0; attempt < 5; attempt += 1) {
      try {
        rmSync(tempRoot, { recursive: true, force: true, maxRetries: 3, retryDelay: 50 });
      } catch {
        // Re-check after a bounded retry; child processes can briefly retain a path.
      }
      if (!existsSync(tempRoot)) {
        cleanupSucceeded = true;
        return;
      }
    }
    cleanupSucceeded = false;
  };
  for (const signal of ["SIGINT", "SIGTERM", "SIGHUP"]) {
    process.once(signal, () => {
      cleanup();
      process.exit(128 + ({ SIGINT: 2, SIGTERM: 15, SIGHUP: 1 }[signal] ?? 1));
    });
  }
  try {
    mkdirSync(disposableRepo, { recursive: true, mode: 0o700 });
    mkdirSync(stateDir, { recursive: true, mode: 0o700 });
    const buildEnv = buildEnvironment();
    const buildGo = run("npm", ["run", "build:go"], repoRoot, buildEnv);
    if (buildGo.status !== 0) throw new Error("candidate Go build failed");
    const buildOpenCode = run("bin/loaf", ["build", "--target", "opencode"], repoRoot, buildEnv);
    if (buildOpenCode.status !== 0) throw new Error("candidate OpenCode target build failed");
    artifacts = candidateArtifacts();
    smoke.candidate_artifacts = { hooks_path: artifacts.hooks_path, hooks_sha256: artifacts.hooks_sha256, native_binary_path: artifacts.native_binary_path, native_binary_sha256: artifacts.native_binary_sha256 };
    if (!existsSync(candidateBinary)) throw new Error("candidate native binary is missing");
    const pluginURL = pathToFileURL(candidatePlugin).href;
    const env = disposableEnvironment(tempRoot, dbPath, pluginURL);
    const version = run(installedOpenCode, ["--version"], repoRoot, env);
    if (version.status !== 0 || !opencodeVersionMatches(version.stdout)) throw new Error("installed OpenCode version is not exactly 1.18.4");
    if (run("git", ["init", "-q"], disposableRepo, env).status !== 0) throw new Error("disposable Git initialization failed");
    if (run(candidateBinary, ["state", "init", "--json"], disposableRepo, env).status !== 0) throw new Error("isolated Loaf state initialization failed");
    if (run(candidateBinary, ["journal", "log", `discover(smoke): ${marker}`], disposableRepo, env).status !== 0) throw new Error("isolated journal marker write failed");
    const observationPath = join(tempRoot, "opencode-hook-observation.txt");
    const wrapperPath = join(tempRoot, "loaf");
    writeHookObservationWrapper(wrapperPath, candidateBinary, observationPath);
    if ((statSync(wrapperPath).mode & 0o777) !== 0o700) throw new Error("OpenCode hook observation wrapper is not mode 0700");
    const smokeEnv = { ...env, PATH: `${tempRoot}:${env.PATH ?? ""}` };
    const opencodeArgs = ["run", "--format", "json", "--model", "opencode/deepseek-v4-flash-free", "--dir", disposableRepo, prompt];
    const opencode = run(installedOpenCode, opencodeArgs, disposableRepo, smokeEnv, 600000);
    if (!existsSync(observationPath)) throw new Error("OpenCode hook observation file was not written");
    if ((statSync(observationPath).mode & 0o777) !== 0o600) throw new Error("OpenCode hook observation is not mode 0600");
    const observedOutput = readFileSync(observationPath, "utf8");
    const hookObservationMarker = observedOutput.includes(marker);
    const parsed = parseOpenCodeJSONL(opencode.stdout, marker);
    const pluginLoaded = hookObservationMarker;
    smoke = {
      ...smoke,
      exit_code: opencode.status,
      stderr_empty: sanitizedStderr(opencode.stderr) === "",
      stderr: sanitizedStderr(opencode.stderr),
      model_visible_marker_observed: modelVisibleProof(parsed.assistantMarkerMatch, hookObservationMarker),
      assistant_marker_match: parsed.assistantMarkerMatch,
      plugin_loaded: pluginLoaded,
      root_session_lookup_proven: pluginLoaded && artifacts.root_gate_present === true,
      cleanup_succeeded: false,
    };
    if (opencode.status !== 0 || smoke.stderr !== "" || !hookObservationMarker || !parsed.assistantMarkerMatch || !smoke.root_session_lookup_proven) {
      throw new Error("model-visible OpenCode marker smoke did not pass");
    }
  } catch (error) {
    smoke.failure_reason = sanitizeError(error, [[repoRoot, "<repo>"], [tempRoot, "<disposable>"], [process.env.HOME ?? "", "<home>"]]);
    smoke.exit_code = smoke.exit_code || 1;
  } finally {
    cleanup();
    smoke.cleanup_succeeded = cleanupSucceeded;
  }
  writeFileSync(evidencePath, `${JSON.stringify(smoke, null, 2)}\n`);
  process.stdout.write(`${JSON.stringify({ evidence_path: "docs/changes/20260710-journal-reliability-foundation/research/u8-opencode-1.18.4-isolated-smoke.json", exit_code: smoke.exit_code, assistant_marker_match: smoke.assistant_marker_match, plugin_loaded: smoke.plugin_loaded, root_session_lookup_proven: smoke.root_session_lookup_proven, cleanup_succeeded: smoke.cleanup_succeeded }, null, 2)}\n`);
  if (smoke.exit_code !== 0 || !smoke.assistant_marker_match || !smoke.plugin_loaded || !smoke.root_session_lookup_proven || !smoke.cleanup_succeeded) process.exitCode = 1;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
