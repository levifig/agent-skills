/**
 * Release-only PR Classifier
 *
 * Recognizes PRs whose only diff against the base branch is a version bump
 * plus a `[Unreleased]` → `[X.Y.Z]` block move in `CHANGELOG.md`. When a PR
 * matches this strict shape, the empty-`[Unreleased]` block check in
 * `workflow-pre-pr` is bypassed.
 *
 * Strict allowlist, NOT a heuristic. A PR is release-only iff:
 *
 *   1. `git diff <base>..HEAD --name-only` returns ONLY:
 *        - `CHANGELOG.md`, AND
 *        - one or more known version-file paths (the same set `loaf release`
 *          would write to, including monorepo declarations from
 *          `.agents/loaf.json` `release.versionFiles`)
 *      Any other changed file disqualifies. No exceptions.
 *
 *   2. AND `CHANGELOG.md` at HEAD has a non-empty `## [<version>]` section
 *      whose version matches the version recorded in every detected
 *      version file.
 *
 * Failures of either condition fall through to the existing
 * empty-`[Unreleased]` behavior. Errors (e.g. gh missing for base detection,
 * unreadable files) ALSO fall through — the classifier never grants a bypass
 * on its own failure.
 *
 * Shares the base-ref resolver implemented for `loaf release --pre-merge`
 * (TASK-144) and the version-file detection implemented for monorepo
 * support (TASK-143).
 */

import { existsSync, readFileSync } from "fs";
import { join } from "path";

import { readLoafConfig } from "../config/agents-config.js";
import {
  defaultRunner,
  resolveBaseBranch,
  type CommandRunner,
} from "./base.js";
import { extractChangelogSection } from "./post-merge.js";
import { detectVersionFiles, type VersionFile } from "./version.js";

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

export interface ReleaseOnlyPrInput {
  /** Project root used for git/gh/file operations. */
  cwd: string;
  /** Optional pre-resolved base ref. When omitted, `resolveBaseBranch` runs. */
  base?: string;
  /** Current branch name. Required when `base` is omitted. */
  currentBranch?: string;
  /** Command runner for DI in tests. Defaults to real spawn. */
  runner?: CommandRunner;
}

// ─────────────────────────────────────────────────────────────────────────────
// Classifier
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Returns `true` iff the current HEAD is a release-only PR per the strict
 * allowlist + version-match rule. Any failure (including command errors,
 * unreadable files, malformed config) returns `false`. Never throws.
 */
export async function isReleaseOnlyPR(
  input: ReleaseOnlyPrInput,
): Promise<boolean> {
  try {
    const runner = input.runner ?? defaultRunner;
    const cwd = input.cwd;

    // ── Step 1: resolve base ref ───────────────────────────────────────
    let base: string;
    if (input.base && input.base.trim().length > 0) {
      base = input.base.trim();
    } else {
      if (!input.currentBranch || input.currentBranch.trim().length === 0) {
        return false;
      }
      const result = await resolveBaseBranch({
        currentBranch: input.currentBranch,
        cwd,
        runner,
      });
      base = result.base;
    }

    // ── Step 2: detect version files (honors monorepo config) ──────────
    let versionFiles: VersionFile[];
    try {
      const loafConfig = readLoafConfig(cwd);
      const configOverrides = Array.isArray(loafConfig.release?.versionFiles)
        ? (loafConfig.release?.versionFiles as string[])
        : [];
      versionFiles = detectVersionFiles(cwd, { configOverrides });
    } catch {
      // Bad config / missing declared paths — fall through.
      return false;
    }

    if (versionFiles.length === 0) {
      // No version files at all means we cannot prove a release shape.
      return false;
    }

    // ── Step 3: collect diffed files vs base ───────────────────────────
    const diffResult = runner(
      "git",
      ["diff", `${base}..HEAD`, "--name-only"],
      { cwd },
    );
    if (diffResult.notFound) return false;
    if (diffResult.exitCode !== 0) return false;

    const changed = new Set(
      diffResult.stdout
        .split("\n")
        .map((s) => s.trim())
        .filter((s) => s.length > 0),
    );

    if (changed.size === 0) return false;

    // ── Step 4: enforce strict allowlist ───────────────────────────────
    // Allowlist = CHANGELOG.md + every detected version-file relative path.
    const allowlist = new Set<string>(["CHANGELOG.md"]);
    for (const file of versionFiles) {
      allowlist.add(file.relativePath.replace(/\\/g, "/"));
    }

    // Every changed file must be in the allowlist.
    for (const file of changed) {
      if (!allowlist.has(file)) return false;
    }

    // Required: CHANGELOG.md must be in the diff.
    if (!changed.has("CHANGELOG.md")) return false;

    // Required: at least one version file must be in the diff.
    const hasVersionFile = versionFiles.some((f) =>
      changed.has(f.relativePath.replace(/\\/g, "/")),
    );
    if (!hasVersionFile) return false;

    // ── Step 5: extract and validate changelog section ─────────────────
    const changelogPath = join(cwd, "CHANGELOG.md");
    if (!existsSync(changelogPath)) return false;

    let changelogContent: string;
    try {
      changelogContent = readFileSync(changelogPath, "utf-8");
    } catch {
      return false;
    }

    // The version we are validating against is the version recorded in the
    // version files at HEAD (post-bump). All version files are required to
    // agree (this is the same invariant `loaf release --post-merge`
    // guardrail 4 enforces).
    const candidateVersion = versionFiles[0].currentVersion;
    const versionsAgree = versionFiles.every(
      (f) => f.currentVersion === candidateVersion,
    );
    if (!versionsAgree) return false;

    // Find the `## [<version>]` section and verify it has list items.
    const body = extractChangelogSection(changelogContent, candidateVersion);
    if (body === null) return false;
    if (body.length === 0) return false;

    return true;
  } catch {
    // Any unexpected exception falls through to "not classified".
    return false;
  }
}
