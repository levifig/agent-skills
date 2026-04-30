/**
 * Base Branch Detection
 *
 * Resolves the base branch for `loaf release --pre-merge`, the release-only PR
 * classifier (workflow-pre-pr), AND the post-merge guardrail flow.
 *
 * Strict 4-step priority order, first match wins:
 *
 *   1. Explicit --base flag.
 *   2. Open PR's base branch via `gh pr view --head <current> --json baseRefName`
 *      (only when a PR exists AND is OPEN). Skipped when `skipPRLookup: true`
 *      (post-merge flow — PR is closed/merged by then).
 *   3. User override via `git config loaf.release.base`.
 *   4. Default branch via `gh repo view --json defaultBranchRef -q .defaultBranchRef.name`,
 *      falling back to parsing `git symbolic-ref refs/remotes/origin/HEAD` when gh
 *      is not available.
 *
 * If all enabled steps fail, throws an actionable error.
 *
 * The resolver shells out via an injectable `CommandRunner`. The default runner
 * uses `execFileSync`; tests inject a mock runner so they can stub gh/git
 * responses without touching the real environment.
 */

import { execFileSync } from "child_process";

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Result of running an external command. Shape mirrors what we need from a
 * synchronous spawn: stdout text, exit code, and a marker for "binary missing"
 * (ENOENT) vs "ran but non-zero".
 */
export interface CommandResult {
  /** Captured stdout, trimmed by the runner. Empty string on failure. */
  stdout: string;
  /** Exit code. 0 on success. Non-zero on command failure. */
  exitCode: number;
  /**
   * True when the command itself could not be located (ENOENT, e.g. `gh` is
   * not installed). Distinct from a command that ran and exited non-zero.
   */
  notFound: boolean;
}

export type CommandRunner = (
  command: string,
  args: string[],
  options: { cwd: string },
) => CommandResult;

export interface BaseDetectionInput {
  /** Value of the explicit --base flag, if passed. */
  explicit?: string;
  /** Current branch (e.g. `git symbolic-ref --short HEAD`). */
  currentBranch: string;
  /** Working directory for git/gh invocations. */
  cwd: string;
  /** Optional command runner for tests. Defaults to a real-spawn runner. */
  runner?: CommandRunner;
  /**
   * Skip the open-PR lookup (step 2). Used by the post-merge guardrail flow,
   * where the PR is closed/merged by definition and `gh pr view` would either
   * return nothing or be flaky. When true, sources collapse to
   * 'explicit' | 'config' | 'default'.
   */
  skipPRLookup?: boolean;
}

export type BaseDetectionSource = "explicit" | "pr" | "config" | "default";

export interface BaseDetectionResult {
  base: string;
  source: BaseDetectionSource;
}

// ─────────────────────────────────────────────────────────────────────────────
// Default runner (real spawn)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Default command runner. Uses `execFileSync` so we never invoke a shell —
 * arguments are passed as an array.
 *
 * Treats ENOENT (binary missing) distinctly from non-zero exit so the resolver
 * can fall through cleanly when `gh` is not installed.
 */
export const defaultRunner: CommandRunner = (command, args, options) => {
  try {
    const stdout = execFileSync(command, args, {
      cwd: options.cwd,
      encoding: "utf-8",
      stdio: ["ignore", "pipe", "ignore"],
    });
    return { stdout: stdout.trim(), exitCode: 0, notFound: false };
  } catch (error: unknown) {
    if (error && typeof error === "object") {
      const errno = (error as { code?: string }).code;
      if (errno === "ENOENT") {
        return { stdout: "", exitCode: -1, notFound: true };
      }
      if ("status" in error) {
        const status = (error as { status?: number | null }).status;
        return {
          stdout: "",
          exitCode: typeof status === "number" ? status : 1,
          notFound: false,
        };
      }
    }
    // Unknown failure mode — surface as non-zero exit, not ENOENT.
    return { stdout: "", exitCode: 1, notFound: false };
  }
};

// ─────────────────────────────────────────────────────────────────────────────
// Resolver
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Resolve the base branch for a pre-merge release.
 *
 * @throws when all four detection steps yield nothing.
 */
