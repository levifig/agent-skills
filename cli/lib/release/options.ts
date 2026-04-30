/**
 * Release Option Validation & Normalization
 *
 * Validation and normalization helpers for CLI options
 * used by the `loaf release` command. Extracted for testability.
 */

import { execFileSync } from "child_process";
import type { BumpType } from "./version.js";

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

export interface ReleaseOptions {
  dryRun?: boolean;
  bump?: string;
  base?: string;
  tag?: boolean;
  gh?: boolean;
  yes?: boolean;
  /** Repeatable `--version-file <path>` overrides. Replaces declared + auto-detected paths. */
  versionFile?: string[];
  /**
   * `--pre-merge` convenience flag. Bundles `--no-tag --no-gh --base <auto>`.
   * Auto-detection follows a 4-step priority order (see `cli/lib/release/base.ts`).
   * Explicit `--tag`, `--gh`, or `--base` overrides the bundled defaults.
   */
  preMerge?: boolean;
  /**
   * `--post-merge` finalization flag. Skips the bump+commit flow entirely
   * and instead runs the 8-point guardrail check + tag/push/release/cleanup
   * sequence on the just-merged release commit. See
   * `cli/lib/release/post-merge.ts`.
   *
   * Incompatible with --bump, --dry-run, --no-tag, --tag, --no-gh, --gh,
   * --base, --version-file, --yes (post-merge has no bump and no draft step).
   */
  postMerge?: boolean;
}

export interface NormalizedFlags {
  skipTag: boolean;
  skipGh: boolean;
}

// ─────────────────────────────────────────────────────────────────────────────
// Validation
// ─────────────────────────────────────────────────────────────────────────────

const VALID_BUMPS: BumpType[] = ["major", "minor", "patch", "prerelease", "release"];

/**
 * Validate a --bump value. Returns the validated BumpType or throws
 * with a descriptive message.
 */
export function validateBumpType(value: string): BumpType {
  if (!VALID_BUMPS.includes(value as BumpType)) {
    throw new Error(
      `Invalid bump type "${value}". Must be one of: ${VALID_BUMPS.join(", ")}`,
    );
  }
  return value as BumpType;
}

/**
 * Validate that a git ref exists and resolves to a commit.
 * Returns the resolved ref name — which may differ from the input
 * if the literal ref doesn't exist but `origin/<ref>` does.
 *
 * This handles the common case where a clone has `origin/main` but
 * no local `main` branch.
 */
export function validateBaseRef(cwd: string, ref: string): string {
  const attemptedRefs = getCandidateRefs(ref);

  for (const candidate of attemptedRefs) {
    if (refResolves(cwd, candidate)) return candidate;
  }

  throw new Error(
    `Base ref "${ref}" does not exist or is not reachable. ` +
    `Tried ${attemptedRefs.map((candidate) => `"${candidate}"`).join(" and ")}. ` +
    `If this is a remote branch, run: git fetch origin ${ref}`,
  );
}

function getCandidateRefs(ref: string): string[] {
  if (ref.includes("/")) return [ref];
  return [ref, `origin/${ref}`];
}

function refResolves(cwd: string, ref: string): boolean {
  try {
    execFileSync("git", ["rev-parse", "--verify", `${ref}^{commit}`], {
      cwd,
      encoding: "utf-8",
      stdio: ["ignore", "pipe", "ignore"],
    });
    return true;
  } catch (error: unknown) {
    // Expected: git ran but the ref was invalid (non-zero exit code).
    // Unexpected: git not found, permission denied, etc. — propagate.
    if (error && typeof error === "object" && "status" in error) {
      return false;
    }
    throw error;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Normalization
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Normalize skip flags from CLI options.
 *
 * --no-tag implies --no-gh because `gh release create` auto-creates
 * missing tags on GitHub, which would defeat the purpose of --no-tag.
 */
export function normalizeSkipFlags(options: ReleaseOptions): NormalizedFlags {
  const skipTag = options.tag === false;
  const skipGh = options.gh === false || skipTag;
  return { skipTag, skipGh };
}
