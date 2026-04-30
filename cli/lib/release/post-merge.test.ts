/**
 * Post-Merge Guardrail + Action Sequence Tests
 *
 * Unit tests for `checkPostMergeGuardrails` + `executePostMergeActions`
 * (SPEC-031 / TASK-142).
 *
 * Strategy:
 *
 *   - Inject a fake `CommandRunner` (mirroring the pattern from base.test.ts).
 *   - For filesystem reads (CHANGELOG.md, version files), drive a temp directory
 *     so `existsSync` and `readFileSync` see the right content. Each test owns
 *     its own tempdir; cleanup runs in afterEach.
 *
 * The verbatim idempotency abort messages are asserted with `expect.stringMatching`
 * + literal strings — any drift from the spec breaks the test.
 */

import { describe, it, expect, afterEach, beforeEach } from "vitest";
import { mkdtempSync, rmSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";

import {
  checkPostMergeGuardrails,
  executePostMergeActions,
  extractChangelogSection,
  type PostMergeLogger,
} from "./post-merge.js";
import type { CommandResult, CommandRunner } from "./base.js";

// ─────────────────────────────────────────────────────────────────────────────
// Test runner factory — pattern mirrors base.test.ts but with a richer surface
// ─────────────────────────────────────────────────────────────────────────────

interface RunnerCall {
  command: string;
  args: string[];
}

interface RunnerScript {
  /**
   * Each entry maps a `<command> <first-arg> <second-arg?>` key to the result.
   * Falls back to `defaults` for anything unmatched. Lookup order:
   *   1. Exact match on `<command> <args[0]> <args[1]>` (when args.length >= 2)
   *   2. Match on `<command> <args[0]>` (when args.length >= 1)
   *   3. Match on `<command>` alone
   *   4. defaults[`<command>`]
   *   5. notFound (signals "unexpected call" — surfaces loudly in tests)
   */
  responses?: Record<string, CommandResult>;
  /** Default per-command response for anything unmatched. */
  defaults?: Record<string, CommandResult>;
  /** When true, all `gh` calls return notFound (binary missing). */
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

    if (command === "gh" && script.ghMissing) return notFound;

    const responses = script.responses ?? {};
    const defaults = script.defaults ?? {};

    if (args.length >= 2) {
      const key = `${command} ${args[0]} ${args[1]}`;
      if (key in responses) return responses[key];
    }
    if (args.length >= 1) {
      const key = `${command} ${args[0]}`;
      if (key in responses) return responses[key];
    }
    if (command in responses) return responses[command];
    if (command in defaults) return defaults[command];

    return failed;
  };

  return { runner, calls };
}

const ok = (stdout = ""): CommandResult => ({
  stdout,
  exitCode: 0,
  notFound: false,
});

const exit = (code: number): CommandResult => ({
  stdout: "",
  exitCode: code,
  notFound: false,
});

const notFound: CommandResult = {
  stdout: "",
  exitCode: -1,
  notFound: true,
};

// ─────────────────────────────────────────────────────────────────────────────
// Tempdir scaffold — gives each test a clean repo-shaped fixture
// ─────────────────────────────────────────────────────────────────────────────

interface Fixture {
  cwd: string;
  cleanup(): void;
}

function makeFixture(opts: {
  packageJson?: { version: string };
  changelog?: string;
  // Other version files can be added if needed; default is package.json.
  pyprojectToml?: { version: string; section?: string };
}): Fixture {
  const cwd = mkdtempSync(join(tmpdir(), "post-merge-test-"));

  if (opts.packageJson) {
    writeFileSync(
      join(cwd, "package.json"),
      JSON.stringify({ name: "fixture", version: opts.packageJson.version }, null, 2),
    );
  }
  if (opts.pyprojectToml) {
    const section = opts.pyprojectToml.section ?? "project";
    writeFileSync(
      join(cwd, "pyproject.toml"),
      `[${section}]\nname = "fixture"\nversion = "${opts.pyprojectToml.version}"\n`,
    );
  }
  if (opts.changelog !== undefined) {
    writeFileSync(join(cwd, "CHANGELOG.md"), opts.changelog);
  }

  return {
    cwd,
    cleanup: () => rmSync(cwd, { recursive: true, force: true }),
  };
}

/**
 * The "everything is set up correctly" baseline runner script. Tests override
 * specific keys to simulate guardrail failures.
 */
