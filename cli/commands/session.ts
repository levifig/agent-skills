/**
 * loaf session command
 *
 * Session journal management for tracking work state and continuity.
 */

import { Command } from "commander";
import { execSync, spawn as nodeSpawn } from "child_process";
import { createHash } from "crypto";
import {
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  writeFileSync,
  renameSync,
  unlinkSync,
  openSync,
  closeSync,
  statSync,
  copyFileSync,
} from "fs";
import { join, dirname, basename } from "path";
import { fileURLToPath } from "url";
import matter from "gray-matter";

import { loadKnowledgeFiles } from "../lib/kb/loader.js";
import { findGitRoot, loadKbConfig } from "../lib/kb/resolve.js";
import { checkAllStaleness } from "../lib/kb/staleness.js";
import { findAgentsDir } from "../lib/tasks/resolve.js";
import { isLinearIntegrationDisabled } from "../lib/detect/mcp.js";
import { extractSummary } from "../lib/journal/extractor.js";
import { readStdin } from "./check.js";

// ANSI color helpers
const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const red = (s: string) => `\x1b[31m${s}\x1b[0m`;
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;
const cyan = (s: string) => `\x1b[36m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

interface SessionFrontmatter {
  branch: string;
  status: "active" | "stopped" | "done" | "blocked" | "archived";
  created: string;
  last_updated?: string;
  last_entry?: string;
  archived_at?: string;
  archived_by?: string;
  linear_issue?: string;
  linear_url?: string;
  task?: string;
  spec?: string;
  title?: string;
  claude_session_id?: string;
  enriched_at?: string;
}

/** Hook JSON input from Claude Code stdin */
interface HookInput {
  session_id?: string;
  agent_id?: string;
  agent_type?: string;
  transcript_path?: string;
  cwd?: string;
  permission_mode?: string;
  source?: string;  // "clear", "resume", etc.
  reason?: string;  // "clear", "prompt_input_exit", etc.
}

/** Parse hook JSON from stdin (returns empty object if stdin is TTY or invalid) */
async function parseHookInput(): Promise<HookInput> {
  if (process.stdin.isTTY) return {};

  let data = "";
  return new Promise<HookInput>((resolve) => {
    const timer = setTimeout(() => {
      finish({});
    }, 100);

    const onData = (chunk: string) => { data += chunk; };
    const onEnd = () => {
      clearTimeout(timer);
      try {
        finish(data.trim() ? JSON.parse(data) as HookInput : {});
      } catch {
        finish({});
      }
    };
    const onError = () => { finish({}); };

    let finished = false;
    function finish(result: HookInput) {
      if (finished) return;
      finished = true;
      clearTimeout(timer);
      process.stdin.removeListener("data", onData);
      process.stdin.removeListener("end", onEnd);
      process.stdin.removeListener("error", onError);
      if (typeof process.stdin.unref === "function") process.stdin.unref();
      resolve(result);
    }

    process.stdin.setEncoding("utf8");
    process.stdin.on("data", onData);
    process.stdin.on("end", onEnd);
    process.stdin.on("error", onError);
  });
}

type EntryType =
  | "start"
  | "resume"
  | "pause"
  | "clear"
  | "progress"
  | "commit"
  | "pr"
  | "merge"
  | "decision"
  | "discover"
  | "finding"
  | "block"
  | "unblock"
  | "spark"
  | "todo"
  | "assume"
  | "branch"
  | "task"
  | "linear"
  | "hypothesis"
  | "try"
  | "reject"
  | "compact"
  | "skill"
  | "wrap";

interface JournalEntry {
  type: EntryType;
  scope?: string;
  description: string;
}

interface SpecFrontmatterWithBranch {
  id: string;
  title: string;
  branch?: string;
  status?: string;
  session?: string;
  [key: string]: unknown;
}

// ─────────────────────────────────────────────────────────────────────────────
// .loaf-state — Local Machine State (gitignored)
// ─────────────────────────────────────────────────────────────────────────────

interface LoafState {
  last_housekeeping?: string;  // ISO timestamp
  housekeeping_pending?: boolean;
}

/** 24 hours in milliseconds */
const HOUSEKEEPING_INTERVAL_MS = 24 * 60 * 60 * 1000;

function getLoafStatePath(agentsDir: string): string {
  return join(agentsDir, ".loaf-state");
}

function readLoafState(agentsDir: string): LoafState {
  const statePath = getLoafStatePath(agentsDir);
  if (!existsSync(statePath)) return {};
  try {
    return JSON.parse(readFileSync(statePath, "utf-8")) as LoafState;
  } catch {
    return {};
  }
}

function writeLoafState(agentsDir: string, state: LoafState): void {
  writeFileSync(getLoafStatePath(agentsDir), JSON.stringify(state, null, 2) + "\n", "utf-8");
}

/** Check if housekeeping is due (>24h since last run) and set pending flag */
function checkHousekeepingDue(agentsDir: string): void {
  const state = readLoafState(agentsDir);
  const lastRun = state.last_housekeeping ? new Date(state.last_housekeeping).getTime() : 0;
  const age = Date.now() - lastRun;

  if (age > HOUSEKEEPING_INTERVAL_MS) {
    state.housekeeping_pending = true;
    writeLoafState(agentsDir, state);
  }
}

/** Mark housekeeping as completed */
function markHousekeepingDone(agentsDir: string): void {
  const state = readLoafState(agentsDir);
  state.last_housekeeping = new Date().toISOString();
  state.housekeeping_pending = false;
  writeLoafState(agentsDir, state);
}

/** Check if housekeeping is pending and should run */
function isHousekeepingPending(agentsDir: string): boolean {
  const state = readLoafState(agentsDir);
  return !!state.housekeeping_pending;
}

// ─────────────────────────────────────────────────────────────────────────────
// Session Lifecycle Helpers
// ─────────────────────────────────────────────────────────────────────────────

function getProjectRoot(agentsDir: string): string {
  return dirname(agentsDir);
}

function getInstalledTemplateCandidates(): string[] {
  const homeDir = process.env.HOME || process.env.USERPROFILE || "";
  const configHome = process.env.XDG_CONFIG_HOME || join(homeDir, ".config");

  return [
    join(configHome, "opencode", "templates", "soul.md"),
    join(homeDir, ".cursor", "templates", "soul.md"),
    join(process.env.CODEX_HOME || join(homeDir, ".codex"), "templates", "soul.md"),
    join(homeDir, ".amp", "templates", "soul.md"),
    process.env.CLAUDE_PLUGIN_ROOT ? join(process.env.CLAUDE_PLUGIN_ROOT, "templates", "soul.md") : "",
  ].filter(Boolean);
}

function resolveSoulTemplate(agentsDir: string): string | null {
  const projectRoot = getProjectRoot(agentsDir);
  const moduleDir = dirname(fileURLToPath(import.meta.url));

  for (const candidate of [
    join(agentsDir, "templates", "soul.md"),
    join(projectRoot, "templates", "soul.md"),
    join(projectRoot, "content", "templates", "soul.md"),
    join(moduleDir, "..", "templates", "soul.md"),
    join(moduleDir, "..", "..", "templates", "soul.md"),
    join(moduleDir, "..", "content", "templates", "soul.md"),
    join(moduleDir, "..", "..", "content", "templates", "soul.md"),
    join(projectRoot, "SOUL.md"),
    ...getInstalledTemplateCandidates(),
  ]) {
    if (existsSync(candidate)) {
      return candidate;
    }
  }

  return null;
}

function validateSoulMd(agentsDir: string): { exists: boolean; restored: boolean } {
  const soulPath = join(agentsDir, "SOUL.md");

  if (existsSync(soulPath)) {
    return { exists: true, restored: false };
  }

  const templatePath = resolveSoulTemplate(agentsDir);
  if (!templatePath) {
    return { exists: false, restored: false };
  }

  try {
    copyFileSync(templatePath, soulPath);
    return { exists: true, restored: true };
  } catch {
    return { exists: false, restored: false };
  }
}

function countStaleKnowledge(): number {
  const gitRoot = findGitRoot();
  const config = loadKbConfig(gitRoot);
  const local = config.local.filter((dir) => existsSync(join(gitRoot, dir)));

  if (local.length === 0) {
    return 0;
  }

  const effectiveConfig = { ...config, local };
  const files = loadKnowledgeFiles(gitRoot, effectiveConfig);
  return checkAllStaleness(gitRoot, files, effectiveConfig).filter((result) => result.isStale).length;
}

function getKnowledgeNudgeFilePath(projectRoot: string, branch: string): string {
  const hash = createHash("md5").update(`${projectRoot}:${branch}`).digest("hex").slice(0, 8);
  return join("/tmp", `loaf-kb-nudged-${hash}`);
}

function consumeKnowledgeNudges(projectRoot: string, branch: string): string[] {
  const nudgeFile = getKnowledgeNudgeFilePath(projectRoot, branch);
  if (!existsSync(nudgeFile)) {
    return [];
  }

  const files = readFileSync(nudgeFile, "utf-8")
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);

  unlinkSync(nudgeFile);
  return [...new Set(files)];
}

// ─────────────────────────────────────────────────────────────────────────────
// File Locking for Atomic Operations
// ─────────────────────────────────────────────────────────────────────────────

/** Staleness threshold in milliseconds (30 seconds) */
const LOCK_STALENESS_THRESHOLD = 30000;

/** Lock file content format: JSON with PID and timestamp */
interface LockFileContent {
  pid: number;
  timestamp: number;
}

/** Check if a process is still running (signal 0 = existence check) */
function isProcessAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

/** Read lock file content safely */
function readLockContent(lockPath: string): LockFileContent | null {
  try {
    const raw = readFileSync(lockPath, 'utf-8');
    return JSON.parse(raw) as LockFileContent;
  } catch {
    return null;
  }
}

/** Check if lock file is stale (older than threshold OR owning process is dead) */
function isLockStale(lockPath: string): boolean {
  try {
    const stats = statSync(lockPath);
    const age = Date.now() - stats.mtimeMs;
    if (age > LOCK_STALENESS_THRESHOLD) return true;

    // Young lock — check if the owning process is still alive
    const content = readLockContent(lockPath);
    if (content?.pid && !isProcessAlive(content.pid)) return true;

    return false;
  } catch {
    // If we can't read the lock file, assume it's stale
    return true;
  }
}

/** Force-release a stale lock */
function forceReleaseLock(lockPath: string): void {
  try {
    unlinkSync(lockPath);
  } catch {
    // Lock may already be gone (removed by another process) — not an error
  }
}

/** Simple file lock using lock file with retry logic and staleness detection */
async function acquireLock(lockPath: string, maxRetries = 50, retryDelay = 100): Promise<void> {
  for (let i = 0; i < maxRetries; i++) {
    try {
      // Check if existing lock is stale
      if (existsSync(lockPath) && isLockStale(lockPath)) {
        forceReleaseLock(lockPath);
      }

      // Try to create lock file atomically with O_EXCL
      const fd = openSync(lockPath, 'wx');
      // Write PID and timestamp to lock file
      const lockContent: LockFileContent = {
        pid: process.pid,
        timestamp: Date.now()
      };
      writeFileSync(fd, JSON.stringify(lockContent), 'utf-8');
      closeSync(fd);
      return; // Lock acquired
    } catch (err: unknown) {
      const code = (err as NodeJS.ErrnoException)?.code;

      if (code === 'EEXIST') {
        // Lock file exists and wasn't stale (or couldn't be removed) — retry
        await new Promise(resolve => setTimeout(resolve, retryDelay));
        continue;
      }

      // Non-contention errors — fail immediately, retrying won't help
      if (code === 'ENOENT') {
        throw new Error(
          `Lock directory does not exist: ${dirname(lockPath)}\n` +
          `  Ensure .agents/sessions/ exists in your working directory.`
        );
      }
      if (code === 'EACCES' || code === 'EPERM') {
        throw new Error(
          `Permission denied creating lock file: ${lockPath}\n` +
          `  Check directory permissions for: ${dirname(lockPath)}`
        );
      }
      throw err;
    }
  }

  // Exhausted retries — provide diagnostic info to help the user self-service
  let diagnostic = '';
  try {
    const content = readLockContent(lockPath);
    const stats = statSync(lockPath);
    const ageSeconds = Math.round((Date.now() - stats.mtimeMs) / 1000);
    const alive = content?.pid ? isProcessAlive(content.pid) : false;
    diagnostic =
      `\n  Lock age: ${ageSeconds}s, PID: ${content?.pid ?? 'unknown'}, process alive: ${alive}` +
      `\n  Remove manually: rm "${lockPath}"`;
  } catch { /* ignore diagnostic failures */ }

  throw new Error(
    `Could not acquire lock after ${maxRetries} retries: ${lockPath}${diagnostic}`
  );
}

function releaseLock(lockPath: string): void {
  try {
    unlinkSync(lockPath);
  } catch {
    // Ignore errors on unlock
  }
}

/** Atomic write using temp file + rename */
function writeFileAtomic(filePath: string, content: string): void {
  const tempPath = `${filePath}.tmp.${Date.now()}.${Math.random().toString(36).slice(2, 11)}`;
  try {
    writeFileSync(tempPath, content, 'utf-8');
    renameSync(tempPath, filePath);
  } catch (err) {
    // Cleanup temp file on failure
    try { unlinkSync(tempPath); } catch { /* ignore */ }
    throw err;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

/** Get current git branch, or commit SHA if detached HEAD */
function getCurrentBranch(): string {
  try {
    const branch = execSync("git branch --show-current", {
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
    }).trim();
    
    // Empty branch indicates detached HEAD
    if (!branch) {
      // Get short commit SHA for detached HEAD
      const commitSha = execSync("git rev-parse --short HEAD", {
        encoding: "utf-8",
        stdio: ["pipe", "pipe", "pipe"],
      }).trim();
      return `detached-${commitSha}`;
    }
    
    return branch;
  } catch {
    return "unknown";
  }
}

/** Get recent commits with structured data */
interface Commit {
  hash: string;
  message: string;
  timestamp: number;
}

function getRecentCommits(count = 5): Commit[] {
  try {
    const output = execSync(`git log --pretty=format:"%H %ct %s" -${count}`, {
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
    }).trim();

    if (!output) return [];

    return output.split("\n").map((line) => {
      const firstSpace = line.indexOf(" ");
      const secondSpace = line.indexOf(" ", firstSpace + 1);
      return {
        hash: line.substring(0, 7),
        timestamp: parseInt(line.substring(firstSpace + 1, secondSpace), 10) * 1000,
        message: line.substring(secondSpace + 1),
      };
    });
  } catch {
    return [];
  }
}

/** Count uncommitted (modified/staged/untracked) files */
function getUncommittedCount(): number {
  try {
    const output = execSync("git status --porcelain", {
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
    }).trim();
    if (!output) return 0;
    return output.split("\n").length;
  } catch {
    return 0;
  }
}

/** Get last commit SHA */
function getLastCommitSha(): string {
  try {
    return execSync("git rev-parse --short HEAD", {
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
    }).trim();
  } catch {
    return "unknown";
  }
}

/** Find spec ID linked to a branch */
function findSpecForBranch(
  agentsDir: string,
  branch: string
): { id: string; title: string; sessionFile?: string } | null {
  const specsDir = join(agentsDir, "specs");
  if (!existsSync(specsDir)) return null;

  const files = readdirSync(specsDir).filter((f) => f.endsWith(".md"));

  // Only exact branch match - never assume stale specs are "renamed"
  for (const file of files) {
    try {
      const content = readFileSync(join(specsDir, file), "utf-8");
      const parsed = matter(content);
      const fm = parsed.data as SpecFrontmatterWithBranch;

      if (fm.branch === branch) {
        return { 
          id: fm.id || file.replace(/\.md$/, ""), 
          title: fm.title || "Untitled",
          sessionFile: fm.session
        };
      }
    } catch {
      continue;
    }
  }

  return null;
}

/** Update spec file with session filename (for branch rename recovery) */
function updateSpecSessionField(agentsDir: string, specId: string, sessionFileName: string): void {
  const specsDir = join(agentsDir, "specs");
  if (!existsSync(specsDir)) return;

  const files = readdirSync(specsDir).filter((f) => f.endsWith(".md"));

  for (const file of files) {
    if (file.includes(specId)) {
      const filePath = join(specsDir, file);
      try {
        const content = readFileSync(filePath, "utf-8");
        const parsed = matter(content);
        
        // Only update if session field is missing or different
        if (parsed.data.session !== sessionFileName) {
          parsed.data.session = sessionFileName;
          const newContent = matter.stringify(parsed.content, parsed.data as Record<string, unknown>);
          writeFileSync(filePath, newContent, "utf-8");
        }
      } catch {
        // Silently fail - spec update is best-effort
      }
      break;
    }
  }
}

/** Get session file path (timestamped filename per SPEC-020, with collision avoidance) */
function getSessionFilePath(agentsDir: string): string {
  const sessionsDir = join(agentsDir, "sessions");
  const base = getTimestampForFilename();
  let path = join(sessionsDir, `${base}-session.md`);
  let counter = 1;
  while (existsSync(path)) {
    path = join(sessionsDir, `${base}-${counter}-session.md`);
    counter++;
  }
  return path;
}

/** Get timestamp in filename format: YYYYMMDD-HHMMSS */
function getTimestampForFilename(): string {
  const now = new Date();
  const year = now.getFullYear();
  const month = String(now.getMonth() + 1).padStart(2, '0');
  const day = String(now.getDate()).padStart(2, '0');
  const hour = String(now.getHours()).padStart(2, '0');
  const minute = String(now.getMinutes()).padStart(2, '0');
  const second = String(now.getSeconds()).padStart(2, '0');
  return `${year}${month}${day}-${hour}${minute}${second}`;
}

/** Extract journal entries from session content (lines matching [YYYY-MM-DD HH:MM] pattern) */
function extractJournalLines(content: string): string[] {
  return content.split("\n").filter(line => /^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}\] /.test(line));
}

/** Merge journal entries from a secondary session into a primary session file */
function consolidateSession(primaryPath: string, secondary: { filePath: string; data: SessionFrontmatter; content: string }, agentsDir: string): void {
  const primary = readSessionFile(primaryPath);
  if (!primary) return;

  // Extract journal entries from secondary that aren't already in primary
  const primaryEntries = new Set(extractJournalLines(primary.content));
  const secondaryEntries = extractJournalLines(secondary.content);
  const newEntries = secondaryEntries.filter(e => !primaryEntries.has(e));

  if (newEntries.length > 0) {
    // Append new entries before the last line of the journal
    const timestamp = getDateTimeString();
    const mergeMarker = `[${timestamp}] session(merge): consolidated from ${basename(secondary.filePath)}`;
    const entriesToAdd = [mergeMarker, ...newEntries];

    // Use appendEntry synchronously via direct file manipulation
    const trimmedContent = primary.content.trimEnd();
    const newContent = matter.stringify(
      trimmedContent + "\n" + entriesToAdd.join("\n") + "\n",
      { ...primary.data, last_updated: getTimestamp(), last_entry: getTimestamp() } as unknown as Record<string, unknown>
    );
    writeFileAtomic(primaryPath, newContent);
  }

  // Delete the duplicate — its content is now in the primary
  try { unlinkSync(secondary.filePath); } catch { /* already gone */ }
}

/** Find session by claude_session_id — scans active AND archived sessions, consolidates splits */
function findSessionByClaudeId(agentsDir: string, claudeSessionId: string, currentBranch?: string): { filePath: string; data: SessionFrontmatter; content: string } | null {
  const sessionsDir = join(agentsDir, "sessions");
  if (!existsSync(sessionsDir)) return null;

  type SessionMatch = { filePath: string; data: SessionFrontmatter; content: string; isArchived: boolean };
  const matches: SessionMatch[] = [];

  // Scan active sessions
  const activeFiles = readdirSync(sessionsDir).filter(f => f.endsWith(".md") && !f.startsWith("archive"));
  for (const file of activeFiles) {
    const filePath = join(sessionsDir, file);
    const session = readSessionFile(filePath);
    if (session && session.data.claude_session_id === claudeSessionId) {
      matches.push({ filePath, data: session.data, content: session.content, isArchived: false });
    }
  }

  // Scan archive
  const archiveDir = join(sessionsDir, "archive");
  if (existsSync(archiveDir)) {
    const archiveFiles = readdirSync(archiveDir).filter(f => f.endsWith(".md"));
    for (const file of archiveFiles) {
      const filePath = join(archiveDir, file);
      const session = readSessionFile(filePath);
      if (session && session.data.claude_session_id === claudeSessionId) {
        matches.push({ filePath, data: session.data, content: session.content, isArchived: true });
      }
    }
  }

  if (matches.length === 0) return null;

  // Pick primary: prefer current branch match, then most recent
  matches.sort((a, b) => {
    // Current branch wins
    if (currentBranch) {
      const aMatch = a.data.branch === currentBranch ? 0 : 1;
      const bMatch = b.data.branch === currentBranch ? 0 : 1;
      if (aMatch !== bMatch) return aMatch - bMatch;
    }
    // Active over archived
    if (a.isArchived !== b.isArchived) return a.isArchived ? 1 : -1;
    // Most recent wins
    const aTime = a.data.last_updated || a.data.last_entry || "0";
    const bTime = b.data.last_updated || b.data.last_entry || "0";
    return bTime.localeCompare(aTime);
  });

  const primary = matches[0];

  // Restore from archive if needed
  if (primary.isArchived) {
    const restoredPath = join(sessionsDir, basename(primary.filePath));
    try {
      renameSync(primary.filePath, restoredPath);
      primary.filePath = restoredPath;
      primary.isArchived = false;
    } catch {
      // Use in place if move fails
    }
  }

  // Consolidate duplicates into primary
  for (const secondary of matches.slice(1)) {
    consolidateSession(primary.filePath, secondary, agentsDir);
  }

  // Re-read primary after consolidation
  if (matches.length > 1) {
    const refreshed = readSessionFile(primary.filePath);
    if (refreshed) {
      return { filePath: primary.filePath, data: refreshed.data, content: refreshed.content };
    }
  }

  return { filePath: primary.filePath, data: primary.data, content: primary.content };
}

/** Find active session for a branch by scanning files */
function findActiveSessionForBranch(agentsDir: string, branch: string): { filePath: string; data: SessionFrontmatter; content: string } | null {
  const sessionsDir = join(agentsDir, "sessions");
  if (!existsSync(sessionsDir)) return null;
  
  const files = readdirSync(sessionsDir).filter(f => f.endsWith(".md") && !f.startsWith("archive"));
  
  // Collect all candidate sessions for this branch
  const candidates: Array<{ filePath: string; data: SessionFrontmatter; content: string }> = [];
  
  for (const file of files) {
    const filePath = join(sessionsDir, file);
    const session = readSessionFile(filePath);
    if (session && session.data.branch === branch && session.data.status !== "archived") {
      candidates.push({ filePath, data: session.data, content: session.content });
    }
  }
  
  // If no session found by branch name, check for renamed branch via spec linkage
  // This handles: git branch -m old-name new-name (explicit rename only)
  // We verify the rename by checking git reflog for the explicit "Branch: renamed" pattern
  if (candidates.length === 0) {
    // Get current branch's creation info from reflog
    let parentBranch: string | null = null;
    try {
      // Check reflog for explicit rename pattern from "git branch -m old new"
      const reflogOutput = execSync(`git reflog show --format='%H %gs' ${branch} 2>/dev/null | head -10`, { encoding: "utf-8" });
      
      // ONLY match explicit rename: "Branch: renamed refs/heads/old-branch to refs/heads/new-branch"
      // Do NOT match "branch: Created from ..." (that's normal branch creation)
      // Do NOT match "checkout: moving from ..." (that's branch switching)
      const renamedMatch = reflogOutput.match(/Branch: renamed refs\/heads\/([^\s]+) to/);
      
      if (renamedMatch) {
        parentBranch = renamedMatch[1];
      }
    } catch {
      // Git command failed, skip rename detection
    }
    
    // If we found a parent branch, look for sessions that were on that branch
    if (parentBranch) {
      const specsDir = join(agentsDir, "specs");
      if (existsSync(specsDir)) {
        const specFiles = readdirSync(specsDir).filter(f => f.endsWith(".md"));
        
        for (const specFile of specFiles) {
          try {
            const specPath = join(specsDir, specFile);
            const specContent = readFileSync(specPath, "utf-8");
            const specParsed = matter(specContent);
            const specFm = specParsed.data as SpecFrontmatterWithBranch;
            
            // Only consider specs linked to the PARENT branch (the one we came from)
            if (specFm.branch === parentBranch && specFm.session) {
              const sessionPath = join(agentsDir, "sessions", specFm.session);
              if (existsSync(sessionPath)) {
                const session = readSessionFile(sessionPath);
                // Verify this session was indeed on the parent branch and is non-archived
                if (session && session.data.branch === parentBranch && session.data.status !== "archived") {
                  // RENAME CONFIRMED: Update both session and spec to new branch name
                  session.data.branch = branch;
                  const newSessionContent = matter.stringify(session.content, session.data as unknown as Record<string, unknown>);
                  writeFileSync(sessionPath, newSessionContent, "utf-8");
                  
                  specFm.branch = branch;
                  const newSpecContent = matter.stringify(specParsed.content, specFm as Record<string, unknown>);
                  writeFileSync(specPath, newSpecContent, "utf-8");
                  
                  candidates.push({ filePath: sessionPath, data: session.data, content: session.content });
                  break; // Found it
                }
              }
            }
          } catch {
            // Continue to next spec
            continue;
          }
        }
      }
    }
  }
  
  // Branch switch detection: if no session matches the current branch,
  // but exactly one active session exists, adopt it (common flow: start on main, then branch)
  if (candidates.length === 0) {
    const allSessions: Array<{ filePath: string; data: SessionFrontmatter; content: string }> = [];
    for (const file of files) {
      const filePath = join(sessionsDir, file);
      const session = readSessionFile(filePath);
      if (session && session.data.status === "active") {
        allSessions.push({ filePath, data: session.data, content: session.content });
      }
    }

    if (allSessions.length === 1) {
      const session = allSessions[0];
      const oldBranch = session.data.branch;
      session.data.branch = branch;
      const newContent = matter.stringify(session.content, session.data as unknown as Record<string, unknown>);
      writeFileSync(session.filePath, newContent, "utf-8");
      candidates.push(session);
    }
  }

  if (candidates.length === 0) return null;

  // Prioritize: active > stopped/blocked/done > others
  // Sort by status priority (lower number = higher priority)
  const statusPriority: Record<string, number> = {
    active: 1,
    stopped: 2,
    blocked: 2,
    done: 2,
  };
  
  candidates.sort((a, b) => {
    // First: sort by status priority
    const priorityA = statusPriority[a.data.status] ?? 3;
    const priorityB = statusPriority[b.data.status] ?? 3;
    if (priorityA !== priorityB) {
      return priorityA - priorityB;
    }
    
    // Second: tie-break by recency (newer wins)
    // Use last_updated from frontmatter, or fall back to last_entry, or 0
    const timeA = a.data.last_updated || a.data.last_entry || "0";
    const timeB = b.data.last_updated || b.data.last_entry || "0";
    return timeB.localeCompare(timeA); // descending (newer first)
  });
  
  return candidates[0];
}

/** Atomically get existing session or create new one (prevents concurrent creation) */
async function getOrCreateSession(
  agentsDir: string,
  branch: string,
  specInfo: { id: string; title: string } | null,
  claudeSessionId?: string
): Promise<{ filePath: string; data: SessionFrontmatter; content: string; isNew: boolean }> {
  const sessionsDir = join(agentsDir, "sessions");
  const lockPath = join(sessionsDir, `.create-${branch.replace(/[^a-zA-Z0-9-]/g, '-')}.lock`);

  await acquireLock(lockPath, 100, 50);
  try {
    const existing = findActiveSessionForBranch(agentsDir, branch);
    if (existing) {
      return { ...existing, isNew: false };
    }

    const filePath = getSessionFilePath(agentsDir);
    createSessionFile(filePath, branch, specInfo, claudeSessionId);
    const session = readSessionFile(filePath)!;
    return { filePath, data: session.data, content: session.content, isNew: true };
  } finally {
    releaseLock(lockPath);
  }
}

/** Read session file or return null */
function readSessionFile(filePath: string): { data: SessionFrontmatter; content: string } | null {
  if (!existsSync(filePath)) return null;

  try {
    const raw = readFileSync(filePath, "utf-8");
    const parsed = matter(raw);
    const rawData = parsed.data as unknown as Record<string, unknown>;
    
    // Handle legacy nested frontmatter migration (SPEC-020 format change)
    // Old format: { session: { branch, status, created, ... }, otherFields... }
    // New format: { branch, status, created, ... }
    // Migration strategy: nested fields take precedence over top-level fields on collision
    if (rawData.session && typeof rawData.session === "object") {
      const nested = rawData.session as Record<string, unknown>;
      // Start with all existing top-level fields
      const migratedData: Record<string, unknown> = { ...rawData };
      // Remove the nested session object itself
      delete migratedData.session;
      // Add/replace fields from nested session data
      for (const [key, value] of Object.entries(nested)) {
        migratedData[key] = value;
      }
      return {
        data: migratedData as unknown as SessionFrontmatter,
        content: parsed.content,
      };
    }
    
    return { 
      data: rawData as unknown as SessionFrontmatter, 
      content: parsed.content 
    };
  } catch {
    return null;
  }
}

/** Parse entry string into structured format */
function parseEntry(entry: string): JournalEntry | null {
  // Match type(scope): description or type: description
  const match = entry.match(/^([a-z]+)(?:\(([^)]+)\))?:\s*(.+)$/);
  if (!match) return null;

  const [, type, scope, description] = match;
  const validTypes: EntryType[] = [
    "start", "resume", "pause", "clear", "progress", "commit", "pr", "merge",
    "decision", "discover", "finding", "block", "unblock",
    "spark", "todo", "assume",
    // New types
    "branch", "task", "linear", "hypothesis", "try", "reject", "compact",
    "skill", "wrap"
  ];

  if (!validTypes.includes(type as EntryType)) return null;

  return {
    type: type as EntryType,
    scope,
    description,
  };
}

/** Get current timestamp in ISO format */
function getTimestamp(): string {
  return new Date().toISOString();
}

/** Get date-time string for journal entries (YYYY-MM-DD HH:MM) */
function getDateTimeString(): string {
  const now = new Date();
  const date = now.toISOString().split("T")[0];
  const time = now.toTimeString().split(":")[0] + ":" + now.toTimeString().split(":")[1];
  return `${date} ${time}`;
}

/** Create new session file with compact inline journal format */
function createSessionFile(
  filePath: string,
  branch: string,
  specInfo: { id: string; title: string } | null,
  claudeSessionId?: string
): SessionFrontmatter | null {
  // Check if file already exists (from concurrent process)
  if (existsSync(filePath)) {
    return null;
  }

  const now = getTimestamp();
  const frontmatter: SessionFrontmatter = {
    branch,
    status: "active",
    created: now,
    last_entry: now,
  };

  // Only add spec field when there's a linked spec
  if (specInfo?.id) {
    frontmatter.spec = specInfo.id;
  }

  // Store claude_session_id if provided
  if (claudeSessionId) {
    frontmatter.claude_session_id = claudeSessionId;
  }

  const title = specInfo ? `${specInfo.id}: ${specInfo.title}` : "Ad-hoc";
  const sessionIdSuffix = claudeSessionId ? ` (session ${claudeSessionId.slice(0, 8)})` : "";
  const entry = `[${getDateTimeString()}] session(start):  === SESSION STARTED ===${sessionIdSuffix}`;

  const body = `# Session: ${title}\n\n## Journal\n\n${entry}\n`;

  const content = matter.stringify(body, frontmatter as unknown as Record<string, unknown>);

  // Use atomic write with 'wx' flag to prevent race conditions
  try {
    const fd = openSync(filePath, 'wx');
    writeFileSync(fd, content, 'utf-8');
    closeSync(fd);
  } catch {
    // File already exists (another process created it)
    return null;
  }

  return frontmatter;
}

