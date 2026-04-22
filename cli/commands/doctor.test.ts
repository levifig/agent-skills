/**
 * Doctor Command Tests
 *
 * Tests for the `loaf doctor` command — alignment diagnostics for symlinks,
 * stale files, fenced-section versioning, and duplication.
 *
 * Each check is exercised pass/warn/fail/skip. Fix flow is covered for the
 * three fixable checks. Exit-code logic is validated by invoking the CLI
 * binary as a subprocess, mirroring the pattern in check.test.ts.
 *
 * @vitest-environment node
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { execFileSync } from "child_process";
import {
  existsSync,
  lstatSync,
  mkdirSync,
  readFileSync,
  readlinkSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "fs";
import { dirname, join } from "path";

import { runDoctor, CHECKS, type CheckContext } from "./doctor.js";

// ─────────────────────────────────────────────────────────────────────────────
// Fixtures
// ─────────────────────────────────────────────────────────────────────────────

const TEST_ROOT = join(process.cwd(), ".test-doctor-command");

// Strip ANSI escape codes so substring assertions don't trip on colour.
const stripAnsi = (s: string) => s.replace(/\x1b\[\d+m/g, "");

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

function write(path: string, content: string): void {
  const full = join(TEST_ROOT, path);
  mkdirSync(dirname(full), { recursive: true });
  writeFileSync(full, content);
}

function makeSymlink(linkRel: string, targetRel: string): void {
  const linkPath = join(TEST_ROOT, linkRel);
  mkdirSync(dirname(linkPath), { recursive: true });
  // Use a relative target, matching real project layout.
  symlinkSync(targetRel, linkPath);
}

function fencedContent(version = "2.0.0-dev.27"): string {
  return [
    `<!-- loaf:managed:start v${version} -->`,
    "<!-- Maintained by loaf install/upgrade — do not edit manually -->",
    "## Loaf Framework",
    "",
    "Sample fenced content.",
    "<!-- loaf:managed:end -->",
    "",
  ].join("\n");
}

/** Run a single check in isolation. Returns its CheckResult. */
function runSingle(name: string) {
  const check = CHECKS.find((c) => c.name === name);
  if (!check) throw new Error(`Unknown check: ${name}`);
  const ctx: CheckContext = { projectRoot: TEST_ROOT };
  return { check, result: check.run(ctx) };
}

/** Capture stdout lines emitted by runDoctor (it uses console.log). */
function captureDoctor(options: { fix?: boolean; verbose?: boolean } = {}) {
  const lines: string[] = [];
  const spy = vi.spyOn(console, "log").mockImplementation((...args) => {
    lines.push(args.map((a) => String(a)).join(" "));
  });
  try {
    const outcome = runDoctor({
      fix: options.fix ?? false,
      verbose: options.verbose ?? false,
      projectRoot: TEST_ROOT,
    });
    return { outcome, output: stripAnsi(lines.join("\n")) };
  } finally {
    spy.mockRestore();
  }
}

// Read current loaf version so fenced-version tests stay in lockstep with
// package.json (no duplicate source of truth in the test).
function getInstalledVersion(): string {
  const pkg = JSON.parse(readFileSync(join(process.cwd(), "package.json"), "utf-8"));
  return pkg.version as string;
}

// ─────────────────────────────────────────────────────────────────────────────
// Setup / Teardown
// ─────────────────────────────────────────────────────────────────────────────

beforeEach(() => {
  try {
    rmSync(TEST_ROOT, { recursive: true, force: true });
  } catch {
    // ignore
  }
  mkdirSync(TEST_ROOT, { recursive: true });
});

afterEach(() => {
  rmSync(TEST_ROOT, { recursive: true, force: true });
});

// ─────────────────────────────────────────────────────────────────────────────
// Check: agents-symlink
// ─────────────────────────────────────────────────────────────────────────────

