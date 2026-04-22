/**
 * Symlinks Helper Tests
 *
 * Exercises the state machine in `ensureSymlink` and the project-level
 * convenience wrapper `ensureProjectSymlinks`. Prompts are injected via the
 * `prompt` option so these tests never touch stdin/readline.
 *
 * @vitest-environment node
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
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
import { dirname, join, relative } from "path";

import {
  ensureSymlink,
  ensureProjectSymlinks,
  mergeContentIntoCanonical,
  needsRootAgentsSymlink,
  relativeLinkTarget,
  stripLoafFence,
  type EnsureSymlinkResult,
} from "./symlinks.js";

// ─────────────────────────────────────────────────────────────────────────────
// Fixture root — created fresh per test so state can't leak.
// ─────────────────────────────────────────────────────────────────────────────

const TEST_ROOT = join(process.cwd(), ".test-install-symlinks");

function write(relPath: string, content: string): void {
  const full = join(TEST_ROOT, relPath);
  mkdirSync(dirname(full), { recursive: true });
  writeFileSync(full, content);
}

function makeSymlink(linkRel: string, targetRel: string): void {
  const linkPath = join(TEST_ROOT, linkRel);
  mkdirSync(dirname(linkPath), { recursive: true });
  symlinkSync(targetRel, linkPath);
}

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

/** Build a stub prompt that always answers the given value. */
function stubPrompt(answer: boolean): (q: string) => Promise<boolean> {
  return async () => answer;
}

/** Build a stub prompt that records every question and answers from a queue. */
function recordingPrompt(answers: boolean[]): {
  prompt: (q: string) => Promise<boolean>;
  questions: string[];
} {
  const questions: string[] = [];
  let i = 0;
  const prompt = async (q: string): Promise<boolean> => {
    questions.push(q);
    const v = answers[i++];
    return v === undefined ? false : v;
  };
  return { prompt, questions };
}

// ─────────────────────────────────────────────────────────────────────────────
// ensureSymlink — state 1: missing → create silently.
// ─────────────────────────────────────────────────────────────────────────────

describe("ensureSymlink: missing link", () => {
  it("creates the symlink when nothing exists at linkPath", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    const linkPath = join(TEST_ROOT, ".claude", "CLAUDE.md");
    const relTarget = "../.agents/AGENTS.md";

    const result = await ensureSymlink(
      linkPath,
      relTarget,
      ".claude/CLAUDE.md",
    );

    expect(result.action).toBe("created");
    expect(lstatSync(linkPath).isSymbolicLink()).toBe(true);
    expect(readlinkSync(linkPath)).toBe(relTarget);
  });

  it("creates the parent directory if it does not exist", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    // No .claude/ dir yet — helper must mkdir it.
    const linkPath = join(TEST_ROOT, ".claude", "CLAUDE.md");

    const result = await ensureSymlink(
      linkPath,
      "../.agents/AGENTS.md",
      ".claude/CLAUDE.md",
    );

    expect(result.action).toBe("created");
    expect(existsSync(join(TEST_ROOT, ".claude"))).toBe(true);
  });

  it("does not prompt the user when creating from nothing", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const { prompt, questions } = recordingPrompt([]);

    const result = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      { prompt },
    );

    expect(result.action).toBe("created");
    expect(questions).toHaveLength(0);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// ensureSymlink — state 2: correct symlink → skip silently.
// ─────────────────────────────────────────────────────────────────────────────

