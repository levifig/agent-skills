/**
 * Release Command Integration Tests
 *
 * Tests that the `loaf release` command correctly wires option parsing
 * to the underlying helpers. These are subprocess tests that invoke the
 * CLI, so they verify the full Commander → handler → helper chain,
 * not just the helpers in isolation.
 *
 * The CLI is built from source in beforeAll, so these tests always
 * exercise current code regardless of dist-cli/ state.
 *
 * Most tests use --dry-run, so no files or git state are modified. The
 * "release commit subject" suite at the bottom is the lone exception:
 * it runs against an isolated temp git repo with --no-tag --no-gh and
 * asserts the actual emitted commit subject — closing the SPEC-031
 * Test Conditions gate that requires the subject be explicitly tested.
 */

import { describe, it, expect, beforeAll } from "vitest";
import { execFileSync, spawnSync } from "child_process";
import {
  mkdtempSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "fs";
import { tmpdir } from "os";
import { join } from "path";

// ─────────────────────────────────────────────────────────────────────────────
// Setup — build the CLI from source before running integration tests
// ─────────────────────────────────────────────────────────────────────────────

const CLI_PATH = join(process.cwd(), "dist-cli", "index.js");

beforeAll(() => {
  execFileSync("npm", ["run", "build:cli"], {
    cwd: process.cwd(),
    stdio: "ignore",
    timeout: 30_000,
  });
}, 30_000);

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

interface RunResult {
  stdout: string;
  stderr: string;
  exitCode: number;
}

/** Run the release command with given args. Never throws — captures exit code. */
function runRelease(...args: string[]): RunResult {
  // spawnSync captures stdout AND stderr regardless of exit code, which
  // matters for warnings emitted on stderr alongside a 0 exit.
  const result = spawnSync("node", [CLI_PATH, "release", ...args], {
    cwd: process.cwd(),
    encoding: "utf-8",
    stdio: ["ignore", "pipe", "pipe"],
    timeout: 15_000,
  });
  return {
    stdout: result.stdout ?? "",
    stderr: result.stderr ?? "",
    exitCode: result.status ?? 1,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// --bump validation wiring
// ─────────────────────────────────────────────────────────────────────────────

describe("--bump flag", () => {
  it("rejects invalid bump type with exit code 1", () => {
    const result = runRelease("--bump", "bogus", "--dry-run");
    expect(result.exitCode).toBe(1);
    expect(result.stderr).toContain('Invalid bump type "bogus"');
  });

  it("accepts valid bump type and skips interactive prompt", () => {
    const result = runRelease("--bump", "prerelease", "--dry-run");
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("via --bump flag");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// --base validation wiring
// ─────────────────────────────────────────────────────────────────────────────

describe("--base flag", () => {
  it("rejects nonexistent ref with exit code 1", () => {
    const result = runRelease("--base", "definitely-not-a-ref", "--dry-run");
    expect(result.exitCode).toBe(1);
    expect(result.stderr).toContain("does not exist or is not reachable");
  });

  it("accepts valid ref and scopes commits", () => {
    // HEAD..HEAD = 0 commits, but the ref validation should pass
    const result = runRelease("--base", "HEAD", "--dry-run");
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("via --base flag");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// --no-tag / --no-gh wiring
// ─────────────────────────────────────────────────────────────────────────────

describe("--no-tag implies --no-gh", () => {
  it("shows both tag and gh as skipped when only --no-tag is passed", () => {
    const result = runRelease("--no-tag", "--dry-run");
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("--no-tag — skipped");
    expect(result.stdout).toContain("--no-gh — skipped");
  });

  it("skips only gh when only --no-gh is passed", () => {
    const result = runRelease("--no-gh", "--dry-run");
    expect(result.exitCode).toBe(0);
    // Tag step should NOT be skipped
    expect(result.stdout).not.toContain("--no-tag — skipped");
    // GH step should be skipped
    expect(result.stdout).toContain("--no-gh — skipped");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// --yes flag wiring
// ─────────────────────────────────────────────────────────────────────────────

describe("--yes flag", () => {
  it("is accepted as a valid option", () => {
    // --yes with --dry-run: dry-run exits before confirmation, so --yes has
    // no visible effect, but the flag must parse without error
    const result = runRelease("--yes", "--dry-run");
    expect(result.exitCode).toBe(0);
  });

  it("short form -y is accepted", () => {
    const result = runRelease("-y", "--dry-run");
    expect(result.exitCode).toBe(0);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// --version-file flag wiring (SPEC-031 / TASK-143)
// ─────────────────────────────────────────────────────────────────────────────

describe("--version-file flag", () => {
  it("aborts cleanly when a declared path does not exist", () => {
    const result = runRelease(
      "--version-file",
      "definitely/missing/path.json",
      "--dry-run",
    );
    expect(result.exitCode).toBe(1);
    expect(result.stderr).toContain(
      "version file definitely/missing/path.json not found",
    );
  });

  it("uses the overridden file (ignoring root package.json) when path exists", () => {
    // package.json at project root has the real loaf version. Pointing
    // --version-file at it explicitly is a no-op for content but proves
    // the override is wired and the dry-run preview shows that file.
    const result = runRelease("--version-file", "package.json", "--dry-run");
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("package.json");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// --pre-merge flag wiring (SPEC-031 / TASK-144)
// ─────────────────────────────────────────────────────────────────────────────

describe("--pre-merge flag", () => {
  it("bundles --no-tag --no-gh and prints the auto-detected base", () => {
    // Running against the loaf repo itself: no PR for the test branch, no
    // git config override → step 4 wins. We only assert that *some* base
    // is auto-detected (the value depends on the local clone), and that
    // tag + gh are skipped.
    const result = runRelease("--pre-merge", "--dry-run");
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("Auto-detected base:");
    expect(result.stdout).toContain("--no-tag — skipped");
    expect(result.stdout).toContain("--no-gh — skipped");
  });

  it("explicit --base short-circuits auto-detection (no 'Auto-detected base' line)", () => {
    // Use HEAD as the explicit base — it always resolves and produces a clean
    // "no commits" exit before the action list. We assert that auto-detection
    // did NOT run (no banner) and that the explicit ref was used.
    const result = runRelease("--pre-merge", "--base", "HEAD", "--dry-run");
    expect(result.exitCode).toBe(0);
    // Auto-detection is skipped when --base is explicit.
    expect(result.stdout).not.toContain("Auto-detected base:");
    // Explicit base is still used.
    expect(result.stdout).toContain("via --base flag");
  });

  it("emits a warning when --pre-merge is combined with --tag (and tags anyway)", () => {
    // Using --base origin/main forces a commit-list with content so the
    // action list is displayed (and we can verify the tag step is NOT
    // greyed out as skipped).
    const result = runRelease(
      "--pre-merge",
      "--tag",
      "--base",
      "origin/main",
      "--dry-run",
    );
    expect(result.exitCode).toBe(0);
    expect(result.stderr).toContain(
      "--tag overrides --pre-merge default",
    );
    // Tag is NOT skipped in the action list.
    expect(result.stdout).not.toContain("--no-tag — skipped");
    // GH release is still bundled-skipped.
    expect(result.stdout).toContain("--no-gh — skipped");
  });

  it("emits a warning when --pre-merge is combined with --gh", () => {
    // Note: --pre-merge bundles --no-tag, and (per normalizeSkipFlags)
    // --no-tag implies --no-gh because `gh release create` would auto-push
    // the missing tag. So --gh cannot fully override the bundled --no-gh
    // when --no-tag is also active. The warning still fires for transparency,
    // but the action list reflects the implication.
    const result = runRelease(
      "--pre-merge",
      "--gh",
      "--base",
      "origin/main",
      "--dry-run",
    );
    expect(result.exitCode).toBe(0);
    expect(result.stderr).toContain(
      "--gh overrides --pre-merge default",
    );
  });

  it("--gh + --tag override (--pre-merge with both) creates a real release", () => {
    // Combining --pre-merge with --tag AND --gh restores the full default
    // pipeline (tag + gh), with two warnings on stderr.
    const result = runRelease(
      "--pre-merge",
      "--tag",
      "--gh",
      "--base",
      "origin/main",
      "--dry-run",
    );
    expect(result.exitCode).toBe(0);
    expect(result.stderr).toContain("--tag overrides --pre-merge default");
    expect(result.stderr).toContain("--gh overrides --pre-merge default");
    // Neither tag nor gh should be skipped in the action list.
    expect(result.stdout).not.toContain("--no-tag — skipped");
    expect(result.stdout).not.toContain("--no-gh — skipped");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// --post-merge flag wiring (SPEC-031 / TASK-142)
// ─────────────────────────────────────────────────────────────────────────────

describe("--post-merge flag", () => {
  it("rejects combination with --bump", () => {
    const result = runRelease("--post-merge", "--bump", "patch");
    expect(result.exitCode).toBe(1);
    expect(result.stderr).toContain(
      "--post-merge is incompatible with --bump",
    );
  });

  it("rejects combination with --dry-run", () => {
    const result = runRelease("--post-merge", "--dry-run");
    expect(result.exitCode).toBe(1);
    expect(result.stderr).toContain(
      "--post-merge is incompatible with --dry-run",
    );
  });

  it("rejects combination with --no-tag", () => {
    const result = runRelease("--post-merge", "--no-tag");
    expect(result.exitCode).toBe(1);
    expect(result.stderr).toContain("--post-merge is incompatible with --no-tag");
  });

  it("rejects combination with --no-gh", () => {
    const result = runRelease("--post-merge", "--no-gh");
    expect(result.exitCode).toBe(1);
    expect(result.stderr).toContain("--post-merge is incompatible with --no-gh");
  });

  it("rejects combination with --base", () => {
    const result = runRelease("--post-merge", "--base", "main");
    expect(result.exitCode).toBe(1);
    expect(result.stderr).toContain("--post-merge is incompatible with --base");
  });

  it("rejects combination with --pre-merge", () => {
    const result = runRelease("--post-merge", "--pre-merge");
    expect(result.exitCode).toBe(1);
    expect(result.stderr).toContain(
      "--post-merge is incompatible with --pre-merge",
    );
  });

  it("aborts with guardrail failure on the loaf repo (no chore: release HEAD)", () => {
    // Running on the loaf repo itself: HEAD is not a chore: release commit,
    // so guardrail 3 (subject shape) should fail. The test asserts that the
    // guardrail layer runs and surfaces a non-zero exit with a helpful
    // message — the exact guardrail that trips depends on local git state
    // (branch, worktree cleanliness), so we accept any guardrail failure.
    const result = runRelease("--post-merge");
    expect(result.exitCode).toBe(1);
    expect(result.stderr).toMatch(/guardrail \d+ failed/);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Regression guards
// ─────────────────────────────────────────────────────────────────────────────

describe("existing behavior preserved", () => {
  it("--dry-run alone exits 0 without modifying anything", () => {
    const result = runRelease("--dry-run");
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("No changes made");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Release commit subject (SPEC-031 Test Conditions)
//
// SPEC-031 line 176 mandates: "loaf release commits with subject
// `chore: release v<semver>` — explicitly tested, not just observed in
// fixtures." Earlier tests run with --dry-run and so never exercise the
// commit code path. This suite runs against a throwaway git repo with
// --no-tag --no-gh and asserts the produced commit subject directly.
//
// Side effect: the release flow's Step 3 (`loaf build`) walks from the
// CLI bundle's __dirname to find the loaf root, so a real build of the
// loaf repo runs as part of this test. That's deterministic — `loaf build`
// is idempotent against the loaf source — but it is NOT a no-op. Hence
// the longer per-test timeout and the captured stdio.
// ─────────────────────────────────────────────────────────────────────────────

describe("release commit subject", () => {
  it(
    "commits with subject 'chore: release v<X.Y.Z>' (no --dry-run, real git commit)",
    () => {
      // Set up an isolated git repo with package.json + curated CHANGELOG.
      const repoRoot = realpathSync(
        mkdtempSync(join(tmpdir(), "loaf-release-subject-")),
      );
      try {
        execFileSync("git", ["init", "-b", "main"], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        execFileSync("git", ["config", "user.email", "test@test.com"], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        execFileSync("git", ["config", "user.name", "Test"], {
          cwd: repoRoot,
          stdio: "ignore",
        });

        // Initial state: package.json @ 1.0.0, CHANGELOG with curated
        // [Unreleased] entries (so auto-generation from commits is bypassed
        // and the test is independent of which commit subjects we manufacture).
        writeFileSync(
          join(repoRoot, "package.json"),
          JSON.stringify({ name: "fixture", version: "1.0.0" }, null, 2) + "\n",
        );
        writeFileSync(
          join(repoRoot, "CHANGELOG.md"),
          [
            "# Changelog",
            "",
            "## [Unreleased]",
            "",
            "- Added something useful",
            "- Fixed something annoying",
            "",
            "## [1.0.0] - 2024-01-01",
            "",
            "- Initial release",
            "",
          ].join("\n"),
        );
        execFileSync("git", ["add", "."], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        execFileSync("git", ["commit", "-m", "chore: initial"], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        const baseSha = execFileSync("git", ["rev-parse", "HEAD"], {
          cwd: repoRoot,
          encoding: "utf-8",
        }).trim();

        // Add at least one commit between base and HEAD so commits.length > 0
        // (release exits early when there are no commits since the base ref).
        writeFileSync(join(repoRoot, "src.txt"), "first version\n");
        execFileSync("git", ["add", "."], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        execFileSync("git", ["commit", "-m", "feat: add src.txt"], {
          cwd: repoRoot,
          stdio: "ignore",
        });

        // Run a real release. --no-tag --no-gh isolates the assertion to the
        // commit subject (no tag pushing, no gh release). --yes skips the
        // confirmation prompt. --base anchors commit detection at the
        // initial commit.
        const result = spawnSync(
          "node",
          [
            CLI_PATH,
            "release",
            "--bump",
            "patch",
            "--yes",
            "--no-tag",
            "--no-gh",
            "--base",
            baseSha,
          ],
          {
            cwd: repoRoot,
            encoding: "utf-8",
            stdio: ["ignore", "pipe", "pipe"],
            timeout: 60_000,
          },
        );

        expect(result.status).toBe(0);

        // The bumped version is 1.0.0 → 1.0.1. The release flow tags the
        // commit `chore: release v<X.Y.Z>` (with no PR-number suffix —
        // that suffix is added by GitHub's squash-merge, not by `loaf release`).
        const subject = execFileSync(
          "git",
          ["log", "-1", "--pretty=%s"],
          {
            cwd: repoRoot,
            encoding: "utf-8",
          },
        ).trim();

        expect(subject).toBe("chore: release v1.0.1");

        // Bonus: the produced subject must match the chore-shape regex used
        // by `cli/commands/check.ts` for the workflow-pre-pr escape hatch.
        // Mirroring the regex inline keeps this test self-contained — if
        // either the regex or the emitted subject drifts, this assertion
        // catches it.
        const RELEASE_COMMIT_SUBJECT_REGEX =
          /^chore: release v\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?(?:\+[a-zA-Z0-9.-]+)?(?:\s+\(#\d+\))?$/;
        expect(RELEASE_COMMIT_SUBJECT_REGEX.test(subject)).toBe(true);
      } finally {
        rmSync(repoRoot, { recursive: true, force: true });
      }
    },
    90_000,
  );
});

// ─────────────────────────────────────────────────────────────────────────────
// Build step (TASK-149): release runs `npm run build` for Node projects
//
// Background: prior to this fix, Step 3 of the release flow always invoked
// `loaf build` (content-only). For projects that bundle their own CLI
// (loaf itself does — `plugins/loaf/bin/loaf` is bundled by tsup), this left
// the bundled binary with the previous version baked in. The fix detects
// Node projects via `package.json` with a `build` script and runs
// `npm run build` instead.
// ─────────────────────────────────────────────────────────────────────────────

describe("release runs npm run build for Node projects (TASK-149)", () => {
  it(
    "invokes the package.json build script during the release commit",
    () => {
      const repoRoot = realpathSync(
        mkdtempSync(join(tmpdir(), "loaf-release-build-")),
      );
      try {
        execFileSync("git", ["init", "-b", "main"], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        execFileSync("git", ["config", "user.email", "test@test.com"], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        execFileSync("git", ["config", "user.name", "Test"], {
          cwd: repoRoot,
          stdio: "ignore",
        });

        // package.json with a `build` script that writes a marker file.
        // The marker captures the current package.json version at build
        // time, so we can prove the build ran AFTER the version bump.
        writeFileSync(
          join(repoRoot, "package.json"),
          JSON.stringify(
            {
              name: "fixture",
              version: "1.0.0",
              scripts: {
                build:
                  "node -e \"const fs=require('fs');const p=JSON.parse(fs.readFileSync('package.json','utf-8'));fs.writeFileSync('build-marker.txt','built v'+p.version+'\\n');\"",
              },
            },
            null,
            2,
          ) + "\n",
        );
        writeFileSync(
          join(repoRoot, "CHANGELOG.md"),
          [
            "# Changelog",
            "",
            "## [Unreleased]",
            "",
            "- Initial change",
            "",
            "## [1.0.0] - 2024-01-01",
            "",
            "- Initial release",
            "",
          ].join("\n"),
        );
        execFileSync("git", ["add", "."], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        execFileSync("git", ["commit", "-m", "chore: initial"], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        const baseSha = execFileSync("git", ["rev-parse", "HEAD"], {
          cwd: repoRoot,
          encoding: "utf-8",
        }).trim();

        // Add a feat commit so commits.length > 0
        writeFileSync(join(repoRoot, "src.txt"), "v1\n");
        execFileSync("git", ["add", "."], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        execFileSync("git", ["commit", "-m", "feat: add src.txt"], {
          cwd: repoRoot,
          stdio: "ignore",
        });

        const result = spawnSync(
          "node",
          [
            CLI_PATH,
            "release",
            "--bump",
            "patch",
            "--yes",
            "--no-tag",
            "--no-gh",
            "--base",
            baseSha,
          ],
          {
            cwd: repoRoot,
            encoding: "utf-8",
            stdio: ["ignore", "pipe", "pipe"],
            timeout: 60_000,
          },
        );

        expect(result.status).toBe(0);
        // Action-list preview should advertise `npm run build`, not `loaf build`
        expect(result.stdout).toContain("Run npm run build");
        // Execution log should confirm npm was invoked
        expect(result.stdout).toContain("Ran npm run build");

        // Marker file must exist and reflect the BUMPED version (1.0.1)
        const markerPath = join(repoRoot, "build-marker.txt");
        const marker = execFileSync("cat", [markerPath], {
          encoding: "utf-8",
        }).trim();
        expect(marker).toBe("built v1.0.1");

        // Marker file must be part of the release commit
        const filesInCommit = execFileSync(
          "git",
          ["show", "HEAD", "--name-only", "--pretty=format:"],
          {
            cwd: repoRoot,
            encoding: "utf-8",
          },
        ).trim();
        expect(filesInCommit.split("\n")).toContain("build-marker.txt");
      } finally {
        rmSync(repoRoot, { recursive: true, force: true });
      }
    },
    90_000,
  );

  it(
    "falls back to `loaf build` when package.json has no build script",
    () => {
      const repoRoot = realpathSync(
        mkdtempSync(join(tmpdir(), "loaf-release-no-build-")),
      );
      try {
        execFileSync("git", ["init", "-b", "main"], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        execFileSync("git", ["config", "user.email", "test@test.com"], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        execFileSync("git", ["config", "user.name", "Test"], {
          cwd: repoRoot,
          stdio: "ignore",
        });

        // package.json WITHOUT a build script → should use `loaf build`
        writeFileSync(
          join(repoRoot, "package.json"),
          JSON.stringify({ name: "fixture", version: "1.0.0" }, null, 2) +
            "\n",
        );
        writeFileSync(
          join(repoRoot, "CHANGELOG.md"),
          [
            "# Changelog",
            "",
            "## [Unreleased]",
            "",
            "- Initial change",
            "",
            "## [1.0.0] - 2024-01-01",
            "",
            "- Initial release",
            "",
          ].join("\n"),
        );
        execFileSync("git", ["add", "."], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        execFileSync("git", ["commit", "-m", "chore: initial"], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        const baseSha = execFileSync("git", ["rev-parse", "HEAD"], {
          cwd: repoRoot,
          encoding: "utf-8",
        }).trim();

        writeFileSync(join(repoRoot, "src.txt"), "v1\n");
        execFileSync("git", ["add", "."], {
          cwd: repoRoot,
          stdio: "ignore",
        });
        execFileSync("git", ["commit", "-m", "feat: add src.txt"], {
          cwd: repoRoot,
          stdio: "ignore",
        });

        const result = spawnSync(
          "node",
          [
            CLI_PATH,
            "release",
            "--bump",
            "patch",
            "--yes",
            "--no-tag",
            "--no-gh",
            "--base",
            baseSha,
          ],
          {
            cwd: repoRoot,
            encoding: "utf-8",
            stdio: ["ignore", "pipe", "pipe"],
            timeout: 60_000,
          },
        );

        expect(result.status).toBe(0);
        // Action list should still say "Run loaf build" (fallback path)
        expect(result.stdout).toContain("Run loaf build");
        expect(result.stdout).not.toContain("Run npm run build");
      } finally {
        rmSync(repoRoot, { recursive: true, force: true });
      }
    },
    90_000,
  );
});