/** Append journal entries to session file (atomic with file locking) */
async function appendEntry(
  filePath: string,
  entryLines: string[],
  updateFrontmatter: (data: SessionFrontmatter) => void,
  autoResume?: boolean
): Promise<boolean> {
  const lockPath = `${filePath}.lock`;
  await acquireLock(lockPath);
  let didResume = false;
  try {
    const session = readSessionFile(filePath);
    if (!session) {
      throw new Error("Session file not found");
    }

    // Auto-resume: re-check status under lock with fresh data to avoid
    // duplicate RESUMED markers from concurrent loaf session log calls.
    if (autoResume && session.data.status === "stopped") {
      const ts = getDateTimeString();
      entryLines = [`[${ts}] session(resume): === SESSION RESUMED ===`, ...entryLines];
      session.data.status = "active";
      didResume = true;
    }

    updateFrontmatter(session.data);

    // Blank line rules:
    // - session(stop) always has one blank line after it
    // - session(start)/session(resume) always have one blank line before them
    // trimEnd() strips trailing blank lines, so we reconstruct the separator
    const trimmedContent = session.content.trimEnd();
    const hasNewSession = entryLines.some(line =>
      /session\((resume|start)\):/.test(line)
    );
    const endsWithStop = /session\(stop\):/.test(
      trimmedContent.split('\n').filter(l => l.trim()).pop() || ''
    );

    const separator = (hasNewSession || endsWithStop) ? '\n\n' : '\n';
    const newContent = matter.stringify(
      trimmedContent + separator + entryLines.join("\n") + "\n",
      session.data as unknown as Record<string, unknown>
    );

    writeFileAtomic(filePath, newContent);
  } finally {
    releaseLock(lockPath);
  }
  return didResume;
}

