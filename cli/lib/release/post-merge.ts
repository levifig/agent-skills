/**
 * Post-Merge Release Finalization
 *
 * Implements `loaf release --post-merge` (SPEC-031 / TASK-142).
 *
 * After a `chore: release v<semver>` PR has been squash-merged onto the base
 * branch, `--post-merge` finalizes the release: it verifies HEAD state against
 * an 8-point guardrail checklist and then runs the tag-push-release-cleanup
 * action sequence.
 *
 * 8-point guardrail (all required, ordered, any failure aborts before tag/release):
 *
 *   1. Clean worktree (no uncommitted changes).
 *   2. On the detected base branch (or cleanly fast-forwardable to it).
 *   3. HEAD subject matches `^chore: release v<semver>( \(#\d+\))?$`.
 *   4. Extracted version (from commit subject) matches the version recorded in
 *      every detected version file at HEAD. Per-file diagnostic on mismatch.
 *   5. `git diff HEAD^ HEAD --name-only` includes `CHANGELOG.md` AND at least
 *      one version file path.
 *   6. CHANGELOG.md contains a non-empty `## [<version>]` section.
 *   7. No existing local tag, remote tag, or GH release for `v<version>`.
 *   8. HEAD itself is not already tagged.
 *
 * Action sequence (after all guardrails pass):
 *
 *   1. Capture feature branch name (best-effort, from `gh pr view <PR#>` if
 *      HEAD subject carries a `(#N)` suffix; otherwise undefined).
 *   2. `git tag -a v<version> -m "Release <version>"`
 *   3. `git push origin v<version>` (explicit, before `gh release create`).
 *   4. `gh release create v<version> --title "v<version>" --notes "<body>"`.
 *   5. `git pull --rebase origin <base>` (best-effort).
 *   6. Best-effort branch deletion: `git branch -d <feature>` then
 *      `git push origin --delete <feature>` (warn on either failure).
 *
 * Light idempotency: each guardrail check is rerun-safe. Aborts produce
 * actionable messages naming the manual fix path.
 */

import { execFileSync } from "child_process";
import { existsSync, readFileSync } from "fs";
import { join } from "path";

import {
  defaultRunner,
  resolveBaseBranch,
  type CommandResult,
  type CommandRunner,
} from "./base.js";
import { detectVersionFiles } from "./version.js";
import type { VersionFile } from "./version.js";
import { readLoafConfig } from "../config/agents-config.js";

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

export interface PostMergeContext {
  cwd: string;
  /** Optional command runner for tests; defaults to real `execFileSync` runner. */
  runner?: CommandRunner;
  /** Optional version-file overrides (mirrors the CLI surface). */
  cliVersionFileOverrides?: string[];
}

export interface PostMergeAbort {
  ok: false;
  /** Which guardrail aborted (1–8). */
  guardrail: number;
  /** User-facing actionable message. */
  message: string;
}

export interface PostMergeReady {
  ok: true;
  /** Version extracted from the HEAD commit subject. */
  version: string;
  /** Resolved base branch (current branch when guardrail 2 passed). */
  base: string;
  /** Feature branch name, when derivable from the PR-number suffix. */
  featureBranch?: string;
  /** Lines of the CHANGELOG section body (no header). Empty when no body. */
  changelogBody: string;
  /** The detected version files at HEAD (used by guardrail 4 + diff check). */
  versionFiles: VersionFile[];
}

export type PostMergeResult = PostMergeAbort | PostMergeReady;

export interface PostMergeActionResult {
  tagged: boolean;
  pushed: boolean;
  released: boolean;
  pulled: boolean;
  deleted: { local?: boolean; remote?: boolean };
}

// ─────────────────────────────────────────────────────────────────────────────
// Constants
// ─────────────────────────────────────────────────────────────────────────────

