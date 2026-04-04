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
}

type EntryType =
  | "resume"
  | "pause"
  | "progress"
  | "commit"
  | "pr"
  | "merge"
  | "decide"
  | "discover"
  | "conclude"
  | "block"
  | "unblock"
  | "spark"
  | "todo"
  | "assume"
  // New types from SPEC-020
  | "branch"
  | "task"
  | "linear"
  | "hypothesis"
  | "try"
  | "reject"
  | "compact";

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
// Session Lifecycle Helpers (SPEC-020 compliance)
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
function getSessionFilePath(agentsDir: string, branch: string, timestamp?: string): string {
  // Use timestamped filename: YYYYMMDD-HHMMSS-slug.md
  const ts = timestamp || getTimestampForFilename();
  const slug = branch.replace(/^feat\//, "").replace(/[^a-zA-Z0-9-_]/g, "-").replace(/-+/g, "-");
  return join(agentsDir, "sessions", `${ts}-${slug}.md`);
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
  // Use a branch-level lock for session creation
  const lockPath = join(sessionsDir, `.create-${branch.replace(/[^a-zA-Z0-9-]/g, '-')}.lock`);
  let lockAcquired = false;
  
  try {
    // Acquire creation lock
    await acquireLock(lockPath, 100, 50);
    lockAcquired = true;
    
    // Re-check for existing session under lock with retry for filesystem sync
    // This handles the race condition where another process just created the file
    let existing: { filePath: string; data: SessionFrontmatter; content: string } | null = null;
    for (let i = 0; i < 10; i++) {
      await new Promise(resolve => setTimeout(resolve, 20));
      existing = findActiveSessionForBranch(agentsDir, branch);
      if (existing) break;
    }
    
    if (existing) {
      return { ...existing, isNew: false };
    }
    
    // Create new session file (or detect if another process created it)
    const filePath = getSessionFilePath(agentsDir, branch);
    const created = createSessionFile(filePath, branch, specInfo);
    
    if (!created) {
      // Another process created the file while we were waiting for the lock
      // Read the existing session
      const existingSession = readSessionFile(filePath);
      if (existingSession) {
        return { filePath, data: existingSession.data, content: existingSession.content, isNew: false };
      }
      // File was created but we couldn't read it - try to create anyway
      const fallbackSession = readSessionFile(filePath);
      if (fallbackSession) {
        return { filePath, data: fallbackSession.data, content: fallbackSession.content, isNew: false };
      }
      // This shouldn't happen, but fall back to treating as new
    }
    
    const session = readSessionFile(filePath)!;
    
    return { filePath, data: session.data, content: session.content, isNew: true };
  } finally {
    if (lockAcquired) {
      releaseLock(lockPath);
    }
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
    "resume", "pause", "progress", "commit", "pr", "merge",
    "decide", "discover", "conclude", "block", "unblock",
    "spark", "todo", "assume",
    // New types
    "branch", "task", "linear", "hypothesis", "try", "reject", "compact"
  ];

  if (!validTypes.includes(type as EntryType)) return null;

  return {
    type: type as EntryType,
    scope,
    description,
  };
}

/** Format entry for journal */
function formatEntry(entry: JournalEntry): string {
  const scope = entry.scope ? `(${entry.scope})` : "";
  return `- ${entry.type}${scope}: ${entry.description}`;
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

  const title = specInfo ? `${specInfo.id}: ${specInfo.title}` : branch;
  const entry = `- ${getDateTimeString()} resume(${branch}): session started`;
  
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

/** Append inline journal entry to session file (atomic with file locking) */
async function appendEntry(
  filePath: string,
  entryLines: string[],
  updateFrontmatter: (data: SessionFrontmatter) => void
): Promise<void> {
  const lockPath = `${filePath}.lock`;
  let lockAcquired = false;
  
  try {
    // Acquire exclusive lock
    await acquireLock(lockPath);
    lockAcquired = true;
    
    // Read fresh state under lock
    const session = readSessionFile(filePath);
    if (!session) {
      throw new Error("Session file not found");
    }

    // Update frontmatter
    updateFrontmatter(session.data);

    // Build new content: add blank line before entry if needed
    const trimmedContent = session.content.trimEnd();
    const separator = trimmedContent.endsWith('\n') ? '\n' : '\n\n';
    const newContent = matter.stringify(
      trimmedContent + separator + entryLines.join("\n") + "\n",
      session.data as unknown as Record<string, unknown>
    );

    // Atomic write
    writeFileAtomic(filePath, newContent);
  } finally {
    // Only release lock if we acquired it
    if (lockAcquired) {
      releaseLock(lockPath);
    }
  }
}

/** Extract recent journal entries for context display (compact inline format) */
function extractRecentEntries(content: string, count = 15): string[] {
  const lines = content.split("\n");
  const entries: string[] = [];

  // Match inline journal entries: "- YYYY-MM-DD HH:MM type(scope): description"
  const entryPattern = /^- \d{4}-\d{2}-\d{2} \d{2}:\d{2} /;

  for (const line of lines) {
    if (entryPattern.test(line)) {
      entries.push(line.trim());
    }
  }

  // Return last 'count' entries
  return entries.slice(-count);
}

/** Extract decide entries from session (compact inline format) */
function extractDecideEntries(content: string): string[] {
  const decideEntries: string[] = [];
  const lines = content.split("\n");

  // Match inline format: "- YYYY-MM-DD HH:MM decide(scope): description"
  for (const line of lines) {
    if (line.match(/^- \d{4}-\d{2}-\d{2} \d{2}:\d{2} decide\([^)]+\):/)) {
      decideEntries.push(line.trim());
    }
  }

  return decideEntries;
}

/** Persist decisions to spec changelog */
function persistDecisionsToSpec(
  agentsDir: string,
  specId: string,
  decideEntries: string[],
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
  const entryHeader = `- ${dateStr} — Session ${sessionBranch} archived: ${decideEntries.length} decision(s) extracted`;
  
  let updatedContent: string;
  
  if (changelogMatch) {
    // Insert after "## Changelog\n"
    const insertPos = changelogMatch.index! + changelogMatch[0].length;
    const decisionsList = decideEntries.map((e) => `  ${e}`).join("\n");
    const entry = `${entryHeader}\n${decisionsList}\n\n`;
    updatedContent = specContent.slice(0, insertPos) + entry + specContent.slice(insertPos);
  } else {
    // Append Changelog section at end
    const decisionsList = decideEntries.map((e) => `  ${e}`).join("\n");
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
    .action(async () => {
      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        console.error(`  ${red("error:")} Could not find .agents/ directory`);
        process.exit(1);
      }

      // Ensure sessions directory exists
      const sessionsDir = join(agentsDir, "sessions");
      if (!existsSync(sessionsDir)) {
        mkdirSync(sessionsDir, { recursive: true });
      }

      // Detect current branch
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

      // Find linked spec
      const specInfo = findSpecForBranch(agentsDir, branch);
      if (specInfo) {
        console.log(`  Linked to: ${bold(specInfo.id)} — ${specInfo.title}`);
      } else {
        console.log(`  ${yellow("!")} No linked spec found — creating ad-hoc session`);
      }

      // Atomically get existing session or create new one
      const { filePath: sessionFilePath, data: sessionData, content: sessionContent, isNew } = 
        await getOrCreateSession(agentsDir, branch, specInfo);
      
      if (isNew) {
        console.log(`  ${green("+")} Creating new session file`);
        
        // Update linked spec with session filename (for branch rename recovery)
        if (specInfo) {
          const sessionFileName = basename(sessionFilePath);
          updateSpecSessionField(agentsDir, specInfo.id, sessionFileName);
        }
      } else {
        console.log(`  ${green("✓")} Resuming existing session`);
      }

      // Compute state
      const lastCommit = getLastCommitSha();
      const commits = getRecentCommits(3);
      const { completed, total } = countTasksInSession(sessionContent);

      // Build journal entries (inline format per SPEC-020)
      // For NEW sessions: createSessionFile() already added initial resume entry
      // For EXISTING sessions: only add resume entry if the session was not already active
      const timestamp = getDateTimeString();
      const journalLines: string[] = [];
      
      const shouldAppendResume = !isNew && sessionData.status !== "active";
      
      if (shouldAppendResume) {
        journalLines.push(`- ${timestamp} resume(${branch}): session resumed`);
        if (lastCommit !== "unknown") {
          journalLines.push(`- ${timestamp} resume(${branch}): from commit ${lastCommit}`);
        }
        if (completed > 0 || total > 0) {
          journalLines.push(`- ${timestamp} progress: ${completed}/${total} tasks completed`);
        }
        if (commits.length > 0) {
          for (const commit of commits.slice(0, 3)) {
            journalLines.push(`- ${timestamp} commit(${commit.hash}): "${commit.message}"`);
          }
        }
      }

      // Append entries to journal (only if we built some)
      if (journalLines.length > 0) {
        await appendEntry(
          sessionFilePath,
          journalLines,
          (data: SessionFrontmatter) => {
            data.status = "active";
            data.last_updated = getTimestamp();
            data.last_entry = getTimestamp();
          }
        );
      } else if (sessionData.status !== "active" || isNew) {
        // If we didn't append anything, we still might need to update the status for new sessions
        // (Though if it's new, it's already set to active, but we'll update timestamps)
        sessionData.status = "active";
        sessionData.last_updated = getTimestamp();
        sessionData.last_entry = getTimestamp();
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

      // Output recent entries for context
      const currentSessionContent = readSessionFile(sessionFilePath)?.content || sessionContent;
      const recentEntries = extractRecentEntries(currentSessionContent, 15);
      if (recentEntries.length > 0) {
        console.log(`\n  ${bold("Recent journal entries:")}\n`);
        for (const entry of recentEntries.slice(0, 20)) {
          const lines = entry.split("\n");
          for (const line of lines.slice(0, 5)) {
            console.log(`    ${line}`);
          }
          if (lines.length > 5) {
            console.log(`    ${gray("...")}`);
          }
        }
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

      // Find active session by branch lookup
      let existingSession = findActiveSessionForBranch(agentsDir, branch);
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
      
      const sessionFilePath = existingSession.filePath;
      const session = existingSession;

      console.log(`\n  ${bold("loaf session end")}\n`);

      // Compute progress
      const lastCommit = getLastCommitSha();
      const commits = getRecentCommits(5);
      const { completed, total } = countTasksInSession(session.content);
      const commitCount = commits.length;

      // Build pause entries (inline format per SPEC-020)
      const timestamp = getDateTimeString();
      const journalLines: string[] = [
        `- ${timestamp} pause(${branch}): session paused`,
        `- ${timestamp} progress: ${completed}/${total} tasks completed, ${commitCount} commits`,
      ];
      if (lastCommit !== "unknown") {
        journalLines.push(`- ${timestamp} conclude(${branch}): at commit ${lastCommit}`);
      }

      // Prompt for final entries
      console.log(`  ${yellow("?")} Consider adding final entries:`);
      console.log(`    ${gray("loaf session log \"decide(scope): key decision\"")}`);
      console.log(`    ${gray("loaf session log \"conclude(scope): final notes\"")}`);
      console.log(`    ${gray("loaf session log \"todo(next): follow-up task\"")}`);
      console.log();

      // Append pause entries
      await appendEntry(
        sessionFilePath,
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

      console.log(`  ${green("✓")} Session paused: ${gray(sessionFilePath.replace(agentsDir, ".agents"))}`);
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

      // Find active session by branch lookup
      let existingSession = findActiveSessionForBranch(agentsDir, branch);
      if (!existingSession) {
        // For hook calls, no-op gracefully instead of erroring
        // This prevents hooks from failing when no session exists
        if (options.fromHook) {
          process.exit(0);
        }
        console.error(`  ${red("error:")} No active session found for branch ${branch}. Run 'loaf session start' first.`);
        process.exit(1);
      }
      
      const sessionFilePath = existingSession.filePath;
      const session = existingSession;

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
              // Extract commit message if available
              const msgMatch = command.match(/-m\s+['"]([^'"]+)['"]/) || command.match(/-m\s+(\S+)/);
              const message = msgMatch ? msgMatch[1] : "commit";
              const hash = getLastCommitSha();
              entryText = `commit(${hash}): ${message}`;
            } else if (command.includes("gh pr create")) {
              // Extract PR title from command
              const titleMatch = command.match(/--title\s+["']([^"']+)["']/);
              const title = titleMatch ? titleMatch[1] : "";
              
              // Try to extract PR number from hook output (if available)
              // The raw field may contain tool output with PR URL
              const raw = hookData.raw;
              let prNum = "";
              if (raw && typeof raw === "string") {
                // Match PR URL patterns like https://github.com/owner/repo/pull/123
                const prUrlMatch = raw.match(/https:\/\/github\.com\/[^\/]+\/[^\/]+\/pull\/(\d+)/);
                if (prUrlMatch) {
                  prNum = prUrlMatch[1];
                }
              }
              
              if (prNum && title) {
                entryText = `pr(#${prNum}): ${title}`;
              } else if (title) {
                entryText = `pr: ${title}`;
              } else {
                entryText = `pr: PR created`;
              }
            } else if (command.includes("gh pr merge")) {
              // Try to extract PR number from URL or direct argument
              const prMatch = command.match(/merge\s+https:\/\/github\.com\/[^\/]+\/[^\/]+\/pull\/(\d+)/) ||
                               command.match(/pr\s+merge\s+(\d+)/);
              const prNum = prMatch ? prMatch[1] : "unknown";
              entryText = `merge(#${prNum}): merged`;
            } else {
              // Generic entry for unrecognized command
              entryText = `progress(hook): ${command.slice(0, 100)}${command.length > 100 ? "..." : ""}`;
            }
          } else {
            // Fallback: try legacy fields for backward compatibility
            if (hookData.commit) {
              entryText = `commit(${hookData.commit}): ${hookData.message || "commit"}`;
            } else if (hookData.pr) {
              entryText = `pr(#${hookData.pr}): ${hookData.title || "PR created"}`;
            } else if (hookData.merge) {
              entryText = `merge(#${hookData.merge}): merged`;
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
        console.error(`    ${gray("loaf session log \"decide(hooks): remove bash wrappers\"")}`);
        console.error(`    ${gray("loaf session log \"todo(next): implement tests\"")}`);
        process.exit(1);
      }

      // Validate entry format
      const parsedEntry = parseEntry(entryText);
      if (!parsedEntry) {
        console.error(`  ${red("error:")} Invalid entry format. Use: type(scope): description`);
        console.error(`  ${gray("Examples:")}`);
        console.error(`    ${gray("loaf session log \"decide(hooks): remove bash wrappers\"")}`);
        console.error(`    ${gray("loaf session log \"todo(next): implement tests\"")}`);
        process.exit(1);
      }

      // Append entry (inline format with timestamp)
      const timestamp = getDateTimeString();
      const formattedEntry = `- ${timestamp} ${entryText}`;
      
      await appendEntry(
        sessionFilePath,
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

      // Find active session by branch lookup
      let existingSession = findActiveSessionForBranch(agentsDir, branch);
      if (!existingSession) {
        console.error(`  ${red("error:")} No active session found for branch ${branch}`);
        process.exit(1);
      }
      
      const sessionFilePath = existingSession.filePath;
      const session = existingSession;

      console.log(`\n  ${bold("loaf session archive")}\n`);

      // Ensure archive directory exists
      const archiveDir = join(agentsDir, "sessions", "archive");
      if (!existsSync(archiveDir)) {
        mkdirSync(archiveDir, { recursive: true });
      }

      // Extract key decisions
      const decideEntries = extractDecideEntries(session.content);
      if (decideEntries.length > 0) {
        console.log(`  ${bold("Key decisions extracted:")}`);
        for (const entry of decideEntries.slice(0, 10)) {
          console.log(`    ${entry}`);
        }
        if (decideEntries.length > 10) {
          console.log(`    ${gray(`... and ${decideEntries.length - 10} more`)}`);
        }
        console.log();

        // Persist to spec changelog if linked
        if (session.data.spec && decideEntries.length > 0) {
          const result = persistDecisionsToSpec(
            agentsDir,
            session.data.spec,
            decideEntries,
            session.data.branch || branch
          );
          if (result.success) {
            console.log(`  ${green("✓")} ${result.message}`);
          } else {
            console.log(`  ${yellow("⚠")} Could not persist to spec: ${result.message}`);
          }
          console.log();
        }
      }

      // Move to archive first (atomic on same filesystem), then update frontmatter.
      // This avoids a corruption window where the file has status: archived but is
      // still in sessions/ — if the process crashes between write and rename the
      // session becomes invisible to both `session list` and the archive directory.
      const fileName = basename(sessionFilePath);
      const archivePath = join(archiveDir, fileName);

      try {
        renameSync(sessionFilePath, archivePath);
      } catch (err) {
        console.error(`  ${red("error:")} Failed to move file: ${err}`);
        process.exit(1);
      }

      // Now update frontmatter in the already-archived file
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