function happyPathScript(opts: {
  version: string;
  base?: string;
  current?: string;
  prNumber?: string;
  changedFiles?: string[];
  featureBranch?: string;
}): RunnerScript {
  const version = opts.version;
  const base = opts.base ?? "main";
  const current = opts.current ?? base;
  const prSuffix = opts.prNumber ? ` (#${opts.prNumber})` : "";
  const changedFiles = (opts.changedFiles ?? ["CHANGELOG.md", "package.json"]).join(
    "\n",
  );

  const responses: Record<string, CommandResult> = {
    "git status": ok(""),
    "git symbolic-ref --short": ok(current),
    "git symbolic-ref refs/remotes/origin/HEAD": ok(`refs/remotes/origin/${base}`),
    "git config --get": exit(1),
    "git log": ok(`chore: release v${version}${prSuffix}`),
    "git diff": ok(changedFiles),
    "git tag --list": ok(""),
    "git tag --points-at": ok(""),
    "git ls-remote": ok(""),
    "gh release view": exit(1),
    "gh repo view": ok(base),
    "git tag -a": ok(""),
    "git push": ok(""),
    "gh release create": ok(""),
    "git pull": ok(""),
    "git branch -d": ok(""),
    "gh pr view": opts.featureBranch ? ok(opts.featureBranch) : exit(1),
  };
  return { responses };
}

// ─────────────────────────────────────────────────────────────────────────────
// extractChangelogSection helper
// ─────────────────────────────────────────────────────────────────────────────