describe("ensureSymlink: correct symlink", () => {
  it("reports already-correct without touching disk", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    makeSymlink("AGENTS.md", ".agents/AGENTS.md");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const { prompt, questions } = recordingPrompt([]);

    const result = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      { prompt },
    );

    expect(result.action).toBe("already-correct");
    expect(questions).toHaveLength(0);
    // Symlink still present and pointed at the same target.
    expect(readlinkSync(linkPath)).toBe(".agents/AGENTS.md");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// ensureSymlink — state 3: wrong symlink target.
// ─────────────────────────────────────────────────────────────────────────────

describe("ensureSymlink: wrong symlink target", () => {
  it("prompts and relinks when the user accepts", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write("other.md", "other target\n");
    makeSymlink("AGENTS.md", "other.md");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const { prompt, questions } = recordingPrompt([true]);

    const result: EnsureSymlinkResult = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      { prompt },
    );

    expect(result.action).toBe("relinked");
    expect(questions).toHaveLength(1);
    expect(questions[0]).toContain("Relink?");
    expect(readlinkSync(linkPath)).toBe(".agents/AGENTS.md");
  });

  it("leaves the wrong symlink in place when the user declines", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write("other.md", "other target\n");
    makeSymlink("AGENTS.md", "other.md");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const { prompt, questions } = recordingPrompt([false]);

    const result = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      { prompt },
    );

    expect(result.action).toBe("declined-relink");
    expect(questions).toHaveLength(1);
    // Symlink untouched — still points at other.md.
    expect(readlinkSync(linkPath)).toBe("other.md");
  });

  it("skips with no-tty action in non-interactive mode", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write("other.md", "other target\n");
    makeSymlink("AGENTS.md", "other.md");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const { prompt, questions } = recordingPrompt([]);

    const result = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      { prompt, nonInteractive: true },
    );

    expect(result.action).toBe("skipped-no-tty");
    expect(questions).toHaveLength(0);
    // Symlink untouched.
    expect(readlinkSync(linkPath)).toBe("other.md");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// ensureSymlink — state 4: regular file at linkPath.
// ─────────────────────────────────────────────────────────────────────────────

describe("ensureSymlink: regular file at linkPath", () => {
  it("backs up the file and replaces with a symlink when user accepts", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write("AGENTS.md", "user content\n");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const backupPath = `${linkPath}.bak`;
    const { prompt, questions } = recordingPrompt([true]);

    const result = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      { prompt },
    );

    expect(result.action).toBe("replaced-file");
    expect(result.backupPath).toBe(backupPath);
    expect(questions).toHaveLength(1);
    expect(questions[0]).toContain("drift");
    expect(questions[0]).toContain(".bak");

    // The backup contains the original user content.
    expect(readFileSync(backupPath, "utf-8")).toBe("user content\n");
    // The symlink now points at the canonical file.
    expect(lstatSync(linkPath).isSymbolicLink()).toBe(true);
    expect(readlinkSync(linkPath)).toBe(".agents/AGENTS.md");
  });

  it("leaves the real file untouched when user declines", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write("AGENTS.md", "user content\n");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const { prompt, questions } = recordingPrompt([false]);

    const result = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      { prompt },
    );

    expect(result.action).toBe("declined-replace");
    expect(questions).toHaveLength(1);
    // File is still a real file, unchanged.
    expect(lstatSync(linkPath).isSymbolicLink()).toBe(false);
    expect(readFileSync(linkPath, "utf-8")).toBe("user content\n");
    expect(existsSync(`${linkPath}.bak`)).toBe(false);
  });

  it("mentions the drift consequence in the prompt text", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write("AGENTS.md", "user content\n");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const { prompt, questions } = recordingPrompt([false]);

    await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      { prompt },
    );

    // The fallout message must warn about fenced sections drifting.
    expect(questions[0]).toMatch(/drift/i);
    expect(questions[0]).toMatch(/fenced section/i);
  });

  it("overwrites a stale .bak from a previous attempt", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write("AGENTS.md", "new user content\n");
    write("AGENTS.md.bak", "very old content\n");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const { prompt } = recordingPrompt([true]);

    const result = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      { prompt },
    );

    expect(result.action).toBe("replaced-file");
    // The newest real content wins in the backup — old stale .bak is gone.
    expect(readFileSync(`${linkPath}.bak`, "utf-8")).toBe(
      "new user content\n",
    );
  });

  it("skips with no-tty action in non-interactive mode", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write("AGENTS.md", "user content\n");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const { prompt, questions } = recordingPrompt([]);

    const result = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      { prompt, nonInteractive: true },
    );

    expect(result.action).toBe("skipped-no-tty");
    expect(questions).toHaveLength(0);
    // Real file is untouched — no destructive action without confirmation.
    expect(lstatSync(linkPath).isSymbolicLink()).toBe(false);
    expect(readFileSync(linkPath, "utf-8")).toBe("user content\n");
    expect(existsSync(`${linkPath}.bak`)).toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// needsRootAgentsSymlink / relativeLinkTarget — pure helpers.
// ─────────────────────────────────────────────────────────────────────────────

