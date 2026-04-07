/**
 * loaf session command
 *
 * Session journal management for tracking work state and continuity.
 */

import { Command } from "commander";
import { execSync } from "child_process";
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
  status: "active" | "paused" | "blocked" | "complete" | "archived";
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
}

/** Hook JSON input from Claude Code stdin */
interface HookInput {
  session_id?: string;
  agent_id?: string;
  agent_type?: string;
  transcript_path?: string;
  cwd?: string;
  permission_mode?: string;
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
  | "progress"
  | "commit"
  | "pr"
  | "merge"
  | "decision"
  | "discover"
  | "conclude"
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
  | "skill";

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

/** Check if lock file is stale (older than threshold) */
function isLockStale(lockPath: string): boolean {
  try {
    const stats = statSync(lockPath);
    const age = Date.now() - stats.mtimeMs;
    return age > LOCK_STALENESS_THRESHOLD;
  } catch {
    // If we can't read the lock file, assume it's stale
    return true;
  }
}

/** Force-release a stale lock */
function forceReleaseLock(lockPath: string): void {
  try {
    unlinkSync(lockPath);
    console.log(`  Released stale lock: ${lockPath}`);
  } catch {
    // Ignore errors
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
    } catch {
      // Lock file exists (and is not stale), wait and retry
      await new Promise(resolve => setTimeout(resolve, retryDelay));
    }
  }
  throw new Error(`Could not acquire lock after ${maxRetries} retries: ${lockPath}`);
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
}

