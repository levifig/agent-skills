/**
 * Symlink Enforcement for Loaf Install
 *
 * Shared helper for creating and repairing the symlinks that consolidate
 * project prompt-overlay files around a single canonical file.
 *
 * The Loaf convention:
 *   - `.agents/AGENTS.md` is the canonical source of truth.
 *   - `.claude/CLAUDE.md` symlinks to it so Claude Code sees the same content.
 *   - `./AGENTS.md` at the project root symlinks to it for agents.md spec
 *     compatibility (tools look at the project root for AGENTS.md).
 *
 * Without these symlinks, `loaf install` writes fenced sections to both
 * real files, which drift as the framework version evolves.
 *
 * When a real file is already present at one of the symlink paths, we
 * migrate its non-fence content into the canonical `.agents/AGENTS.md`
 * before replacing it with a symlink. A `.bak` sibling is always written
 * so no user content is lost, and we dedupe or collide headings by simply
 * appending under a `## Migrated from <path>` heading — the user resolves
 * the merge.
 *
 * This module is pure logic plus an injected prompt function, so it is
 * trivially testable. `install.ts` and `init.ts` share the same state
 * machine rather than duplicating it.
 */

import {
  existsSync,
  lstatSync,
  mkdirSync,
  readFileSync,
  readlinkSync,
  renameSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "fs";
import { dirname, isAbsolute, join, relative, resolve } from "path";
import { createInterface } from "readline";

import { findFencedSection } from "./fenced-section.js";

/** Outcome of a single ensureSymlink call. */
export type EnsureSymlinkAction =
  | "created"
  | "already-correct"
  | "relinked"
  | "declined-relink"
  | "replaced-file"
  | "declined-replace"
  | "skipped-no-tty"
  | "error";

export interface EnsureSymlinkResult {
  action: EnsureSymlinkAction;
  /** Human-readable one-line message describing what happened. */
  message: string;
  /** If a real file was backed up, path to the .bak file. */
  backupPath?: string;
  /** True if user content was merged into the canonical file during migration. */
  merged?: boolean;
  /** On error, the thrown reason. */
  error?: string;
}

/** Options for ensureSymlink. */
export interface EnsureSymlinkOptions {
  /**
   * Prompt function used to ask the user yes/no questions interactively.
   * Must return `true` for "yes", `false` for "no". Injected so tests can
   * stub without touching stdin/readline.
   */
  prompt?: (question: string) => Promise<boolean>;

  /**
   * If true, never prompt — skip any state that would require confirmation
   * and return a skipped-no-tty result. Retained for callers that still
   * need the explicit "do nothing" escape hatch.
   */
  nonInteractive?: boolean;

  /**
   * If true, treat every prompt as an implicit "yes" and perform the safe
   * migration (merge → .bak → symlink) without asking. Set automatically
   * when stdin is not a TTY, and by the `--yes` flag on `loaf install`.
   */
  assumeYes?: boolean;

  /**
   * Absolute path to the canonical `.agents/AGENTS.md`. Required when
   * `assumeYes` is set and a real file may need migrating — the helper
   * merges stripped content into this file. If omitted, migration falls
   * back to a plain back-up-and-symlink (legacy behaviour).
   */
  canonicalPath?: string;

  /**
   * Project root used to compute short relative paths for the
   * `## Migrated from <path>` heading. Defaults to the parent of
   * `linkPath` — callers that want tidy headings (e.g. project-root
   * style) should pass the real project root.
   */
  projectRoot?: string;
}

/**
 * Resolve a symlink target and normalise it to an absolute path.
 * Returns null on any read error.
 */
function resolveSymlinkTarget(linkPath: string): string | null {
  try {
    const target = readlinkSync(linkPath);
    if (isAbsolute(target)) return target;
    return resolve(dirname(linkPath), target);
  } catch {
    return null;
  }
}

/** True if `path` exists as a file, dir, or symlink (broken links included). */
function pathExists(path: string): boolean {
  try {
    lstatSync(path);
    return true;
  } catch {
    return false;
  }
}

/** True if `path` is a symlink (broken or not). */
function isSymlink(path: string): boolean {
  try {
    return lstatSync(path).isSymbolicLink();
  } catch {
    return false;
  }
}

/**
 * Compare a symlink's resolved target against an expected absolute path.
 * Normalises both sides by resolving relative segments.
 */
function symlinkPointsTo(linkPath: string, expectedAbs: string): boolean {
  const resolved = resolveSymlinkTarget(linkPath);
  if (!resolved) return false;
  return resolve(resolved) === resolve(expectedAbs);
}

/** Default readline-based prompt. Resolves to `false` on EOF or non-y input. */
function defaultPrompt(question: string): Promise<boolean> {
  if (!process.stdin.isTTY) {
    return Promise.resolve(false);
  }
  const rl = createInterface({
    input: process.stdin,
    output: process.stdout,
  });
  return new Promise((resolvePromise) => {
    let answered = false;
    rl.on("close", () => {
      if (!answered) {
        answered = true;
        resolvePromise(false);
      }
    });
    rl.question(question, (answer) => {
      answered = true;
      rl.close();
      resolvePromise(answer.trim().toLowerCase().startsWith("y"));
    });
  });
}

/**
 * Strip any Loaf-managed fenced section from `content`, returning the
 * surrounding text with trim. If the fence spans the entire file (with
 * only whitespace outside), the result is an empty string.
 */
export function stripLoafFence(content: string): string {
  const fence = findFencedSection(content);
  if (!fence) return content.trim();

  const before = content.substring(0, fence.startIndex);
  const after = content.substring(fence.endIndex);
  return (before + after).trim();
}

/**
 * Merge `strippedContent` into the canonical file at `canonicalPath`.
 *   - If canonical is missing, its body becomes the stripped content.
 *   - If canonical exists, stripped content is appended under a
 *     `## Migrated from <relSourcePath>` heading.
 * Heading collisions are NOT deduped — the user resolves them.
 *
 * Returns true if anything was written, false if the stripped content was
 * empty (nothing to migrate).
 */
export function mergeContentIntoCanonical(
  canonicalPath: string,
  strippedContent: string,
  relSourcePath: string,
): boolean {
  if (strippedContent.length === 0) return false;

  if (!existsSync(canonicalPath)) {
    mkdirSync(dirname(canonicalPath), { recursive: true });
    writeFileSync(canonicalPath, strippedContent + "\n");
    return true;
  }

  const existing = readFileSync(canonicalPath, "utf-8");
  const trimmedExisting = existing.replace(/\s+$/, "");
  const heading = `## Migrated from ${relSourcePath}`;
  const appended =
    trimmedExisting +
    "\n\n" +
    heading +
    "\n\n" +
    strippedContent +
    "\n";
  writeFileSync(canonicalPath, appended);
  return true;
}

/**
 * Ensure `linkPath` is a symlink pointing at `relativeTarget` (interpreted
 * relative to the link's own directory, matching the convention in init.ts
 * and the doctor fix hook).
 *
 * State machine:
 *   1. linkPath does not exist      → create symlink
 *   2. linkPath is correct symlink  → already-correct (silent)
 *   3. linkPath is wrong symlink    → prompt to relink (auto-yes under assumeYes)
 *   4. linkPath is a real file      → prompt to merge content + back up + symlink
 *                                     (auto-yes under assumeYes)
 *
 * In pure non-interactive mode (nonInteractive: true, assumeYes: false),
 * steps 3 and 4 are skipped with a `skipped-no-tty` action.
 *
 * `description` is a short human label (e.g. ".claude/CLAUDE.md") used in
 * prompts and output messages. Pass it relative to the project root for
 * readability, not the absolute on-disk path.
 */
export async function ensureSymlink(
  linkPath: string,
  relativeTarget: string,
  description: string,
  options: EnsureSymlinkOptions = {},
): Promise<EnsureSymlinkResult> {
  const prompt = options.prompt ?? defaultPrompt;
  const nonInteractive = options.nonInteractive ?? false;
  const assumeYes = options.assumeYes ?? false;

  // Resolve the expected absolute target for comparison. The symlink itself
  // will still be written with the relative path for portability.
  const expectedAbs = resolve(dirname(linkPath), relativeTarget);

  // State 1: Nothing at linkPath → create.
  if (!pathExists(linkPath)) {
    try {
      const parent = dirname(linkPath);
      if (!existsSync(parent)) {
        mkdirSync(parent, { recursive: true });
      }
      symlinkSync(relativeTarget, linkPath);
      return {
        action: "created",
        message: `Created ${description} -> ${relativeTarget}`,
      };
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      return {
        action: "error",
        message: `Failed to create ${description}: ${msg}`,
        error: msg,
      };
    }
  }

  // State 2 / 3: linkPath exists as a symlink.
  if (isSymlink(linkPath)) {
    if (symlinkPointsTo(linkPath, expectedAbs)) {
      return {
        action: "already-correct",
        message: `${description} already points to ${relativeTarget}`,
      };
    }

    // Wrong target. Needs confirmation unless assumeYes.
    const actualTarget = resolveSymlinkTarget(linkPath) ?? "<unreadable>";
    let approved = assumeYes;
    if (!approved) {
      if (nonInteractive) {
        return {
          action: "skipped-no-tty",
          message:
            `${description} points to the wrong target (${actualTarget}); ` +
            `skipped in non-interactive mode`,
        };
      }
      approved = await prompt(
        `  ${description} points to ${actualTarget}, not ${relativeTarget}. Relink? [y/N] `,
      );
    }
    if (!approved) {
      return {
        action: "declined-relink",
        message: `Left ${description} pointing at ${actualTarget}`,
      };
    }

    try {
      rmSync(linkPath, { force: true });
      symlinkSync(relativeTarget, linkPath);
      return {
        action: "relinked",
        message: `Relinked ${description} -> ${relativeTarget}`,
      };
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      return {
        action: "error",
        message: `Failed to relink ${description}: ${msg}`,
        error: msg,
      };
    }
  }

  // State 4: linkPath exists as a regular file (or directory, which we
  // treat like a real file — never auto-deleted). Safe migration path:
  // strip fence → merge → .bak → symlink. Prompt unless assumeYes.
  let approved = assumeYes;
  if (!approved) {
    if (nonInteractive) {
      return {
        action: "skipped-no-tty",
        message:
          `${description} exists as a real file; skipped in non-interactive ` +
          `mode (fenced sections may drift between it and .agents/AGENTS.md)`,
      };
    }
    approved = await prompt(
      `  ${description} exists as a regular file. Without the symlink, ` +
        `loaf install writes fenced sections to BOTH paths; they may drift. ` +
        `Merge its content into .agents/AGENTS.md, back it up as ` +
        `${description}.bak, and replace with a symlink? [y/N] `,
    );
  }
  if (!approved) {
    return {
      action: "declined-replace",
      message:
        `Left ${description} as a regular file (fenced sections may drift)`,
    };
  }

  const backupPath = `${linkPath}.bak`;
  try {
    // 1. Read the source file's contents and strip any Loaf-managed fence
    //    so we don't duplicate the framework section into the canonical
    //    file — installFencedSection will re-add a single authoritative
    //    fence after this step.
    const sourceContent = readFileSync(linkPath, "utf-8");
    const stripped = stripLoafFence(sourceContent);

    // 2. If we have a canonical path, merge user content into it.
    //    The heading uses a path relative to the project root for
    //    readability (falls back to linkPath's basename if not provided).
    let merged = false;
    const canonicalPath = options.canonicalPath;
    if (canonicalPath && stripped.length > 0) {
      const root = options.projectRoot ?? dirname(linkPath);
      const relSource = relative(root, linkPath) || linkPath;
      merged = mergeContentIntoCanonical(canonicalPath, stripped, relSource);
    }

    // 3. Back up the original file. If a stale .bak already exists from a
    //    previous attempt, overwrite it — the real file is the user's
    //    current content and takes precedence.
    if (pathExists(backupPath)) {
      rmSync(backupPath, { force: true, recursive: true });
    }
    renameSync(linkPath, backupPath);

    // 4. Defensive: before creating the symlink, make sure the canonical
    //    file exists. The stripped content may have been empty (fence-only
    //    source), in which case mergeContentIntoCanonical is a no-op and
    //    canonicalPath may still be absent. Creating a dangling symlink
    //    here would make the next fenced-section read fail with ENOENT.
    //    ensureProjectSymlinks pre-creates canonical, but ensureSymlink
    //    must stay safe when called directly.
    if (canonicalPath && !existsSync(canonicalPath)) {
      mkdirSync(dirname(canonicalPath), { recursive: true });
      writeFileSync(canonicalPath, "");
    }

    // 5. Create the symlink in place of the original file.
    symlinkSync(relativeTarget, linkPath);

    const migrationSuffix = merged
      ? ` (merged content into canonical)`
      : "";
    return {
      action: "replaced-file",
      message:
        `Backed up ${description} to ${description}.bak and created ` +
        `symlink -> ${relativeTarget}${migrationSuffix}`,
      backupPath,
      merged,
    };
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    return {
      action: "error",
      message: `Failed to replace ${description}: ${msg}`,
      error: msg,
    };
  }
}

/**
 * Targets whose fenced sections land in `.agents/AGENTS.md`. When any of
 * these is installed, the root `./AGENTS.md` symlink should be enforced so
 * tools scanning the project root see the same content.
 */
export const AGENTS_MD_TARGETS = new Set([
  "cursor",
  "codex",
  "opencode",
  "amp",
  "gemini",
]);

/** True if the selected targets require the root AGENTS.md symlink. */
export function needsRootAgentsSymlink(targets: Iterable<string>): boolean {
  for (const t of targets) {
    if (AGENTS_MD_TARGETS.has(t)) return true;
  }
  return false;
}

/** Compute the relative link target from `linkPath` to `canonicalPath`. */
export function relativeLinkTarget(
  linkPath: string,
  canonicalPath: string,
): string {
  // Resolve both sides so relative inputs (e.g. from process.cwd()) behave
  // the same as absolute ones, then compute the link-dir-relative path.
  return relative(resolve(dirname(linkPath)), resolve(canonicalPath));
}

/**
 * Project-level convenience. Ensures both the `.claude/CLAUDE.md` and
 * root `./AGENTS.md` symlinks are in place, subject to which targets the
 * caller is installing. Returns a map of link description -> result so the
 * caller can render a consolidated summary.
 *
 * When canonical `.agents/AGENTS.md` is absent, it is created lazily by
 * the merge helper in `ensureSymlink` as soon as a real source file needs
 * migrating (so users who adopt Loaf with only a pre-existing `./AGENTS.md`
 * still have their content preserved). If no migration happens and no
 * canonical file exists, we skip (no links to create anyway).
 */
export async function ensureProjectSymlinks(params: {
  projectRoot: string;
  selectedTargets: Iterable<string>;
  hasClaudeCode: boolean;
  nonInteractive?: boolean;
  assumeYes?: boolean;
  prompt?: (question: string) => Promise<boolean>;
}): Promise<Record<string, EnsureSymlinkResult>> {
  const { projectRoot, selectedTargets, hasClaudeCode } = params;
  const results: Record<string, EnsureSymlinkResult> = {};

  const canonical = join(projectRoot, ".agents", "AGENTS.md");

  const targetsArr = Array.from(selectedTargets);
  const wantClaude =
    hasClaudeCode || targetsArr.includes("claude-code");
  const wantRootAgents = needsRootAgentsSymlink(targetsArr);

  // If we would create no links at all (neither scope requested), there is
  // genuinely nothing to do.
  if (!wantClaude && !wantRootAgents) {
    return results;
  }

  // Ensure the canonical file exists before we create any symlinks. On a
  // fresh install this is empty — the subsequent fenced-section write flows
  // through the symlink and populates it. Without this step, symlinks would
  // either be dangling (ensureSymlink state 4) or never created at all
  // (original early-bail), and fenced-section writes would land as sibling
  // real files at .claude/CLAUDE.md / ./AGENTS.md and drift on next install.
  if (!existsSync(canonical)) {
    mkdirSync(dirname(canonical), { recursive: true });
    writeFileSync(canonical, "");
  }

  const commonOptions: EnsureSymlinkOptions = {
    prompt: params.prompt,
    nonInteractive: params.nonInteractive,
    assumeYes: params.assumeYes,
    canonicalPath: canonical,
    projectRoot,
  };

  if (wantClaude) {
    const claudeLink = join(projectRoot, ".claude", "CLAUDE.md");
    const relTarget = relativeLinkTarget(claudeLink, canonical);
    results[".claude/CLAUDE.md"] = await ensureSymlink(
      claudeLink,
      relTarget,
      ".claude/CLAUDE.md",
      commonOptions,
    );
  }

  if (wantRootAgents) {
    const rootAgentsLink = join(projectRoot, "AGENTS.md");
    const relTarget = relativeLinkTarget(rootAgentsLink, canonical);
    results["./AGENTS.md"] = await ensureSymlink(
      rootAgentsLink,
      relTarget,
      "./AGENTS.md",
      commonOptions,
    );
  }

  return results;
}
