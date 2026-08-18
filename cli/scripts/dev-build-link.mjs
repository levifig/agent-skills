import { lstatSync, mkdirSync, readFileSync, readlinkSync, renameSync, rmSync, symlinkSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join, resolve } from "node:path";

export function refreshDevBuildLink(launcher, options = {}) {
  const home = options.home || homedir();
  const warn = options.warn || console.warn;
  const link = join(home, ".local", "bin", "loaf");
  mkdirSync(dirname(link), { recursive: true });

  let existing;
  try {
    existing = lstatSync(link);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }

  if (existing && !existing.isSymbolicLink()) {
    warn(`WARN: not linking latest dev build; ${link} is not a symlink`);
    return { status: "conflict", link };
  }
  if (existing && !isLoafLauncherLink(link)) {
    warn(`WARN: not linking latest dev build; ${link} points outside a Loaf package`);
    return { status: "conflict", link };
  }

  const temporary = `${link}.tmp-${process.pid}`;
  rmSync(temporary, { force: true });
  try {
    symlinkSync(launcher, temporary);
    renameSync(temporary, link);
  } finally {
    rmSync(temporary, { force: true });
  }
  return { status: "linked", link };
}

function isLoafLauncherLink(link) {
  const target = resolve(dirname(link), readlinkSync(link));
  const packageRoot = dirname(dirname(target));
  try {
    const manifest = JSON.parse(readFileSync(join(packageRoot, "package.json"), "utf8"));
    return manifest.name === "loaf" && target === join(packageRoot, "bin", "loaf");
  } catch {
    return false;
  }
}