export async function resolveBaseBranch(
  input: BaseDetectionInput,
): Promise<BaseDetectionResult> {
  const runner = input.runner ?? defaultRunner;
  const { cwd, currentBranch, explicit } = input;

  // Step 1: explicit --base flag wins. No shell-out at all.
  if (explicit && explicit.trim().length > 0) {
    return { base: explicit.trim(), source: "explicit" };
  }

  // Step 2: open PR's base branch (only when gh is available AND PR is OPEN).
  // Skipped entirely when the caller passes skipPRLookup (post-merge flow).
  if (!input.skipPRLookup) {
    const prBase = lookupOpenPrBase(runner, cwd, currentBranch);
    if (prBase) return { base: prBase, source: "pr" };
  }

  // Step 3: git config loaf.release.base.
  const configBase = lookupGitConfigBase(runner, cwd);
  if (configBase) return { base: configBase, source: "config" };

  // Step 4: default branch via gh, then origin/HEAD fallback.
  const defaultBase = lookupDefaultBranch(runner, cwd);
  if (defaultBase) return { base: defaultBase, source: "default" };

  throw new Error(
    "Could not auto-detect base branch. Pass --base <ref> explicitly, " +
      "or set git config loaf.release.base <ref>",
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Step implementations
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Step 2: open PR's base branch.
 *
 * Uses `gh pr view <branch> --json baseRefName,state -q '...'`. Filters on
 * state === "OPEN" so a closed/merged PR doesn't accidentally win over the
 * config or default-branch steps.
 *
 * Returns null when:
 *   - gh is not installed,
 *   - no PR is associated with the current branch,
 *   - the associated PR is not OPEN,
 *   - any other gh failure.
 */
function lookupOpenPrBase(
  runner: CommandRunner,
  cwd: string,
  currentBranch: string,
): string | null {
  // `gh pr view <branch>` resolves the PR for that branch. The single-quoted
  // jq-style filter emits `baseRefName` only when the PR is OPEN; otherwise
  // empty stdout, which we treat as "no match, fall through".
  const result = runner(
    "gh",
    [
      "pr",
      "view",
      currentBranch,
      "--json",
      "baseRefName,state",
      "-q",
      "select(.state == \"OPEN\") | .baseRefName",
    ],
    { cwd },
  );

  if (result.notFound) return null;
  if (result.exitCode !== 0) return null;

  const base = result.stdout.trim();
  return base.length > 0 ? base : null;
}

/**
 * Step 3: user override via `git config loaf.release.base`.
 *
 * Uses `git config --get` so a missing key exits non-zero (and we fall through)
 * without printing to stderr. `git config` is always available when `git` is.
 */
function lookupGitConfigBase(runner: CommandRunner, cwd: string): string | null {
  const result = runner(
    "git",
    ["config", "--get", "loaf.release.base"],
    { cwd },
  );

  if (result.notFound) return null;
  if (result.exitCode !== 0) return null;

  const base = result.stdout.trim();
  return base.length > 0 ? base : null;
}

/**
 * Step 4: default branch.
 *
 * Primary path: `gh repo view --json defaultBranchRef -q .defaultBranchRef.name`.
 * Fallback (when gh is unavailable OR fails): parse
 * `git symbolic-ref refs/remotes/origin/HEAD`, stripping the
 * `refs/remotes/origin/` prefix.
 */
function lookupDefaultBranch(runner: CommandRunner, cwd: string): string | null {
  const ghResult = runner(
    "gh",
    ["repo", "view", "--json", "defaultBranchRef", "-q", ".defaultBranchRef.name"],
    { cwd },
  );

  if (!ghResult.notFound && ghResult.exitCode === 0) {
    const base = ghResult.stdout.trim();
    if (base.length > 0) return base;
  }

  // Fallback: parse origin/HEAD. Handles the gh-missing case AND the
  // gh-installed-but-unauthenticated case.
  const symRefResult = runner(
    "git",
    ["symbolic-ref", "refs/remotes/origin/HEAD"],
    { cwd },
  );

  if (symRefResult.notFound) return null;
  if (symRefResult.exitCode !== 0) return null;

  const raw = symRefResult.stdout.trim();
  const prefix = "refs/remotes/origin/";
  if (raw.startsWith(prefix)) {
    const base = raw.slice(prefix.length);
    return base.length > 0 ? base : null;
  }

  // Unexpected shape — fall through.
  return null;
}