function getRecentCommits(count = 5): Commit[] {
  try {
    const output = execSync(`git log --pretty=format:"%H %s" -${count}`, {
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
    }).trim();

    if (!output) return [];

    return output.split("\n").map((line) => {
      const spaceIndex = line.indexOf(" ");
      return {
        hash: line.substring(0, 7), // Short hash
        message: line.substring(spaceIndex + 1),
      };
    });
  } catch {
    return [];
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

/** Get session file path (timestamped filename per SPEC-020) */
function getSessionFilePath(agentsDir: string): string {
  return join(agentsDir, "sessions", `${getTimestampForFilename()}-session.md`);
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
  
  if (candidates.length === 0) return null;
  
  // Prioritize: active > paused/blocked/complete > others
  // Sort by status priority (lower number = higher priority)
  const statusPriority: Record<string, number> = {
    active: 1,
    paused: 2,
    blocked: 2,
    complete: 2,
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
  specInfo: { id: string; title: string } | null
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
    createSessionFile(filePath, branch, specInfo);
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
    "start", "resume", "pause", "progress", "commit", "pr", "merge",
    "decision", "discover", "conclude", "block", "unblock",
    "spark", "todo", "assume",
    // New types
    "branch", "task", "linear", "hypothesis", "try", "reject", "compact",
    "skill"
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
  specInfo: { id: string; title: string } | null
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

  const title = specInfo ? `${specInfo.id}: ${specInfo.title}` : "Ad-hoc";
  const entry = `[${getDateTimeString()}] session(start):  === SESSION STARTED ===`;

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
  updateFrontmatter: (data: SessionFrontmatter) => void
): Promise<void> {
  const lockPath = `${filePath}.lock`;
  await acquireLock(lockPath);
  try {
    const session = readSessionFile(filePath);
    if (!session) {
      throw new Error("Session file not found");
    }

    updateFrontmatter(session.data);

    // Blank line before session(resume) entries (visual separation — STOPPED always has trailing blank line)
    const trimmedContent = session.content.trimEnd();
    const hasNewSession = entryLines.some(line =>
      /session\(resume\):/.test(line)
    );

    const separator = hasNewSession ? '\n\n' : '\n';
    const newContent = matter.stringify(
      trimmedContent + separator + entryLines.join("\n") + "\n",
      session.data as unknown as Record<string, unknown>
    );

    writeFileAtomic(filePath, newContent);
  } finally {
    releaseLock(lockPath);
  }
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

function quickArchiveSession(filePath: string, agentsDir: string): void {
  const archiveDir = join(agentsDir, "sessions", "archive");
  if (!existsSync(archiveDir)) {
    mkdirSync(archiveDir, { recursive: true });
  }

  try {
    const session = readSessionFile(filePath);
    if (session) {
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

      const existingSession = findActiveSessionForBranch(agentsDir, branch);

      let sessionFilePath: string;
      let sessionData: SessionFrontmatter;
      let sessionContent: string;
      let isResume = false;
      let isNewConversation = false;

      if (existingSession && (options.resume || existingSession.data.status === "active")) {
        // --resume flag, or session is active (wasn't properly closed)
        sessionFilePath = existingSession.filePath;
        sessionData = existingSession.data;
        sessionContent = existingSession.content;

        // Detect new conversation via claude_session_id comparison
        const storedSessionId = existingSession.data.claude_session_id;
        if (hookInput.session_id && storedSessionId && hookInput.session_id !== storedSessionId) {
          // Different session_id = new conversation on same branch
          isNewConversation = true;
          isResume = true;
        } else {
          isResume = existingSession.data.status !== "active";
        }

        console.log(`  ${green("✓")} Resuming existing session`);
      } else {
        if (existingSession) {
          quickArchiveSession(existingSession.filePath, agentsDir);
          console.log(`  ${yellow("!")} Closed previous session`);
        }

        const result = await getOrCreateSession(agentsDir, branch, specInfo);
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
        journalLines.push(`[${timestamp}] session(resume): === SESSION RESUMED ===`);
        if (lastCommit !== "unknown") {
          journalLines.push(`[${timestamp}] session(context): from commit ${lastCommit}`);
        }
        if (completed > 0 || total > 0) {
          journalLines.push(`[${timestamp}] session(progress): ${completed}/${total} tasks completed`);
        }
        if (commits.length > 0) {
          for (const commit of commits.slice(0, 3)) {
            if (!sessionContent.includes(`commit(${commit.hash})`)) {
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

      console.log();
    });

  // ── loaf session end ───────────────────────────────────────────────────────

  session
    .command("end")
    .description("End session with progress summary")
    .option("--if-active", "Exit successfully when no active session exists")
    .action(async (options: { ifActive?: boolean }) => {
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

      const existingSession = findActiveSessionForBranch(agentsDir, branch);
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

      const journalLines: string[] = [
        `[${timestamp}] session(conclude): ${concludeText}`,
        `[${timestamp}] session(stop):   === SESSION STOPPED ===`,
        '',
      ];
      console.log(`  ${yellow("?")} Consider adding final entries:`);
      console.log(`    ${gray("loaf session log \"decision(scope): key decision\"")}`);
      console.log(`    ${gray("loaf session log \"conclude(scope): final notes\"")}`);
      console.log(`    ${gray("loaf session log \"todo(next): follow-up task\"")}`);
      console.log();

      await appendEntry(
        existingSession.filePath,
        journalLines,
        (data: SessionFrontmatter) => {
          data.status = "paused";
          data.last_updated = getTimestamp();
          data.last_entry = getTimestamp();
        }
      );

      const knowledgeFiles = consumeKnowledgeNudges(findGitRoot(), branch);
      if (knowledgeFiles.length > 0) {
        console.log(`  ${yellow("⚠")} Knowledge consolidation recommended for ${knowledgeFiles.length} file(s):`);
        for (const file of knowledgeFiles) {
          console.log(`    ${gray(file)}`);
        }
        console.log(`    ${gray("Run 'loaf kb review <file>' before ending.")}`);
        console.log();
      }

      console.log(`  ${green("✓")} Session stopped: ${gray(existingSession.filePath.replace(agentsDir, ".agents"))}`);
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
          
          // Extract command from generic tool payload
          // Support both flat (tool_input) and nested (tool.input) formats for cross-harness compatibility
          const command = hookData.tool_input?.command || 
                          hookData.tool?.input?.command || 
                          hookData.input?.command;
          
          if (command && typeof command === "string") {
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
          } else {
            // Fallback: try legacy fields for backward compatibility
            if (hookData.commit) {
              entryText = `commit(${hookData.commit}): ${hookData.message || "commit"}`;
            } else if (hookData.pr) {
              entryText = `pr(create): ${hookData.title || "PR created"} (#${hookData.pr})`;
            } else if (hookData.merge) {
              entryText = `pr(merge): #${hookData.merge}`;
            } else {
              // No command found - this is a hook-safe no-op for when session doesn't exist
              // Exit 0 so hooks don't fail, but don't log anything
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
      await appendEntry(
        existingSession.filePath,
        [formattedEntry],
        (data: SessionFrontmatter) => {
          data.last_updated = getTimestamp();
          data.last_entry = getTimestamp();
        }
      );

      console.log(`  ${green("✓")} Logged: ${cyan(entryText)}`);
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
}
