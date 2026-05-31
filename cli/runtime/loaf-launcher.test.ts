import { chmodSync, copyFileSync, mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { describe, expect, it } from "vitest";

const launcherSource = join(process.cwd(), "cli", "runtime", "loaf-launcher.cjs");
const nativeRuntimeIt = process.platform === "win32" ? it.skip : it;

describe("loaf launcher", () => {
  it("delegates legacy commands to the bundled TypeScript fallback when native runtime is missing", () => {
    const fixture = createLauncherFixture();
    try {
      writeFallback(fixture.root);

      const result = spawnSync(process.execPath, [fixture.launcher, "check", "--hook", "check-secrets"], {
        encoding: "utf8",
      });

      expect(result.status).toBe(0);
      expect(result.stdout).toContain("fallback check --hook check-secrets");
    } finally {
      fixture.cleanup();
    }
  });

  it("returns an actionable error for Go-only commands when native runtime is missing", () => {
    const fixture = createLauncherFixture();
    try {
      writeFallback(fixture.root);

      const result = spawnSync(process.execPath, [fixture.launcher, "state", "status"], {
        encoding: "utf8",
      });

      expect(result.status).toBe(2);
      expect(result.stderr).toContain(`requires a native Loaf runtime for ${process.platform}-${process.arch}`);
      expect(result.stderr).toContain("legacy TypeScript commands");
    } finally {
      fixture.cleanup();
    }
  });

  nativeRuntimeIt("runs the native runtime when present and points it at the TypeScript fallback", () => {
    const fixture = createLauncherFixture();
    try {
      const fallback = writeFallback(fixture.root);
      writeNativeRuntime(fixture.root);

      const result = spawnSync(process.execPath, [fixture.launcher, "task", "list"], {
        encoding: "utf8",
      });

      expect(result.status).toBe(0);
      expect(result.stdout).toContain("native task list");
      expect(result.stdout).toContain(`legacy=${realpathSync(fallback)}`);
    } finally {
      fixture.cleanup();
    }
  });
});

function createLauncherFixture() {
  const root = mkdtempSync(join(tmpdir(), "loaf-launcher-"));
  const binDir = join(root, "bin");
  mkdirSync(binDir, { recursive: true });
  const launcher = join(binDir, "loaf");
  copyFileSync(launcherSource, launcher);
  chmodSync(launcher, 0o755);
  return {
    root,
    launcher,
    cleanup: () => rmSync(root, { recursive: true, force: true }),
  };
}

function writeFallback(root: string): string {
  const fallback = join(root, "dist-cli", "index.js");
  mkdirSync(join(root, "dist-cli"), { recursive: true });
  writeFileSync(
    fallback,
    "#!/usr/bin/env node\nconsole.log('fallback ' + process.argv.slice(2).join(' '));\n",
    { mode: 0o755 }
  );
  return fallback;
}

function writeNativeRuntime(root: string): string {
  const nativeName = process.platform === "win32" ? "loaf.exe" : "loaf";
  const nativePath = join(root, "bin", "native", `${process.platform}-${process.arch}`, nativeName);
  mkdirSync(join(root, "bin", "native", `${process.platform}-${process.arch}`), { recursive: true });
  writeFileSync(
    nativePath,
    "#!/bin/sh\nprintf 'native %s\\n' \"$*\"\nprintf 'legacy=%s\\n' \"$LOAF_LEGACY_CLI\"\n",
    { mode: 0o755 }
  );
  return nativePath;
}
