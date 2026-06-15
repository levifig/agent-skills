#!/usr/bin/env node
/**
 * Package native Loaf archives for GitHub Releases and Homebrew.
 */

import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import {
  chmodSync,
  cpSync,
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { basename, dirname, join } from "node:path";

const rootDir = process.cwd();
const packageJSON = JSON.parse(readFileSync(join(rootDir, "package.json"), "utf8"));
const version = packageJSON.version;

if (!version) {
  console.error("ERROR: package.json missing version.");
  process.exit(1);
}

const releaseTargets = [
  "darwin-arm64",
  "darwin-x64",
  "linux-arm64",
  "linux-x64",
  "win32-arm64",
  "win32-x64",
];
const requestedTargets = (process.env.LOAF_RELEASE_TARGETS || process.env.LOAF_BUILD_TARGETS || releaseTargets.join(","))
  .split(",")
  .map((target) => target.trim())
  .filter(Boolean)
  .filter((target, index, targets) => targets.indexOf(target) === index);

const outDir = join(rootDir, "dist", "release");
rmSync(outDir, { recursive: true, force: true });
mkdirSync(outDir, { recursive: true });
const workDir = mkdtempSync(join(tmpdir(), "loaf-release-"));

const checksums = [];
for (const target of requestedTargets) {
  const nativeName = target.startsWith("win32-") ? "loaf.exe" : "loaf";
  const nativeSource = join(rootDir, "bin", "native", target, nativeName);
  if (!existsSync(nativeSource)) {
    console.error(`ERROR: missing native binary for ${target}: ${nativeSource}`);
    console.error("Run `npm run build:release` before packaging release archives.");
    process.exit(1);
  }

  const packageName = `loaf_${version}_${target}`;
  const packageRoot = join(workDir, packageName);
  mkdirSync(join(packageRoot, "bin"), { recursive: true });
  cpSync(nativeSource, join(packageRoot, "bin", nativeName));
  chmodSync(join(packageRoot, "bin", nativeName), 0o755);

  for (const entry of ["package.json", "README.md", "CHANGELOG.md"]) {
    copyIfPresent(join(rootDir, entry), join(packageRoot, entry));
  }
  for (const dir of ["config", "content", "dist", "plugins"]) {
    copyIfPresent(join(rootDir, dir), join(packageRoot, dir), { recursive: true });
  }
  rmSync(join(packageRoot, "dist", "release"), { recursive: true, force: true });

  const archiveName = `${packageName}.tar.gz`;
  const archivePath = join(outDir, archiveName);
  const tar = spawnSync("tar", ["-czf", archivePath, "-C", workDir, packageName], {
    cwd: rootDir,
    stdio: "inherit",
  });
  if (tar.status !== 0) {
    process.exit(tar.status ?? 1);
  }
  checksums.push(`${sha256(archivePath)}  ${archiveName}`);
  console.log(`✓ Packaged ${archiveName}`);
}

writeFileSync(join(outDir, "checksums.txt"), checksums.join("\n") + "\n");
rmSync(workDir, { recursive: true, force: true });
console.log(`✓ Wrote ${join(outDir, "checksums.txt")}`);

function copyIfPresent(from, to, options = {}) {
  if (!existsSync(from)) {
    return;
  }
  mkdirSync(dirname(to), { recursive: true });
  cpSync(from, to, {
    recursive: Boolean(options.recursive),
    filter: (source) => basename(source) !== ".DS_Store",
  });
}

function sha256(path) {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}