function extractRecentEntries(content: string, count = 15): string[] {
  const lines = content.split("\n");
  const entries: string[] = [];
  const entryPattern = /^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}\] /;

  for (const line of lines) {
    if (entryPattern.test(line)) {
      entries.push(line.trim());
    }
  }

  // Return last 'count' entries
  return entries.slice(-count);
}

function extractDecisionEntries(content: string): string[] {
  const entries: string[] = [];
  const lines = content.split("\n");

  for (const line of lines) {
    if (line.match(/^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}\] decision\([^)]+\):/)) {
      entries.push(line.trim());
    }
  }

  return entries;
}

/** Persist decisions to spec changelog */
function persistDecisionsToSpec(
  agentsDir: string,
  specId: string,
  decisionEntries: string[],
  sessionBranch: string
): { success: boolean; message: string } {
  // Find spec file
  const specsDir = join(agentsDir, "specs");
  if (!existsSync(specsDir)) {
    return { success: false, message: "No specs directory found" };
  }

  // Look for spec file with matching ID
  const specFiles = readdirSync(specsDir).filter((f) => f.endsWith(".md"));
  const specFile = specFiles.find((f) => f.includes(specId));

  if (!specFile) {
    return { success: false, message: `Spec ${specId} not found` };
  }

  const specPath = join(specsDir, specFile);
  const specContent = readFileSync(specPath, "utf-8");

  // Find or create Changelog section
  const changelogMatch = specContent.match(/\n## Changelog\n/);
  const dateStr = new Date().toISOString().split("T")[0];
  const entryHeader = `- ${dateStr} — Session ${sessionBranch} archived: ${decisionEntries.length} decision(s) extracted`;
  
  let updatedContent: string;
  
  if (changelogMatch) {
    // Insert after "## Changelog\n"
    const insertPos = changelogMatch.index! + changelogMatch[0].length;
    const decisionsList = decisionEntries.map((e) => `  ${e}`).join("\n");
    const entry = `${entryHeader}\n${decisionsList}\n\n`;
    updatedContent = specContent.slice(0, insertPos) + entry + specContent.slice(insertPos);
  } else {
    // Append Changelog section at end
    const decisionsList = decisionEntries.map((e) => `  ${e}`).join("\n");
    const entry = `\n## Changelog\n\n${entryHeader}\n${decisionsList}\n`;
    updatedContent = specContent + entry;
  }

  writeFileSync(specPath, updatedContent, "utf-8");
  return { success: true, message: `Appended to ${specFile}` };
}

/** Count tasks by status from session content */
function countTasksInSession(content: string): { completed: number; total: number } {
  const completed = (content.match(/- \[x\]/gi) || []).length;
  const total = (content.match(/- \[[ x]\]/gi) || []).length;
  return { completed, total };
}

/** Count meaningful journal activity from session content */
function countJournalActivity(content: string): {
  commits: number;
  decisions: number;
  entries: number;
} {
  const systemTypes = new Set(["session"]);
  const pattern = /^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}\] (\w+)\(/gm;
  let commits = 0;
  let decisions = 0;
  let entries = 0;
  let match;
  while ((match = pattern.exec(content)) !== null) {
    const type = match[1];
    if (systemTypes.has(type)) continue;
    entries++;
    if (type === "commit") commits++;
    if (type === "decision") decisions++;
  }
  return { commits, decisions, entries };
}