describe("extractChangelogSection", () => {
  it("returns body lines when section exists with at least one item", () => {
    const content = `# Changelog\n\n## [Unreleased]\n\n- _No unreleased changes yet._\n\n## [1.2.3] - 2026-04-29\n\n### Added\n- New feature (abc1234)\n\n## [1.2.2] - 2026-04-01\n\n- Old feature (def5678)\n`;
    const body = extractChangelogSection(content, "1.2.3");
    expect(body).not.toBeNull();
    expect(body!.some((l) => l.includes("New feature"))).toBe(true);
  });

  it("returns null when section is absent", () => {
    const content = `# Changelog\n\n## [1.2.2]\n- something\n`;
    expect(extractChangelogSection(content, "1.2.3")).toBeNull();
  });

  it("returns empty array when section is present but has no list items", () => {
    const content = `# Changelog\n\n## [1.2.3] - 2026-04-29\n\n_section was reserved but never filled in_\n\n## [1.2.2]\n- something\n`;
    expect(extractChangelogSection(content, "1.2.3")).toEqual([]);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Guardrails 1–8
// ─────────────────────────────────────────────────────────────────────────────

describe("checkPostMergeGuardrails — guardrail 1 (clean worktree)", () => {
  let fixture: Fixture;
  beforeEach(() => {
    fixture = makeFixture({
      packageJson: { version: "1.2.3" },
      changelog: validChangelog("1.2.3"),
    });
  });
  afterEach(() => fixture.cleanup());

  it("aborts when the worktree has uncommitted changes", async () => {
    const script = happyPathScript({ version: "1.2.3" });
    script.responses!["git status"] = ok(" M cli/lib/release/post-merge.ts");
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(1);
    expect(result.message).toContain("uncommitted changes");
  });
});

describe("checkPostMergeGuardrails — guardrail 2 (on base branch)", () => {
  let fixture: Fixture;
  beforeEach(() => {
    fixture = makeFixture({
      packageJson: { version: "1.2.3" },
      changelog: validChangelog("1.2.3"),
    });
  });
  afterEach(() => fixture.cleanup());

  it("never invokes `gh pr view <branch>` for base resolution (uses unified resolver with skipPRLookup)", async () => {
    // Regression guard for the "two state models" drift: post-merge MUST NOT
    // shell out to `gh pr view <branch> --json baseRefName,...` because by
    // post-merge the PR is closed/merged. The unified resolver short-circuits
    // step 2 when called with skipPRLookup: true.
    const script = happyPathScript({ version: "1.2.3" });
    const { runner, calls } = makeRunner(script);

    await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });

    // `gh pr view <branch> --json baseRefName,state ...` is the base-resolution
    // call signature; `gh pr view <prNumber> --json headRefName ...` is the
    // separate feature-branch lookup. We forbid the former.
    const baseLookupCalls = calls.filter(
      (c) =>
        c.command === "gh" &&
        c.args[0] === "pr" &&
        c.args[1] === "view" &&
        c.args.includes("baseRefName,state"),
    );
    expect(baseLookupCalls).toEqual([]);
  });

  it("aborts when current branch is not the base and not fast-forwardable", async () => {
    const script = happyPathScript({
      version: "1.2.3",
      base: "main",
      current: "feat/something",
    });
    // Override the merge-base check to fail (not an ancestor).
    script.responses!["git merge-base"] = exit(1);
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(2);
    expect(result.message).toContain("not the base branch");
    expect(result.message).toContain("main");
  });

  it("passes when current branch IS the base", async () => {
    const script = happyPathScript({ version: "1.2.3", base: "main", current: "main" });
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    // We're not asserting full pass here — just that guardrail 2 didn't trip.
    if (!result.ok) {
      expect(result.guardrail).not.toBe(2);
    }
  });

  it("aborts on detached HEAD", async () => {
    const script = happyPathScript({ version: "1.2.3" });
    script.responses!["git symbolic-ref --short"] = exit(1);
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(2);
    expect(result.message).toContain("detached HEAD");
  });
});

describe("checkPostMergeGuardrails — guardrail 3 (subject shape)", () => {
  let fixture: Fixture;
  beforeEach(() => {
    fixture = makeFixture({
      packageJson: { version: "1.2.3" },
      changelog: validChangelog("1.2.3"),
    });
  });
  afterEach(() => fixture.cleanup());

  it("aborts when HEAD subject is not a chore: release commit", async () => {
    const script = happyPathScript({ version: "1.2.3" });
    script.responses!["git log"] = ok("feat: add new feature");
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(3);
    expect(result.message).toContain("does not match");
    expect(result.message).toContain("chore: release v<semver>");
  });

  it("aborts on non-shape chore: release like 'chore: release notes draft'", async () => {
    const script = happyPathScript({ version: "1.2.3" });
    script.responses!["git log"] = ok("chore: release notes draft");
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(3);
  });

  it("accepts the PR-number suffix form", async () => {
    const script = happyPathScript({
      version: "1.2.3",
      prNumber: "42",
    });
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    if (!result.ok) {
      expect(result.guardrail).not.toBe(3);
    }
  });
});

describe("checkPostMergeGuardrails — guardrail 4 (version match)", () => {
  let fixture: Fixture;
  afterEach(() => fixture?.cleanup?.());

  it("aborts when package.json version disagrees with the commit subject", async () => {
    fixture = makeFixture({
      packageJson: { version: "1.2.3" }, // file says 1.2.3
      changelog: validChangelog("1.2.4"),
    });
    const script = happyPathScript({ version: "1.2.4" }); // subject says 1.2.4
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(4);
    expect(result.message).toContain("package.json");
    expect(result.message).toContain("1.2.3");
    expect(result.message).toContain("1.2.4");
  });

  it("names every mismatching file in the diagnostic", async () => {
    fixture = makeFixture({
      packageJson: { version: "1.2.4" }, // matches
      pyprojectToml: { version: "1.2.3" }, // does not match
      changelog: validChangelog("1.2.4"),
    });
    const script = happyPathScript({ version: "1.2.4" });
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(4);
    expect(result.message).toContain("pyproject.toml");
    expect(result.message).toContain("1.2.3");
  });
});

describe("checkPostMergeGuardrails — guardrail 5 (diff includes CHANGELOG + version file)", () => {
  let fixture: Fixture;
  beforeEach(() => {
    fixture = makeFixture({
      packageJson: { version: "1.2.3" },
      changelog: validChangelog("1.2.3"),
    });
  });
  afterEach(() => fixture.cleanup());

  it("aborts when CHANGELOG.md is not in the diff", async () => {
    const script = happyPathScript({
      version: "1.2.3",
      changedFiles: ["package.json"], // missing CHANGELOG.md
    });
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(5);
    expect(result.message).toContain("CHANGELOG");
  });

  it("aborts when no version file is in the diff", async () => {
    const script = happyPathScript({
      version: "1.2.3",
      changedFiles: ["CHANGELOG.md", "README.md"], // no version file
    });
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(5);
    expect(result.message).toContain("version-file diff");
  });
});

describe("checkPostMergeGuardrails — guardrail 6 (CHANGELOG section non-empty)", () => {
  let fixture: Fixture;
  afterEach(() => fixture?.cleanup?.());

  it("aborts when the version section is missing", async () => {
    fixture = makeFixture({
      packageJson: { version: "1.2.3" },
      changelog: `# Changelog\n\n## [Unreleased]\n- _No unreleased changes yet._\n`,
    });
    const script = happyPathScript({ version: "1.2.3" });
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(6);
    expect(result.message).toContain("no `## [1.2.3]` section");
  });

  it("aborts when the version section has no list items", async () => {
    fixture = makeFixture({
      packageJson: { version: "1.2.3" },
      changelog: `# Changelog\n\n## [Unreleased]\n- _No unreleased changes yet._\n\n## [1.2.3] - 2026-04-29\n\n_was reserved but never filled in_\n`,
    });
    const script = happyPathScript({ version: "1.2.3" });
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(6);
    expect(result.message).toContain("no list items");
  });
});

describe("checkPostMergeGuardrails — guardrail 7 (tag/release collisions)", () => {
  let fixture: Fixture;
  beforeEach(() => {
    fixture = makeFixture({
      packageJson: { version: "1.2.3" },
      changelog: validChangelog("1.2.3"),
    });
  });
  afterEach(() => fixture.cleanup());

  it("aborts with the exact spec message when tag exists locally", async () => {
    const script = happyPathScript({ version: "1.2.3" });
    script.responses!["git tag --list"] = ok("v1.2.3");
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(7);
    expect(result.message).toBe(
      "tag v1.2.3 already exists locally — run `git tag -d v1.2.3` and rerun",
    );
  });

  it("aborts with the exact spec message when tag exists on remote", async () => {
    const script = happyPathScript({ version: "1.2.3" });
    // Local clean, remote has the tag.
    script.responses!["git ls-remote"] = ok(
      "abc123 refs/tags/v1.2.3",
    );
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(7);
    expect(result.message).toBe(
      "tag v1.2.3 already exists on remote — run `git push origin :refs/tags/v1.2.3` and rerun",
    );
  });

  it("aborts with the exact spec message when GH release exists", async () => {
    const script = happyPathScript({ version: "1.2.3" });
    script.responses!["gh release view"] = ok("release exists");
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(7);
    expect(result.message).toBe(
      "GH release v1.2.3 already exists — visit the release page and delete it manually before rerunning",
    );
  });
});

describe("checkPostMergeGuardrails — guardrail 8 (HEAD already tagged)", () => {
  let fixture: Fixture;
  beforeEach(() => {
    fixture = makeFixture({
      packageJson: { version: "1.2.3" },
      changelog: validChangelog("1.2.3"),
    });
  });
  afterEach(() => fixture.cleanup());

  it("aborts with the exact spec message when HEAD is already tagged", async () => {
    const script = happyPathScript({ version: "1.2.3" });
    script.responses!["git tag --points-at"] = ok("v1.2.3-rc1");
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.guardrail).toBe(8);
    expect(result.message).toBe(
      "HEAD is already tagged as v1.2.3-rc1; this is not a fresh post-merge state",
    );
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Happy path → all guardrails pass
// ─────────────────────────────────────────────────────────────────────────────

describe("checkPostMergeGuardrails — happy path", () => {
  let fixture: Fixture;
  beforeEach(() => {
    fixture = makeFixture({
      packageJson: { version: "1.2.3" },
      changelog: validChangelog("1.2.3"),
    });
  });
  afterEach(() => fixture.cleanup());

  it("returns ok with version, base, and changelog body when all guardrails pass", async () => {
    const script = happyPathScript({ version: "1.2.3" });
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.version).toBe("1.2.3");
    expect(result.base).toBe("main");
    expect(result.changelogBody.length).toBeGreaterThan(0);
    expect(result.changelogBody).toContain("New feature");
  });

  it("captures the feature branch from the PR-number suffix when available", async () => {
    const script = happyPathScript({
      version: "1.2.3",
      prNumber: "42",
      featureBranch: "feat/cool-thing",
    });
    const { runner } = makeRunner(script);

    const result = await checkPostMergeGuardrails({ cwd: fixture.cwd, runner });
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.featureBranch).toBe("feat/cool-thing");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Idempotency — partial-failure rerun produces actionable message
// ─────────────────────────────────────────────────────────────────────────────

describe("checkPostMergeGuardrails — light idempotency", () => {
  let fixture: Fixture;
  beforeEach(() => {
    fixture = makeFixture({
      packageJson: { version: "1.2.3" },
      changelog: validChangelog("1.2.3"),
    });
  });
  afterEach(() => fixture.cleanup());

  it("first run: simulate 'tag created locally, push failed'; second run: aborts at guardrail 7 with actionable message; third run (after manual cleanup): succeeds end-to-end", async () => {
    // Simulate state AFTER a partial run: tag exists locally.
    const stage2Script = happyPathScript({ version: "1.2.3" });
    stage2Script.responses!["git tag --list"] = ok("v1.2.3");
    const { runner: stage2Runner } = makeRunner(stage2Script);

    const stage2 = await checkPostMergeGuardrails({
      cwd: fixture.cwd,
      runner: stage2Runner,
    });
    expect(stage2.ok).toBe(false);
    if (!stage2.ok) {
      expect(stage2.guardrail).toBe(7);
      expect(stage2.message).toBe(
        "tag v1.2.3 already exists locally — run `git tag -d v1.2.3` and rerun",
      );
    }

    // User runs `git tag -d v1.2.3`. Third run: tag is gone, all clean.
    const stage3Script = happyPathScript({ version: "1.2.3" });
    const { runner: stage3Runner } = makeRunner(stage3Script);

    const stage3 = await checkPostMergeGuardrails({
      cwd: fixture.cwd,
      runner: stage3Runner,
    });
    expect(stage3.ok).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// executePostMergeActions
// ─────────────────────────────────────────────────────────────────────────────

describe("executePostMergeActions", () => {
  let fixture: Fixture;
  beforeEach(() => {
    fixture = makeFixture({
      packageJson: { version: "1.2.3" },
      changelog: validChangelog("1.2.3"),
    });
  });
  afterEach(() => fixture.cleanup());

  it("happy path: tag, push, release, pull, delete (local + remote) all succeed", async () => {
    const script = happyPathScript({ version: "1.2.3" });
    const { runner } = makeRunner(script);

    const result = await executePostMergeActions(
      { cwd: fixture.cwd, runner },
      {
        ok: true,
        version: "1.2.3",
        base: "main",
        featureBranch: "feat/cool-thing",
        changelogBody: "- A nifty change",
        versionFiles: [],
      },
    );

    expect(result.tagged).toBe(true);
    expect(result.pushed).toBe(true);
    expect(result.released).toBe(true);
    expect(result.pulled).toBe(true);
    expect(result.deleted.local).toBe(true);
    expect(result.deleted.remote).toBe(true);
  });

  it("local branch delete fails → remote delete still attempted, deleted.local=false", async () => {
    const script = happyPathScript({ version: "1.2.3" });
    script.responses!["git branch -d"] = exit(1);
    const { runner } = makeRunner(script);

    const result = await executePostMergeActions(
      { cwd: fixture.cwd, runner },
      {
        ok: true,
        version: "1.2.3",
        base: "main",
        featureBranch: "feat/cool-thing",
        changelogBody: "- A nifty change",
        versionFiles: [],
      },
    );

    expect(result.deleted.local).toBe(false);
    expect(result.deleted.remote).toBe(true);
  });

  it("pull fails → warn-and-continue, pulled=false but tag/release still succeed", async () => {
    const script = happyPathScript({ version: "1.2.3" });
    script.responses!["git pull"] = exit(1);
    const warnings: string[] = [];
    const logger: PostMergeLogger = {
      ok: () => undefined,
      warn: (m) => warnings.push(m),
      err: () => undefined,
    };
    const { runner } = makeRunner(script);

    const result = await executePostMergeActions(
      { cwd: fixture.cwd, runner },
      {
        ok: true,
        version: "1.2.3",
        base: "main",
        changelogBody: "- A nifty change",
        versionFiles: [],
      },
      logger,
    );

    expect(result.tagged).toBe(true);
    expect(result.released).toBe(true);
    expect(result.pulled).toBe(false);
    expect(warnings.some((w) => w.includes("Failed to pull"))).toBe(true);
  });

  it("skips branch deletion entirely when no feature branch is known", async () => {
    const script = happyPathScript({ version: "1.2.3" });
    const { runner, calls } = makeRunner(script);

    const result = await executePostMergeActions(
      { cwd: fixture.cwd, runner },
      {
        ok: true,
        version: "1.2.3",
        base: "main",
        changelogBody: "- A nifty change",
        versionFiles: [],
      },
    );

    expect(result.deleted.local).toBeUndefined();
    expect(result.deleted.remote).toBeUndefined();
    expect(calls.find((c) => c.command === "git" && c.args[0] === "branch")).toBeUndefined();
  });

  it("tag push failure throws — caller handles the exit", async () => {
    const script = happyPathScript({ version: "1.2.3" });
    script.responses!["git push"] = exit(1);
    const { runner } = makeRunner(script);

    await expect(
      executePostMergeActions(
        { cwd: fixture.cwd, runner },
        {
          ok: true,
          version: "1.2.3",
          base: "main",
          changelogBody: "- A nifty change",
          versionFiles: [],
        },
      ),
    ).rejects.toThrow(/failed to push tag/);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Build a CHANGELOG.md fixture with a populated [Unreleased] (stub-only) and
 * a versioned section that has at least one list item.
 */
function validChangelog(version: string): string {
  return [
    "# Changelog",
    "",
    "## [Unreleased]",
    "",
    "- _No unreleased changes yet._",
    "",
    `## [${version}] - 2026-04-29`,
    "",
    "### Added",
    "- New feature (abc1234)",
    "",
  ].join("\n");
}
