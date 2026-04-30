/**
 * Base Branch Detection Tests
 *
 * Unit tests for `resolveBaseBranch` (SPEC-031 / TASK-144).
 *
 * Strategy: inject a fake `CommandRunner` that captures every call and returns
 * scripted responses keyed on the command name + arguments. This lets us
 * assert both the precedence order AND that earlier-step matches short-circuit
 * later-step shell-outs (no gh call when --base is explicit, etc.).
 */

import { describe, it, expect } from "vitest";
import {
  resolveBaseBranch,
  type CommandRunner,
  type CommandResult,
} from "./base.js";

// ─────────────────────────────────────────────────────────────────────────────
// Test runner factory
// ─────────────────────────────────────────────────────────────────────────────

interface RunnerCall {
  command: string;
  args: string[];
}

interface RunnerScript {
  /**
   * For each command-line invocation, return a scripted response. Match on
   * `command` (e.g. "gh", "git") and the first arg (e.g. "pr", "config",
   * "symbolic-ref"). Anything not matched returns notFound:true so we surface
   * unexpected calls loudly.
   */
  ghPrView?: CommandResult;
  ghRepoView?: CommandResult;
  gitConfig?: CommandResult;
  gitSymbolicRef?: CommandResult;
  /** When true, simulates `gh` binary missing (ENOENT) for ALL gh calls. */
  ghMissing?: boolean;
}

function makeRunner(script: RunnerScript): {
  runner: CommandRunner;
  calls: RunnerCall[];
} {
  const calls: RunnerCall[] = [];
  const notFound: CommandResult = { stdout: "", exitCode: -1, notFound: true };
  const failed: CommandResult = { stdout: "", exitCode: 1, notFound: false };

  const runner: CommandRunner = (command, args) => {
    calls.push({ command, args: [...args] });

    if (command === "gh") {
      if (script.ghMissing) return notFound;
      if (args[0] === "pr" && args[1] === "view") {
        return script.ghPrView ?? failed;
      }
      if (args[0] === "repo" && args[1] === "view") {
        return script.ghRepoView ?? failed;
      }
      return failed;
    }

    if (command === "git") {
      if (args[0] === "config") {
        return script.gitConfig ?? failed;
      }
      if (args[0] === "symbolic-ref") {
        return script.gitSymbolicRef ?? failed;
      }
      return failed;
    }

    return notFound;
  };

  return { runner, calls };
}

const ok = (stdout: string): CommandResult => ({
  stdout,
  exitCode: 0,
  notFound: false,
});

const exit = (exitCode: number): CommandResult => ({
  stdout: "",
  exitCode,
  notFound: false,
});

// ─────────────────────────────────────────────────────────────────────────────
// Step 1: explicit --base flag wins
// ─────────────────────────────────────────────────────────────────────────────