/** Build progress summary string, omitting zero-value segments */
function buildProgressLine(stats: {
  completed: number;
  total: number;
  commits: number;
  decisions: number;
  entries: number;
}): string {
  const parts: string[] = [];
  if (stats.total > 0) parts.push(`${stats.completed}/${stats.total} tasks`);
  if (stats.commits > 0) parts.push(`${stats.commits} commits`);
  if (stats.decisions > 0) parts.push(`${stats.decisions} decisions`);
  const other = stats.entries - stats.commits - stats.decisions;
  if (other > 0) parts.push(`${other} entries`);
  return parts.length > 0 ? parts.join(", ") : "no activity logged";
}

/** Build the ## Current State section content (startup/programmatic use only — agent writes richer state on Stop) */
function buildCurrentStateSection(_sessionContent: string): string {
  const timestamp = getDateTimeString();
  const branch = getCurrentBranch();
  const commits = getRecentCommits(1);
  const uncommitted = getUncommittedCount();

  const lines: string[] = [];
  lines.push(`## Current State (${timestamp})`);
  lines.push("");
  lines.push(`Branch: ${branch}`);

  if (commits.length > 0 && commits[0].hash !== "unknown") {
    lines.push(`Last commit: ${commits[0].hash} — ${commits[0].message}`);
  }

  if (uncommitted > 0) {
    lines.push(`Uncommitted: ${uncommitted} file${uncommitted === 1 ? "" : "s"}`);
  }

  return lines.join("\n");
}

