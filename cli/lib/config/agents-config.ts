/**
 * Read/write `.agents/loaf.json` for project configuration and integration toggles.
 *
 * This is the single typed surface for `loaf.json`. Format conventions
 * (2-space indent, trailing newline, key preservation) live here so every
 * writer agrees.
 *
 * Worktree behavior (SPEC-036, A3):
 *   When invoked from a linked git worktree, `.agents/loaf.json` is resolved
 *   to the **main worktree's** copy. Without this routing, callers that pass a
 *   linked worktree's root would read/write a stray `loaf.json` next to the
 *   `.moved-to` back-pointer — invisible to `loaf release` and friends, which
 *   resolve the main `.agents/` via `findAgentsDir`. See SPEC-042 Track A.
 *
 *   If the linked worktree's recorded main has been removed, these helpers
 *   THROW with the same actionable message `loaf migrate worktree-storage`
 *   surfaces. Silently writing a stale shadow config is the precise bug this
 *   routing exists to prevent.
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "fs";
import { join } from "path";

import { findMainWorktreeRoot } from "../tasks/resolve.js";
import {
  buildMainMissingMessage,
  probeMainWorktreeTarget,
  readGitdirPointerMainRoot,
} from "../migrate/worktree-storage.js";

export interface LoafConfig {
  knowledge?: {
    local?: string[];
    staleness_threshold_days?: number;
    imports?: string[];
  };
  integrations?: Record<string, { enabled: boolean }>;
  release?: {
    /** Repo-relative paths to version files. Replaces root auto-detection when set. */
    versionFiles?: string[];
  };
  [key: string]: unknown;
}

/**
 * Resolve the effective directory that hosts `.agents/loaf.json` for a given
 * `projectRoot`. In a linked worktree this returns the main worktree's root;
 * everywhere else it returns `projectRoot` unchanged.
 *
 * Three cases:
 *
 *   1. **Single-checkout** (or non-git): `findMainWorktreeRoot` returns null
 *      AND there is no linked-worktree `.git` pointer file. The legacy
 *      behavior of using `projectRoot` is correct — there's nothing else to
 *      resolve to.
 *
 *   2. **Linked worktree with healthy main**: `findMainWorktreeRoot` returns
 *      a path that exists as a directory. Return the main worktree root.
 *
 *   3. **Linked worktree with unreachable main**: `findMainWorktreeRoot`
 *      either returns null (because `git rev-parse` failed on the deleted
 *      main) or a path that doesn't exist as a directory. In both cases we
 *      detect the linked worktree from its `.git` pointer file and throw —
 *      silently falling back to `projectRoot` would create a stale
 *      `loaf.json` invisible to every other tool that resolves through the
 *      main worktree (the exact bug SPEC-042 Track A eliminates).
 */
function resolveEffectiveRoot(projectRoot: string): string {
  const mainRoot = findMainWorktreeRoot(projectRoot);
  if (mainRoot) {
    const probe = probeMainWorktreeTarget(mainRoot);
    if (probe.isDirectory) return mainRoot;
    // Main resolved but is gone — refuse to write a stale shadow config.
    throw new Error(buildMainMissingMessage(mainRoot, probe.exists));
  }

  // `findMainWorktreeRoot` returned null. This means either a healthy
  // single-checkout, a non-git path, OR a linked worktree whose main has
  // been deleted (in which case `git rev-parse --git-common-dir` fails and
  // the resolver returns null). The `.git` pointer file is still on disk in
  // the deleted-main case and encodes the recorded main path; use it to
  // distinguish the two single-checkout-shaped null returns.
  const pointerMainRoot = readGitdirPointerMainRoot(projectRoot);
  if (!pointerMainRoot) return projectRoot; // genuine single-checkout / non-git
  const pointerProbe = probeMainWorktreeTarget(pointerMainRoot);
  if (pointerProbe.isDirectory) return pointerMainRoot;
  throw new Error(buildMainMissingMessage(pointerMainRoot, pointerProbe.exists));
}

/**
 * Resolve the absolute path to the effective `.agents/loaf.json` for the given
 * `projectRoot`. Routes through the main worktree under SPEC-036.
 */
function resolveEffectiveConfigPath(projectRoot: string): string {
  return join(resolveEffectiveRoot(projectRoot), ".agents", "loaf.json");
}

/**
 * Absolute path to `.agents/loaf.json` for a project root.
 *
 * In a linked git worktree this resolves to the **main worktree's** copy so
 * every caller — whether they reach via this helper or via `findAgentsDir` —
 * agrees on a single source of truth (SPEC-036).
 */
export function loafConfigPath(projectRoot: string): string {
  return resolveEffectiveConfigPath(projectRoot);
}

export function readLoafConfig(projectRoot: string): LoafConfig {
  const p = resolveEffectiveConfigPath(projectRoot);
  if (!existsSync(p)) return {};
  try {
    const raw = readFileSync(p, "utf-8");
    return JSON.parse(raw) as LoafConfig;
  } catch {
    return {};
  }
}

/**
 * Write `loaf.json`, ensuring the `.agents/` directory exists. Format:
 * 2-space indent + trailing newline. Single source of truth for the file
 * format — every writer in the codebase delegates here.
 *
 * Routes through `resolveEffectiveRoot` so a write from a linked worktree
 * lands in the main worktree's `.agents/`, never next to the `.moved-to`
 * back-pointer.
 */
function writeLoafConfigRaw(
  projectRoot: string,
  next: Record<string, unknown>,
): void {
  const effectiveRoot = resolveEffectiveRoot(projectRoot);
  const agentsDir = join(effectiveRoot, ".agents");
  if (!existsSync(agentsDir)) {
    mkdirSync(agentsDir, { recursive: true });
  }
  writeFileSync(
    join(agentsDir, "loaf.json"),
    `${JSON.stringify(next, null, 2)}\n`,
    "utf-8",
  );
}

export function mergeLoafConfigIntegrations(
  projectRoot: string,
  updates: Partial<{ linear: { enabled: boolean }; serena: { enabled: boolean } }>,
): void {
  const existing = readLoafConfig(projectRoot);
  const integrations = {
    ...existing.integrations,
  };
  if (updates.linear !== undefined) {
    integrations.linear = updates.linear;
  }
  if (updates.serena !== undefined) {
    integrations.serena = updates.serena;
  }
  const next: LoafConfig = {
    ...existing,
    integrations,
  };
  writeLoafConfigRaw(projectRoot, next as Record<string, unknown>);
}