describe("resolveBaseBranch — explicit --base", () => {
  it("returns the explicit value with source 'explicit'", async () => {
    const { runner, calls } = makeRunner({});
    const result = await resolveBaseBranch({
      explicit: "develop",
      currentBranch: "feat/whatever",
      cwd: "/tmp/repo",
      runner,
    });

    expect(result).toEqual({ base: "develop", source: "explicit" });
    // Critical: explicit short-circuits — no shell-outs at all.
    expect(calls).toEqual([]);
  });

  it("trims whitespace on the explicit value", async () => {
    const { runner } = makeRunner({});
    const result = await resolveBaseBranch({
      explicit: "  release/2.0  ",
      currentBranch: "feat/whatever",
      cwd: "/tmp/repo",
      runner,
    });
    expect(result.base).toBe("release/2.0");
  });

  it("ignores empty-string explicit and falls through to step 2", async () => {
    const { runner, calls } = makeRunner({
      ghPrView: ok("main"),
    });
    const result = await resolveBaseBranch({
      explicit: "",
      currentBranch: "feat/whatever",
      cwd: "/tmp/repo",
      runner,
    });
    expect(result).toEqual({ base: "main", source: "pr" });
    // Step 2 was called, so explicit was treated as not-set.
    expect(calls.some((c) => c.command === "gh" && c.args[0] === "pr")).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Step 2: open PR's base wins over config + default
// ─────────────────────────────────────────────────────────────────────────────

describe("resolveBaseBranch — open PR base", () => {
  it("returns the PR base when gh pr view returns a value", async () => {
    const { runner, calls } = makeRunner({
      ghPrView: ok("release/2.0"),
      // Even with config + default set, PR base wins.
      gitConfig: ok("staging"),
      ghRepoView: ok("main"),
    });
    const result = await resolveBaseBranch({
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
    });

    expect(result).toEqual({ base: "release/2.0", source: "pr" });
    // Step 3 + 4 should NOT have run (short-circuit).
    expect(
      calls.find((c) => c.command === "git" && c.args[0] === "config"),
    ).toBeUndefined();
    expect(
      calls.find((c) => c.command === "gh" && c.args[0] === "repo"),
    ).toBeUndefined();
  });

  it("falls through when gh pr view returns empty (no PR or non-OPEN)", async () => {
    const { runner } = makeRunner({
      ghPrView: ok(""),
      gitConfig: ok("staging"),
    });
    const result = await resolveBaseBranch({
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
    });
    expect(result).toEqual({ base: "staging", source: "config" });
  });

  it("falls through when gh pr view exits non-zero", async () => {
    const { runner } = makeRunner({
      ghPrView: exit(1),
      gitConfig: ok("staging"),
    });
    const result = await resolveBaseBranch({
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
    });
    expect(result).toEqual({ base: "staging", source: "config" });
  });

  it("falls through when gh is missing", async () => {
    const { runner } = makeRunner({
      ghMissing: true,
      gitConfig: ok("staging"),
    });
    const result = await resolveBaseBranch({
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
    });
    expect(result).toEqual({ base: "staging", source: "config" });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Step 3: git config wins over default
// ─────────────────────────────────────────────────────────────────────────────

describe("resolveBaseBranch — git config override", () => {
  it("returns the configured value when no PR but config set", async () => {
    const { runner, calls } = makeRunner({
      ghPrView: ok(""),
      gitConfig: ok("release/2.0"),
      ghRepoView: ok("main"), // would win at step 4 if we got there
    });
    const result = await resolveBaseBranch({
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
    });

    expect(result).toEqual({ base: "release/2.0", source: "config" });
    // Step 4 should NOT have run.
    expect(
      calls.find((c) => c.command === "gh" && c.args[0] === "repo"),
    ).toBeUndefined();
  });

  it("falls through when git config returns empty", async () => {
    const { runner } = makeRunner({
      ghPrView: ok(""),
      gitConfig: ok(""),
      ghRepoView: ok("main"),
    });
    const result = await resolveBaseBranch({
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
    });
    expect(result).toEqual({ base: "main", source: "default" });
  });

  it("falls through when git config exits non-zero (key not set)", async () => {
    const { runner } = makeRunner({
      ghPrView: ok(""),
      gitConfig: exit(1),
      ghRepoView: ok("main"),
    });
    const result = await resolveBaseBranch({
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
    });
    expect(result).toEqual({ base: "main", source: "default" });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Step 4: default branch fallback (gh repo view + origin/HEAD)
// ─────────────────────────────────────────────────────────────────────────────

describe("resolveBaseBranch — default branch fallback", () => {
  it("returns gh-detected default branch", async () => {
    const { runner } = makeRunner({
      ghPrView: ok(""),
      gitConfig: exit(1),
      ghRepoView: ok("main"),
    });
    const result = await resolveBaseBranch({
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
    });
    expect(result).toEqual({ base: "main", source: "default" });
  });

  it("falls back to origin/HEAD parsing when gh is missing", async () => {
    const { runner } = makeRunner({
      ghMissing: true,
      gitConfig: exit(1),
      gitSymbolicRef: ok("refs/remotes/origin/main"),
    });
    const result = await resolveBaseBranch({
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
    });
    expect(result).toEqual({ base: "main", source: "default" });
  });

  it("falls back to origin/HEAD when gh repo view exits non-zero", async () => {
    const { runner } = makeRunner({
      ghPrView: ok(""),
      gitConfig: exit(1),
      ghRepoView: exit(1),
      gitSymbolicRef: ok("refs/remotes/origin/develop"),
    });
    const result = await resolveBaseBranch({
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
    });
    expect(result).toEqual({ base: "develop", source: "default" });
  });

  it("strips refs/remotes/origin/ prefix correctly", async () => {
    const { runner } = makeRunner({
      ghMissing: true,
      gitConfig: exit(1),
      gitSymbolicRef: ok("refs/remotes/origin/release/2.0"),
    });
    const result = await resolveBaseBranch({
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
    });
    expect(result.base).toBe("release/2.0");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// All four steps fail → actionable error
// ─────────────────────────────────────────────────────────────────────────────

describe("resolveBaseBranch — all steps fail", () => {
  it("throws an actionable error when nothing resolves", async () => {
    const { runner } = makeRunner({
      ghMissing: true,
      gitConfig: exit(1),
      gitSymbolicRef: exit(1),
    });

    await expect(
      resolveBaseBranch({
        currentBranch: "feat/x",
        cwd: "/tmp/repo",
        runner,
      }),
    ).rejects.toThrow(
      /Could not auto-detect base branch\. Pass --base <ref> explicitly, or set git config loaf\.release\.base <ref>/,
    );
  });

  it("throws when gh repo view returns empty AND origin/HEAD is missing", async () => {
    const { runner } = makeRunner({
      ghPrView: ok(""),
      gitConfig: exit(1),
      ghRepoView: ok(""),
      gitSymbolicRef: exit(1),
    });

    await expect(
      resolveBaseBranch({
        currentBranch: "feat/x",
        cwd: "/tmp/repo",
        runner,
      }),
    ).rejects.toThrow(/Could not auto-detect base branch/);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// skipPRLookup option (post-merge flow)
// ─────────────────────────────────────────────────────────────────────────────

describe("resolveBaseBranch — skipPRLookup", () => {
  it("never invokes `gh pr view` when skipPRLookup is true", async () => {
    // Even with a PR base scripted to win at step 2, the runner should not be
    // called for `gh pr view` at all. Step 3 (config) wins instead.
    const { runner, calls } = makeRunner({
      ghPrView: ok("would-be-pr-base"),
      gitConfig: ok("config-base"),
      ghRepoView: ok("default-base"),
    });
    const result = await resolveBaseBranch({
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
      skipPRLookup: true,
    });

    expect(result).toEqual({ base: "config-base", source: "config" });
    // Critical: `gh pr view` was never invoked.
    expect(
      calls.find((c) => c.command === "gh" && c.args[0] === "pr"),
    ).toBeUndefined();
  });

  it("falls through PR-tier even when a PR exists; default-branch wins when config is unset", async () => {
    // Mirrors the post-merge scenario: closed/merged PR yields nothing useful,
    // config not set, default branch is the answer.
    const { runner, calls } = makeRunner({
      ghPrView: ok("would-be-pr-base"), // would have won at step 2 if not skipped
      gitConfig: exit(1), // not set
      ghRepoView: ok("main"),
    });
    const result = await resolveBaseBranch({
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
      skipPRLookup: true,
    });

    expect(result).toEqual({ base: "main", source: "default" });
    expect(
      calls.find((c) => c.command === "gh" && c.args[0] === "pr"),
    ).toBeUndefined();
  });

  it("preserves PR-tier behavior when skipPRLookup is unset (regression)", async () => {
    // Same fixture as the prior test but without skipPRLookup — PR-tier fires
    // and wins. Guards against accidental flag inversion.
    const { runner, calls } = makeRunner({
      ghPrView: ok("pr-base"),
      gitConfig: ok("config-base"),
      ghRepoView: ok("default-base"),
    });
    const result = await resolveBaseBranch({
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
    });

    expect(result).toEqual({ base: "pr-base", source: "pr" });
    expect(
      calls.find((c) => c.command === "gh" && c.args[0] === "pr"),
    ).toBeDefined();
  });

  it("explicit --base still wins when skipPRLookup is true", async () => {
    const { runner, calls } = makeRunner({});
    const result = await resolveBaseBranch({
      explicit: "release/2.0",
      currentBranch: "feat/x",
      cwd: "/tmp/repo",
      runner,
      skipPRLookup: true,
    });
    expect(result).toEqual({ base: "release/2.0", source: "explicit" });
    // No shell-outs at all when explicit wins, regardless of skipPRLookup.
    expect(calls).toEqual([]);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Combined precedence assertion
// ─────────────────────────────────────────────────────────────────────────────

describe("resolveBaseBranch — combined precedence", () => {
  it("explicit > pr > config > default in a single fixture", async () => {
    // All four sources have distinct values. Walk them down by removing the
    // higher-priority sources one at a time and asserting the resolution
    // shifts accordingly.

    // 1. Explicit wins.
    {
      const { runner } = makeRunner({
        ghPrView: ok("pr-base"),
        gitConfig: ok("config-base"),
        ghRepoView: ok("default-base"),
      });
      const r = await resolveBaseBranch({
        explicit: "explicit-base",
        currentBranch: "feat/x",
        cwd: "/tmp/repo",
        runner,
      });
      expect(r).toEqual({ base: "explicit-base", source: "explicit" });
    }

    // 2. No explicit → PR wins.
    {
      const { runner } = makeRunner({
        ghPrView: ok("pr-base"),
        gitConfig: ok("config-base"),
        ghRepoView: ok("default-base"),
      });
      const r = await resolveBaseBranch({
        currentBranch: "feat/x",
        cwd: "/tmp/repo",
        runner,
      });
      expect(r).toEqual({ base: "pr-base", source: "pr" });
    }

    // 3. No PR → config wins.
    {
      const { runner } = makeRunner({
        ghPrView: ok(""),
        gitConfig: ok("config-base"),
        ghRepoView: ok("default-base"),
      });
      const r = await resolveBaseBranch({
        currentBranch: "feat/x",
        cwd: "/tmp/repo",
        runner,
      });
      expect(r).toEqual({ base: "config-base", source: "config" });
    }

    // 4. No PR, no config → default wins.
    {
      const { runner } = makeRunner({
        ghPrView: ok(""),
        gitConfig: exit(1),
        ghRepoView: ok("default-base"),
      });
      const r = await resolveBaseBranch({
        currentBranch: "feat/x",
        cwd: "/tmp/repo",
        runner,
      });
      expect(r).toEqual({ base: "default-base", source: "default" });
    }
  });
});