/** Extract the Current State section text from session content, or null if absent */
function extractCurrentState(content: string): string | null {
  const stateMatch = content.match(/## Current State \([^)]+\)\n([\s\S]*?)(?=\n## |\n*$)/);
  if (!stateMatch) return null;
  return stateMatch[0].trim();
}

/** Write/replace the ## Current State section in a session file (locked, atomic) */
async function writeCurrentState(filePath: string, stateSection: string): Promise<void> {
  const lockPath = `${filePath}.lock`;
  await acquireLock(lockPath);
  try {
    const session = readSessionFile(filePath);
    if (!session) {
      throw new Error("Session file not found");
    }

    let body = session.content;

    // Check for existing ## Current State section
    const currentStatePattern = /## Current State \([^)]*\)[\s\S]*?(?=## Journal)/;
    const journalPattern = /## Journal/;

    if (currentStatePattern.test(body)) {
      // Replace existing section
      body = body.replace(currentStatePattern, stateSection + "\n\n");
    } else if (journalPattern.test(body)) {
      // Insert before ## Journal
      body = body.replace(journalPattern, stateSection + "\n\n## Journal");
    } else {
      // Defensive: no ## Journal heading, append at end
      body = body.trimEnd() + "\n\n" + stateSection + "\n";
    }

    session.data.last_updated = getTimestamp();
    const newContent = matter.stringify(body, session.data as unknown as Record<string, unknown>);
    writeFileAtomic(filePath, newContent);
  } finally {
    releaseLock(lockPath);
  }
}

function quickArchiveSession(filePath: string, agentsDir: string): void {
  const archiveDir = join(agentsDir, "sessions", "archive");
  if (!existsSync(archiveDir)) {
    mkdirSync(archiveDir, { recursive: true });
  }

  try {
    const session = readSessionFile(filePath);
    if (session) {
      // Clean up enrichment temp file if it exists
      if (session.data.claude_session_id) {
        const tmpFile = join(agentsDir, "tmp", `${session.data.claude_session_id}-enrichment.txt`);
        try { unlinkSync(tmpFile); } catch { /* file may not exist */ }
      }

      session.data.status = "archived";
      session.data.archived_at = getTimestamp();
      const content = matter.stringify(session.content, session.data as unknown as Record<string, unknown>);
      writeFileSync(filePath, content, "utf-8");
    }
    renameSync(filePath, join(archiveDir, basename(filePath)));
  } catch {
    // Concurrent process may have already archived this session
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Command
// ─────────────────────────────────────────────────────────────────────────────

// ─────────────────────────────────────────────────────────────────────────────
// Enrichment Helpers
// ─────────────────────────────────────────────────────────────────────────────

/** Derive the Claude Code project config directory for the current working directory */
function deriveClaudeProjectDir(): string {
  const configDir = process.env.CLAUDE_CONFIG_DIR
    || join(process.env.HOME || '', '.config', 'claude');
  const cwd = process.cwd();
  const hash = cwd.replace(/\//g, '-');  // /Users/foo → -Users-foo
  return join(configDir, 'projects', hash);
}

/** Build the enrichment prompt for the librarian agent */
function buildEnrichmentPrompt(sessionPath: string, summaryPath: string, dryRun: boolean): string {
  let prompt = `Enrich the session journal at ${sessionPath}.

Conversation summary (pre-filtered from JSONL log): ${summaryPath}

Read both files. Compare the conversation summary against existing journal entries.
Identify semantic events that are NOT already captured in the journal:
- Decisions with rationale (decision)
- Discoveries or things learned (discover)
- Blockers encountered or resolved (block/unblock)
- Important context that would help someone understand this session

Skip: commits, PRs, skill invocations — those are already logged by hooks.
Skip: routine tool calls, file reads — not journal-worthy.

Append missing entries to the ## Journal section using the format:
[YYYY-MM-DD HH:MM] type(scope): description

Do not edit the frontmatter. Do not create new sections.`;

  if (dryRun) {
    prompt += '\n\nOutput the entries you would add as plain text. Do NOT edit any files.';
  }

  return prompt;
}

export function registerSessionCommand(program: Command): void {
  const session = program
    .command("session")
    .description("Manage session journals");

  // ── loaf session start ─────────────────────────────────────────────────────

  session
    .command("start")
    .description("Start/resume session for current branch")
    .option("--resume", "Resume existing paused session instead of creating new")
    .option("--force", "Force session creation, bypassing subagent detection")
    .action(async (options: { resume?: boolean; force?: boolean }) => {
      // Parse hook JSON from stdin (when invoked as a hook by Claude Code)
      const hookInput = await parseHookInput();

      // Enrichment isolation: suppress session creation during enrichment agent runs
      if (process.env.LOAF_ENRICHMENT === '1') {
        process.exit(0);
      }

      // Subagent detection: agent_id is only present for subagents
      if (hookInput.agent_id && !options.force) {
        process.exit(0);
      }

      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        console.error(`  ${red("error:")} Could not find .agents/ directory`);
        process.exit(1);
      }

      const sessionsDir = join(agentsDir, "sessions");
      if (!existsSync(sessionsDir)) {
        mkdirSync(sessionsDir, { recursive: true });
      }

      const branch = getCurrentBranch();
      if (branch === "unknown") {
        console.error(`  ${red("error:")} Not in a git repository`);
        process.exit(1);
      }

      if (branch.startsWith("detached-")) {
        console.error(`  ${yellow("⚠")} Warning: In detached HEAD state (${branch})`);
        console.error(`  ${gray("Sessions in detached HEAD state are isolated to the specific commit.")}`);
      }

      console.log(`\n  ${bold("loaf session start")}\n`);
      console.log(`  Branch: ${cyan(branch)}`);

      const specInfo = findSpecForBranch(agentsDir, branch);
      if (specInfo) {
        console.log(`  Linked to: ${bold(specInfo.id)} — ${specInfo.title}`);
      } else {
        console.log(`  ${yellow("!")} No linked spec found — creating ad-hoc session`);
      }

      // Session ID-first lookup: strongest continuity signal, searches active + archive
      const sessionByClaudeId = hookInput.session_id
        ? findSessionByClaudeId(agentsDir, hookInput.session_id, branch)
        : null;

      // Branch-based fallback
      const existingSession = sessionByClaudeId || findActiveSessionForBranch(agentsDir, branch);

      let sessionFilePath: string;
      let sessionData: SessionFrontmatter;
      let sessionContent: string;
      let isResume = false;

      const sameConversation = sessionByClaudeId != null;
      const isClearResume = existingSession && hookInput.source === "clear";

      // Determine if this is a new conversation vs same conversation
      const isNewConversation = existingSession
        && hookInput.session_id
        && existingSession.data.claude_session_id
        && hookInput.session_id !== existingSession.data.claude_session_id
        && hookInput.source !== "clear";

      if (existingSession && sameConversation) {
        // Same claude_session_id — always resume, strongest signal (even from archive)
        sessionFilePath = existingSession.filePath;
        sessionData = existingSession.data;
        sessionContent = existingSession.content;
        isResume = existingSession.data.status !== "active";

        // Update branch if it changed (e.g., git checkout while in same conversation)
        if (existingSession.data.branch !== branch) {
          existingSession.data.branch = branch;
        }

        console.log(`  ${green("✓")} Resuming existing session`);
      } else if (isClearResume) {
        // Context cleared (/clear) — new session_id but same work; treat as resume
        sessionFilePath = existingSession.filePath;
        sessionData = existingSession.data;
        sessionContent = existingSession.content;
        isResume = true;

        console.log(`  ${green("✓")} Resuming existing session (after /clear)`);
      } else if (isNewConversation) {
        // Different claude_session_id = new conversation on same branch
        // Close the stale session and create a fresh file
        const timestamp = getDateTimeString();
        await appendEntry(
          existingSession.filePath,
          [
            `[${timestamp}] session(end): closed by new conversation`,
            `[${timestamp}] session(stop):   === SESSION STOPPED ===`,
            '',
          ],
          (data: SessionFrontmatter) => {
            data.status = "stopped";
            data.last_updated = getTimestamp();
            data.last_entry = getTimestamp();
          }
        );
        console.log(`  ${yellow("!")} Closed stale session (different conversation)`);

        // Create new file directly (getOrCreateSession would find the just-stopped session)
        sessionFilePath = getSessionFilePath(agentsDir);
        createSessionFile(sessionFilePath, branch, specInfo, hookInput.session_id);
        const newSession = readSessionFile(sessionFilePath)!;
        sessionData = newSession.data;
        sessionContent = newSession.content;

        console.log(`  ${green("+")} Creating new session file`);
        if (specInfo) {
          const sessionFileName = basename(sessionFilePath);
          updateSpecSessionField(agentsDir, specInfo.id, sessionFileName);
        }
      } else if (existingSession && (options.resume || existingSession.data.status === "active")) {
        // --resume flag or session still active (no session_id available for comparison)
        sessionFilePath = existingSession.filePath;
        sessionData = existingSession.data;
        sessionContent = existingSession.content;
        isResume = existingSession.data.status !== "active";

        console.log(`  ${green("✓")} Resuming existing session`);
      } else {
        if (existingSession) {
          quickArchiveSession(existingSession.filePath, agentsDir);
          console.log(`  ${yellow("!")} Closed previous session`);
        }

        const result = await getOrCreateSession(agentsDir, branch, specInfo, hookInput.session_id);
        sessionFilePath = result.filePath;
        sessionData = result.data;
        sessionContent = result.content;

        if (result.isNew) {
          console.log(`  ${green("+")} Creating new session file`);
          if (specInfo) {
            const sessionFileName = basename(sessionFilePath);
            updateSpecSessionField(agentsDir, specInfo.id, sessionFileName);
          }
        } else {
          console.log(`  ${green("✓")} Resuming existing session`);
          isResume = true;
        }
      }

      const lastCommit = getLastCommitSha();
      const commits = getRecentCommits(3);
      const { completed, total } = countTasksInSession(sessionContent);
      const timestamp = getDateTimeString();
      const journalLines: string[] = [];

      if (isResume) {
        const resumeIdSuffix = hookInput.session_id ? ` (session ${hookInput.session_id.slice(0, 8)})` : "";
        journalLines.push(`[${timestamp}] session(resume): === SESSION RESUMED ===${resumeIdSuffix}`);
        if (lastCommit !== "unknown") {
          journalLines.push(`[${timestamp}] session(context): from commit ${lastCommit}`);
        }
        if (completed > 0 || total > 0) {
          journalLines.push(`[${timestamp}] session(progress): ${completed}/${total} tasks completed`);
        }
        // Backfill commits made AFTER the last session entry (e.g., manual git work between sessions)
        if (commits.length > 0) {
          const lastEntryTime = sessionData.last_entry ? new Date(sessionData.last_entry).getTime() : 0;
          for (const commit of commits.slice(0, 3)) {
            if (!sessionContent.includes(`commit(${commit.hash})`) && commit.timestamp > lastEntryTime) {
              journalLines.push(`[${timestamp}] commit(${commit.hash}): ${commit.message}`);
            }
          }
        }
      }

      if (journalLines.length > 0) {
        await appendEntry(
          sessionFilePath,
          journalLines,
          (data: SessionFrontmatter) => {
            data.status = "active";
            data.last_updated = getTimestamp();
            data.last_entry = getTimestamp();
            if (hookInput.session_id) {
              data.claude_session_id = hookInput.session_id;
            }
          }
        );
      } else if (sessionData.status !== "active" || hookInput.session_id) {
        sessionData.status = "active";
        sessionData.last_updated = getTimestamp();
        sessionData.last_entry = getTimestamp();
        if (hookInput.session_id) {
          sessionData.claude_session_id = hookInput.session_id;
        }
        const newContent = matter.stringify(sessionContent, sessionData as unknown as Record<string, unknown>);
        writeFileSync(sessionFilePath, newContent, "utf-8");
      }

      console.log(`  ${green("✓")} Session active: ${gray(sessionFilePath.replace(agentsDir, ".agents"))}`);

      const soulStatus = validateSoulMd(agentsDir);
      if (soulStatus.restored) {
        console.log(`  ${yellow("⚠")} SOUL.md was missing — restored from template`);
      } else if (!soulStatus.exists) {
        console.log(`  ${yellow("⚠")} SOUL.md not found — run 'loaf install' to set up project`);
      }

      const staleKbCount = countStaleKnowledge();
      if (staleKbCount > 0) {
        console.log(`  ${yellow("⚠")} ${staleKbCount} stale knowledge file(s) — run 'loaf kb review'`);
      }

      const currentContent = readSessionFile(sessionFilePath)?.content || sessionContent;

      // Surface Current State section on resume (higher-level overview before entries)
      if (isResume) {
        const currentState = extractCurrentState(currentContent);
        if (currentState) {
          // Extract heading timestamp and body lines
          const stateLines = currentState.split("\n");
          const heading = stateLines[0]; // "## Current State (timestamp)"
          const headingMatch = heading.match(/## Current State \(([^)]+)\)/);
          const headingTimestamp = headingMatch ? headingMatch[1] : "";
          const bodyLines = stateLines.slice(1).filter(line => line.trim() !== "");

          console.log(`\n  ${bold(`Current State (${headingTimestamp}):`)}\n`);
          for (const line of bodyLines) {
            console.log(`    ${line}`);
          }
        }
      }

      const recentEntries = extractRecentEntries(currentContent, 15);
      if (recentEntries.length > 0) {
        console.log(`\n  ${bold("Recent journal entries:")}\n`);
        for (const entry of recentEntries) {
          console.log(`    ${entry}`);
        }
      }

      // Suggest /rename when a spec is linked to the branch
      if (specInfo) {
        const slug = specInfo.title.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "").slice(0, 30);
        console.log(`\n  ${gray(`Suggestion: /rename ${specInfo.id}-${slug}`)}`);
      }

      // Check if housekeeping is pending (flagged by previous session end)
      if (isHousekeepingPending(agentsDir)) {
        console.log(`\n  ${yellow("⚠")} Housekeeping pending — run ${cyan("loaf session housekeeping")} or ${cyan("/housekeeping")}`);
      }

      console.log();
    });

  // ── loaf session end ───────────────────────────────────────────────────────

  session
    .command("end")
    .description("End session with progress summary")
    .option("--if-active", "Exit successfully when no active session exists")
    .option("--wrap", "Close session as done (used after /wrap writes summary)")
    .action(async (options: { ifActive?: boolean; wrap?: boolean }) => {
      // Parse hook JSON from stdin (when invoked as a hook by Claude Code)
      const hookInput = await parseHookInput();

      // Enrichment isolation: suppress session end during enrichment agent runs
      if (process.env.LOAF_ENRICHMENT === '1') {
        process.exit(0);
      }

      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        console.error(`  ${red("error:")} Could not find .agents/ directory`);
        process.exit(1);
      }

      const branch = getCurrentBranch();
      if (branch === "unknown") {
        console.error(`  ${red("error:")} Not in a git repository`);
        process.exit(1);
      }

      // Session ID-first lookup, then branch fallback
      const existingSession = (hookInput.session_id
        ? findSessionByClaudeId(agentsDir, hookInput.session_id, branch)
        : null) || findActiveSessionForBranch(agentsDir, branch);
      if (!existingSession) {
        if (options.ifActive) {
          process.exit(0);
        }
        console.error(`  ${red("error:")} No active session found for branch ${branch}`);
        process.exit(1);
      }
      if (options.ifActive && existingSession.data.status !== "active") {
        process.exit(0);
      }

      console.log(`\n  ${bold("loaf session end")}\n`);

      // Handle context clear: log marker and keep session active
      if (hookInput.reason === "clear") {
        const timestamp = getDateTimeString();
        await appendEntry(
          existingSession.filePath,
          [`[${timestamp}] session(clear):  === CONTEXT CLEARED ===`],
          (data: SessionFrontmatter) => {
            data.last_updated = getTimestamp();
            data.last_entry = getTimestamp();
          }
        );

        console.log(`  ${green("✓")} Context cleared: ${gray(existingSession.filePath.replace(agentsDir, ".agents"))}`);
        console.log();
        return;
      }

      const lastCommit = getLastCommitSha();
      const { completed, total } = countTasksInSession(existingSession.content);
      const activity = countJournalActivity(existingSession.content);
      const timestamp = getDateTimeString();
      const progressText = buildProgressLine({
        completed,
        total,
        commits: activity.commits,
        decisions: activity.decisions,
        entries: activity.entries,
      });

      // Build conclude line: commit + progress stats
      const concludeParts: string[] = [];
      if (lastCommit !== "unknown") concludeParts.push(`at commit ${lastCommit}`);
      if (progressText !== "no activity logged") concludeParts.push(progressText);
      const concludeText = concludeParts.length > 0 ? concludeParts.join(", ") : "session ended";

      const isWrap = !!options.wrap;

      if (isWrap) {
        // Wrap: log wrap marker, set status to done, but do NOT write
        // session(end)/session(stop) — the SessionEnd hook handles stop
        // when the conversation actually ends. This prevents journal entries
        // (merge commits, etc.) appearing after stop markers.
        const journalLines: string[] = [
          `[${timestamp}] session(wrap): ${concludeText}`,
        ];

        await appendEntry(
          existingSession.filePath,
          journalLines,
          (data: SessionFrontmatter) => {
            data.status = "done";
            data.last_updated = getTimestamp();
            data.last_entry = getTimestamp();
          }
        );
      } else {
        // Normal end: write stop markers
        const journalLines: string[] = [
          `[${timestamp}] session(end): ${concludeText}`,
          `[${timestamp}] session(stop):   === SESSION STOPPED ===`,
          '',
        ];

        console.log(`  ${yellow("?")} Consider adding final entries:`);
        console.log(`    ${gray("loaf session log \"decision(scope): key decision\"")}`);
        console.log(`    ${gray("loaf session log \"finding(scope): final notes\"")}`);
        console.log(`    ${gray("loaf session log \"todo(next): follow-up task\"")}`);
        console.log();

        await appendEntry(
          existingSession.filePath,
          journalLines,
          (data: SessionFrontmatter) => {
            data.status = "stopped";
            data.last_updated = getTimestamp();
            data.last_entry = getTimestamp();
          }
        );
      }

      // Wrap-specific: persist decisions to spec changelog and clean up Current State
      if (isWrap) {
        // Re-read session content after journal append
        const freshSession = readSessionFile(existingSession.filePath);
        const sessionContent = freshSession?.content || existingSession.content;

        // Persist decisions to spec changelog
        if (existingSession.data.spec) {
          const decisionEntries = extractDecisionEntries(sessionContent);
          if (decisionEntries.length > 0) {
            const result = persistDecisionsToSpec(
              agentsDir,
              existingSession.data.spec,
              decisionEntries,
              existingSession.data.branch || branch
            );
            if (result.success) {
              console.log(`  ${green("✓")} Decisions persisted: ${result.message}`);
            } else {
              console.log(`  ${yellow("⚠")} Could not persist decisions: ${result.message}`);
            }
          }
        }

        // Strip ## Current State if ## Session Wrap-Up exists (wrap summary replaces it)
        if (sessionContent.includes("## Session Wrap-Up")) {
          const currentStatePattern = /\n## Current State \([^)]*\)[\s\S]*?(?=\n## |\n*$)/;
          if (currentStatePattern.test(sessionContent)) {
            const cleanedContent = sessionContent.replace(currentStatePattern, "");
            const newFileContent = matter.stringify(
              cleanedContent,
              (freshSession?.data || existingSession.data) as unknown as Record<string, unknown>
            );
            writeFileAtomic(existingSession.filePath, newFileContent);
            console.log(`  ${green("✓")} Replaced Current State with wrap summary`);
          }
        }
      }

      const knowledgeFiles = consumeKnowledgeNudges(findGitRoot(), branch);
      if (knowledgeFiles.length > 0) {
        console.log(`  ${yellow("⚠")} Knowledge consolidation recommended for ${knowledgeFiles.length} file(s):`);
        for (const file of knowledgeFiles) {
          console.log(`    ${gray(file)}`);
        }
        console.log(`    ${gray("Run 'loaf kb review <file>' before ending.")}`);
        console.log();
      }

      // Check if housekeeping is due (sets pending flag for next session start)
      checkHousekeepingDue(agentsDir);

      const statusLabel = isWrap ? "done" : "stopped";
      console.log(`  ${green("✓")} Session ${statusLabel}: ${gray(existingSession.filePath.replace(agentsDir, ".agents"))}`);
      console.log();
    });

  // ── loaf session log ───────────────────────────────────────────────────────

  session
    .command("log [entry]")
    .description("Log entry to session journal")
    .option("--from-hook", "Parse entry from hook stdin")
    .option("--detect-linear", "Detect Linear magic words in recent commits")
    .action(async (entry: string, options: { fromHook?: boolean; detectLinear?: boolean }) => {
      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        console.error(`  ${red("error:")} Could not find .agents/ directory`);
        process.exit(1);
      }

      const branch = getCurrentBranch();
      if (branch === "unknown") {
        console.error(`  ${red("error:")} Not in a git repository`);
        process.exit(1);
      }
      
      if (branch.startsWith("detached-")) {
        console.error(`  ${yellow("⚠")} Warning: In detached HEAD state (${branch})`);
      }

      const existingSession = findActiveSessionForBranch(agentsDir, branch);
      if (!existingSession) {
        if (options.fromHook) {
          process.exit(0);
        }
        console.error(`  ${red("error:")} No active session found for branch ${branch}. Run 'loaf session start' first.`);
        process.exit(1);
      }

      let entryText = entry;

      // Handle --from-hook: parse JSON from stdin
      if (options.fromHook) {
        try {
          const stdin = await readStdin(); // Read from fd 0 (stdin)
          const hookData = JSON.parse(stdin);

          // Detect hook event type — TaskCompleted uses hook_event_name, not tool_name
          const hookEventName = hookData.hook_event_name;
          const toolName = hookData.tool_name || hookData.tool?.name;

          if (hookEventName === "TaskCompleted" || toolName === "TaskCompleted") {
            // TaskCompleted hook event — payload has task_id, task_subject, task_description
            // Prefer description (richer context for compaction recovery and wrap summaries)
            const description = hookData.task_description || hookData.task_subject || "task";
            entryText = `task(completed): ${description}`;
          }

          // Extract command from generic tool payload (Bash tools)
          // Support both flat (tool_input) and nested (tool.input) formats for cross-harness compatibility
          const command = hookData.tool_input?.command ||
                          hookData.tool?.input?.command ||
                          hookData.input?.command;

          if (!entryText && command && typeof command === "string") {
            // Parse Bash command to detect entry type
            if (command.includes("git commit")) {
              const hash = getLastCommitSha();
              const isAmend = command.includes("--amend");

              if (isAmend) {
                // Find previous commit entry in session to get the old hash
                const prevMatch = existingSession.content.match(/\[.*?\] commit\(([a-f0-9]+)\):/g);
                const lastEntry = prevMatch?.[prevMatch.length - 1];
                const oldHash = lastEntry?.match(/commit\(([a-f0-9]+)\)/)?.[1] || "unknown";
                entryText = `commit(${hash}): amended commit(${oldHash})`;
              } else {
                // Read actual commit message from git (works for -m, editor commits)
                let message = "";
                try {
                  message = execSync("git log -1 --format=%s", {
                    encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"],
                  }).trim();
                } catch { /* fallback to command parsing */ }
                // Fall back to parsing -m flag if git log didn't produce a useful message
                if (!message) {
                  const msgMatch = command.match(/-m\s+['"]([^'"]+)['"]/) || command.match(/-m\s+(\S+)/);
                  message = msgMatch ? msgMatch[1] : "commit";
                }
                entryText = `commit(${hash}): ${message}`;
              }
            } else if (command.includes("gh pr create")) {
              // Extract PR title from command
              const titleMatch = command.match(/--title\s+["']([^"']+)["']/);
              const title = titleMatch ? titleMatch[1] : "PR created";

              // Try to extract PR number from tool_response (post-tool hook output)
              let prSuffix = "";
              const response = hookData.tool_response;
              if (response) {
                const stdout = typeof response === "string" ? response : response.stdout;
                if (stdout && typeof stdout === "string") {
                  const prUrlMatch = stdout.match(/https:\/\/github\.com\/[^\/]+\/[^\/]+\/pull\/(\d+)/);
                  if (prUrlMatch) {
                    prSuffix = ` (#${prUrlMatch[1]})`;
                  }
                }
              }

              entryText = `pr(create): ${title}${prSuffix}`;
            } else if (command.includes("gh pr merge")) {
              // Extract PR number from URL or direct argument
              const prMatch = command.match(/merge\s+https:\/\/github\.com\/[^\/]+\/[^\/]+\/pull\/(\d+)/) ||
                               command.match(/pr\s+merge\s+(\d+)/);
              const prNum = prMatch ? prMatch[1] : "unknown";
              entryText = `pr(merge): #${prNum} merged`;
            } else {
              // Unrecognized command — skip silently (only log known patterns)
              process.exit(0);
            }
          } else if (!entryText) {
            // Fallback: try legacy fields for backward compatibility
            if (hookData.commit) {
              entryText = `commit(${hookData.commit}): ${hookData.message || "commit"}`;
            } else if (hookData.pr) {
              entryText = `pr(create): ${hookData.title || "PR created"} (#${hookData.pr})`;
            } else if (hookData.merge) {
              entryText = `pr(merge): #${hookData.merge}`;
            } else {
              // No recognized tool or command — exit silently
              process.exit(0);
            }
          }
        } catch (err) {
          console.error(`  ${red("error:")} Failed to parse stdin JSON: ${err}`);
          process.exit(1);
        }
      }

      // Handle --detect-linear: scan commits for magic words
      if (options.detectLinear) {
        try {
          if (isLinearIntegrationDisabled(findGitRoot())) {
            process.exit(0);
          }
          const commits = getRecentCommits(3);
          const detections: Array<{ issueId: string; action: string; commit: string }> = [];

          // Magic word patterns: Fixes/Closes/Resolves + ISSUE-ID
          const magicPattern = /\b(fixe?s?d?|close?s?d?|resolve?s?d?)\s+([A-Z]+-\d+)/gi;

          for (const commit of commits) {
            const matches = commit.message.matchAll(magicPattern);
            for (const match of matches) {
              detections.push({
                issueId: match[2],
                action: match[1].toLowerCase(),
                commit: commit.hash,
              });
            }
          }

          if (detections.length === 0) {
            console.log(`  ${gray("ℹ")} No Linear magic words detected in recent commits`);
            process.exit(0);
          }

          // Create entry text summarizing detections
          const uniqueIssues = [...new Set(detections.map((d) => d.issueId))];
          entryText = `discover(linear): found magic words for ${uniqueIssues.join(", ")}`;
        } catch (err) {
          console.error(`  ${red("error:")} Failed to scan commits: ${err}`);
          process.exit(1);
        }
      }

      // Validate entry is provided
      if (!entryText) {
        console.error(`  ${red("error:")} Entry text is required. Use: loaf session log "type(scope): description"`);
        console.error(`  ${gray("Examples:")}`);
        console.error(`    ${gray("loaf session log \"decision(hooks): remove bash wrappers\"")}`);
        console.error(`    ${gray("loaf session log \"todo(next): implement tests\"")}`);
        process.exit(1);
      }

      // Validate entry format
      const parsedEntry = parseEntry(entryText);
      if (!parsedEntry) {
        console.error(`  ${red("error:")} Invalid entry format. Use: type(scope): description`);
        console.error(`  ${gray("Examples:")}`);
        console.error(`    ${gray("loaf session log \"decision(hooks): remove bash wrappers\"")}`);
        console.error(`    ${gray("loaf session log \"todo(next): implement tests\"")}`);
        process.exit(1);
      }

      const timestamp = getDateTimeString();
      const formattedEntry = `[${timestamp}] ${entryText}`;

      const mayNeedResume = existingSession.data.status === "stopped";

      const didResume = await appendEntry(
        existingSession.filePath,
        [formattedEntry],
        (data: SessionFrontmatter) => {
          data.last_updated = getTimestamp();
          data.last_entry = getTimestamp();
        },
        mayNeedResume
      );

      if (didResume) {
        console.log(`  ${green("\u2713")} Auto-resumed stopped session`);
      }
      console.log(`  ${green("\u2713")} Logged: ${cyan(entryText)}`);
    });

  // ── loaf session archive ───────────────────────────────────────────────────

  session
    .command("archive")
    .description("Archive completed session")
    .option("--branch <branch>", "Archive session for specific branch (default: current)")
    .action(async (options: { branch?: string }) => {
      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        console.error(`  ${red("error:")} Could not find .agents/ directory`);
        process.exit(1);
      }

      const branch = options.branch || getCurrentBranch();
      if (branch === "unknown" && !options.branch) {
        console.error(`  ${red("error:")} Not in a git repository. Use --branch <branch>`);
        process.exit(1);
      }

      const existingSession = findActiveSessionForBranch(agentsDir, branch);
      if (!existingSession) {
        console.error(`  ${red("error:")} No active session found for branch ${branch}`);
        process.exit(1);
      }

      console.log(`\n  ${bold("loaf session archive")}\n`);

      // Ensure archive directory exists
      const archiveDir = join(agentsDir, "sessions", "archive");
      if (!existsSync(archiveDir)) {
        mkdirSync(archiveDir, { recursive: true });
      }

      const decisionEntries = extractDecisionEntries(existingSession.content);
      if (decisionEntries.length > 0) {
        console.log(`  ${bold("Key decisions extracted:")}`);
        for (const entry of decisionEntries.slice(0, 10)) {
          console.log(`    ${entry}`);
        }
        if (decisionEntries.length > 10) {
          console.log(`    ${gray(`... and ${decisionEntries.length - 10} more`)}`);
        }
        console.log();

        if (existingSession.data.spec && decisionEntries.length > 0) {
          const result = persistDecisionsToSpec(
            agentsDir,
            existingSession.data.spec,
            decisionEntries,
            existingSession.data.branch || branch
          );
          if (result.success) {
            console.log(`  ${green("✓")} ${result.message}`);
          } else {
            console.log(`  ${yellow("⚠")} Could not persist to spec: ${result.message}`);
          }
          console.log();
        }
      }

      // Clean up enrichment temp file before archiving
      if (existingSession.data.claude_session_id) {
        const tmpFile = join(agentsDir, "tmp", `${existingSession.data.claude_session_id}-enrichment.txt`);
        try { unlinkSync(tmpFile); } catch { /* file may not exist */ }
      }

      // Move first, then update frontmatter — avoids a window where status is
      // archived but the file is still in sessions/
      const archivePath = join(archiveDir, basename(existingSession.filePath));

      try {
        renameSync(existingSession.filePath, archivePath);
      } catch (err) {
        console.error(`  ${red("error:")} Failed to move file: ${err}`);
        process.exit(1);
      }

      const now = getTimestamp();
      const archivedSession = readSessionFile(archivePath);
      if (archivedSession) {
        archivedSession.data.status = "archived";
        archivedSession.data.archived_at = now;
        archivedSession.data.last_updated = now;

        const newContent = matter.stringify(
          archivedSession.content,
          archivedSession.data as unknown as Record<string, unknown>
        );
        writeFileSync(archivePath, newContent, "utf-8");
      }

      console.log(`  ${green("✓")} Archived: ${gray(archivePath.replace(agentsDir, ".agents"))}`);

      console.log();
    });

  // ── loaf session housekeeping ─────────────────────────────────────────────

  session
    .command("housekeeping")
    .description("Run session housekeeping: orphans, splits, archival, linkage repair")
    .option("--dry-run", "Report what would be done without making changes")
    .action(async (options: { dryRun?: boolean }) => {
      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        console.error(`  ${red("error:")} Could not find .agents/ directory`);
        process.exit(1);
      }

      const sessionsDir = join(agentsDir, "sessions");
      if (!existsSync(sessionsDir)) {
        console.log(`  ${gray("No sessions directory. Nothing to do.")}`);
        markHousekeepingDone(agentsDir);
        return;
      }

      const dryRun = !!options.dryRun;
      console.log(`\n  ${bold("loaf session housekeeping")}${dryRun ? gray(" (dry run)") : ""}\n`);

      // Collect all sessions
      const files = readdirSync(sessionsDir).filter(f => f.endsWith(".md"));
      const sessions: Array<{ filePath: string; fileName: string; data: SessionFrontmatter; content: string }> = [];
      for (const file of files) {
        const filePath = join(sessionsDir, file);
        const s = readSessionFile(filePath);
        if (s) {
          sessions.push({ filePath, fileName: file, data: s.data, content: s.content });
        }
      }

      if (sessions.length === 0) {
        console.log(`  ${gray("No sessions found.")}`);
        markHousekeepingDone(agentsDir);
        return;
      }

      // Get list of existing git branches
      let gitBranches: Set<string>;
      try {
        const branchOutput = execSync("git branch --list --format='%(refname:short)'", {
          encoding: "utf-8",
          stdio: ["pipe", "pipe", "pipe"],
        }).trim();
        gitBranches = new Set(branchOutput.split("\n").map(b => b.trim()).filter(Boolean));
      } catch {
        gitBranches = new Set();
      }

      const report = {
        orphansArchived: 0,
        orphansFlagged: 0,
        splitsConsolidated: 0,
        ageArchived: 0,
        linkageFixed: 0,
        total: sessions.length,
      };

      // 1. Detect orphans — sessions whose branch no longer exists
      console.log(`  ${bold("Orphan detection")}`);
      const orphans = sessions.filter(s =>
        s.data.status !== "archived" &&
        s.data.status !== "done" &&
        s.data.branch &&
        !s.data.branch.startsWith("detached-") &&
        !gitBranches.has(s.data.branch)
      );

      if (orphans.length === 0) {
        console.log(`    ${green("✓")} No orphaned sessions`);
      } else {
        for (const orphan of orphans) {
          const activity = countJournalActivity(orphan.content);
          const isEmpty = activity.entries === 0 && activity.commits === 0;

          if (isEmpty) {
            if (!dryRun) {
              quickArchiveSession(orphan.filePath, agentsDir);
            }
            console.log(`    ${yellow("→")} Archived empty orphan: ${gray(orphan.fileName)} (branch ${orphan.data.branch} deleted)`);
            report.orphansArchived++;
          } else {
            console.log(`    ${yellow("⚠")} Orphan needs review: ${orphan.fileName} (branch ${orphan.data.branch} deleted, ${activity.entries} entries)`);
            report.orphansFlagged++;
          }
        }
      }

      // 2. Detect and consolidate splits (same claude_session_id, multiple files)
      console.log(`\n  ${bold("Split detection")}`);
      const bySessionId = new Map<string, typeof sessions>();
      for (const s of sessions) {
        if (s.data.claude_session_id && s.data.status !== "archived") {
          const existing = bySessionId.get(s.data.claude_session_id) || [];
          existing.push(s);
          bySessionId.set(s.data.claude_session_id, existing);
        }
      }

      const splits = [...bySessionId.entries()].filter(([, group]) => group.length > 1);
      if (splits.length === 0) {
        console.log(`    ${green("✓")} No split sessions`);
      } else {
        for (const [sessionId, group] of splits) {
          console.log(`    ${yellow("→")} Consolidating ${group.length} files for session ${sessionId.slice(0, 8)}`);
          if (!dryRun) {
            group.sort((a, b) => {
              if (a.data.status === "active" && b.data.status !== "active") return -1;
              if (b.data.status === "active" && a.data.status !== "active") return 1;
              const aTime = a.data.last_updated || a.data.last_entry || "0";
              const bTime = b.data.last_updated || b.data.last_entry || "0";
              return bTime.localeCompare(aTime);
            });
            for (const secondary of group.slice(1)) {
              consolidateSession(group[0].filePath, secondary, agentsDir);
            }
          }
          report.splitsConsolidated++;
        }
      }

      // 3. Age-based archival — done sessions older than 7 days
      console.log(`\n  ${bold("Age-based archival")}`);
      const AGE_THRESHOLD_MS = 7 * 24 * 60 * 60 * 1000;
      const completeSessions = sessions.filter(s => s.data.status === "done");
      let ageArchived = 0;

      for (const s of completeSessions) {
        const completedAt = s.data.last_updated || s.data.last_entry;
        if (!completedAt) continue;
        const age = Date.now() - new Date(completedAt).getTime();
        if (age > AGE_THRESHOLD_MS) {
          if (!dryRun) {
            quickArchiveSession(s.filePath, agentsDir);
          }
          const days = Math.floor(age / (24 * 60 * 60 * 1000));
          console.log(`    ${yellow("→")} Archived: ${gray(s.fileName)} (done, ${days}d old)`);
          ageArchived++;
        }
      }
      report.ageArchived = ageArchived;

      if (ageArchived === 0) {
        console.log(`    ${green("✓")} No sessions past age threshold`);
      }

      // 4. Session/spec linkage repair
      console.log(`\n  ${bold("Spec linkage")}`);
      const specsDir = join(agentsDir, "specs");
      let linkageFixed = 0;

      if (existsSync(specsDir)) {
        const specFiles = readdirSync(specsDir).filter(f => f.endsWith(".md"));
        for (const specFile of specFiles) {
          try {
            const specPath = join(specsDir, specFile);
            const specContent = readFileSync(specPath, "utf-8");
            const specParsed = matter(specContent);
            const specFm = specParsed.data as SpecFrontmatterWithBranch;

            if (specFm.session) {
              const sessionPath = join(sessionsDir, specFm.session);
              const archivePath = join(sessionsDir, "archive", specFm.session);

              if (!existsSync(sessionPath) && !existsSync(archivePath)) {
                const matchingSession = sessions.find(s =>
                  s.data.spec === specFm.id || s.data.branch === specFm.branch
                );
                if (matchingSession) {
                  const correctFileName = basename(matchingSession.filePath);
                  if (!dryRun) {
                    specFm.session = correctFileName;
                    const newContent = matter.stringify(specParsed.content, specFm as Record<string, unknown>);
                    writeFileSync(specPath, newContent, "utf-8");
                  }
                  console.log(`    ${yellow("→")} Fixed: ${specFile} → ${correctFileName}`);
                  linkageFixed++;
                } else {
                  console.log(`    ${yellow("⚠")} Broken link: ${specFile} references missing ${specFm.session}`);
                }
              }
            }
          } catch {
            continue;
          }
        }
      }
      report.linkageFixed = linkageFixed;

      if (linkageFixed === 0) {
        console.log(`    ${green("✓")} All spec links valid`);
      }

      // 5. KB staleness
      console.log(`\n  ${bold("Knowledge base")}`);
      const staleCount = countStaleKnowledge();
      if (staleCount > 0) {
        console.log(`    ${yellow("⚠")} ${staleCount} stale knowledge file(s) — run ${cyan("loaf kb review")}`);
      } else {
        console.log(`    ${green("✓")} All knowledge files current`);
      }

      // Mark housekeeping done
      if (!dryRun) {
        markHousekeepingDone(agentsDir);
      }

      // Summary
      const actions = report.orphansArchived + report.splitsConsolidated + report.ageArchived + report.linkageFixed;
      console.log(`\n  ${bold("Summary:")} ${report.total} sessions scanned, ${actions} action(s)${dryRun ? " (dry run)" : ""}`);
      if (report.orphansFlagged > 0) {
        console.log(`  ${yellow("⚠")} ${report.orphansFlagged} orphan(s) need manual review`);
      }
      console.log();
    });

  // ── loaf session enrich ──────────────────────────────────────────────────

  session
    .command("enrich [file]")
    .description("Enrich session journal from JSONL conversation log")
    .option("--dry-run", "Show what would be added without writing")
    .option("--model <model>", "Override model for the librarian call")
    .action(async (file: string | undefined, options: { dryRun?: boolean; model?: string }) => {
      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        console.error(`  ${red("error:")} Could not find .agents/ directory`);
        process.exit(1);
      }

      // Resolve session file — explicit path or active session
      let sessionPath: string;
      let sessionData: SessionFrontmatter;

      if (file) {
        // Explicit file path — resolve relative to cwd or .agents/sessions/
        const candidates = [
          file,
          join(agentsDir, "sessions", file),
        ];
        const resolved = candidates.find(c => existsSync(c));
        if (!resolved) {
          console.error(`  ${red("error:")} Session file not found: ${file}`);
          process.exit(1);
        }
        sessionPath = resolved;
        const parsed = readSessionFile(sessionPath);
        if (!parsed) {
          console.error(`  ${red("error:")} Could not parse session file: ${sessionPath}`);
          process.exit(1);
        }
        sessionData = parsed.data;
      } else {
        // Find active session for current branch
        const branch = getCurrentBranch();
        if (branch === "unknown") {
          console.error(`  ${red("error:")} Not in a git repository`);
          process.exit(1);
        }
        const existing = findActiveSessionForBranch(agentsDir, branch);
        if (!existing) {
          console.error(`  ${red("error:")} No active session found for branch ${branch}`);
          process.exit(1);
        }
        sessionPath = existing.filePath;
        sessionData = existing.data;
      }

      // Validate claude_session_id
      const claudeSessionId = sessionData.claude_session_id;
      if (!claudeSessionId) {
        console.error(`  ${red("error:")} Session has no claude_session_id in frontmatter. Enrichment requires a linked Claude Code session.`);
        console.error(`  ${gray("Session file:")} ${sessionPath}`);
        process.exit(1);
      }

      // Derive JSONL path
      const projectDir = deriveClaudeProjectDir();
      const jsonlPath = join(projectDir, `${claudeSessionId}.jsonl`);

      // Primary path check, then fallback scan
      let resolvedJsonlPath: string | null = null;
      if (existsSync(jsonlPath)) {
        resolvedJsonlPath = jsonlPath;
      } else {
        // Fallback: scan project directory for <session_id>.jsonl
        const targetName = `${claudeSessionId}.jsonl`;
        if (existsSync(projectDir)) {
          try {
            const files = readdirSync(projectDir);
            if (files.includes(targetName)) {
              resolvedJsonlPath = join(projectDir, targetName);
            }
          } catch {
            // Directory not readable — fall through to error
          }
        }
      }

      if (!resolvedJsonlPath) {
        console.error(`  ${red("error:")} JSONL not found at ${jsonlPath}`);
        console.error(`  ${gray("Claude session ID:")} ${claudeSessionId}`);
        console.error(`  ${gray("Project directory:")} ${projectDir}`);
        process.exit(1);
      }

      console.log(`\n  ${bold("loaf session enrich")}${options.dryRun ? gray(" (dry run)") : ""}\n`);
      console.log(`  Session: ${gray(sessionPath.replace(agentsDir, ".agents"))}`);
      console.log(`  Claude session: ${gray(claudeSessionId.slice(0, 8))}`);

      // Extract conversation summary
      const enrichedAt = sessionData.enriched_at;
      const extractionResult = await extractSummary(
        resolvedJsonlPath,
        projectDir,
        claudeSessionId,
        agentsDir,
        enrichedAt,
      );

      if (extractionResult.isEmpty) {
        console.log(`  ${green("✓")} No new entries since last enrichment — nothing to do`);
        console.log();
        process.exit(0);
      }

      // Check claude binary availability (after no-op exit so we don't
      // fail unnecessarily when there's nothing to enrich)
      try {
        execSync('command -v claude', { stdio: 'pipe' });
      } catch {
        console.error(`  ${red("error:")} claude binary not found. Install Claude Code CLI to use enrichment.`);
        process.exit(1);
      }

      console.log(`  Summary: ${gray(extractionResult.summaryPath.replace(agentsDir, ".agents"))}`);
      if (extractionResult.latestTimestamp) {
        console.log(`  Latest entry: ${gray(extractionResult.latestTimestamp)}`);
      }

      // Build enrichment prompt
      const prompt = buildEnrichmentPrompt(sessionPath, extractionResult.summaryPath, !!options.dryRun);

      // Build agent args
      const agentArgs: string[] = [
        '--agent', 'librarian',
        '-p',
        '--no-session-persistence',
        '--permission-mode', 'acceptEdits',
        '--max-turns', '10',
      ];

      if (options.model) {
        agentArgs.push('--model', options.model);
      }

      if (options.dryRun) {
        agentArgs.push('--disallowedTools', 'Edit,Write');
      }

      // Spawn librarian agent with LOAF_ENRICHMENT isolation
      // Prompt is piped via stdin (not positional arg) to handle multi-line
      // content and avoid shell argument length limits.
      console.log(`  ${gray("Spawning librarian agent...")}`);

      const exitCode = await new Promise<number>((resolve) => {
        const childEnv = { ...process.env, LOAF_ENRICHMENT: '1' };
        // In normal mode, ignore stdout to prevent pipe buffer deadlock if the
        // librarian produces >64KB of output. In dry-run mode, pipe stdout so
        // we can echo it to the terminal.
        const childStdio: ['pipe', 'pipe' | 'ignore', 'pipe'] = [
          'pipe',
          options.dryRun ? 'pipe' : 'ignore',
          'pipe',
        ];
        const child = nodeSpawn('claude', agentArgs, {
          env: childEnv,
          stdio: childStdio,
        });

        // Write prompt to stdin and close it
        child.stdin?.write(prompt);
        child.stdin?.end();

        let stderr = '';

        child.stdout?.on('data', (data: Buffer) => {
          if (options.dryRun) {
            process.stdout.write(data);
          }
        });

        child.stderr?.on('data', (data: Buffer) => {
          stderr += data.toString();
        });

        child.on('close', (code) => {
          if (code !== 0 && stderr) {
            // Check for agent-not-found patterns
            if (stderr.includes('agent') && (stderr.includes('not found') || stderr.includes('No such'))) {
              console.error(`  ${red("error:")} Librarian agent not found. Ensure Loaf is installed (loaf install --to claude-code)`);
            } else {
              console.error(`  ${red("error:")} Librarian agent exited with code ${code}`);
              if (stderr.trim()) {
                console.error(`  ${gray(stderr.trim())}`);
              }
            }
          }
          resolve(code ?? 1);
        });

        child.on('error', (err) => {
          if ((err as NodeJS.ErrnoException).code === 'ENOENT') {
            console.error(`  ${red("error:")} claude binary not found. Install Claude Code CLI to use enrichment.`);
          } else {
            console.error(`  ${red("error:")} Failed to spawn librarian agent: ${err.message}`);
          }
          resolve(1);
        });
      });

      if (exitCode !== 0) {
        console.error(`  ${red("error:")} Enrichment failed — watermark NOT advanced`);
        console.log();
        process.exit(1);
      }

      // Advance enriched_at watermark on success
      if (!options.dryRun && extractionResult.latestTimestamp) {
        const lockPath = `${sessionPath}.lock`;
        await acquireLock(lockPath);
        try {
          const freshSession = readSessionFile(sessionPath);
          if (freshSession) {
            freshSession.data.enriched_at = extractionResult.latestTimestamp;
            freshSession.data.last_updated = getTimestamp();
            const newContent = matter.stringify(
              freshSession.content,
              freshSession.data as unknown as Record<string, unknown>,
            );
            writeFileAtomic(sessionPath, newContent);
          }
        } finally {
          releaseLock(lockPath);
        }
        console.log(`  ${green("✓")} Watermark advanced to ${extractionResult.latestTimestamp}`);
      } else if (options.dryRun) {
        console.log(`  ${gray("Dry run — watermark NOT advanced")}`);
      }

      console.log(`  ${green("✓")} Enrichment complete`);
      console.log();
    });

  // ── loaf session list ────────────────────────────────────────────────────

  session
    .command("list")
    .description("List all active and archived sessions")
    .option("--all", "Include archived sessions")
    .action(async (options: { all?: boolean }) => {
      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        console.error(`  ${red("error:")} Could not find .agents/ directory`);
        process.exit(1);
      }

      const sessionsDir = join(agentsDir, "sessions");
      if (!existsSync(sessionsDir)) {
        console.log(`\n  ${gray("No sessions directory found.")}`);
        console.log(`  Run ${cyan("loaf session start")} to create your first session.\n`);
        return;
      }

      console.log(`\n  ${bold("loaf session list")}\n`);

      // Collect active sessions
      const activeSessions: Array<{
        filePath: string;
        data: SessionFrontmatter;
        content: string;
        isArchived: boolean;
      }> = [];

      const files = readdirSync(sessionsDir).filter(f => f.endsWith(".md"));
      for (const file of files) {
        const filePath = join(sessionsDir, file);
        const session = readSessionFile(filePath);
        if (session && session.data.status !== "archived") {
          activeSessions.push({ ...session, filePath, isArchived: false });
        }
      }

      // Collect archived sessions if requested
      const archivedSessions: typeof activeSessions = [];
      if (options.all) {
        const archiveDir = join(sessionsDir, "archive");
        if (existsSync(archiveDir)) {
          const archiveFiles = readdirSync(archiveDir).filter(f => f.endsWith(".md"));
          for (const file of archiveFiles) {
            const filePath = join(archiveDir, file);
            const session = readSessionFile(filePath);
            if (session) {
              archivedSessions.push({ ...session, filePath, isArchived: true });
            }
          }
        }
      }

      // Display active sessions
      if (activeSessions.length === 0) {
        console.log(`  ${gray("No active sessions found.")}`);
      } else {
        console.log(`  ${bold("Active Sessions")}:`);
        for (const s of activeSessions) {
          const statusColor = s.data.status === "active" ? green : yellow;
          const specInfo = s.data.spec ? gray(` (${s.data.spec})`) : "";
          const lastUpdated = s.data.last_updated 
            ? gray(` — updated ${new Date(s.data.last_updated).toLocaleDateString()}`)
            : "";
          console.log(`    ${statusColor("●")} ${s.data.branch}${specInfo}${lastUpdated}`);
          console.log(`      ${gray(s.filePath.replace(agentsDir, ".agents"))}`);
        }
      }

      // Display archived sessions
      if (options.all && archivedSessions.length > 0) {
        console.log();
        console.log(`  ${bold("Archived Sessions")}:`);
        for (const s of archivedSessions) {
          const archivedDate = s.data.archived_at 
            ? gray(` — archived ${new Date(s.data.archived_at).toLocaleDateString()}`)
            : "";
          console.log(`    ${gray("○")} ${s.data.branch}${archivedDate}`);
          console.log(`      ${gray(s.filePath.replace(agentsDir, ".agents"))}`);
        }
      }

      // Summary
      console.log();
      console.log(`  ${activeSessions.length} active${options.all ? `, ${archivedSessions.length} archived` : ""}`);
      console.log();
    });

  // ── loaf session state ───────────────────────────────────────────────────

  const state = session
    .command("state")
    .description("Manage session state snapshot");

  state
    .command("update")
    .alias("--update")
    .description("Update Current State section in active session file")
    .action(async () => {
      // Parse hook JSON from stdin (for agent_id detection)
      const hookInput = await parseHookInput();

      // Skip for subagents (same pattern as session start)
      if (hookInput.agent_id) {
        process.exit(0);
      }

      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        // Hook-safe: no .agents/ directory
        process.exit(0);
      }

      const branch = getCurrentBranch();
      if (branch === "unknown") {
        process.exit(0);
      }

      // Session ID-first lookup, then branch fallback
      const existingSession = (hookInput.session_id
        ? findSessionByClaudeId(agentsDir, hookInput.session_id, branch)
        : null) || findActiveSessionForBranch(agentsDir, branch);
      if (!existingSession) {
        // Hook-safe: no active session
        process.exit(0);
      }

      // Re-read session to get latest content (may have been updated by other hooks)
      const freshSession = readSessionFile(existingSession.filePath);
      if (!freshSession) {
        process.exit(0);
      }

      const stateSection = buildCurrentStateSection(freshSession.content);
      await writeCurrentState(existingSession.filePath, stateSection);
    });

  // ── loaf session context ──────────────────────────────────────────────────

  const context = session
    .command("context")
    .description("Session context for hooks and agents");

  context
    .command("for-prompt")
    .alias("--for-prompt")
    .description("Print implementation principles for UserPromptSubmit hook injection")
    .action(async () => {
      const hookInput = await parseHookInput();

      // Skip for subagents — they have their own instructions
      if (hookInput.agent_id) {
        process.exit(0);
      }

      // Static implementation principles — cached after first injection
      const lines: string[] = [];
      lines.push("[Implementation Principles]");
      lines.push("- When the user's message is a QUESTION, answer it and STOP. Do not implement anything.");
      lines.push("  Wait for explicit instructions before taking action.");
      lines.push("- Create a Task BEFORE any tool use that changes something (Edit, Write, Bash, etc.).");
      lines.push("  No threshold — if it mutates, track it. TaskCompleted events auto-log to the session journal.");
      lines.push("  Create tasks before starting work, update status as you go, mark complete when done.");
      lines.push("- Delegate code changes to agents — orchestrator coordinates, doesn't implement");
      lines.push("- Log decisions: loaf session log \"decision(scope): description\"");
      lines.push("- One concern per agent, parallel when independent");
      lines.push("- Keep session file handoff-ready");

      // Print to stdout — exit 0 means this becomes model context
      console.log(lines.join("\n"));
    });

  context
    .command("for-resumption")
    .alias("--for-resumption")
    .description("Print rich resumption context after compaction (PostCompact hook)")
    .action(async () => {
      const hookInput = await parseHookInput();

      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        process.exit(0);
      }

      const branch = getCurrentBranch();
      if (branch === "unknown") {
        process.exit(0);
      }

      // Session ID-first lookup, then branch fallback
      const existingSession = (hookInput.session_id
        ? findSessionByClaudeId(agentsDir, hookInput.session_id, branch)
        : null) || findActiveSessionForBranch(agentsDir, branch);
      if (!existingSession) {
        console.log("=== POST-COMPACTION RESUMPTION ===");
        console.log("");
        console.log("WARNING: No active session found. Read .agents/sessions/ manually.");
        process.exit(0);
      }

      const freshSession = readSessionFile(existingSession.filePath);
      const sessionContent = freshSession?.content || existingSession.content;
      const sessionPath = existingSession.filePath.replace(agentsDir, ".agents");

      // Header
      console.log("=== POST-COMPACTION RESUMPTION ===");
      console.log("");
      console.log(`Session: ${sessionPath}`);
      console.log(`Branch: ${branch}`);

      // Spec linkage
      if (existingSession.data.spec) {
        const specInfo = findSpecForBranch(agentsDir, branch);
        if (specInfo) {
          console.log(`Spec: ${specInfo.id} — ${specInfo.title}`);
        } else {
          console.log(`Spec: ${existingSession.data.spec}`);
        }
      }

      // Git state
      const lastCommit = getLastCommitSha();
      const uncommitted = getUncommittedCount();
      if (lastCommit !== "unknown") {
        const commits = getRecentCommits(1);
        console.log(`Last commit: ${commits[0]?.hash || lastCommit} — ${commits[0]?.message || ""}`);
      }
      if (uncommitted > 0) {
        console.log(`Uncommitted: ${uncommitted} file${uncommitted === 1 ? "" : "s"}`);
      }

      // Current State (the rich summary written by the model before compaction)
      console.log("");
      const currentState = extractCurrentState(sessionContent);
      if (currentState) {
        console.log(currentState);
      } else {
        console.log("WARNING: No ## Current State was written before compaction.");
        console.log("Read the session file's ## Journal section for context.");
      }

      // Recent journal entries (last 20 for resumption context)
      console.log("");
      console.log("## Recent Journal");
      console.log("");
      const recentEntries = extractRecentEntries(sessionContent, 20);
      if (recentEntries.length > 0) {
        for (const entry of recentEntries) {
          console.log(entry);
        }
      } else {
        console.log("(no journal entries)");
      }

      // Instructions for the model
      console.log("");
      console.log("---");
      console.log("Resume work from where the state summary left off.");
      console.log("Do not ask 'where were we?' — the context above tells you.");
      console.log(`If you need more detail, read the full session file: ${sessionPath}`);
    });

  context
    .command("for-compact")
    .alias("--for-compact")
    .description("Log compact marker and print journal flush instructions (PreCompact hook)")
    .action(async () => {
      const hookInput = await parseHookInput();

      // Skip for subagents
      if (hookInput.agent_id) {
        process.exit(0);
      }

      const agentsDir = findAgentsDir();
      const branch = getCurrentBranch();

      // 1. Log compact marker to session journal (absorbs compact.sh)
      if (agentsDir && branch !== "unknown") {
        const existingSession = (hookInput.session_id
          ? findSessionByClaudeId(agentsDir, hookInput.session_id, branch)
          : null) || findActiveSessionForBranch(agentsDir, branch);

        if (existingSession) {
          const timestamp = getDateTimeString();
          await appendEntry(
            existingSession.filePath,
            [`[${timestamp}] compact(session): context compaction triggered`],
            (data: SessionFrontmatter) => {
              data.last_updated = getTimestamp();
              data.last_entry = getTimestamp();
            }
          );

          // 2. Check Current State staleness
          const freshSession = readSessionFile(existingSession.filePath);
          const content = freshSession?.content || existingSession.content;
          const stateMatch = content.match(/## Current State \((\d{4}-\d{2}-\d{2} \d{2}:\d{2})\)/);

          if (!stateMatch) {
            // No Current State section at all
          } else {
            const tsStr = stateMatch[1];
            const tsParts = tsStr.split(/[-: ]/);
            const tsDate = new Date(
              parseInt(tsParts[0]), parseInt(tsParts[1]) - 1, parseInt(tsParts[2]),
              parseInt(tsParts[3]), parseInt(tsParts[4])
            );
            const ageMinutes = Math.floor((Date.now() - tsDate.getTime()) / 60000);
            if (ageMinutes > 5) {
              // Stale state will be caught by the nudge instructions below
            }
          }
        }
      }

      // 3. Print nudge instructions (injected as context via exit 0)
      const lines: string[] = [];
      lines.push("CONTEXT COMPACTION IMMINENT: Your conversation context will be compacted soon.");
      lines.push("");
      lines.push("REQUIRED — two actions before the model responds:");
      lines.push("");
      lines.push("1. **Flush journal entries.** Log all unrecorded decisions, discoveries, and progress:");
      lines.push("   - `decision(scope): key decisions made this session`");
      lines.push("   - `discover(scope): important findings`");
      lines.push("   - `finding(scope): analysis result`");
      lines.push('   Run `loaf session log "type(scope): description"` for each.');
      lines.push("");
      lines.push("2. **Write state summary.** Replace the session file's `## Current State` heading");
      lines.push("   with `## Current State (YYYY-MM-DD HH:MM)` (current timestamp) and write");
      lines.push("   a structured summary using this format:");
      lines.push("");
      lines.push("   ```");
      lines.push("   ## Current State (YYYY-MM-DD HH:MM)");
      lines.push("");
      lines.push("   **Working on:** spec/task — brief description");
      lines.push("   **Status:** one-line build/test/progress status");
      lines.push("");
      lines.push("   **Done this session:**");
      lines.push("   - bullet per significant change");
      lines.push("");
      lines.push("   **Blocked:** (omit if none)");
      lines.push("   - blockers with context");
      lines.push("");
      lines.push("   **Next:**");
      lines.push("   - immediate follow-ups");
      lines.push("   ```");
      lines.push("");
      lines.push("   This is the resumption context after compaction — write it as if briefing");
      lines.push("   a colleague who just walked in. Be specific (file names, commit hashes,");
      lines.push("   flag names), not vague. The timestamp lets future compactions detect stale");
      lines.push("   summaries. Update `last_updated` frontmatter via `loaf session state update`.");
      lines.push("");
      lines.push("The journal IS your external memory. Entries not flushed now are lost forever.");

      console.log(lines.join("\n"));
    });
}