describe("needsRootAgentsSymlink", () => {
  it("returns true when any AGENTS.md-writing target is present", () => {
    expect(needsRootAgentsSymlink(["cursor"])).toBe(true);
    expect(needsRootAgentsSymlink(["codex"])).toBe(true);
    expect(needsRootAgentsSymlink(["opencode"])).toBe(true);
    expect(needsRootAgentsSymlink(["amp"])).toBe(true);
    expect(needsRootAgentsSymlink(["gemini"])).toBe(true);
    expect(needsRootAgentsSymlink(["claude-code", "cursor"])).toBe(true);
  });

  it("returns false when only claude-code is present", () => {
    expect(needsRootAgentsSymlink(["claude-code"])).toBe(false);
  });

  it("returns false for an empty selection", () => {
    expect(needsRootAgentsSymlink([])).toBe(false);
  });
});

describe("relativeLinkTarget", () => {
  it("produces a link-dir-relative path from link to canonical", () => {
    const linkPath = "/tmp/project/.claude/CLAUDE.md";
    const canonical = "/tmp/project/.agents/AGENTS.md";
    expect(relativeLinkTarget(linkPath, canonical)).toBe(
      "../.agents/AGENTS.md",
    );
  });

  it("produces a sibling-style path for root-level links", () => {
    const linkPath = "/tmp/project/AGENTS.md";
    const canonical = "/tmp/project/.agents/AGENTS.md";
    expect(relativeLinkTarget(linkPath, canonical)).toBe(
      join(".agents", "AGENTS.md"),
    );
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// ensureProjectSymlinks — orchestration layer.
// ─────────────────────────────────────────────────────────────────────────────

describe("ensureProjectSymlinks", () => {
  it("creates an empty canonical and symlinks on a fresh install", async () => {
    // Fresh repo: no .agents/, no .claude/, no ./AGENTS.md. Installer must
    // seed an empty canonical and point the requested symlinks at it so the
    // subsequent fenced-section write flows through the symlink rather than
    // landing as a sibling real file that drifts on the next install.
    const canonical = join(TEST_ROOT, ".agents", "AGENTS.md");
    expect(existsSync(canonical)).toBe(false);

    const results = await ensureProjectSymlinks({
      projectRoot: TEST_ROOT,
      selectedTargets: ["cursor"],
      hasClaudeCode: true,
    });

    // Canonical file now exists and is empty.
    expect(existsSync(canonical)).toBe(true);
    expect(readFileSync(canonical, "utf-8")).toBe("");

    // Both symlinks were created and resolve to the canonical.
    expect(Object.keys(results).sort()).toEqual(
      [".claude/CLAUDE.md", "./AGENTS.md"].sort(),
    );
    expect(results[".claude/CLAUDE.md"]!.action).toBe("created");
    expect(results["./AGENTS.md"]!.action).toBe("created");

    const claudeLink = join(TEST_ROOT, ".claude", "CLAUDE.md");
    const rootLink = join(TEST_ROOT, "AGENTS.md");
    expect(lstatSync(claudeLink).isSymbolicLink()).toBe(true);
    expect(lstatSync(rootLink).isSymbolicLink()).toBe(true);
    // Reading through the symlinks reaches the empty canonical without ENOENT.
    expect(readFileSync(claudeLink, "utf-8")).toBe("");
    expect(readFileSync(rootLink, "utf-8")).toBe("");
  });

  it("returns empty map when no target scopes a managed symlink", async () => {
    // Neither Claude Code nor any AGENTS.md-writing target is in scope, so
    // there are no links to enforce and we bail before any filesystem writes.
    const canonical = join(TEST_ROOT, ".agents", "AGENTS.md");

    const results = await ensureProjectSymlinks({
      projectRoot: TEST_ROOT,
      selectedTargets: [],
      hasClaudeCode: false,
    });

    expect(Object.keys(results)).toHaveLength(0);
    // We did not create the canonical — there was no reason to.
    expect(existsSync(canonical)).toBe(false);
  });

  it("creates only .claude/CLAUDE.md when only Claude Code is in scope", async () => {
    write(".agents/AGENTS.md", "# canonical\n");

    const results = await ensureProjectSymlinks({
      projectRoot: TEST_ROOT,
      selectedTargets: [],
      hasClaudeCode: true,
    });

    expect(Object.keys(results)).toEqual([".claude/CLAUDE.md"]);
    expect(results[".claude/CLAUDE.md"]!.action).toBe("created");
    expect(
      lstatSync(join(TEST_ROOT, ".claude", "CLAUDE.md")).isSymbolicLink(),
    ).toBe(true);
    // Root AGENTS.md was not requested, so it should not exist.
    expect(existsSync(join(TEST_ROOT, "AGENTS.md"))).toBe(false);
  });

  it("creates only ./AGENTS.md when only AGENTS.md-writing targets are in scope", async () => {
    write(".agents/AGENTS.md", "# canonical\n");

    const results = await ensureProjectSymlinks({
      projectRoot: TEST_ROOT,
      selectedTargets: ["cursor"],
      hasClaudeCode: false,
    });

    expect(Object.keys(results)).toEqual(["./AGENTS.md"]);
    expect(results["./AGENTS.md"]!.action).toBe("created");
    expect(existsSync(join(TEST_ROOT, ".claude", "CLAUDE.md"))).toBe(false);
  });

  it("creates both symlinks when both scopes are present", async () => {
    write(".agents/AGENTS.md", "# canonical\n");

    const results = await ensureProjectSymlinks({
      projectRoot: TEST_ROOT,
      selectedTargets: ["cursor", "codex"],
      hasClaudeCode: true,
    });

    expect(Object.keys(results).sort()).toEqual(
      [".claude/CLAUDE.md", "./AGENTS.md"].sort(),
    );
    expect(results[".claude/CLAUDE.md"]!.action).toBe("created");
    expect(results["./AGENTS.md"]!.action).toBe("created");
  });

  it("uses project-relative link targets", async () => {
    write(".agents/AGENTS.md", "# canonical\n");

    await ensureProjectSymlinks({
      projectRoot: TEST_ROOT,
      selectedTargets: ["cursor"],
      hasClaudeCode: true,
    });

    const claudeLink = join(TEST_ROOT, ".claude", "CLAUDE.md");
    const rootLink = join(TEST_ROOT, "AGENTS.md");
    // Sanity-check the targets are the readable, relative ones.
    expect(readlinkSync(claudeLink)).toBe(
      relative(dirname(claudeLink), join(TEST_ROOT, ".agents", "AGENTS.md")),
    );
    expect(readlinkSync(rootLink)).toBe(
      relative(dirname(rootLink), join(TEST_ROOT, ".agents", "AGENTS.md")),
    );
  });

  it("honours a stubbed prompt and propagates declined results", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    // Wrong target already in place — will prompt.
    write("other.md", "wrong\n");
    makeSymlink("AGENTS.md", "other.md");

    const results = await ensureProjectSymlinks({
      projectRoot: TEST_ROOT,
      selectedTargets: ["cursor"],
      hasClaudeCode: false,
      prompt: stubPrompt(false),
    });

    expect(results["./AGENTS.md"]!.action).toBe("declined-relink");
  });

  it("returns skipped-no-tty for non-interactive replacement states", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write("AGENTS.md", "user content\n");

    const results = await ensureProjectSymlinks({
      projectRoot: TEST_ROOT,
      selectedTargets: ["cursor"],
      hasClaudeCode: false,
      nonInteractive: true,
    });

    expect(results["./AGENTS.md"]!.action).toBe("skipped-no-tty");
    // Nothing destructive happened.
    expect(lstatSync(join(TEST_ROOT, "AGENTS.md")).isSymbolicLink()).toBe(
      false,
    );
  });

  it("runs migration automatically under assumeYes with no prompt", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write("AGENTS.md", "# project instructions\n\nsome content\n");

    const { prompt, questions } = recordingPrompt([]);

    const results = await ensureProjectSymlinks({
      projectRoot: TEST_ROOT,
      selectedTargets: ["cursor"],
      hasClaudeCode: false,
      assumeYes: true,
      prompt,
    });

    expect(results["./AGENTS.md"]!.action).toBe("replaced-file");
    expect(results["./AGENTS.md"]!.merged).toBe(true);
    expect(questions).toHaveLength(0);

    // Symlink created, backup kept, canonical now contains the merged body.
    const rootLink = join(TEST_ROOT, "AGENTS.md");
    expect(lstatSync(rootLink).isSymbolicLink()).toBe(true);
    expect(existsSync(`${rootLink}.bak`)).toBe(true);
    const canonical = readFileSync(
      join(TEST_ROOT, ".agents", "AGENTS.md"),
      "utf-8",
    );
    expect(canonical).toContain("some content");
    expect(canonical).toContain("## Migrated from AGENTS.md");
  });

  it("migrates from both .claude/CLAUDE.md and ./AGENTS.md into canonical", async () => {
    write(".agents/AGENTS.md", "# canonical body\n");
    write("AGENTS.md", "root agent notes\n");
    write(".claude/CLAUDE.md", "claude-specific notes\n");

    const results = await ensureProjectSymlinks({
      projectRoot: TEST_ROOT,
      selectedTargets: ["cursor"],
      hasClaudeCode: true,
      assumeYes: true,
    });

    expect(results["./AGENTS.md"]!.action).toBe("replaced-file");
    expect(results[".claude/CLAUDE.md"]!.action).toBe("replaced-file");

    const canonical = readFileSync(
      join(TEST_ROOT, ".agents", "AGENTS.md"),
      "utf-8",
    );
    // Both sources are appended under their own migration heading — the
    // user resolves any collision, we don't dedupe.
    expect(canonical).toContain("## Migrated from AGENTS.md");
    expect(canonical).toContain("root agent notes");
    expect(canonical).toContain(join(".claude", "CLAUDE.md"));
    expect(canonical).toContain("claude-specific notes");

    // Both .bak files exist.
    expect(existsSync(join(TEST_ROOT, "AGENTS.md.bak"))).toBe(true);
    expect(existsSync(join(TEST_ROOT, ".claude", "CLAUDE.md.bak"))).toBe(true);
  });

  it("creates canonical lazily when only a root AGENTS.md is present", async () => {
    // Adopter case: user has ./AGENTS.md but no .agents/ directory yet.
    write("AGENTS.md", "# project instructions\n\nhello\n");

    const results = await ensureProjectSymlinks({
      projectRoot: TEST_ROOT,
      selectedTargets: ["cursor"],
      hasClaudeCode: false,
      assumeYes: true,
    });

    expect(results["./AGENTS.md"]!.action).toBe("replaced-file");
    expect(results["./AGENTS.md"]!.merged).toBe(true);

    const canonical = join(TEST_ROOT, ".agents", "AGENTS.md");
    expect(existsSync(canonical)).toBe(true);
    // Since canonical was absent, the body IS the stripped source — no
    // `## Migrated from` heading in this first-write path.
    expect(readFileSync(canonical, "utf-8")).toContain("hello");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// ensureSymlink — assumeYes skips prompts for wrong-symlink + real-file.
// ─────────────────────────────────────────────────────────────────────────────

describe("ensureSymlink: assumeYes", () => {
  it("auto-relinks wrong symlinks without prompting", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write("other.md", "wrong\n");
    makeSymlink("AGENTS.md", "other.md");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const { prompt, questions } = recordingPrompt([]);

    const result = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      { prompt, assumeYes: true },
    );

    expect(result.action).toBe("relinked");
    expect(questions).toHaveLength(0);
    expect(readlinkSync(linkPath)).toBe(".agents/AGENTS.md");
  });

  it("auto-migrates real files without prompting", async () => {
    const canonical = join(TEST_ROOT, ".agents", "AGENTS.md");
    write(".agents/AGENTS.md", "# canonical\n");
    write("AGENTS.md", "user content\n");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const { prompt, questions } = recordingPrompt([]);

    const result = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      {
        prompt,
        assumeYes: true,
        canonicalPath: canonical,
        projectRoot: TEST_ROOT,
      },
    );

    expect(result.action).toBe("replaced-file");
    expect(result.merged).toBe(true);
    expect(questions).toHaveLength(0);

    // Contents merged, backup kept, symlink in place.
    expect(readFileSync(`${linkPath}.bak`, "utf-8")).toBe("user content\n");
    expect(readFileSync(canonical, "utf-8")).toContain("user content");
    expect(lstatSync(linkPath).isSymbolicLink()).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Migration internals — stripLoafFence + mergeContentIntoCanonical.
// ─────────────────────────────────────────────────────────────────────────────

describe("stripLoafFence", () => {
  it("returns the file unchanged when no fence is present", () => {
    const input = "# My project\n\nregular content\n";
    expect(stripLoafFence(input)).toBe(input.trim());
  });

  it("removes a fence sandwiched between user content", () => {
    const input =
      "# Header\n\n<!-- loaf:managed:start v1.0.0 -->\n" +
      "## Loaf Framework\nfence body\n" +
      "<!-- loaf:managed:end -->\n\n" +
      "Trailing content\n";
    const result = stripLoafFence(input);
    expect(result).not.toContain("loaf:managed");
    expect(result).toContain("# Header");
    expect(result).toContain("Trailing content");
  });

  it("returns empty when the fence spans the entire file", () => {
    const input =
      "<!-- loaf:managed:start v1.0.0 -->\n" +
      "only fence content\n" +
      "<!-- loaf:managed:end -->\n";
    expect(stripLoafFence(input)).toBe("");
  });

  it("returns empty for whitespace-only files", () => {
    expect(stripLoafFence("\n\n   \n")).toBe("");
  });
});

describe("mergeContentIntoCanonical", () => {
  it("creates canonical with stripped body when canonical is absent", () => {
    const canonical = join(TEST_ROOT, ".agents", "AGENTS.md");
    const wrote = mergeContentIntoCanonical(
      canonical,
      "# hello\n\nworld",
      "AGENTS.md",
    );
    expect(wrote).toBe(true);
    expect(readFileSync(canonical, "utf-8")).toBe("# hello\n\nworld\n");
  });

  it("appends under a `## Migrated from <path>` heading when canonical exists", () => {
    const canonical = join(TEST_ROOT, ".agents", "AGENTS.md");
    write(".agents/AGENTS.md", "# canonical body\n");

    mergeContentIntoCanonical(
      canonical,
      "user content",
      ".claude/CLAUDE.md",
    );

    const merged = readFileSync(canonical, "utf-8");
    expect(merged).toContain("# canonical body");
    expect(merged).toContain("## Migrated from .claude/CLAUDE.md");
    expect(merged).toContain("user content");
    // The heading is separated from prior content by exactly one blank line.
    expect(merged).toMatch(/\n\n## Migrated from /);
  });

  it("is a no-op for empty stripped content", () => {
    const canonical = join(TEST_ROOT, ".agents", "AGENTS.md");
    write(".agents/AGENTS.md", "# existing\n");

    const wrote = mergeContentIntoCanonical(canonical, "", "AGENTS.md");
    expect(wrote).toBe(false);
    // File unchanged — no heading, no extra blank lines.
    expect(readFileSync(canonical, "utf-8")).toBe("# existing\n");
  });

  it("does not dedupe — two calls produce two migration headings", () => {
    const canonical = join(TEST_ROOT, ".agents", "AGENTS.md");
    write(".agents/AGENTS.md", "# canonical\n");

    mergeContentIntoCanonical(canonical, "content A", "a.md");
    mergeContentIntoCanonical(canonical, "content B", "b.md");

    const result = readFileSync(canonical, "utf-8");
    const headingCount = (result.match(/## Migrated from /g) ?? []).length;
    expect(headingCount).toBe(2);
    expect(result).toContain("## Migrated from a.md");
    expect(result).toContain("## Migrated from b.md");
    expect(result).toContain("content A");
    expect(result).toContain("content B");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// ensureSymlink — real-file migration behaviour (assumeYes path).
// ─────────────────────────────────────────────────────────────────────────────

describe("ensureSymlink: merge migration", () => {
  const canonicalPath = () => join(TEST_ROOT, ".agents", "AGENTS.md");

  it("empty source → no merge, just .bak + symlink", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write("AGENTS.md", "");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const result = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      {
        assumeYes: true,
        canonicalPath: canonicalPath(),
        projectRoot: TEST_ROOT,
      },
    );

    expect(result.action).toBe("replaced-file");
    expect(result.merged).toBe(false);
    // Canonical untouched, backup exists, symlink in place.
    expect(readFileSync(canonicalPath(), "utf-8")).toBe("# canonical\n");
    expect(existsSync(`${linkPath}.bak`)).toBe(true);
    expect(lstatSync(linkPath).isSymbolicLink()).toBe(true);
  });

  it("fence-only source → strip leaves empty, no merge", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write(
      "AGENTS.md",
      "<!-- loaf:managed:start v1.0.0 -->\nonly fence\n<!-- loaf:managed:end -->\n",
    );

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const result = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      {
        assumeYes: true,
        canonicalPath: canonicalPath(),
        projectRoot: TEST_ROOT,
      },
    );

    expect(result.action).toBe("replaced-file");
    expect(result.merged).toBe(false);
    // No `## Migrated from` heading — nothing user-authored to preserve.
    expect(readFileSync(canonicalPath(), "utf-8")).toBe("# canonical\n");
    expect(existsSync(`${linkPath}.bak`)).toBe(true);
  });

  it("fence-only source + canonical absent → seeds empty canonical, symlink resolves", async () => {
    // State 4 edge case: the real file has only a Loaf fence (stripped to
    // empty) AND canonical doesn't exist yet. ensureSymlink must seed an
    // empty canonical before creating the symlink so the result isn't
    // dangling — otherwise the next fenced-section read fails with ENOENT.
    write(
      "AGENTS.md",
      "<!-- loaf:managed:start v1.0.0 -->\nonly fence\n<!-- loaf:managed:end -->\n",
    );

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const result = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      {
        assumeYes: true,
        canonicalPath: canonicalPath(),
        projectRoot: TEST_ROOT,
      },
    );

    expect(result.action).toBe("replaced-file");
    expect(result.merged).toBe(false);
    // Canonical was created as an empty shell so the symlink resolves.
    expect(existsSync(canonicalPath())).toBe(true);
    expect(readFileSync(canonicalPath(), "utf-8")).toBe("");
    // Symlink is in place and resolves (reading through it does not ENOENT).
    expect(lstatSync(linkPath).isSymbolicLink()).toBe(true);
    expect(readFileSync(linkPath, "utf-8")).toBe("");
    expect(existsSync(`${linkPath}.bak`)).toBe(true);
  });

  it("user-content source + canonical absent → stripped content becomes body", async () => {
    write("AGENTS.md", "# user instructions\n\nbody text\n");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    const result = await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      {
        assumeYes: true,
        canonicalPath: canonicalPath(),
        projectRoot: TEST_ROOT,
      },
    );

    expect(result.action).toBe("replaced-file");
    expect(result.merged).toBe(true);

    // Canonical created directly from stripped content — no migration heading.
    const canonical = readFileSync(canonicalPath(), "utf-8");
    expect(canonical).toContain("# user instructions");
    expect(canonical).toContain("body text");
    expect(canonical).not.toContain("## Migrated from");
  });

  it("user-content source + canonical exists → appended under heading", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write("AGENTS.md", "user body\n");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      {
        assumeYes: true,
        canonicalPath: canonicalPath(),
        projectRoot: TEST_ROOT,
      },
    );

    const canonical = readFileSync(canonicalPath(), "utf-8");
    expect(canonical).toContain("# canonical");
    expect(canonical).toContain("## Migrated from AGENTS.md");
    expect(canonical).toContain("user body");
  });

  it("always creates a .bak next to the source", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write("AGENTS.md", "important user text\n");

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      {
        assumeYes: true,
        canonicalPath: canonicalPath(),
        projectRoot: TEST_ROOT,
      },
    );

    expect(readFileSync(`${linkPath}.bak`, "utf-8")).toBe(
      "important user text\n",
    );
  });

  it("strips any existing fence before merging (no duplicate framework section)", async () => {
    write(".agents/AGENTS.md", "# canonical\n");
    write(
      "AGENTS.md",
      "# header\n\n<!-- loaf:managed:start v0.9.0 -->\n" +
        "old fence body\n" +
        "<!-- loaf:managed:end -->\n\nkeeper text\n",
    );

    const linkPath = join(TEST_ROOT, "AGENTS.md");
    await ensureSymlink(
      linkPath,
      ".agents/AGENTS.md",
      "./AGENTS.md",
      {
        assumeYes: true,
        canonicalPath: canonicalPath(),
        projectRoot: TEST_ROOT,
      },
    );

    const canonical = readFileSync(canonicalPath(), "utf-8");
    // Fence contents must be gone; user headings + trailing text preserved.
    expect(canonical).not.toContain("loaf:managed");
    expect(canonical).not.toContain("old fence body");
    expect(canonical).toContain("# header");
    expect(canonical).toContain("keeper text");
  });
});