describe("doctor: agents-symlink", () => {
  it("passes when ./AGENTS.md is a symlink to .agents/AGENTS.md", () => {
    write(".agents/AGENTS.md", fencedContent());
    makeSymlink("AGENTS.md", ".agents/AGENTS.md");

    const { result } = runSingle("agents-symlink");
    expect(result.status).toBe("pass");
  });

  it("fails when ./AGENTS.md is missing", () => {
    write(".agents/AGENTS.md", fencedContent());

    const { result } = runSingle("agents-symlink");
    expect(result.status).toBe("fail");
    expect(result.fixable).toBe(true);
  });

  it("fails when ./AGENTS.md is a real file (not a symlink) — now fixable via safe merge", () => {
    write(".agents/AGENTS.md", fencedContent());
    write("AGENTS.md", "# stray real file\n");

    const { result } = runSingle("agents-symlink");
    expect(result.status).toBe("fail");
    // Real files are now fixable — doctor --fix merges + .bak + symlink.
    expect(result.fixable).toBe(true);
  });

  it("fails when ./AGENTS.md symlink points to the wrong target", () => {
    write(".agents/AGENTS.md", fencedContent());
    write("somewhere/else.md", "wrong target\n");
    makeSymlink("AGENTS.md", "somewhere/else.md");

    const { result } = runSingle("agents-symlink");
    expect(result.status).toBe("fail");
    expect(result.fixable).toBe(true);
  });

  it("skips when canonical .agents/AGENTS.md does not exist", () => {
    const { result } = runSingle("agents-symlink");
    expect(result.status).toBe("skip");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Check: claude-symlink
// ─────────────────────────────────────────────────────────────────────────────

describe("doctor: claude-symlink", () => {
  it("passes when .claude/CLAUDE.md is a symlink to .agents/AGENTS.md", () => {
    write(".agents/AGENTS.md", fencedContent());
    makeSymlink(".claude/CLAUDE.md", "../.agents/AGENTS.md");

    const { result } = runSingle("claude-symlink");
    expect(result.status).toBe("pass");
  });

  it("fails when .claude/CLAUDE.md is missing", () => {
    write(".agents/AGENTS.md", fencedContent());

    const { result } = runSingle("claude-symlink");
    expect(result.status).toBe("fail");
    expect(result.fixable).toBe(true);
  });

  it("fails when .claude/CLAUDE.md is a real file — now fixable via safe merge", () => {
    write(".agents/AGENTS.md", fencedContent());
    write(".claude/CLAUDE.md", "# real file\n");

    const { result } = runSingle("claude-symlink");
    expect(result.status).toBe("fail");
    expect(result.fixable).toBe(true);
  });

  it("skips when canonical .agents/AGENTS.md does not exist", () => {
    const { result } = runSingle("claude-symlink");
    expect(result.status).toBe("skip");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Check: canonical-agents-file
// ─────────────────────────────────────────────────────────────────────────────

describe("doctor: canonical-agents-file", () => {
  it("passes when symlinks exist and canonical file exists", () => {
    write(".agents/AGENTS.md", fencedContent());
    makeSymlink("AGENTS.md", ".agents/AGENTS.md");

    const { result } = runSingle("canonical-agents-file");
    expect(result.status).toBe("pass");
  });

  it("fails when symlinks point at a missing canonical file", () => {
    // Create a dangling symlink without the target.
    makeSymlink("AGENTS.md", ".agents/AGENTS.md");

    const { result } = runSingle("canonical-agents-file");
    expect(result.status).toBe("fail");
  });

  it("skips when no symlinks reference .agents/AGENTS.md", () => {
    // No symlinks, no canonical file.
    const { result } = runSingle("canonical-agents-file");
    expect(result.status).toBe("skip");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Check: stale-cursor-mdc
// ─────────────────────────────────────────────────────────────────────────────

describe("doctor: stale-cursor-mdc", () => {
  it("passes when .cursor/rules/loaf.mdc is absent", () => {
    const { result } = runSingle("stale-cursor-mdc");
    expect(result.status).toBe("pass");
  });

  it("fails when .cursor/rules/loaf.mdc exists", () => {
    write(".cursor/rules/loaf.mdc", "---\ndescription: legacy\n---\nold content\n");

    const { result } = runSingle("stale-cursor-mdc");
    expect(result.status).toBe("fail");
    expect(result.fixable).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Check: fenced-version
// ─────────────────────────────────────────────────────────────────────────────

describe("doctor: fenced-version", () => {
  it("skips when .agents/AGENTS.md is absent", () => {
    const { result } = runSingle("fenced-version");
    expect(result.status).toBe("skip");
  });

  it("warns when no fenced section is present", () => {
    write(".agents/AGENTS.md", "# project agents\n\nno fence here\n");

    const { result } = runSingle("fenced-version");
    expect(result.status).toBe("warn");
  });

  it("warns when fenced section version drifts from installed loaf", () => {
    write(".agents/AGENTS.md", fencedContent("0.0.1-test"));

    const { result } = runSingle("fenced-version");
    expect(result.status).toBe("warn");
    expect(result.message).toContain("0.0.1-test");
  });

  it("passes when fenced section matches installed loaf", () => {
    write(".agents/AGENTS.md", fencedContent(getInstalledVersion()));

    const { result } = runSingle("fenced-version");
    expect(result.status).toBe("pass");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Check: duplicate-fenced-sections
// ─────────────────────────────────────────────────────────────────────────────

describe("doctor: duplicate-fenced-sections", () => {
  it("skips when .claude/CLAUDE.md is a symlink", () => {
    write(".agents/AGENTS.md", fencedContent());
    makeSymlink(".claude/CLAUDE.md", "../.agents/AGENTS.md");

    const { result } = runSingle("duplicate-fenced-sections");
    expect(result.status).toBe("skip");
  });

  it("skips when only one side has a fenced section", () => {
    write(".agents/AGENTS.md", fencedContent());
    // .claude/CLAUDE.md absent.
    const { result } = runSingle("duplicate-fenced-sections");
    expect(result.status).toBe("skip");
  });

  it("passes when both are real files but neither has a fenced section", () => {
    write(".agents/AGENTS.md", "# no fence\n");
    write(".claude/CLAUDE.md", "# no fence either\n");

    const { result } = runSingle("duplicate-fenced-sections");
    expect(result.status).toBe("pass");
  });

  it("fails when both real files carry fenced sections", () => {
    write(".agents/AGENTS.md", fencedContent());
    write(".claude/CLAUDE.md", fencedContent());

    const { result } = runSingle("duplicate-fenced-sections");
    expect(result.status).toBe("fail");
    expect(result.message).toContain("Duplicate");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// --fix behaviour
// ─────────────────────────────────────────────────────────────────────────────

describe("doctor: --fix", () => {
  it("creates missing ./AGENTS.md symlink when --fix is set", () => {
    write(".agents/AGENTS.md", fencedContent(getInstalledVersion()));

    const { outcome, output } = captureDoctor({ fix: true });

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    expect(existsSync(linkPath)).toBe(true);
    expect(lstatSync(linkPath).isSymbolicLink()).toBe(true);
    expect(readlinkSync(linkPath)).toBe(".agents/AGENTS.md");

    // Post-fix, agents-symlink should re-evaluate as pass and the run should
    // tally at least one applied fix.
    expect(outcome.report.fixesApplied).toBeGreaterThanOrEqual(1);
    expect(output).toContain("Created ./AGENTS.md");
  });

  it("creates missing .claude/CLAUDE.md symlink when --fix is set", () => {
    write(".agents/AGENTS.md", fencedContent(getInstalledVersion()));

    captureDoctor({ fix: true });

    const linkPath = join(TEST_ROOT, ".claude", "CLAUDE.md");
    expect(existsSync(linkPath)).toBe(true);
    expect(lstatSync(linkPath).isSymbolicLink()).toBe(true);
    // Relative target from .claude/ to .agents/AGENTS.md.
    expect(readlinkSync(linkPath)).toBe("../.agents/AGENTS.md");
  });

  it("removes stale .cursor/rules/loaf.mdc when --fix is set", () => {
    write(".cursor/rules/loaf.mdc", "legacy\n");

    const { outcome } = captureDoctor({ fix: true });

    expect(existsSync(join(TEST_ROOT, ".cursor", "rules", "loaf.mdc"))).toBe(false);
    expect(outcome.report.fixesApplied).toBeGreaterThanOrEqual(1);
  });

  it("migrates a real ./AGENTS.md file into canonical and replaces with symlink", () => {
    write(".agents/AGENTS.md", fencedContent(getInstalledVersion()));
    write("AGENTS.md", "# user notes\n\nimportant project context\n");

    const { outcome } = captureDoctor({ fix: true });

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    // Symlink replaces the real file.
    expect(lstatSync(linkPath).isSymbolicLink()).toBe(true);
    expect(readlinkSync(linkPath)).toBe(".agents/AGENTS.md");

    // .bak safety net preserves the original, untouched.
    const backup = readFileSync(`${linkPath}.bak`, "utf-8");
    expect(backup).toContain("# user notes");
    expect(backup).toContain("important project context");

    // Canonical now contains the user content appended under a migration heading.
    const canonical = readFileSync(
      join(TEST_ROOT, ".agents", "AGENTS.md"),
      "utf-8",
    );
    expect(canonical).toContain("## Migrated from AGENTS.md");
    expect(canonical).toContain("important project context");

    // Exit code is now 0 — nothing is left failing.
    expect(outcome.exitCode).toBe(0);
    expect(outcome.report.fixesApplied).toBeGreaterThanOrEqual(1);
  });

  it("migrates a real .claude/CLAUDE.md file into canonical and replaces with symlink", () => {
    write(".agents/AGENTS.md", fencedContent(getInstalledVersion()));
    write(".claude/CLAUDE.md", "# claude-specific\n\nuser text\n");

    captureDoctor({ fix: true });

    const linkPath = join(TEST_ROOT, ".claude", "CLAUDE.md");
    expect(lstatSync(linkPath).isSymbolicLink()).toBe(true);
    expect(readlinkSync(linkPath)).toBe("../.agents/AGENTS.md");

    // .bak preserves original.
    expect(readFileSync(`${linkPath}.bak`, "utf-8")).toContain(
      "claude-specific",
    );

    const canonical = readFileSync(
      join(TEST_ROOT, ".agents", "AGENTS.md"),
      "utf-8",
    );
    // Relative path from project root has platform-specific separator — use join.
    expect(canonical).toContain(
      `## Migrated from ${join(".claude", "CLAUDE.md")}`,
    );
    expect(canonical).toContain("user text");
  });

  it("resolves duplicate fenced sections by migrating .claude/CLAUDE.md into canonical", () => {
    write(".agents/AGENTS.md", fencedContent(getInstalledVersion()));
    write(
      ".claude/CLAUDE.md",
      "# claude notes\n\n" +
        "<!-- loaf:managed:start v0.1.0 -->\nold fence\n<!-- loaf:managed:end -->\n" +
        "\ntrailing text\n",
    );

    const { outcome } = captureDoctor({ fix: true });

    const claudeLink = join(TEST_ROOT, ".claude", "CLAUDE.md");
    // Now a symlink — duplicate-fenced-sections drift is resolved.
    expect(lstatSync(claudeLink).isSymbolicLink()).toBe(true);
    expect(existsSync(`${claudeLink}.bak`)).toBe(true);

    // Canonical picked up the user content (fence stripped).
    const canonical = readFileSync(
      join(TEST_ROOT, ".agents", "AGENTS.md"),
      "utf-8",
    );
    expect(canonical).toContain("# claude notes");
    expect(canonical).toContain("trailing text");
    // The OLD fence from .claude/CLAUDE.md must not have been carried over —
    // the canonical's own fence is the authoritative one.
    expect(canonical).not.toContain("old fence");

    expect(outcome.report.fixesApplied).toBeGreaterThanOrEqual(1);
  });

  it("rewires ./AGENTS.md when it points at the wrong target", () => {
    write(".agents/AGENTS.md", fencedContent(getInstalledVersion()));
    write("wrong-target.md", "wrong\n");
    makeSymlink("AGENTS.md", "wrong-target.md");

    captureDoctor({ fix: true });

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    expect(readlinkSync(linkPath)).toBe(".agents/AGENTS.md");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Exit-code logic
// ─────────────────────────────────────────────────────────────────────────────

describe("doctor: exit codes", () => {
  it("exits 0 on a fully healthy project", () => {
    write(".agents/AGENTS.md", fencedContent(getInstalledVersion()));
    makeSymlink("AGENTS.md", ".agents/AGENTS.md");
    makeSymlink(".claude/CLAUDE.md", "../.agents/AGENTS.md");

    const { outcome } = captureDoctor();
    expect(outcome.exitCode).toBe(0);
    expect(outcome.report.failures).toBe(0);
  });

  it("exits 1 when any check fails", () => {
    // Stale cursor file — a fail without fix.
    write(".cursor/rules/loaf.mdc", "legacy\n");

    const { outcome } = captureDoctor();
    expect(outcome.exitCode).toBe(1);
    expect(outcome.report.failures).toBeGreaterThan(0);
  });

  it("exits 0 when only warnings are present (version drift)", () => {
    // Healthy symlinks, but fenced section version mismatches.
    write(".agents/AGENTS.md", fencedContent("0.0.1-test"));
    makeSymlink("AGENTS.md", ".agents/AGENTS.md");
    makeSymlink(".claude/CLAUDE.md", "../.agents/AGENTS.md");

    const { outcome } = captureDoctor();
    expect(outcome.exitCode).toBe(0);
    expect(outcome.report.warnings).toBeGreaterThan(0);
    expect(outcome.report.failures).toBe(0);
  });

  it("exits 0 after --fix repairs all failures", () => {
    write(".agents/AGENTS.md", fencedContent(getInstalledVersion()));
    write(".cursor/rules/loaf.mdc", "legacy\n");

    const { outcome } = captureDoctor({ fix: true });
    expect(outcome.exitCode).toBe(0);
    expect(outcome.report.fixesApplied).toBeGreaterThanOrEqual(2);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// --verbose
// ─────────────────────────────────────────────────────────────────────────────

describe("doctor: --verbose", () => {
  it("prints each check name even for passing checks", () => {
    write(".agents/AGENTS.md", fencedContent(getInstalledVersion()));
    makeSymlink("AGENTS.md", ".agents/AGENTS.md");
    makeSymlink(".claude/CLAUDE.md", "../.agents/AGENTS.md");

    const { output } = captureDoctor({ verbose: true });

    // Every registered check name should appear in verbose mode.
    for (const check of CHECKS) {
      expect(output).toContain(check.name);
    }
  });

  it("omits passing check names in default (non-verbose) mode", () => {
    write(".agents/AGENTS.md", fencedContent(getInstalledVersion()));
    makeSymlink("AGENTS.md", ".agents/AGENTS.md");
    makeSymlink(".claude/CLAUDE.md", "../.agents/AGENTS.md");

    const { output } = captureDoctor();

    // No fails and no warnings → no check names printed (just summaries).
    // The identifying "<name> — ..." label shouldn't appear for a passing run.
    expect(output).not.toContain("agents-symlink —");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// CLI binary smoke tests (exit codes from the real binary)
// Uses execFileSync with a fixed arg vector — no shell interpolation.
// ─────────────────────────────────────────────────────────────────────────────

describe("doctor: CLI binary", () => {
  const BINARY = join(process.cwd(), "dist-cli", "index.js");

  function runBinary(
    args: string[],
    cwd: string,
  ): { exitCode: number; stdout: string; stderr: string } {
    try {
      const stdout = execFileSync("node", [BINARY, "doctor", ...args], {
        encoding: "utf-8",
        cwd,
        stdio: ["pipe", "pipe", "pipe"],
        timeout: 15000,
      });
      return { exitCode: 0, stdout, stderr: "" };
    } catch (error: unknown) {
      const err = error as { status?: number; stdout?: string; stderr?: string };
      return {
        exitCode: err.status ?? 1,
        stdout: err.stdout ?? "",
        stderr: err.stderr ?? "",
      };
    }
  }

  it("exits 0 on a healthy project", () => {
    // Skip if the CLI hasn't been built (tests run before build in CI).
    if (!existsSync(BINARY)) return;

    // Healthy: .agents/AGENTS.md with matching version + both symlinks.
    write(".agents/AGENTS.md", fencedContent(getInstalledVersion()));
    makeSymlink("AGENTS.md", ".agents/AGENTS.md");
    makeSymlink(".claude/CLAUDE.md", "../.agents/AGENTS.md");

    const result = runBinary([], TEST_ROOT);
    expect(result.exitCode).toBe(0);
    expect(stripAnsi(result.stdout)).toContain("loaf doctor");
  });

  it("exits 1 when stale .cursor/rules/loaf.mdc is present", () => {
    if (!existsSync(BINARY)) return;

    write(".cursor/rules/loaf.mdc", "legacy\n");
    const result = runBinary([], TEST_ROOT);
    expect(result.exitCode).toBe(1);
  });
});