/**
 * The release commit subject shape — mirrors `cli/commands/check.ts`'s
 * `RELEASE_COMMIT_SUBJECT_REGEX` (TASK-140). Duplicated here to avoid
 * cross-module coupling; if the regex moves to a shared module later, both
 * call sites should switch to the import in lock-step.
 */
const RELEASE_SUBJECT_RE =
  /^chore: release v(\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?(?:\+[a-zA-Z0-9.-]+)?)(?:\s+\(#(\d+)\))?$/;

// ─────────────────────────────────────────────────────────────────────────────
// Runner helpers
// ─────────────────────────────────────────────────────────────────────────────

function run(
  runner: CommandRunner,
  cwd: string,
  command: string,
  args: string[],
): CommandResult {
  return runner(command, args, { cwd });
}

// ─────────────────────────────────────────────────────────────────────────────
// Guardrail helpers (each returns null = pass, string = abort message)
// ─────────────────────────────────────────────────────────────────────────────

/** Guardrail 1: clean worktree (no staged or unstaged changes). */
function checkCleanWorktree(
  runner: CommandRunner,
  cwd: string,
): string | null {
  const result = run(runner, cwd, "git", ["status", "--porcelain"]);
  if (result.exitCode !== 0) {
    return "could not read git status — is this a git repository?";
  }
  if (result.stdout.trim().length > 0) {
    return "uncommitted changes detected — commit or stash before rerunning";
  }
  return null;
}

/**
 * Guardrail 2 helper: get the current branch name. Returns null when detached
 * (the caller treats that as guardrail-2 failure).
 */
function getCurrentBranch(runner: CommandRunner, cwd: string): string | null {
  const result = run(runner, cwd, "git", ["symbolic-ref", "--short", "HEAD"]);
  if (result.exitCode !== 0) return null;
  const branch = result.stdout.trim();
  return branch.length > 0 ? branch : null;
}

/**
 * Guardrail 2: current branch IS the detected base, or cleanly fast-forwardable.
 *
 * "Cleanly fast-forwardable" means `git merge-base --is-ancestor <current> <base>`
 * succeeds — i.e. the user is on a sibling branch that has no diverging commits
 * relative to base. In practice for the post-merge flow the user must already
 * be on base (the squash merge landed there); the FF check is a graceful
 * accommodation for the "user fetched but hasn't checked out base yet" case.
 */
function checkOnBaseBranch(
  runner: CommandRunner,
  cwd: string,
  base: string,
  current: string,
): string | null {
  if (current === base) return null;

  // Fast-forward check: is current an ancestor of base? If yes, they're the
  // same logical commit (or current is behind base, which is fine for a
  // post-merge that has already landed).
  const ffResult = run(runner, cwd, "git", [
    "merge-base",
    "--is-ancestor",
    current,
    base,
  ]);
  if (ffResult.exitCode === 0) return null;

  return `current branch ${current} is not the base branch ${base} — checkout ${base} and rerun`;
}

/** Guardrail 3: HEAD subject matches the chore-release shape. */
function checkSubjectShape(
  runner: CommandRunner,
  cwd: string,
): { version: string; prNumber?: string } | string {
  const result = run(runner, cwd, "git", ["log", "-1", "--pretty=%s"]);
  if (result.exitCode !== 0) {
    return "could not read HEAD subject";
  }
  const subject = result.stdout.trim();
  const match = subject.match(RELEASE_SUBJECT_RE);
  if (!match) {
    return `HEAD subject ${JSON.stringify(subject)} does not match \`chore: release v<semver>\` shape — this is not a post-merge release commit`;
  }
  return {
    version: match[1],
    ...(match[2] ? { prNumber: match[2] } : {}),
  };
}

/**
 * Guardrail 4: extracted version matches every detected version file at HEAD.
 * Returns null (pass) or a per-file diagnostic message.
 */
function checkVersionFilesMatch(
  files: VersionFile[],
  version: string,
): string | null {
  if (files.length === 0) {
    return "no version files detected at HEAD — cannot verify version match";
  }
  const mismatches = files
    .filter((f) => f.currentVersion !== version)
    .map((f) => `${f.relativePath} reports ${f.currentVersion}, expected ${version}`);
  if (mismatches.length > 0) {
    return `version mismatch in version file(s):\n    ${mismatches.join("\n    ")}`;
  }
  return null;
}

/**
 * Guardrail 5: HEAD vs HEAD^ diff includes CHANGELOG.md AND at least one
 * version file path.
 */
function checkDiffFiles(
  runner: CommandRunner,
  cwd: string,
  versionFiles: VersionFile[],
): string | null {
  const result = run(runner, cwd, "git", [
    "diff",
    "HEAD^",
    "HEAD",
    "--name-only",
  ]);
  if (result.exitCode !== 0) {
    return "could not read git diff HEAD^ HEAD — is HEAD a merge of multiple commits or the first commit?";
  }
  const changed = new Set(
    result.stdout
      .split("\n")
      .map((s) => s.trim())
      .filter((s) => s.length > 0),
  );

  const versionFilePaths = versionFiles.map((f) => f.relativePath);
  const hasChangelog = changed.has("CHANGELOG.md");
  const hasVersionFile = versionFilePaths.some((p) => changed.has(p));

  if (!hasChangelog && !hasVersionFile) {
    return "release commit is missing both CHANGELOG.md and any version file diffs — this does not look like a release commit";
  }
  if (!hasChangelog) {
    return "release commit is missing a CHANGELOG.md diff — verify the changelog was updated";
  }
  if (!hasVersionFile) {
    return `release commit is missing a version-file diff (expected one of: ${versionFilePaths.join(", ")})`;
  }
  return null;
}

/**
 * Guardrail 6: CHANGELOG.md at HEAD has a non-empty `## [<version>]` section.
 * Returns the extracted body lines on pass, or an abort message on failure.
 */
function checkChangelogSection(
  cwd: string,
  version: string,
): string | string[] {
  const changelogPath = join(cwd, "CHANGELOG.md");
  if (!existsSync(changelogPath)) {
    return "CHANGELOG.md not found at HEAD";
  }
  let content: string;
  try {
    content = readFileSync(changelogPath, "utf-8");
  } catch {
    return "could not read CHANGELOG.md";
  }
  const body = extractChangelogSection(content, version);
  if (body === null) {
    return `CHANGELOG.md has no \`## [${version}]\` section`;
  }
  if (body.length === 0) {
    return `CHANGELOG.md \`## [${version}]\` section has no list items`;
  }
  return body;
}

/**
 * Extract the body lines under `## [<version>]` (everything between that
 * heading and the next `## [` heading or EOF). Returns null when no matching
 * heading is found, or an array of trimmed body lines (with non-list lines
 * preserved verbatim — the body is the literal section content less the
 * heading).
 *
 * The body is considered "non-empty" when at least one list-item line
 * (`- ` or `* ` after optional leading whitespace) is present. Non-list lines
 * (sub-headings, prose) ARE included in the body but do not satisfy the
 * non-empty check on their own.
 */
export function extractChangelogSection(
  content: string,
  version: string,
): string[] | null {
  const lines = content.split("\n");
  const heading = `## [${version}]`;
  let startIdx = -1;
  for (let i = 0; i < lines.length; i++) {
    if (lines[i].trim().startsWith(heading)) {
      startIdx = i;
      break;
    }
  }
  if (startIdx === -1) return null;

  let endIdx = lines.length;
  for (let i = startIdx + 1; i < lines.length; i++) {
    if (/^## \[/.test(lines[i].trim())) {
      endIdx = i;
      break;
    }
  }

  // Trim leading/trailing blank lines from the body for the message-friendly
  // form. Preserve internal blank lines so multi-section bodies (Added /
  // Changed / Fixed sub-headings) survive verbatim into the GH release notes.
  const raw = lines.slice(startIdx + 1, endIdx);
  let lo = 0;
  let hi = raw.length;
  while (lo < hi && raw[lo].trim().length === 0) lo++;
  while (hi > lo && raw[hi - 1].trim().length === 0) hi--;
  const trimmed = raw.slice(lo, hi);

  // Non-empty check: at least one list item.
  const hasItem = trimmed.some((l) => /^\s*[-*]\s+/.test(l));
  if (!hasItem) return [];
  return trimmed;
}

/**
 * Guardrail 7: no existing local tag, remote tag, or GH release for v<version>.
 *
 * Three sub-checks; abort messages must match SPEC-031 verbatim. Order: local
 * → remote → GH. The GH release check is best-effort: if `gh` is not installed
 * we treat the GH release as absent (the user will see the failure when
 * `gh release create` runs and can manually clean up).
 */
function checkNoExistingTagOrRelease(
  runner: CommandRunner,
  cwd: string,
  version: string,
): string | null {
  const tag = `v${version}`;

  // Local tag: `git tag --list <tag>` returns the tag name on stdout when it exists.
  const localResult = run(runner, cwd, "git", ["tag", "--list", tag]);
  if (localResult.exitCode === 0 && localResult.stdout.trim() === tag) {
    return `tag v${version} already exists locally — run \`git tag -d v${version}\` and rerun`;
  }

  // Remote tag: `git ls-remote --tags origin refs/tags/<tag>`.
  const remoteResult = run(runner, cwd, "git", [
    "ls-remote",
    "--tags",
    "origin",
    `refs/tags/${tag}`,
  ]);
  if (remoteResult.exitCode === 0 && remoteResult.stdout.trim().length > 0) {
    return `tag v${version} already exists on remote — run \`git push origin :refs/tags/v${version}\` and rerun`;
  }

  // GH release: `gh release view <tag>` exits 0 when the release exists.
  // ENOENT (gh missing) is treated as "no release" — let `gh release create`
  // surface the missing-binary error if it gets that far.
  const ghResult = run(runner, cwd, "gh", ["release", "view", tag]);
  if (!ghResult.notFound && ghResult.exitCode === 0) {
    return `GH release v${version} already exists — visit the release page and delete it manually before rerunning`;
  }

  return null;
}

/** Guardrail 8: HEAD itself is not already tagged. */
function checkHeadNotTagged(
  runner: CommandRunner,
  cwd: string,
): string | null {
  const result = run(runner, cwd, "git", ["tag", "--points-at", "HEAD"]);
  if (result.exitCode !== 0) {
    // Non-zero from `git tag --points-at` is rare (it normally exits 0 with
    // empty stdout when no tags point at HEAD). Treat as pass — guardrail 7
    // already covers the tag-collision case.
    return null;
  }
  const tags = result.stdout
    .split("\n")
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
  if (tags.length === 0) return null;
  return `HEAD is already tagged as ${tags[0]}; this is not a fresh post-merge state`;
}

// ─────────────────────────────────────────────────────────────────────────────
// Public guardrail entry point
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Run all 8 guardrails in order. First failure short-circuits with an
 * actionable abort message.
 */
export async function checkPostMergeGuardrails(
  ctx: PostMergeContext,
): Promise<PostMergeResult> {
  const runner = ctx.runner ?? defaultRunner;
  const cwd = ctx.cwd;

  // Guardrail 1: clean worktree.
  const dirty = checkCleanWorktree(runner, cwd);
  if (dirty) return { ok: false, guardrail: 1, message: dirty };

  // Guardrail 2: on base branch (or fast-forwardable). Resolve the base via
  // the unified resolver, skipping the open-PR tier — by post-merge the PR is
  // closed/merged and `gh pr view <branch>` would either return nothing or be
  // flaky. The remaining tiers (explicit / config / default) are sufficient.
  const current = getCurrentBranch(runner, cwd);
  if (!current) {
    return {
      ok: false,
      guardrail: 2,
      message: "detached HEAD — checkout the base branch and rerun",
    };
  }

  let base: string;
  try {
    const detected = await resolveBaseBranch({
      runner,
      cwd,
      currentBranch: current,
      skipPRLookup: true,
    });
    base = detected.base;
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { ok: false, guardrail: 2, message };
  }

  const branchAbort = checkOnBaseBranch(runner, cwd, base, current);
  if (branchAbort) {
    return { ok: false, guardrail: 2, message: branchAbort };
  }

  // Guardrail 3: subject shape.
  const subjectResult = checkSubjectShape(runner, cwd);
  if (typeof subjectResult === "string") {
    return { ok: false, guardrail: 3, message: subjectResult };
  }
  const { version, prNumber } = subjectResult;

  // Detect version files at HEAD. Done here (not earlier) so a missing/bad
  // version-files config surfaces alongside the version-match guardrail
  // rather than as an unrelated upstream error.
  let versionFiles: VersionFile[];
  try {
    const cliOverrides = ctx.cliVersionFileOverrides ?? [];
    const loafConfig = readLoafConfig(cwd);
    const configOverrides = Array.isArray(loafConfig.release?.versionFiles)
      ? (loafConfig.release?.versionFiles as string[])
      : [];
    versionFiles = detectVersionFiles(cwd, {
      cliOverrides,
      configOverrides,
    });
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { ok: false, guardrail: 4, message };
  }

  // Guardrail 4: version matches every version file.
  const versionAbort = checkVersionFilesMatch(versionFiles, version);
  if (versionAbort) {
    return { ok: false, guardrail: 4, message: versionAbort };
  }

  // Guardrail 5: diff includes CHANGELOG + version file.
  const diffAbort = checkDiffFiles(runner, cwd, versionFiles);
  if (diffAbort) {
    return { ok: false, guardrail: 5, message: diffAbort };
  }

  // Guardrail 6: CHANGELOG section non-empty.
  const sectionResult = checkChangelogSection(cwd, version);
  if (typeof sectionResult === "string") {
    return { ok: false, guardrail: 6, message: sectionResult };
  }
  const changelogBody = sectionResult.join("\n");

  // Guardrail 7: no existing tag or GH release.
  const collisionAbort = checkNoExistingTagOrRelease(runner, cwd, version);
  if (collisionAbort) {
    return { ok: false, guardrail: 7, message: collisionAbort };
  }

  // Guardrail 8: HEAD not already tagged.
  const headTaggedAbort = checkHeadNotTagged(runner, cwd);
  if (headTaggedAbort) {
    return { ok: false, guardrail: 8, message: headTaggedAbort };
  }

  // All guardrails passed. Best-effort feature-branch lookup via PR number.
  const featureBranch = prNumber
    ? lookupFeatureBranchByPr(runner, cwd, prNumber)
    : undefined;

  return {
    ok: true,
    version,
    base,
    ...(featureBranch ? { featureBranch } : {}),
    changelogBody,
    versionFiles,
  };
}

/**
 * Best-effort feature branch lookup via `gh pr view <PR#>`. Returns undefined
 * on any failure (gh missing, PR number invalid, network failure). The caller
 * skips the branch-delete step when undefined — that's a warn-and-continue
 * scenario.
 */
function lookupFeatureBranchByPr(
  runner: CommandRunner,
  cwd: string,
  prNumber: string,
): string | undefined {
  const result = run(runner, cwd, "gh", [
    "pr",
    "view",
    prNumber,
    "--json",
    "headRefName",
    "-q",
    ".headRefName",
  ]);
  if (result.notFound) return undefined;
  if (result.exitCode !== 0) return undefined;
  const value = result.stdout.trim();
  return value.length > 0 ? value : undefined;
}

// ─────────────────────────────────────────────────────────────────────────────
// Action sequence
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Logger shape used by `executePostMergeActions`. Each method takes a single
 * line of message — the impl in `release.ts` decorates with color/prefix.
 */
export interface PostMergeLogger {
  ok(message: string): void;
  warn(message: string): void;
  err(message: string): void;
}

const noopLogger: PostMergeLogger = {
  ok: () => undefined,
  warn: () => undefined,
  err: () => undefined,
};

/**
 * Execute the post-merge action sequence after `checkPostMergeGuardrails`
 * has returned `ok: true`. Tag + push + GH release failures throw (caller
 * surfaces and exits non-zero). Pull and branch deletion are best-effort —
 * failures log a warning and do not throw.
 */
export async function executePostMergeActions(
  ctx: PostMergeContext,
  ready: PostMergeReady,
  logger: PostMergeLogger = noopLogger,
): Promise<PostMergeActionResult> {
  const runner = ctx.runner ?? defaultRunner;
  const cwd = ctx.cwd;
  const tag = `v${ready.version}`;

  const result: PostMergeActionResult = {
    tagged: false,
    pushed: false,
    released: false,
    pulled: false,
    deleted: {},
  };

  // Step 1: feature branch name was already captured during guardrails (via
  // PR-number lookup). Nothing to do here — `ready.featureBranch` is set when
  // available.

  // Step 2: tag locally.
  const tagResult = run(runner, cwd, "git", [
    "tag",
    "-a",
    tag,
    "-m",
    `Release ${ready.version}`,
  ]);
  if (tagResult.exitCode !== 0) {
    throw new Error(`failed to create tag ${tag} (exit ${tagResult.exitCode})`);
  }
  result.tagged = true;
  logger.ok(`Created tag ${tag}`);

  // Step 3: push tag (explicit, before gh release create).
  const pushResult = run(runner, cwd, "git", ["push", "origin", tag]);
  if (pushResult.exitCode !== 0) {
    throw new Error(`failed to push tag ${tag} (exit ${pushResult.exitCode})`);
  }
  result.pushed = true;
  logger.ok(`Pushed tag ${tag}`);

  // Step 4: GH release.
  const releaseResult = run(runner, cwd, "gh", [
    "release",
    "create",
    tag,
    "--title",
    tag,
    "--notes",
    ready.changelogBody,
  ]);
  if (releaseResult.notFound) {
    throw new Error("gh CLI is not installed; cannot create GH release");
  }
  if (releaseResult.exitCode !== 0) {
    throw new Error(`failed to create GH release ${tag} (exit ${releaseResult.exitCode})`);
  }
  result.released = true;
  logger.ok(`Created GH release ${tag}`);

  // Step 5: pull base (best-effort).
  const pullResult = run(runner, cwd, "git", [
    "pull",
    "--rebase",
    "origin",
    ready.base,
  ]);
  if (pullResult.exitCode === 0) {
    result.pulled = true;
    logger.ok(`Pulled latest from origin/${ready.base}`);
  } else {
    logger.warn(`Failed to pull origin/${ready.base} — continuing`);
  }

  // Step 6: best-effort branch delete (only when feature branch is known).
  if (ready.featureBranch) {
    const localDelete = run(runner, cwd, "git", [
      "branch",
      "-d",
      ready.featureBranch,
    ]);
    if (localDelete.exitCode === 0) {
      result.deleted.local = true;
      logger.ok(`Deleted local branch ${ready.featureBranch}`);
    } else {
      result.deleted.local = false;
      logger.warn(
        `Failed to delete local branch ${ready.featureBranch} (may not be fully merged) — continuing`,
      );
    }

    const remoteDelete = run(runner, cwd, "git", [
      "push",
      "origin",
      "--delete",
      ready.featureBranch,
    ]);
    if (remoteDelete.exitCode === 0) {
      result.deleted.remote = true;
      logger.ok(`Deleted remote branch ${ready.featureBranch}`);
    } else {
      result.deleted.remote = false;
      logger.warn(
        `Failed to delete remote branch ${ready.featureBranch} — continuing`,
      );
    }
  }

  return result;
}
