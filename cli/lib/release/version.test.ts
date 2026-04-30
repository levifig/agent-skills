/**
 * Release Version Tests
 *
 * Tests for semver parsing, bumping, and version file detection.
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import {
  parseSemVer,
  formatSemVer,
  bumpVersion,
  detectVersionFiles,
  loadDeclaredVersionFile,
} from "./version.js";

// ─────────────────────────────────────────────────────────────────────────────
// parseSemVer
// ─────────────────────────────────────────────────────────────────────────────

describe("parseSemVer", () => {
  it("parses a stable version", () => {
    expect(parseSemVer("2.0.0")).toEqual({ major: 2, minor: 0, patch: 0 });
  });

  it("parses a pre-release version", () => {
    expect(parseSemVer("2.0.0-dev.6")).toEqual({
      major: 2,
      minor: 0,
      patch: 0,
      prerelease: "dev.6",
    });
  });

  it("parses a pre-release with no numeric suffix", () => {
    expect(parseSemVer("1.0.0-alpha")).toEqual({
      major: 1,
      minor: 0,
      patch: 0,
      prerelease: "alpha",
    });
  });

  it("returns null for invalid versions", () => {
    expect(parseSemVer("not-a-version")).toBeNull();
    expect(parseSemVer("1.2")).toBeNull();
    expect(parseSemVer("1.2.3-")).toBeNull();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// formatSemVer
// ─────────────────────────────────────────────────────────────────────────────

describe("formatSemVer", () => {
  it("formats a stable version", () => {
    expect(formatSemVer({ major: 2, minor: 1, patch: 0 })).toBe("2.1.0");
  });

  it("formats a pre-release version", () => {
    expect(
      formatSemVer({ major: 2, minor: 0, patch: 0, prerelease: "dev.6" }),
    ).toBe("2.0.0-dev.6");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// bumpVersion
// ─────────────────────────────────────────────────────────────────────────────

describe("bumpVersion", () => {
  describe("stable versions", () => {
    it("bumps major", () => {
      expect(bumpVersion("1.2.3", "major")).toBe("2.0.0");
    });

    it("bumps minor", () => {
      expect(bumpVersion("1.2.3", "minor")).toBe("1.3.0");
    });

    it("bumps patch", () => {
      expect(bumpVersion("1.2.3", "patch")).toBe("1.2.4");
    });

    it("returns null for prerelease bump on stable", () => {
      expect(bumpVersion("1.2.3", "prerelease")).toBeNull();
    });

    it("returns null for release bump on stable", () => {
      expect(bumpVersion("1.2.3", "release")).toBeNull();
    });
  });

  describe("pre-release versions", () => {
    it("increments dev counter", () => {
      expect(bumpVersion("2.0.0-dev.6", "prerelease")).toBe("2.0.0-dev.7");
    });

    it("promotes to stable release", () => {
      expect(bumpVersion("2.0.0-dev.6", "release")).toBe("2.0.0");
    });

    it("bumps major from pre-release", () => {
      expect(bumpVersion("2.0.0-dev.6", "major")).toBe("3.0.0");
    });

    it("bumps minor from pre-release", () => {
      expect(bumpVersion("2.0.0-dev.6", "minor")).toBe("2.1.0");
    });

    it("appends .1 to prerelease without numeric suffix", () => {
      expect(bumpVersion("1.0.0-alpha", "prerelease")).toBe("1.0.0-alpha.1");
    });

    it("increments existing numeric suffix", () => {
      expect(bumpVersion("1.0.0-alpha.3", "prerelease")).toBe("1.0.0-alpha.4");
    });

    it("appends .1 to non-numeric suffix segment", () => {
      expect(bumpVersion("1.0.0-alpha.beta", "prerelease")).toBe("1.0.0-alpha.beta.1");
    });
  });

  describe("invalid input", () => {
    it("returns null for unparseable version", () => {
      expect(bumpVersion("garbage", "major")).toBeNull();
    });

    it("returns null for incomplete version", () => {
      expect(bumpVersion("1.2", "patch")).toBeNull();
    });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// detectVersionFiles — monorepo discovery (SPEC-031 / TASK-143)
// ─────────────────────────────────────────────────────────────────────────────

describe("detectVersionFiles — monorepo overrides", () => {
  let tempDir: string;

  beforeEach(() => {
    tempDir = mkdtempSync(join(tmpdir(), "loaf-detect-"));
  });

  afterEach(() => {
    rmSync(tempDir, { recursive: true, force: true });
  });

  function writeFile(relativePath: string, content: string): void {
    const absolutePath = join(tempDir, relativePath);
    const dir = absolutePath.split("/").slice(0, -1).join("/");
    mkdirSync(dir, { recursive: true });
    writeFileSync(absolutePath, content, "utf-8");
  }

  it("falls back to root auto-detect when no overrides set", () => {
    writeFile("package.json", JSON.stringify({ name: "root", version: "1.0.0" }));
    writeFile("backend/pyproject.toml", '[project]\nname = "backend"\nversion = "0.1.0"\n');

    const files = detectVersionFiles(tempDir);

    expect(files).toHaveLength(1);
    expect(files[0].relativePath).toBe("package.json");
    expect(files[0].currentVersion).toBe("1.0.0");
  });

  it("config override replaces root auto-detect (returns only declared file)", () => {
    writeFile("package.json", JSON.stringify({ name: "root", version: "1.0.0" }));
    writeFile("backend/pyproject.toml", '[project]\nname = "backend"\nversion = "0.5.0"\n');

    const files = detectVersionFiles(tempDir, {
      configOverrides: ["backend/pyproject.toml"],
    });

    expect(files).toHaveLength(1);
    expect(files[0].relativePath).toBe("backend/pyproject.toml");
    expect(files[0].currentVersion).toBe("0.5.0");
    expect(files[0].format).toBe("toml-regex");
  });

  it("CLI override replaces both config and auto-detect", () => {
    writeFile("package.json", JSON.stringify({ name: "root", version: "1.0.0" }));
    writeFile("backend/pyproject.toml", '[project]\nversion = "0.5.0"\n');
    writeFile("frontend/package.json", JSON.stringify({ version: "2.3.4" }));

    const files = detectVersionFiles(tempDir, {
      cliOverrides: ["frontend/package.json"],
      configOverrides: ["backend/pyproject.toml"],
    });

    expect(files).toHaveLength(1);
    expect(files[0].relativePath).toBe("frontend/package.json");
    expect(files[0].currentVersion).toBe("2.3.4");
  });

  it("CLI override accepts multiple repeatable paths", () => {
    writeFile("a/package.json", JSON.stringify({ version: "1.1.1" }));
    writeFile("b/package.json", JSON.stringify({ version: "2.2.2" }));

    const files = detectVersionFiles(tempDir, {
      cliOverrides: ["a/package.json", "b/package.json"],
    });

    expect(files).toHaveLength(2);
    expect(files.map((f) => f.relativePath)).toEqual([
      "a/package.json",
      "b/package.json",
    ]);
    expect(files.map((f) => f.currentVersion)).toEqual(["1.1.1", "2.2.2"]);
  });

  it("aborts when a declared path is missing (no partial bump)", () => {
    writeFile("package.json", JSON.stringify({ version: "1.0.0" }));

    expect(() =>
      detectVersionFiles(tempDir, {
        configOverrides: ["backend/pyproject.toml"],
      }),
    ).toThrow(/version file backend\/pyproject\.toml not found/);
  });

  it("aborts when a declared path lacks a parseable version", () => {
    writeFile("backend/pyproject.toml", '[project]\nname = "backend"\n');

    expect(() =>
      detectVersionFiles(tempDir, {
        configOverrides: ["backend/pyproject.toml"],
      }),
    ).toThrow(/version file backend\/pyproject\.toml: could not parse version/);
  });

  it("aborts when a declared JSON file has no version field", () => {
    writeFile("frontend/package.json", JSON.stringify({ name: "frontend" }));

    expect(() =>
      detectVersionFiles(tempDir, {
        cliOverrides: ["frontend/package.json"],
      }),
    ).toThrow(/version file frontend\/package\.json: could not parse version/);
  });

  it("aborts when a declared JSON file is malformed", () => {
    writeFile("frontend/package.json", "{not json");

    expect(() =>
      detectVersionFiles(tempDir, {
        cliOverrides: ["frontend/package.json"],
      }),
    ).toThrow(/version file frontend\/package\.json: could not parse version/);
  });

  it("supports declared marketplace.json paths (nested versionPath)", () => {
    writeFile(
      ".claude-plugin/marketplace.json",
      JSON.stringify({ metadata: { version: "1.2.3" } }),
    );

    const files = detectVersionFiles(tempDir, {
      configOverrides: [".claude-plugin/marketplace.json"],
    });

    expect(files).toHaveLength(1);
    expect(files[0].currentVersion).toBe("1.2.3");
  });
});

describe("loadDeclaredVersionFile", () => {
  let tempDir: string;

  beforeEach(() => {
    tempDir = mkdtempSync(join(tmpdir(), "loaf-load-"));
  });

  afterEach(() => {
    rmSync(tempDir, { recursive: true, force: true });
  });

  it("loads a declared package.json with its version", () => {
    const filePath = join(tempDir, "frontend");
    mkdirSync(filePath, { recursive: true });
    writeFileSync(
      join(filePath, "package.json"),
      JSON.stringify({ version: "9.9.9" }),
      "utf-8",
    );

    const file = loadDeclaredVersionFile(tempDir, "frontend/package.json");

    expect(file.currentVersion).toBe("9.9.9");
    expect(file.format).toBe("json");
    expect(file.relativePath).toBe("frontend/package.json");
  });

  it("loads a declared Cargo.toml with its [package] version", () => {
    const filePath = join(tempDir, "crate");
    mkdirSync(filePath, { recursive: true });
    writeFileSync(
      join(filePath, "Cargo.toml"),
      '[package]\nname = "crate"\nversion = "0.4.2"\n',
      "utf-8",
    );

    const file = loadDeclaredVersionFile(tempDir, "crate/Cargo.toml");

    expect(file.currentVersion).toBe("0.4.2");
    expect(file.format).toBe("toml-regex");
  });

  it("rejects unsupported file extensions with a clear error", () => {
    writeFileSync(join(tempDir, "version.txt"), "1.0.0", "utf-8");

    expect(() => loadDeclaredVersionFile(tempDir, "version.txt")).toThrow(
      /unsupported file type/,
    );
  });
});
